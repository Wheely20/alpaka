package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"alpaka/tools"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

func createMCPServer() *server.MCPServer {
	mcpServer := server.NewMCPServer("alpaka-native-tools", version)
	cfg, _ := loadConfig()

	// Goes through the registered tools and add them if they are not disabled
	for name, def := range tools.Registry {
		if cfg.IsToolActive(name) {
			mcpServer.AddTool(*def.Tool, def.Handler)
		}
	}

	return mcpServer
}

var internalMcpCmd = &cobra.Command{
	Use:    "internal-mcp",
	Short:  "Starts the internal MCP tool server",
	Hidden: true, // hide this command from the help output
	RunE: func(cmd *cobra.Command, args []string) error {
		mcpServer := createMCPServer()
		return server.ServeStdio(mcpServer)
	},
}

const (
	// toolNameSeparator separates server and tool name in the names the model sees.
	// Server names must not contain it, otherwise routing would be ambiguous.
	toolNameSeparator = "__"
	// internalServerName is the name of the built-in Alpaka tool server
	internalServerName = "alpaka"

	defaultMCPTimeout        = 30 * time.Second
	maxMCPTimeoutSeconds     = 600
	defaultMCPMaxResultChars = 16000

	transportStdio = "stdio"
	transportHTTP  = "http"
	transportSSE   = "sse"
)

// mcpConnection is a started MCP client together with its runtime settings
type mcpConnection struct {
	Name           string
	Client         client.MCPClient
	Timeout        time.Duration // timeout for the handshake and for tool calls
	MaxResultChars int           // maximum length of a tool result passed to the model
}

