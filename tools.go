package main

import (
	"fmt"

	"alpaka/tools"

	"github.com/spf13/cobra"
)

// knownInternalTools is a list of all known internal tools
var knownInternalTools = []string{
	"add_numbers",
}

var toolCmd = &cobra.Command{
	Use:   "tool",
	Short: "Manages native MCP tools",
}

var toolListCmd = &cobra.Command{
	Use:   "list",
	Short: "Shows all available native MCP tools",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfig()
		if err != nil {
			return fmt.Errorf("Could not load configuration: %w", err)
		}

		fmt.Println("Native MCP-Werkzeuge:")
		for toolName := range tools.Registry {
			// ANSI Farbcode: 32m = Grün, 31m = Rot, 0m = Reset
			status := "\033[32m[ON]\033[0m "
			if !cfg.IsToolActive(toolName) {
				status = "\033[31m[OFF]\033[0m"
			}
			fmt.Printf("  %s %s\n", status, toolName)
		}
		return nil
	},
}

var toolDisableCmd = &cobra.Command{
	Use:   "disable [name]",
	Short: "Deactivates a native MCP tool",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		toolName := args[0]

		// Check if the tool exists
		if _, exists := tools.Registry[toolName]; !exists {
			return fmt.Errorf("Tool '%s' does not exist", toolName)
		}

		cfg, err := loadConfig()
		if err != nil {
			return fmt.Errorf("Could not load configuration: %w", err)
		}

		if !cfg.IsToolActive(toolName) {
			fmt.Printf("The tool '%s' is already disabled.\n", toolName)
			return nil
		}

		// Add the tool to the disabled list
		cfg.DisabledTools = append(cfg.DisabledTools, toolName)

		if err := saveConfig(cfg); err != nil {
			return fmt.Errorf("Error saving configuration: %w", err)
		}

		fmt.Printf("Tool '%s' has been disabled.\n", toolName)
		return nil
	},
}

var toolEnableCmd = &cobra.Command{
	Use:   "enable [name]",
	Short: "Activates a native MCP tool",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		toolName := args[0]

		cfg, err := loadConfig()
		if err != nil {
			return fmt.Errorf("Could not load configuration: %w", err)
		}

		if cfg.IsToolActive(toolName) {
			fmt.Printf("The tool '%s' is already active.\n", toolName)
			return nil
		}

		// Remove the tool from the disabled list
		var newDisabled []string
		for _, t := range cfg.DisabledTools {
			if t != toolName {
				newDisabled = append(newDisabled, t)
			}
		}
		cfg.DisabledTools = newDisabled

		if err := saveConfig(cfg); err != nil {
			return fmt.Errorf("Error saving configuration: %w", err)
		}

		fmt.Printf("Tool '%s' has been enabled.\n", toolName)
		return nil
	},
}

func init() {
	// Add commands
	toolCmd.AddCommand(toolListCmd)
	toolCmd.AddCommand(toolEnableCmd)
	toolCmd.AddCommand(toolDisableCmd)
}
