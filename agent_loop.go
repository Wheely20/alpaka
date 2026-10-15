package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
	"unicode"

	"alpaka/tools"

	"github.com/mark3labs/mcp-go/mcp"
)

// chat message structure
type ChatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

// tool call structure
type ToolCall struct {
	Index    *int         `json:"index,omitempty"`
	ID       string       `json:"id,omitempty"`
	Type     string       `json:"type,omitempty"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // Das ist ein JSON-String, den das Modell generiert
}

// api response from server
type ChatResponse struct {
	Choices []struct {
		Message      ChatMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
}

// Token usage structure
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

func sendToModelWithTools(ctx context.Context, reader *bufio.Reader, serverURL string, mcpClients map[string]*mcpConnection, history []ChatMessage, availableTools []interface{}) ([]ChatMessage, error) {
	// 1. prepare the request body for the API
	reqData := map[string]interface{}{
		"model":    "default",
		"messages": history,
		"stream":   true,
		"stream_options": map[string]interface{}{
			"include_usage": true, // get stats
		},
	}

	// Only include tools if there are any available
	if len(availableTools) > 0 {
		reqData["tools"] = availableTools
	}

	requestBody, err := json.Marshal(reqData)
	if err != nil {
		return history, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", serverURL+"/v1/chat/completions", bytes.NewBuffer(requestBody))
	if err != nil {
		return history, err
	}
	req.Header.Set("Content-Type", "application/json")

	startTime := time.Now()
	var firstTokenTime time.Time
	var finalUsage *Usage

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return history, err
	}
	defer resp.Body.Close()

	// 2. read the response line by line (streaming)
	scanner := bufio.NewScanner(resp.Body)
	var fullContent string
	var finalFinishReason string
	var accumulatedToolCalls []ToolCall

	for scanner.Scan() {
		line := scanner.Text()

		// Unwichtige Zeilen überspringen
		if !strings.HasPrefix(line, "data: ") || line == "data: [DONE]" {
			continue
		}

		// "data: " abschneiden, um sauberes JSON zu erhalten
		dataStr := strings.TrimPrefix(line, "data: ")

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string     `json:"content"`
					ToolCalls []ToolCall `json:"tool_calls"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
			Usage *Usage `json:"usage,omitempty"` // getting usage stats from the stream
		}

		if err := json.Unmarshal([]byte(dataStr), &chunk); err != nil {
			continue
		}

		// save stats
		if chunk.Usage != nil {
			finalUsage = chunk.Usage
		}

		if len(chunk.Choices) > 0 {
			choice := chunk.Choices[0]
			if choice.FinishReason != "" {
				finalFinishReason = choice.FinishReason
			}

			// a) Text direkt ins Terminal drucken
			if choice.Delta.Content != "" {
				// time of the first token
				if firstTokenTime.IsZero() {
					firstTokenTime = time.Now()
				}
				fmt.Print(choice.Delta.Content)
				fullContent += choice.Delta.Content
			}

			// b) Zerstückelte Tool-Calls wieder zusammenbauen
			for _, tc := range choice.Delta.ToolCalls {
				if tc.Index != nil {
					idx := *tc.Index
					// Slice vergrößern, falls ein neues Tool aufgerufen wird
					for len(accumulatedToolCalls) <= idx {
						accumulatedToolCalls = append(accumulatedToolCalls, ToolCall{})
					}
					// Daten anfügen
					if tc.ID != "" {
						accumulatedToolCalls[idx].ID = tc.ID
					}
					if tc.Type != "" {
						accumulatedToolCalls[idx].Type = tc.Type
					}
					if tc.Function.Name != "" {
						accumulatedToolCalls[idx].Function.Name = tc.Function.Name
					}
					if tc.Function.Arguments != "" {
						accumulatedToolCalls[idx].Function.Arguments += tc.Function.Arguments
					}
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return history, err
	}
	fmt.Println() // Zeilenumbruch nach der fertigen Antwort

	// --- PRINT STATS ---
	if finalUsage != nil && !firstTokenTime.IsZero() {
		promptSecs := firstTokenTime.Sub(startTime).Seconds()
		genSecs := time.Since(firstTokenTime).Seconds()

		promptSpeed := float64(finalUsage.PromptTokens) / promptSecs
		genSpeed := float64(finalUsage.CompletionTokens) / genSecs

		// \033[35m ANSI Farbcode für Magenta/Pink
		fmt.Printf("\n\033[35m[ Prompt: %.1f t/s | Generation: %.1f t/s ]\033[0m\n", promptSpeed, genSpeed)
	}

	// 3. Add the model's response to the history
	responseMsg := ChatMessage{
		Role:      "assistant",
		Content:   fullContent,
		ToolCalls: accumulatedToolCalls,
	}
	history = append(history, responseMsg)

	// --- AGENTIC LOOP ---
	if finalFinishReason == "tool_calls" && len(accumulatedToolCalls) > 0 {
		cfg, cfgErr := loadConfig()
		if cfgErr != nil {
			fmt.Printf("Warning: could not load config, 'confirm_tools' may not apply: %v\n", cfgErr)
		}
		for _, toolCall := range accumulatedToolCalls {
			var toolResultString string

			if requiresToolConfirmation(toolCall.Function.Name, cfg) {
				if !promptToolConfirmation(reader, toolCall.Function.Name, toolCall.Function.Arguments) {
					fmt.Println("\033[31m[Execution denied by user]\033[0m")
					toolResultString = "Tool error: The user denied the execution of this command."
				} else {
					fmt.Printf("[Using Tool: %s...]\n", toolCall.Function.Name)
					toolResultString = executeLocalTool(ctx, mcpClients, toolCall.Function.Name, toolCall.Function.Arguments)
				}
			} else {
				fmt.Printf("[Using Tool: %s...]\n", toolCall.Function.Name)
				toolResultString = executeLocalTool(ctx, mcpClients, toolCall.Function.Name, toolCall.Function.Arguments)
			}

			history = append(history, ChatMessage{
				Role:       "tool",
				ToolCallID: toolCall.ID,
				Name:       toolCall.Function.Name,
				Content:    toolResultString,
			})
		}

		// Rekursion für die Folge-Antwort nach der Tool-Ausführung
		return sendToModelWithTools(ctx, reader, serverURL, mcpClients, history, availableTools)
	}

	return history, nil
}

// executeLocalTool schickt den Tool-Call an den laufenden MCP-Server
func executeLocalTool(ctx context.Context, mcpClients map[string]*mcpConnection, name string, argumentsJSON string) string {
	// 1. Argumente vom Modell (JSON-String) in eine Map umwandeln
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return fmt.Sprintf("System error while parsing arguments: %v", err)
	}

	// 2. Tool-Namen aufteilen (Routing)
	parts := strings.SplitN(name, toolNameSeparator, 2)
	if len(parts) != 2 {
		return fmt.Sprintf("System error: Invalid tool name format '%s'", name)
	}
	serverName := parts[0]
	toolName := parts[1]

	// 3. Den zuständigen Client aus der Map holen
	conn, ok := mcpClients[serverName]
	if !ok {
		return fmt.Sprintf("System error: MCP server '%s' not found or inactive", serverName)
	}

	// 4. Den Request an den MCP-Server zusammenbauen
	req := mcp.CallToolRequest{}
	req.Params.Name = toolName
	req.Params.Arguments = args

	// 5. Das Tool auf dem Server ausführen lassen (mit Timeout, damit ein hängender Server den Chat nicht blockiert)
	callCtx := ctx
	if conn.Timeout > 0 {
		var cancel context.CancelFunc
		callCtx, cancel = context.WithTimeout(ctx, conn.Timeout)
		defer cancel()
	}
	result, err := conn.Client.CallTool(callCtx, req)
	if err != nil {
		if callCtx.Err() == context.DeadlineExceeded {
			return fmt.Sprintf("Error while executing tool: timeout after %s (server '%s')", conn.Timeout, serverName)
		}
		return fmt.Sprintf("Error while executing tool: %v", err)
	}

	// 6. Alle Content-Blöcke auswerten, IsError beachten und die Länge begrenzen
	return formatToolResult(result, conn.MaxResultChars)
}

