package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func TestTransportType(t *testing.T) {
	tests := []struct {
		srv  MCPServerConfig
		want string
	}{
		{MCPServerConfig{Command: "npx"}, transportStdio},
		{MCPServerConfig{URL: "https://x.example/mcp"}, transportHTTP},
		{MCPServerConfig{URL: "https://x.example/sse"}, transportSSE},
		{MCPServerConfig{URL: "https://x.example/sse/"}, transportSSE},
		{MCPServerConfig{URL: "https://x.example/mcp", Type: "sse"}, transportSSE},
		{MCPServerConfig{URL: "https://x.example/mcp", Type: "streamable-http"}, transportHTTP},
		{MCPServerConfig{}, ""},
	}
	for _, tt := range tests {
		if got := transportType(tt.srv); got != tt.want {
			t.Errorf("transportType(%+v) = %q, want %q", tt.srv, got, tt.want)
		}
	}
}

func TestValidateServerConfig(t *testing.T) {
	valid := []MCPServerConfig{
		{Command: "npx"},
		{URL: "https://x.example/mcp"},
		{URL: "http://localhost:8080/sse", Timeout: 60},
	}
	invalid := []MCPServerConfig{
		{},
		{Command: "npx", URL: "https://x.example/mcp"},
		{URL: "ftp://x.example"},
		{URL: "not a url"},
		{Command: "npx", Type: "websocket"},
		{Command: "npx", Type: "http"},
		{URL: "https://x.example/mcp", Type: "stdio"},
		{Command: "npx", Timeout: -1},
		{Command: "npx", Timeout: 100000},
	}
	for _, srv := range valid {
		if err := validateServerConfig(srv); err != nil {
			t.Errorf("expected %+v to be valid, got %v", srv, err)
		}
	}
	for _, srv := range invalid {
		if err := validateServerConfig(srv); err == nil {
			t.Errorf("expected %+v to be invalid", srv)
		}
	}
}

func TestResolveTimeout(t *testing.T) {
	t.Setenv("ALPAKA_MCP_TIMEOUT", "")
	if got := resolveTimeout(MCPServerConfig{}, &Config{}); got != defaultMCPTimeout {
		t.Errorf("default: got %s", got)
	}

	t.Setenv("ALPAKA_MCP_TIMEOUT", "7")
	if got := resolveTimeout(MCPServerConfig{}, &Config{}); got != 7*time.Second {
		t.Errorf("env: got %s", got)
	}

	if got := resolveTimeout(MCPServerConfig{}, &Config{MCPTimeout: 20}); got != 20*time.Second {
		t.Errorf("global config beats env: got %s", got)
	}

	if got := resolveTimeout(MCPServerConfig{Timeout: 5}, &Config{MCPTimeout: 20}); got != 5*time.Second {
		t.Errorf("server beats global: got %s", got)
	}

	t.Setenv("ALPAKA_MCP_TIMEOUT", "garbage")
	if got := resolveTimeout(MCPServerConfig{}, nil); got != defaultMCPTimeout {
		t.Errorf("invalid env falls back to default: got %s", got)
	}
}

func TestInsecureURLWarning(t *testing.T) {
	if w := insecureURLWarning(MCPServerConfig{URL: "http://example.com/mcp"}); w == "" {
		t.Error("expected warning for remote http")
	}
	for _, u := range []string{"https://example.com/mcp", "http://localhost:3000/mcp", "http://127.0.0.1/mcp"} {
		if w := insecureURLWarning(MCPServerConfig{URL: u}); w != "" {
			t.Errorf("unexpected warning for %s: %s", u, w)
		}
	}
}

