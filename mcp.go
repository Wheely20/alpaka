package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
)

func createMCPServer() *server.MCPServer {
	mcpServer := server.NewMCPServer("alpaka-native-tools", version)

	addTool := mcp.NewTool("add_numbers",
		mcp.WithDescription("Adds two numbers. Use this tool when you want to calculate math problems."),
		mcp.WithNumber("a", mcp.Required(), mcp.Description("The first number")),
		mcp.WithNumber("b", mcp.Required(), mcp.Description("The second number")),
	)

	mcpServer.AddTool(addTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, ok := request.Params.Arguments.(map[string]interface{})
		if !ok {
			return mcp.NewToolResultError("Error: Arguments could not be parsed."), nil
		}

		a, okA := args["a"].(float64)
		b, okB := args["b"].(float64)

		if !okA || !okB {
			return mcp.NewToolResultError("Error: Arguments 'a' and 'b' must be valid numbers."), nil
		}

		result := a + b
		resultStr := fmt.Sprintf("The result of %v + %v is %v", a, b, result)
		return mcp.NewToolResultText(resultStr), nil
	})

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

// startMCPClient startet den versteckten internal-mcp Befehl und verbindet sich
func startMCPClient(ctx context.Context) (client.MCPClient, error) {
	// Pfad zur eigenen Executable ermitteln
	execPath, err := os.Executable()
	if err != nil {
		return nil, err
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return nil, err
	}

	// Wir rufen uns selbst auf, aber mit dem versteckten Befehl
	mcpClient, err := client.NewStdioMCPClient(execPath, nil, "internal-mcp")
	if err != nil {
		return nil, fmt.Errorf("Fehler beim Starten des MCP-Clients: %w", err)
	}

	// MCP-Handshake durchführen
	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "alpaka",
		Version: version,
	}

	_, err = mcpClient.Initialize(ctx, initReq)
	if err != nil {
		mcpClient.Close()
		return nil, fmt.Errorf("Fehler bei MCP-Initialisierung: %w", err)
	}

	return mcpClient, nil
}
