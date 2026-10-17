package tools

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

const maxTerminalOutputBytes = 64 * 1024 // 64 KB hard limit

const (
	// ToolTimeoutEnv is set by the Alpaka client for the internal server (seconds).
	ToolTimeoutEnv = "ALPAKA_TOOL_TIMEOUT_SECONDS"
	// defaultTerminalTimeout applies if the variable is not set
	defaultTerminalTimeout = 2 * time.Minute
)

// terminalTimeout returns the deadline for a command. The MCP server runs in a subprocess and
// is not notified when the client gives up, so it has to stop the command on its own.
func terminalTimeout() time.Duration {
	if v := os.Getenv(ToolTimeoutEnv); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return defaultTerminalTimeout
}

// limitedBuffer deletes any output beyond a certain size limit
type limitedBuffer struct {
	buf       bytes.Buffer
	max       int
	truncated bool
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	origLen := len(p)
	remaining := l.max - l.buf.Len()
	if remaining <= 0 {
		l.truncated = true
		return origLen, nil // return success, but don't write anything
	}
	if len(p) > remaining {
		p = p[:remaining]
		l.truncated = true
	}
	_, err := l.buf.Write(p)
	return origLen, err
}

func (l *limitedBuffer) String() string {
	s := l.buf.String()
	if l.truncated {
		s += fmt.Sprintf("\n...[output truncated at %d bytes]", l.max)
	}
	return s
}

func init() {
	tool := mcp.NewTool("execute_terminal",
		mcp.WithDescription("Executes a shell command on the user's system. CRITICAL RULE: The execution environment is headless. You MUST use non-interactive flags (e.g., -y, --yes, --non-interactive) for ANY CLI tools (like claude, codex, npm, git). Interactive prompts will hang and get killed."),
		mcp.WithString("command", mcp.Required(), mcp.Description("The shell command to execute.")),
		mcp.WithString("workdir", mcp.Description("Optional working directory for the command.")),
	)

	RegisterWithConfirmation("execute_terminal", &tool, terminalHandler)
}

func terminalHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, ok := request.Params.Arguments.(map[string]interface{})
	if !ok {
		return mcp.NewToolResultError("Arguments could not be parsed."), nil
	}

	command, okCmd := args["command"].(string)
	if !okCmd || strings.TrimSpace(command) == "" {
		return mcp.NewToolResultError("Parameter 'command' must be a non-empty string."), nil
	}

	workdir, _ := args["workdir"].(string)

	timeout := terminalTimeout()
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(execCtx, "cmd", "/c", command)
	} else {
		cmd = exec.CommandContext(execCtx, "sh", "-c", command)
	}

	if strings.TrimSpace(workdir) != "" {
		cmd.Dir = workdir
	}

	// Setup process group & wait up to 2 seconds for open pipes after cancel
	setupCmdProcessGroup(cmd)
	cmd.WaitDelay = 2 * time.Second

	outBuf := &limitedBuffer{max: maxTerminalOutputBytes}
	cmd.Stdout = outBuf
	cmd.Stderr = outBuf

	err := cmd.Run()
	outStr := strings.TrimSpace(outBuf.String())

	// 1. Timeout: the command and its whole process group have been killed
	if execCtx.Err() == context.DeadlineExceeded {
		msg := fmt.Sprintf("Command timed out after %s and was terminated (it may have waited for interactive user input).", timeout)
		if outStr != "" {
			msg += "\nPartial output:\n" + outStr
		}
		return mcp.NewToolResultError(msg), nil
	}

	// 2. Exit-Code != 0 -> Return actual tool error (IsError: true)
	if err != nil {
		msg := fmt.Sprintf("Command failed (%v).", err)
		if outStr != "" {
			msg += "\nOutput:\n" + outStr
		}
		return mcp.NewToolResultError(msg), nil
	}

	// 3. Success
	if outStr == "" {
		outStr = "Command executed successfully with no output."
	}
	return mcp.NewToolResultText(outStr), nil
}
