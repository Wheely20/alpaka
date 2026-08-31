package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Structure for the MCP configuration of llama.cpp
type MCPConfig struct {
	MCPServers map[string]MCPServerConfig `json:"mcpServers"`
}

type MCPServerConfig struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// generateMCPConfig creates the mcp_config.json in the ~/.alpaka/ directory
func generateMCPConfig() (string, error) {
	alpakaDir, err := getAlpakaDir()
	if err != nil {
		return "", err
	}

	// find path to currently running Alpaka-Binary
	execPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("Could not determine own executable path: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return "", err
	}

	// Configuration populate (calls itself with "internal-mcp")
	config := MCPConfig{
		MCPServers: map[string]MCPServerConfig{
			"alpaka-native-tools": {
				Command: execPath,
				Args:    []string{"internal-mcp"},
			},
		},
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", err
	}

	configPath := filepath.Join(alpakaDir, "mcp_config.json")
	err = os.WriteFile(configPath, data, 0644)
	if err != nil {
		return "", fmt.Errorf("Could not write mcp_config.json: %w", err)
	}

	return configPath, nil
}