// requiresToolConfirmation checks, if a tool needs confirmation before execution
func requiresToolConfirmation(prefixedName string, cfg *Config) bool {
	parts := strings.SplitN(prefixedName, toolNameSeparator, 2)
	if len(parts) != 2 {
		return false
	}
	serverName, toolName := parts[0], parts[1]

	// 1. Interne Tools fragen direkt die Registry
	if serverName == internalServerName {
		if def, ok := tools.Registry[toolName]; ok && def.NeedsConfirmation {
			return true
		}
	}

	// 2. Optional: Liste in Config für externe Tools (z.B. cfg.ConfirmTools)
	if cfg != nil {
		for _, pattern := range cfg.ConfirmTools {
			if pattern == prefixedName || pattern == serverName+toolNameSeparator+"*" {
				return true
			}
		}
	}
	return false
}

// escapeControlChars makes control and invisible formatting characters visible (e.g. \x1b, \x0d, U+202E),
// so the model cannot hide or rewrite parts of a command in the confirmation prompt.
// Newlines and tabs are kept.
func escapeControlChars(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\n' || r == '\t':
			b.WriteRune(r)
		case unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == '\u2028' || r == '\u2029':
			if r < 0x100 {
				fmt.Fprintf(&b, "\\x%02x", r)
			} else {
				fmt.Fprintf(&b, "\\u%04x", r)
			}
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// indentLines indents every line after the first so multi-line commands stay visually grouped
func indentLines(s, indent string) string {
	return strings.ReplaceAll(s, "\n", "\n"+indent)
}

// formatConfirmationPrompt builds the text shown before a tool is executed.
// All model-controlled values are escaped, and every argument is shown.
func formatConfirmationPrompt(toolName string, argumentsJSON string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "\n\033[1;31m[Security Warning]\033[0m The model wants to execute \033[1m%s\033[0m\n", escapeControlChars(toolName))

	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		fmt.Fprintf(&sb, "  Raw arguments: %s\n", indentLines(escapeControlChars(argumentsJSON), "    "))
		return sb.String()
	}

	other := make(map[string]interface{})
	for k, v := range args {
		switch k {
		case "command", "workdir":
			if str, ok := v.(string); ok {
				if k == "command" {
					fmt.Fprintf(&sb, "  Command: \033[33m%s\033[0m\n", indentLines(escapeControlChars(str), "    "))
				} else if strings.TrimSpace(str) != "" {
					fmt.Fprintf(&sb, "  Workdir: %s\n", escapeControlChars(str))
				}
				continue
			}
			other[k] = v // unexpected type: show it raw
		default:
			other[k] = v
		}
	}
	if len(other) > 0 {
		data, _ := json.Marshal(other)
		fmt.Fprintf(&sb, "  Other arguments: %s\n", escapeControlChars(string(data)))
	}
	return sb.String()
}