var serverNameRegex = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*(_[A-Za-z0-9-]+)*$`)

// validateServerName makes sure a server name can safely be used as tool name prefix
func validateServerName(name string) error {
	if name == internalServerName {
		return fmt.Errorf("the name '%s' is reserved", internalServerName)
	}
	if strings.Contains(name, toolNameSeparator) || !serverNameRegex.MatchString(name) {
		return fmt.Errorf("invalid server name '%s': use letters, digits, '-' and single '_' only", name)
	}
	return nil
}

// transportType returns the transport of a server config: explicit "type" or derived from command/url
func transportType(srv MCPServerConfig) string {
	if t := strings.ToLower(strings.TrimSpace(srv.Type)); t != "" {
		if t == "streamable-http" || t == "streamable_http" {
			return transportHTTP
		}
		return t
	}
	if srv.Command != "" {
		return transportStdio
	}
	if srv.URL != "" {
		if u, err := url.Parse(srv.URL); err == nil && strings.HasSuffix(strings.TrimRight(u.Path, "/"), "/sse") {
			return transportSSE
		}
		return transportHTTP
	}
	return ""
}

// validateServerConfig checks that a server config is complete and consistent
func validateServerConfig(srv MCPServerConfig) error {
	switch transportType(srv) {
	case transportStdio:
		if srv.Command == "" {
			return fmt.Errorf("transport 'stdio' needs a command")
		}
		if srv.URL != "" {
			return fmt.Errorf("'command' and 'url' are mutually exclusive")
		}
	case transportHTTP, transportSSE:
		if srv.URL == "" {
			return fmt.Errorf("transport '%s' needs a url", transportType(srv))
		}
		if srv.Command != "" {
			return fmt.Errorf("'command' and 'url' are mutually exclusive")
		}
		u, err := url.Parse(srv.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("invalid url '%s'", srv.URL)
		}
	case "":
		return fmt.Errorf("either 'command' or 'url' is required")
	default:
		return fmt.Errorf("unknown transport type '%s' (use stdio, http or sse)", srv.Type)
	}
	if srv.Timeout < 0 || srv.Timeout > maxMCPTimeoutSeconds {
		return fmt.Errorf("timeout must be between 0 and %d seconds", maxMCPTimeoutSeconds)
	}
	return nil
}

// resolveTimeout: server setting > global config > ALPAKA_MCP_TIMEOUT > default (30s)
func resolveTimeout(srv MCPServerConfig, cfg *Config) time.Duration {
	if srv.Timeout > 0 {
		return time.Duration(srv.Timeout) * time.Second
	}
	if cfg != nil && cfg.MCPTimeout > 0 {
		return time.Duration(cfg.MCPTimeout) * time.Second
	}
	if v := os.Getenv("ALPAKA_MCP_TIMEOUT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return defaultMCPTimeout
}

// resolveMaxResultChars returns the configured maximum tool result length
func resolveMaxResultChars(cfg *Config) int {
	if cfg != nil && cfg.MCPMaxResultChars > 0 {
		return cfg.MCPMaxResultChars
	}
	return defaultMCPMaxResultChars
}

// insecureURLWarning warns if credentials would be sent unencrypted to a remote host
func insecureURLWarning(srv MCPServerConfig) string {
	u, err := url.Parse(srv.URL)
	if err != nil || u.Scheme != "http" {
		return ""
	}
	switch u.Hostname() {
	case "localhost", "127.0.0.1", "::1":
		return ""
	}
	return fmt.Sprintf("url '%s' uses unencrypted http, headers and tokens are sent in plain text", srv.URL)
}

// expandEnvStrict expands ${VAR} / $VAR and reports variables that are not set
func expandEnvStrict(value string) (string, []string) {
	var missing []string
	expanded := os.Expand(value, func(key string) string {
		v, ok := os.LookupEnv(key)
		if !ok {
			missing = append(missing, key)
		}
		return v
	})
	return expanded, missing
}

// expandMapStrict expands all values of a map; it fails if any referenced variable is not set
func expandMapStrict(m map[string]string) (map[string]string, error) {
	result := make(map[string]string, len(m))
	var missing []string
	for k, v := range m {
		expanded, miss := expandEnvStrict(v)
		missing = append(missing, miss...)
		result[k] = expanded
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, fmt.Errorf("environment variable(s) not set: %s", strings.Join(missing, ", "))
	}
	return result, nil
}

// initializeMCPClient performs the MCP handshake with a timeout
func initializeMCPClient(ctx context.Context, c client.MCPClient, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "alpaka", Version: version}
	_, err := c.Initialize(ctx, initReq)
	return err
}

// connectServer starts the transport of an external server and performs the handshake.
// ctx must live as long as the connection (it is used by the SSE stream), the handshake
// has its own timeout.
func connectServer(ctx context.Context, srv MCPServerConfig, timeout time.Duration) (client.MCPClient, error) {
	var (
		c   client.MCPClient
		err error
	)

	switch transportType(srv) {
	case transportStdio:
		// mcp-go prepends the current environment, so these values take precedence
		var env map[string]string
		if env, err = expandMapStrict(srv.Env); err != nil {
			return nil, err
		}
		envList := make([]string, 0, len(env))
		for k, v := range env {
			envList = append(envList, k+"="+v)
		}
		c, err = client.NewStdioMCPClient(srv.Command, envList, srv.Args...)
		if err != nil {
			return nil, fmt.Errorf("could not start: %w", err)
		}

	case transportHTTP:
		var headers map[string]string
		if headers, err = expandMapStrict(srv.Headers); err != nil {
			return nil, err
		}
		hc, herr := client.NewStreamableHttpClient(srv.URL,
			transport.WithHTTPHeaders(headers),
			transport.WithHTTPTimeout(timeout),
		)
		if herr != nil {
			return nil, herr
		}
		if err = hc.Start(ctx); err != nil {
			hc.Close()
			return nil, fmt.Errorf("could not connect: %w", err)
		}
		c = hc

	case transportSSE:
		var headers map[string]string
		if headers, err = expandMapStrict(srv.Headers); err != nil {
			return nil, err
		}
		sc, serr := client.NewSSEMCPClient(srv.URL,
			transport.WithHeaders(headers),
			transport.WithResponseTimeout(timeout),
		)
		if serr != nil {
			return nil, serr
		}
		if err = sc.Start(ctx); err != nil {
			sc.Close()
			return nil, fmt.Errorf("could not connect: %w", err)
		}
		c = sc

	default:
		return nil, fmt.Errorf("unknown transport '%s'", srv.Type)
	}

	if err := initializeMCPClient(ctx, c, timeout); err != nil {
		c.Close()
		return nil, fmt.Errorf("handshake failed: %w", err)
	}
	return c, nil
}

// startMCPClients starts the hidden internal-mcp command and all enabled external servers.
// External servers are started in parallel; failing servers only produce a warning.
func startMCPClients(ctx context.Context) (map[string]*mcpConnection, error) {
	conns := make(map[string]*mcpConnection)
	cfg, err := loadConfig()
	if err != nil {
		return conns, err
	}
	maxChars := resolveMaxResultChars(cfg)

	// 1. Always start the internal Alpaka-Server
	execPath, err := os.Executable()
	if err != nil {
		return conns, err
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return conns, err
	}
	internalTimeout := resolveTimeout(MCPServerConfig{}, cfg)
	// The internal server does not receive cancellations of the client. Native tools that start
	// processes therefore enforce their own, slightly shorter deadline.
	toolTimeout := int((internalTimeout - 2*time.Second).Seconds())
	if toolTimeout < 1 {
		toolTimeout = 1
	}
	internalEnv := []string{fmt.Sprintf("%s=%d", tools.ToolTimeoutEnv, toolTimeout)}
	internalClient, err := client.NewStdioMCPClient(execPath, internalEnv, "internal-mcp")
	if err != nil {
		return conns, fmt.Errorf("could not start internal MCP server: %w", err)
	}
	if err := initializeMCPClient(ctx, internalClient, internalTimeout); err != nil {
		internalClient.Close()
		return conns, fmt.Errorf("MCP initialization of the internal server failed: %w", err)
	}
	conns[internalServerName] = &mcpConnection{
		Name: internalServerName, Client: internalClient, Timeout: internalTimeout, MaxResultChars: maxChars,
	}

	// 2. Start all configured external servers (sorted for stable output)
	names := make([]string, 0, len(cfg.MCPServers))
	for name, srv := range cfg.MCPServers {
		if !srv.Disabled {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	type result struct {
		conn *mcpConnection
		msg  string // warning
	}
	results := make([]result, len(names))

	var g errgroup.Group
	for i, name := range names {
		srv := cfg.MCPServers[name]

		if err := validateServerName(name); err != nil {
			results[i].msg = fmt.Sprintf("Skipping MCP server: %v", err)
			continue
		}
		if err := validateServerConfig(srv); err != nil {
			results[i].msg = fmt.Sprintf("Skipping MCP server '%s': %v", name, err)
			continue
		}
		if w := insecureURLWarning(srv); w != "" {
			fmt.Printf("Warning: MCP server '%s': %s\n", name, w)
		}

		timeout := resolveTimeout(srv, cfg)
		g.Go(func() error {
			c, err := connectServer(ctx, srv, timeout)
			if err != nil {
				results[i].msg = fmt.Sprintf("MCP server '%s' skipped: %v", name, err)
				return nil // a failing server must not stop the others
			}
			results[i].conn = &mcpConnection{Name: name, Client: c, Timeout: timeout, MaxResultChars: maxChars}
			return nil
		})
	}
	g.Wait()

	for i, r := range results {
		if r.msg != "" {
			fmt.Printf("Warning: %s\n", r.msg)
		}
		if r.conn != nil {
			conns[names[i]] = r.conn
		}
	}

	return conns, nil
}
