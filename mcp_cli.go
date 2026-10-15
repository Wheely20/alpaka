package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var mcpAddEnv []string

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
			fmt.Printf("  %s %s (%s %s)\n", status, name, srv.Command, strings.Join(srv.Args, " "))
		}
		return nil
	},
}

var mcpAddCmd = &cobra.Command{
	Use:   "add <name> <command> [args...]",
	Short: "Adds an external MCP-Server (stdio)",
	Long: `Adds an external MCP-Server that is started via stdio.

Example:
  alpaka mcp add github -e GITHUB_PERSONAL_ACCESS_TOKEN='${GITHUB_TOKEN}' npx -y @modelcontextprotocol/server-github

Use ${VAR} in env values to reference environment variables instead of storing secrets in plain text.`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		name, command, cmdArgs := args[0], args[1], args[2:]

		if err := validateServerName(name); err != nil {
			return err
		}

		env := make(map[string]string)
		for _, kv := range mcpAddEnv {
			k, v, ok := strings.Cut(kv, "=")
			if !ok || k == "" {
				return fmt.Errorf("invalid --env value '%s', expected KEY=VALUE", kv)
			}
			env[k] = v
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

		srv := MCPServerConfig{Command: command, Args: cmdArgs}
		if len(env) > 0 {
			srv.Env = env
		}
		cfg.MCPServers[name] = srv

		if err := saveConfig(cfg); err != nil {
			return fmt.Errorf("Error saving configuration: %w", err)
		}
		fmt.Printf("MCP-Server '%s' added.\n", name)
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
	// Stop flag parsing after the first positional args, so flags of the MCP command itself stay untouched
	mcpAddCmd.Flags().SetInterspersed(false)
	mcpAddCmd.Flags().StringArrayVarP(&mcpAddEnv, "env", "e", nil, "Environment variable KEY=VALUE (repeatable, ${VAR} is expanded at start)")

	mcpCmd.AddCommand(mcpListCmd, mcpAddCmd, mcpRemoveCmd, mcpEnableCmd, mcpDisableCmd)
	rootCmd.AddCommand(mcpCmd)
}