// promptToolConfirmation shows the complete command in the main process and reads from the shared reader.
func promptToolConfirmation(reader *bufio.Reader, toolName string, argumentsJSON string) bool {
	fmt.Print(formatConfirmationPrompt(toolName, argumentsJSON))
	fmt.Print("Allow execution? [y/N]: ")
	response, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == "yes"
}

// getAllAvailableTools gets the tools from the MCP-Servers and converts them to the OpenAI format
func getAllAvailableTools(ctx context.Context, clients map[string]*mcpConnection) ([]interface{}, error) {
	var allTools []interface{}

	// Deterministic order
	serverNames := make([]string, 0, len(clients))
	for name := range clients {
		serverNames = append(serverNames, name)
	}
	sort.Strings(serverNames)

	for _, serverName := range serverNames {
		conn := clients[serverName]
		listCtx, cancel := context.WithTimeout(ctx, conn.Timeout)
		toolList, err := conn.Client.ListTools(listCtx, mcp.ListToolsRequest{})
		cancel()
		if err != nil {
			// A server without tool support is fine, everything else deserves a warning
			if !strings.Contains(err.Error(), "method not found") && !strings.Contains(err.Error(), "not supported") {
				fmt.Printf("Warning: Could not list tools of MCP server '%s': %v\n", serverName, err)
			}
			continue
		}

		for _, t := range toolList.Tools {
			// Prefix the tool name: "servername__toolname"
			prefixedName := serverName + toolNameSeparator + t.Name

			allTools = append(allTools, map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name":        prefixedName,
					"description": fmt.Sprintf("[%s] %s", serverName, t.Description),
					"parameters":  t.InputSchema,
				},
			})
		}
	}
	return allTools, nil
}

func startTerminalChat(ctx context.Context, serverURL string, mcpClients map[string]*mcpConnection, sysPrompt string, availableTools []interface{}) {
	reader := bufio.NewReader(os.Stdin)
	var history []ChatMessage

	if sysPrompt != "" {
		history = append(history, ChatMessage{Role: "system", Content: sysPrompt})
	}

	for {
		// \033[1;32m = Fett Grün, \033[32m = Normal Grün, \033[0m = Reset
		fmt.Print("\n\033[1;32m[> \033[0m\033[32m")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		fmt.Print("\033[0m") // revert to default color

		if input == "/exit" {
			break
		}
		if input == "/clear" {
			history = nil
			fmt.Println("\033[33mChat history cleared.\033[0m")
			continue
		}
		if input == "" {
			continue
		}

		// append user message to history
		history = append(history, ChatMessage{Role: "user", Content: input})

		// call the agent loop
		newHistory, err := sendToModelWithTools(ctx, reader, serverURL, mcpClients, history, availableTools)
		if err != nil {
			fmt.Printf("\n  [Error: %v]\n", err)
		} else {
			history = newHistory // update history with the new messages from the model
		}
	}
}
