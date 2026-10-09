package main

import (
	"context"
	"os/exec"
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

func TestMCPInProcessServerAndToolExecution(t *testing.T) {
	ctx := context.Background()

	// Create MCP Server instance
	mcpServer := createMCPServer()

	// Connect in-process client
	mcpClient, err := client.NewInProcessClient(mcpServer)
	if err != nil {
		t.Fatalf("Failed to create in-process MCP client: %v", err)
	}
	defer mcpClient.Close()

	// Perform initialize handshake
	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "alpaka-test",
		Version: "test",
	}
	_, err = mcpClient.Initialize(ctx, initReq)
	if err != nil {
		t.Fatalf("Failed to initialize MCP client: %v", err)
	}

	// 1. Check tool listing
	toolsResult, err := mcpClient.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		t.Fatalf("Failed to list tools: %v", err)
	}

	foundAddNumbers := false
	for _, tool := range toolsResult.Tools {
		if tool.Name == "add_numbers" {
			foundAddNumbers = true
			break
		}
	}
	if !foundAddNumbers {
		t.Fatalf("Expected tool 'add_numbers' not found in tool list")
	}

	// 2. Test tool execution (executeLocalTool)
	argJSON := `{"a": 15, "b": 27}`
	resStr := executeLocalTool(ctx, mcpClient, "add_numbers", argJSON)
	expected := "The result of 15 + 27 is 42"
	if resStr != expected {
		t.Errorf("Expected '%s', got '%s'", expected, resStr)
	}
}

func TestMCPStdioSubprocess(t *testing.T) {
	ctx := context.Background()

	// Build the alpaka binary first
	buildCmd := exec.Command("go", "build", "-o", "alpaka_test_bin", ".")
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("Failed to build alpaka binary: %v", err)
	}
	defer exec.Command("rm", "-f", "alpaka_test_bin").Run()

	// Connect stdio client to binary
	mcpClient, err := client.NewStdioMCPClient("./alpaka_test_bin", nil, "internal-mcp")
	if err != nil {
		t.Fatalf("Failed to start stdio MCP client: %v", err)
	}
	defer mcpClient.Close()

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "alpaka-stdio-test", Version: "test"}

	_, err = mcpClient.Initialize(ctx, initReq)
	if err != nil {
		t.Fatalf("Failed to initialize stdio client: %v", err)
	}

	argJSON := `{"a": 100, "b": 250}`
	resStr := executeLocalTool(ctx, mcpClient, "add_numbers", argJSON)
	expected := "The result of 100 + 250 is 350"
	if resStr != expected {
		t.Errorf("Expected '%s', got '%s'", expected, resStr)
	}
}
