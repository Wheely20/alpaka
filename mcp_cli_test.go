package main

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/client"
)

func TestValidateServerName(t *testing.T) {
	valid := []string{"github", "github-mcp-server", "my_server", "rea2"}
	invalid := []string{"", "alpaka", "a__b", "_a", "a_", "with space", "a/b"}

	for _, n := range valid {
		if err := validateServerName(n); err != nil {
			t.Errorf("expected '%s' to be valid, got %v", n, err)
		}
	}
	for _, n := range invalid {
		if err := validateServerName(n); err == nil {
			t.Errorf("expected '%s' to be invalid", n)
		}
	}
}

func TestExpandEnvStrict(t *testing.T) {
	t.Setenv("ALPAKA_TEST_SET", "secret")
	os.Unsetenv("ALPAKA_TEST_UNSET")

	got, missing := expandEnvStrict("tok-${ALPAKA_TEST_SET}")
	if got != "tok-secret" || len(missing) != 0 {
		t.Errorf("unexpected result '%s' (missing %v)", got, missing)
	}

	_, missing = expandEnvStrict("${ALPAKA_TEST_UNSET}")
	if len(missing) != 1 || missing[0] != "ALPAKA_TEST_UNSET" {
		t.Errorf("expected missing variable to be reported, got %v", missing)
	}
}

func TestExecuteLocalToolRouting(t *testing.T) {
	ctx := context.Background()
	clients := map[string]client.MCPClient{}

	if res := executeLocalTool(ctx, clients, "no_separator", `{}`); !strings.Contains(res, "Invalid tool name format") {
		t.Errorf("expected format error, got '%s'", res)
	}
	if res := executeLocalTool(ctx, clients, "unknown"+toolNameSeparator+"tool", `{}`); !strings.Contains(res, "not found or inactive") {
		t.Errorf("expected unknown server error, got '%s'", res)
	}
}

func TestMCPServerConfigLifecycle(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	if err := initAlpakaDir(); err != nil {
		t.Fatalf("initAlpakaDir: %v", err)
	}

	mcpAddEnv = []string{"TOKEN=${SOME_TOKEN}"}
	defer func() { mcpAddEnv = nil }()
	if err := mcpAddCmd.RunE(mcpAddCmd, []string{"demo", "npx", "-y", "pkg"}); err != nil {
		t.Fatalf("add failed: %v", err)
	}
	if err := mcpAddCmd.RunE(mcpAddCmd, []string{"demo", "npx"}); err == nil {
		t.Error("expected error when adding duplicate server")
	}
	if err := mcpAddCmd.RunE(mcpAddCmd, []string{"bad__name", "npx"}); err == nil {
		t.Error("expected error for invalid server name")
	}

	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	srv := cfg.MCPServers["demo"]
	if srv.Command != "npx" || len(srv.Args) != 2 || srv.Env["TOKEN"] != "${SOME_TOKEN}" {
		t.Errorf("unexpected stored config: %+v", srv)
	}

	if err := setMCPServerDisabled("demo", true); err != nil {
		t.Fatal(err)
	}
	cfg, _ = loadConfig()
	if !cfg.MCPServers["demo"].Disabled {
		t.Error("expected server to be disabled")
	}
	if err := setMCPServerDisabled("demo", false); err != nil {
		t.Fatal(err)
	}
	cfg, _ = loadConfig()
	if cfg.MCPServers["demo"].Disabled {
		t.Error("expected server to be enabled")
	}
	if err := setMCPServerDisabled("missing", true); err == nil {
		t.Error("expected error for unknown server")
	}

	if err := mcpRemoveCmd.RunE(mcpRemoveCmd, []string{"demo"}); err != nil {
		t.Fatal(err)
	}
	cfg, _ = loadConfig()
	if _, exists := cfg.MCPServers["demo"]; exists {
		t.Error("expected server to be removed")
	}
}
