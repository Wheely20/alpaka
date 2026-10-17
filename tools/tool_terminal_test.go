package tools

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

func callTerminal(t *testing.T, command string) *mcp.CallToolResult {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{"command": command}
	res, err := terminalHandler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	return res
}

func resultText(res *mcp.CallToolResult) string {
	var sb strings.Builder
	for _, c := range res.Content {
		if tc, ok := mcp.AsTextContent(c); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String()
}

func TestTerminalHandlerOutputAndErrors(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh syntax")
	}
	res := callTerminal(t, "echo hi")
	if res.IsError || strings.TrimSpace(resultText(res)) != "hi" {
		t.Errorf("unexpected success result: %+v", res)
	}

	res = callTerminal(t, "echo oops >&2; exit 3")
	if !res.IsError || !strings.Contains(resultText(res), "oops") {
		t.Errorf("expected error result with output: %+v", res)
	}
}

func TestTerminalHandlerKillsProcessGroupOnTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh and pgrep")
	}
	t.Setenv(ToolTimeoutEnv, "1")

	start := time.Now()
	res := callTerminal(t, "sleep 8.47 & wait")
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("handler did not stop at the timeout (took %s)", elapsed)
	}
	if !res.IsError || !strings.Contains(resultText(res), "timed out") {
		t.Errorf("expected timeout error, got %+v", res)
	}

	time.Sleep(200 * time.Millisecond)
	if out, _ := exec.Command("pgrep", "-f", "sleep 8.47").Output(); strings.TrimSpace(string(out)) != "" {
		exec.Command("pkill", "-f", "sleep 8.47").Run()
		t.Error("child process survived the timeout")
	}
}

func TestTerminalTimeoutConfig(t *testing.T) {
	t.Setenv(ToolTimeoutEnv, "")
	if terminalTimeout() != defaultTerminalTimeout {
		t.Error("expected default")
	}
	t.Setenv(ToolTimeoutEnv, "7")
	if terminalTimeout() != 7*time.Second {
		t.Error("expected 7s")
	}
	t.Setenv(ToolTimeoutEnv, "-3")
	if terminalTimeout() != defaultTerminalTimeout {
		t.Error("invalid value must fall back to default")
	}
}

func TestLimitedBuffer(t *testing.T) {
	b := &limitedBuffer{max: 10}
	n, err := b.Write([]byte("0123456789ABCDEF"))
	if n != 16 || err != nil {
		t.Errorf("Write must report the full length, got %d, %v", n, err)
	}
	b.Write([]byte("more"))
	s := b.String()
	if !strings.HasPrefix(s, "0123456789") || strings.Contains(s, "ABCDEF") || !strings.Contains(s, "truncated") {
		t.Errorf("unexpected buffer content: %q", s)
	}
}
