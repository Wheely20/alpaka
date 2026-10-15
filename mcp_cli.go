package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var (
	mcpAddEnv       []string
	mcpAddURL       string
	mcpAddTransport string
	mcpAddHeaders   []string
	mcpAddTimeout   int
)

// parseKeyValues parses KEY=VALUE pairs
func parseKeyValues(flag string, pairs []string) (map[string]string, error) {
	result := make(map[string]string)
	for _, kv := range pairs {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("invalid --%s value '%s', expected KEY=VALUE", flag, kv)
		}
		result[k] = v
	}
	return result, nil
}

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Manages external MCP-Server",
}

var mcpListCmd = &cobra.Command{
	Use:   "list",
	Short: "Shows all configured MCP-Server",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfig()
		if err != nil {
			return err
		}

		if len(cfg.MCPServers) == 0 {
			fmt.Println("No configured MCP-Server.")
			return nil
		}

		names := make([]string, 0, len(cfg.MCPServers))
		for name := range cfg.MCPServers {
			names = append(names, name)
		}
		sort.Strings(names)

		fmt.Println("Configured MCP-Server:")
		for _, name := range names {
			srv := cfg.MCPServers[name]
			status := "\033[32m[ON]\033[0m "
			if srv.Disabled {
				status = "\033[31m[OFF]\033[0m"
			}
			target := strings.TrimSpace(srv.Command + " " + strings.Join(srv.Args, " "))
			if srv.URL != "" {
				target = srv.URL
			}
			extra := ""
			if srv.Timeout > 0 {
				extra = fmt.Sprintf(", timeout %ds", srv.Timeout)
			}
			// Only keys of env/headers are shown, never their values
			fmt.Printf("  %s %s [%s] %s%s\n", status, name, transportType(srv), target, extra)
		}
		return nil
	},
}

var mcpAddCmd = &cobra.Command{
	Use:   "add <name> <command> [args...]  |  add <name> --url <url>",
	Short: "Adds an external MCP-Server (stdio or http/sse)",
	Long: `Adds an external MCP-Server.

stdio (local process):
  alpaka mcp add github -e GITHUB_PERSONAL_ACCESS_TOKEN='${GITHUB_TOKEN}' -- npx -y @modelcontextprotocol/server-github

Put '--' in front of the command so that its own arguments (e.g. -y) are not parsed as alpaka flags.

http (remote, streamable HTTP; use --transport sse for the legacy SSE protocol):
  alpaka mcp add remote --url https://example.com/mcp -H 'Authorization=Bearer ${TOKEN}'

Use ${VAR} in env/header values to reference environment variables instead of storing secrets in plain text.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		rest := args[1:]

		if err := validateServerName(name); err != nil {
			return err
		}

		srv := MCPServerConfig{Timeout: mcpAddTimeout}

		env, err := parseKeyValues("env", mcpAddEnv)
		if err != nil {
			return err
		}
		headers, err := parseKeyValues("header", mcpAddHeaders)
		if err != nil {
			return err
		}

		if mcpAddURL != "" {
			if len(rest) > 0 || len(env) > 0 {
				return fmt.Errorf("--url cannot be combined with a command, arguments or --env")
			}
			srv.URL = mcpAddURL
			srv.Headers = headers
			if mcpAddTransport != "" {
				srv.Type = mcpAddTransport
			}
		} else {
			if len(rest) == 0 {
				return fmt.Errorf("a command or --url is required")
			}
			if len(headers) > 0 || mcpAddTransport != "" {
				return fmt.Errorf("--header and --transport can only be used together with --url")
			}
			srv.Command = rest[0]
			srv.Args = rest[1:]
			if len(env) > 0 {
				srv.Env = env
			}
		}

		if err := validateServerConfig(srv); err != nil {
			return err
		}

		cfg, err := loadConfig()
		if err != nil {
			return fmt.Errorf("Could not load configuration: %w", err)
		}
		if cfg.MCPServers == nil {
			cfg.MCPServers = make(map[string]MCPServerConfig)
		}
		if _, exists := cfg.MCPServers[name]; exists {
			return fmt.Errorf("Server '%s' already exists (remove it first)", name)
		}
		cfg.MCPServers[name] = srv

		if err := saveConfig(cfg); err != nil {
			return fmt.Errorf("Error saving configuration: %w", err)
		}
		fmt.Printf("MCP-Server '%s' added.\n", name)
		if w := insecureURLWarning(srv); w != "" {
			fmt.Printf("Warning: %s\n", w)
		}
		return nil
	},
}

var mcpRemoveCmd = &cobra.Command{
	Use:   "remove [name]",
	Short: "Removes a MCP-Server from the configuration",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		cfg, err := loadConfig()
		if err != nil {
			return fmt.Errorf("Could not load configuration: %w", err)
		}
		if _, exists := cfg.MCPServers[name]; !exists {
			return fmt.Errorf("Server '%s' not found", name)
		}
		delete(cfg.MCPServers, name)
		if err := saveConfig(cfg); err != nil {
			return fmt.Errorf("Error saving configuration: %w", err)
		}
		fmt.Printf("MCP-Server '%s' removed.\n", name)
		return nil
	},
}

// setMCPServerDisabled enables or disables a configured server
func setMCPServerDisabled(name string, disabled bool) error {
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("Could not load configuration: %w", err)
	}
	srv, exists := cfg.MCPServers[name]
	if !exists {
		return fmt.Errorf("Server '%s' not found", name)
	}
	if srv.Disabled == disabled {
		state := "enabled"
		if disabled {
			state = "disabled"
		}
		fmt.Printf("MCP-Server '%s' is already %s.\n", name, state)
		return nil
	}
	srv.Disabled = disabled
	cfg.MCPServers[name] = srv
	if err := saveConfig(cfg); err != nil {
		return fmt.Errorf("Error saving configuration: %w", err)
	}
	if disabled {
		fmt.Printf("MCP-Server '%s' deactivated.\n", name)
	} else {
		fmt.Printf("MCP-Server '%s' activated.\n", name)
	}
	return nil
}

var mcpDisableCmd = &cobra.Command{
	Use:   "disable [name]",
	Short: "Deactivates a MCP-Server",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return setMCPServerDisabled(args[0], true)
	},
}

var mcpEnableCmd = &cobra.Command{
	Use:   "enable [name]",
	Short: "Activates a MCP-Server",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return setMCPServerDisabled(args[0], false)
	},
}

func init() {
	mcpAddCmd.Flags().StringArrayVarP(&mcpAddEnv, "env", "e", nil, "Environment variable KEY=VALUE for stdio servers (repeatable, ${VAR} is expanded at start)")
	mcpAddCmd.Flags().StringVar(&mcpAddURL, "url", "", "URL of a remote MCP server (http/sse)")
	mcpAddCmd.Flags().StringVar(&mcpAddTransport, "transport", "", "Transport of a remote server: http (default) or sse")
	mcpAddCmd.Flags().StringArrayVarP(&mcpAddHeaders, "header", "H", nil, "HTTP header KEY=VALUE for remote servers (repeatable, ${VAR} is expanded at start)")
	mcpAddCmd.Flags().IntVar(&mcpAddTimeout, "timeout", 0, "Timeout in seconds for connection and tool calls (default 30)")

	mcpCmd.AddCommand(mcpListCmd, mcpAddCmd, mcpRemoveCmd, mcpEnableCmd, mcpDisableCmd)
	rootCmd.AddCommand(mcpCmd)
}
