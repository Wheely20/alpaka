package main

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"alpaka/tools"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

func TestMCPInProcessServerAndToolExecution(t *testing.T) {
	enableAllNativeTools(t)
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
	resStr := executeLocalTool(ctx, map[string]*mcpConnection{"internal-mcp": {Name: "internal-mcp", Client: mcpClient, Timeout: 10 * time.Second}}, "internal-mcp"+toolNameSeparator+"add_numbers", argJSON)
	expected := "The result of 15 + 27 is 42"
	if resStr != expected {
		t.Errorf("Expected '%s', got '%s'", expected, resStr)
	}
}

func TestMCPStdioSubprocess(t *testing.T) {
	enableAllNativeTools(t)
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
	resStr := executeLocalTool(ctx, map[string]*mcpConnection{"internal-mcp": {Name: "internal-mcp", Client: mcpClient, Timeout: 10 * time.Second}}, "internal-mcp"+toolNameSeparator+"add_numbers", argJSON)
	expected := "The result of 100 + 250 is 350"
	if resStr != expected {
		t.Errorf("Expected '%s', got '%s'", expected, resStr)
	}
}

func TestMCPStdioExecuteTerminal(t *testing.T) {
	enableAllNativeTools(t)
	ctx := context.Background()

	buildCmd := exec.Command("go", "build", "-o", "alpaka_test_bin", ".")
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("Failed to build alpaka binary: %v", err)
	}
	defer exec.Command("rm", "-f", "alpaka_test_bin").Run()

	mcpClient, err := client.NewStdioMCPClient("./alpaka_test_bin", []string{tools.ToolTimeoutEnv + "=1"}, "internal-mcp")
	if err != nil {
		t.Fatalf("Failed to start stdio MCP client: %v", err)
	}
	defer mcpClient.Close()

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "alpaka-stdio-test", Version: "test"}
	if _, err := mcpClient.Initialize(ctx, initReq); err != nil {
		t.Fatalf("Failed to initialize stdio client: %v", err)
	}

	conns := map[string]*mcpConnection{
		internalServerName: {
			Name:           internalServerName,
			Client:         mcpClient,
			Timeout:        5 * time.Second,
			MaxResultChars: defaultMCPMaxResultChars,
		},
	}
	toolName := internalServerName + toolNameSeparator + "execute_terminal"

	// 1. Erfolgreicher Befehl über Stdio (darf den RPC-Stream nicht stören)
	res := executeLocalTool(ctx, conns, toolName, `{"command": "echo hello_stdio"}`)
	if strings.TrimSpace(res) != "hello_stdio" {
		t.Errorf("Expected 'hello_stdio', got %q", res)
	}

	// 2. Fehlgeschlagener Befehl muss als "Tool error:" (IsError: true) zurückkommen
	errRes := executeLocalTool(ctx, conns, toolName, `{"command": "echo fail_msg >&2 && exit 42"}`)
	if !strings.HasPrefix(errRes, "Tool error:") || !strings.Contains(errRes, "fail_msg") {
		t.Errorf("Expected Tool error with 'fail_msg', got %q", errRes)
	}

	// 3. Der Server bekommt den Abbruch des Clients nicht mit. Das Tool muss den Befehl samt
	// Kindprozessen selbst nach ALPAKA_TOOL_TIMEOUT_SECONDS (hier 1s) beenden.
	if runtime.GOOS == "windows" {
		return
	}
	conns[internalServerName].Timeout = 500 * time.Millisecond
	start := time.Now()
	timeoutRes := executeLocalTool(ctx, conns, toolName, `{"command": "sleep 9.31 & wait"}`)
	if time.Since(start) > 4*time.Second {
		t.Errorf("tool call was not interrupted (took %s)", time.Since(start))
	}
	if !strings.Contains(timeoutRes, "timeout") && !strings.Contains(timeoutRes, "timed out") {
		t.Errorf("Expected timeout message, got %q", timeoutRes)
	}

	deadline := time.Now().Add(6 * time.Second)
	for {
		out, _ := exec.Command("pgrep", "-f", "sleep 9.31").Output()
		if len(strings.TrimSpace(string(out))) == 0 {
			break
		}
		if time.Now().After(deadline) {
			exec.Command("pkill", "-f", "sleep 9.31").Run()
			t.Fatalf("process 'sleep 9.31' is still running after the tool timeout")
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// enableAllNativeTools isolates the test from the real ~/.alpaka and enables all native
// tools (they are disabled by default). Subprocesses inherit the temporary HOME.
func enableAllNativeTools(t *testing.T) {
	t.Helper()
	// "go build" derives its caches from HOME; pin them so the temp HOME doesn't get a read-only modcache
	for _, key := range []string{"GOPATH", "GOMODCACHE", "GOCACHE"} {
		if out, err := exec.Command("go", "env", key).Output(); err == nil {
			t.Setenv(key, strings.TrimSpace(string(out)))
		}
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	if err := os.MkdirAll(os.Getenv("HOME")+"/.alpaka", 0755); err != nil {
		t.Fatal(err)
	}
	cfg := &Config{}
	for name := range tools.Registry {
		cfg.EnabledTools = append(cfg.EnabledTools, name)
	}
	if err := saveConfig(cfg); err != nil {
		t.Fatal(err)
	}
}
