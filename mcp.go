package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"alpaka/tools"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
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
	// mcpConnectTimeout limits the handshake (npx may need to download packages first)
	mcpConnectTimeout = 30 * time.Second
)

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

// initializeMCPClient performs the MCP handshake with a timeout
func initializeMCPClient(ctx context.Context, c client.MCPClient) error {
	ctx, cancel := context.WithTimeout(ctx, mcpConnectTimeout)
	defer cancel()

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "alpaka", Version: version}
	_, err := c.Initialize(ctx, initReq)
	return err
}

// startMCPClients starts the hidden internal-mcp command and all enabled external servers
func startMCPClients(ctx context.Context) (map[string]client.MCPClient, error) {
	clients := make(map[string]client.MCPClient)
	cfg, err := loadConfig()
	if err != nil {
		return clients, err
	}

	// 1. Always start the internal Alpaka-Server
	execPath, err := os.Executable()
	if err != nil {
		return clients, err
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return clients, err
	}
	internalClient, err := client.NewStdioMCPClient(execPath, nil, "internal-mcp")
	if err != nil {
		return clients, fmt.Errorf("could not start internal MCP server: %w", err)
	}
	if err := initializeMCPClient(ctx, internalClient); err != nil {
		internalClient.Close()
		return clients, fmt.Errorf("MCP initialization of the internal server failed: %w", err)
	}
	clients[internalServerName] = internalClient

	// 2. Start all configured external servers (sorted for stable output)
	names := make([]string, 0, len(cfg.MCPServers))
	for name := range cfg.MCPServers {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		srvCfg := cfg.MCPServers[name]
		if srvCfg.Disabled {
			continue
		}
		if err := validateServerName(name); err != nil {
			fmt.Printf("Warning: Skipping MCP server: %v\n", err)
			continue
		}

		// Resolve secrets (e.g. ${GITHUB_TOKEN}) and warn about unset variables.
		// mcp-go already prepends the current environment, so the values below take precedence.
		var env []string
		skip := false
		for k, v := range srvCfg.Env {
			expanded, missing := expandEnvStrict(v)
			if len(missing) > 0 {
				fmt.Printf("Warning: MCP server '%s' skipped, environment variable(s) not set: %s\n", name, strings.Join(missing, ", "))
				skip = true
				break
			}
			env = append(env, fmt.Sprintf("%s=%s", k, expanded))
		}
		if skip {
			continue
		}

		mcpClient, err := client.NewStdioMCPClient(srvCfg.Command, env, srvCfg.Args...)
		if err != nil {
			fmt.Printf("Warning: Could not start MCP Server '%s': %v\n", name, err)
			continue
		}

		if err := initializeMCPClient(ctx, mcpClient); err != nil {
			mcpClient.Close()
			fmt.Printf("Warning: Handshake with '%s' failed: %v\n", name, err)
			continue
		}

		clients[name] = mcpClient
	}

	return clients, nil
}
