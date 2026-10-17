package main

import (
	"bufio"
	"strings"
	"testing"
)

func TestEscapeControlChars(t *testing.T) {
	tests := []struct{ in, want string }{
		{"ls -la", "ls -la"},
		{"a\nb\tc", "a\nb\tc"},
		{"safe\r rm -rf /", `safe\x0d rm -rf /`},
		{"\x1b[2K\x1b[1Aecho hi", `\x1b[2K\x1b[1Aecho hi`},
		{"x\u202ey", `x\u202ey`}, // bidi override
		{"ümlaut ✓", "ümlaut ✓"},
	}
	for _, tt := range tests {
		if got := escapeControlChars(tt.in); got != tt.want {
			t.Errorf("escapeControlChars(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestFormatConfirmationPrompt(t *testing.T) {
	// Escape sequences must not reach the terminal, the real command must stay visible
	p := formatConfirmationPrompt("alpaka__execute_terminal", `{"command":"echo ok\r\u001b[2Krm -rf ~","workdir":"/tmp"}`)
	if strings.Contains(p, "\r") || strings.Contains(p, "\x1b[2K") {
		t.Errorf("control characters leaked into the prompt: %q", p)
	}
	if !strings.Contains(p, `rm -rf ~`) || !strings.Contains(p, "Workdir: /tmp") {
		t.Errorf("command or workdir missing: %q", p)
	}

	// Multi-line commands are shown completely
	p = formatConfirmationPrompt("t", `{"command":"echo a\necho b"}`)
	if !strings.Contains(p, "echo a") || !strings.Contains(p, "echo b") {
		t.Errorf("multi-line command incomplete: %q", p)
	}

	// Unknown arguments are always shown
	p = formatConfirmationPrompt("t", `{"command":"ls","evil":"x"}`)
	if !strings.Contains(p, "Other arguments") || !strings.Contains(p, "evil") {
		t.Errorf("extra argument not shown: %q", p)
	}

	// A command with an unexpected type and invalid JSON are shown raw
	p = formatConfirmationPrompt("t", `{"command":["a","b"]}`)
	if !strings.Contains(p, "Other arguments") {
		t.Errorf("non-string command not shown: %q", p)
	}
	p = formatConfirmationPrompt("t", "not json \x1b[31m")
	if !strings.Contains(p, "Raw arguments") || strings.Contains(p, "\x1b[31m") {
		t.Errorf("invalid json not handled: %q", p)
	}
}

func TestPromptToolConfirmationAnswers(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"y\n", true},
		{"YES\n", true},
		{" yes \n", true},
		{"n\n", false},
		{"\n", false},
		{"maybe\n", false},
		{"y", false}, // EOF without newline = no confirmation
		{"", false},
	}
	for _, tt := range tests {
		r := bufio.NewReader(strings.NewReader(tt.input))
		if got := promptToolConfirmation(r, "t", `{"command":"ls"}`); got != tt.want {
			t.Errorf("input %q: got %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestRequiresToolConfirmation(t *testing.T) {
	cfg := &Config{ConfirmTools: []string{"github__create_issue", "remote__*"}}
	cases := map[string]bool{
		"alpaka__execute_terminal": true,  // registry flag
		"alpaka__add_numbers":      false, // no flag
		"github__create_issue":     true,
		"github__list_issues":      false,
		"remote__anything":         true,
		"invalid":                  false,
	}
	for name, want := range cases {
		if got := requiresToolConfirmation(name, cfg); got != want {
			t.Errorf("requiresToolConfirmation(%q) = %v, want %v", name, got, want)
		}
	}
	if !requiresToolConfirmation("alpaka__execute_terminal", nil) {
		t.Error("registry flag must also apply without config")
	}
}