// requireBearer wraps a handler and rejects requests without the expected Authorization header
func requireBearer(next http.Handler, token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func callAddNumbers(t *testing.T, conn *mcpConnection) string {
	t.Helper()
	return executeLocalTool(context.Background(), map[string]*mcpConnection{"remote": conn},
		"remote"+toolNameSeparator+"add_numbers", `{"a": 2, "b": 3}`)
}

func TestConnectServerStreamableHTTPWithHeaders(t *testing.T) {
	enableAllNativeTools(t)
	t.Setenv("ALPAKA_TEST_TOKEN", "s3cret")
	ts := httptest.NewServer(requireBearer(server.NewStreamableHTTPServer(createMCPServer()), "s3cret"))
	defer ts.Close()

	srv := MCPServerConfig{URL: ts.URL + "/mcp", Headers: map[string]string{"Authorization": "Bearer ${ALPAKA_TEST_TOKEN}"}}
	c, err := connectServer(context.Background(), srv, 10*time.Second)
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer c.Close()

	conn := &mcpConnection{Name: "remote", Client: c, Timeout: 10 * time.Second}
	if got := callAddNumbers(t, conn); got != "The result of 2 + 3 is 5" {
		t.Errorf("unexpected result: %q", got)
	}

	tools, err := getAllAvailableTools(context.Background(), map[string]*mcpConnection{"remote": conn})
	if err != nil || len(tools) == 0 {
		t.Fatalf("expected prefixed tools, got %v (%v)", tools, err)
	}
}

func TestConnectServerHTTPWrongToken(t *testing.T) {
	ts := httptest.NewServer(requireBearer(server.NewStreamableHTTPServer(createMCPServer()), "right"))
	defer ts.Close()

	srv := MCPServerConfig{URL: ts.URL + "/mcp", Headers: map[string]string{"Authorization": "Bearer wrong"}}
	if c, err := connectServer(context.Background(), srv, 5*time.Second); err == nil {
		c.Close()
		t.Error("expected handshake to fail with a wrong token")
	}
}

func TestConnectServerSSE(t *testing.T) {
	enableAllNativeTools(t)
	ts := server.NewTestServer(createMCPServer())
	defer ts.Close()

	srv := MCPServerConfig{URL: ts.URL + "/sse"}
	if transportType(srv) != transportSSE {
		t.Fatalf("expected sse transport")
	}
	c, err := connectServer(context.Background(), srv, 10*time.Second)
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer c.Close()

	conn := &mcpConnection{Name: "remote", Client: c, Timeout: 10 * time.Second}
	if got := callAddNumbers(t, conn); got != "The result of 2 + 3 is 5" {
		t.Errorf("unexpected result: %q", got)
	}
}

func TestConnectServerMissingEnvVar(t *testing.T) {
	srv := MCPServerConfig{URL: "http://localhost:1/mcp", Headers: map[string]string{"Authorization": "Bearer ${ALPAKA_DEFINITELY_UNSET_VAR}"}}
	_, err := connectServer(context.Background(), srv, time.Second)
	if err == nil || !strings.Contains(err.Error(), "ALPAKA_DEFINITELY_UNSET_VAR") {
		t.Errorf("expected missing variable error, got %v", err)
	}
}

func TestToolCallTimeout(t *testing.T) {
	s := server.NewMCPServer("slow", "test")
	s.AddTool(mcp.NewTool("sleep"), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		select {
		case <-time.After(5 * time.Second):
		case <-ctx.Done():
		}
		return mcp.NewToolResultText("done"), nil
	})
	ts := httptest.NewServer(server.NewStreamableHTTPServer(s))
	defer ts.Close()

	c, err := connectServer(context.Background(), MCPServerConfig{URL: ts.URL + "/mcp"}, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	conn := &mcpConnection{Name: "slow", Client: c, Timeout: 300 * time.Millisecond}
	start := time.Now()
	res := executeLocalTool(context.Background(), map[string]*mcpConnection{"slow": conn}, "slow"+toolNameSeparator+"sleep", `{}`)
	if time.Since(start) > 3*time.Second {
		t.Errorf("tool call was not interrupted by the timeout")
	}
	if !strings.Contains(res, "timeout") {
		t.Errorf("expected timeout message, got %q", res)
	}
}

func TestFormatToolResult(t *testing.T) {
	multi := &mcp.CallToolResult{Content: []mcp.Content{
		mcp.NewTextContent("first"),
		mcp.NewTextContent("second"),
		mcp.NewImageContent("AAAA", "image/png"),
		mcp.NewEmbeddedResource(mcp.TextResourceContents{URI: "file:///a.txt", Text: "hello"}),
		mcp.NewEmbeddedResource(mcp.BlobResourceContents{URI: "file:///b.bin", MIMEType: "application/octet-stream", Blob: "QUJD"}),
		mcp.NewResourceLink("file:///c", "c-file", "", ""),
	}}
	got := formatToolResult(multi, 0)
	for _, want := range []string{"first", "second", "[Image image/png", "[Resource file:///a.txt]\nhello", "[Binary resource file:///b.bin", "[Resource link: file:///c - c-file]"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in %q", want, got)
		}
	}

	errRes := mcp.NewToolResultError("boom")
	if got := formatToolResult(errRes, 0); got != "Tool error: boom" {
		t.Errorf("unexpected error format: %q", got)
	}

	structured := &mcp.CallToolResult{StructuredContent: map[string]any{"a": 1}}
	if got := formatToolResult(structured, 0); got != `{"a":1}` {
		t.Errorf("unexpected structured format: %q", got)
	}

	if got := formatToolResult(&mcp.CallToolResult{}, 0); !strings.Contains(got, "did not return any content") {
		t.Errorf("unexpected empty format: %q", got)
	}

	long := mcp.NewToolResultText(strings.Repeat("ä", 50))
	got = formatToolResult(long, 10)
	if !strings.HasPrefix(got, strings.Repeat("ä", 10)) || !strings.Contains(got, "[truncated 40 chars]") {
		t.Errorf("unexpected truncation: %q", got)
	}
}

func TestResolveMaxResultChars(t *testing.T) {
	if resolveMaxResultChars(nil) != defaultMCPMaxResultChars || resolveMaxResultChars(&Config{}) != defaultMCPMaxResultChars {
		t.Error("expected default")
	}
	if resolveMaxResultChars(&Config{MCPMaxResultChars: 500}) != 500 {
		t.Error("expected configured value")
	}
}
