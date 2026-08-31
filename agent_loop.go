package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/mark3labs/mcp-go/client"
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
	ID       string       `json:"id"`
	Type     string       `json:"type"`
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

func sendToModelWithTools(ctx context.Context, serverURL string, mcpClient client.MCPClient, history []ChatMessage, availableTools []interface{}) ([]ChatMessage, error) {
	// 1. prepare request
	requestBody, err := json.Marshal(map[string]interface{}{
		"model":    "default",
		"messages": history,
		"tools":    availableTools,
	})
	if err != nil {
		return history, err
	}

	// 2. send HTTP POST to local llama-server
	req, err := http.NewRequestWithContext(ctx, "POST", serverURL+"/v1/chat/completions", bytes.NewBuffer(requestBody))
	if err != nil {
		return history, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return history, err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	var chatResp ChatResponse
	if err := json.Unmarshal(bodyBytes, &chatResp); err != nil {
		return history, fmt.Errorf("Error while parsing server response: %v", err)
	}

	// 3. check if response is empty
	if len(chatResp.Choices) == 0 {
		return history, fmt.Errorf("Empty response from server")
	}

	responseMsg := chatResp.Choices[0].Message
	finishReason := chatResp.Choices[0].FinishReason

	// add answer to history
	history = append(history, responseMsg)

	// --- AGENTIC LOOP ---
	// Wenn finish_reason == "tool_calls" ist, will das Modell ein Werkzeug nutzen
	if finishReason == "tool_calls" && len(responseMsg.ToolCalls) > 0 {
		for _, toolCall := range responseMsg.ToolCalls {
			fmt.Printf("\n[Alpaka führt Tool aus: %s...]\n", toolCall.Function.Name)

			// Den Request via MCP ausführen
			toolResultString := executeLocalTool(ctx, mcpClient, toolCall.Function.Name, toolCall.Function.Arguments)

			history = append(history, ChatMessage{
				Role:       "tool",
				ToolCallID: toolCall.ID,
				Name:       toolCall.Function.Name,
				Content:    toolResultString,
			})
		}

		// history goes back to the model for further processing
		return sendToModelWithTools(ctx, serverURL, mcpClient, history, availableTools)
	}

	// return the answer
	fmt.Printf("\nKI: %s\n", responseMsg.Content)
	return history, nil
}

// executeLocalTool schickt den Tool-Call an den laufenden MCP-Server
func executeLocalTool(ctx context.Context, mcpClient client.MCPClient, name string, argumentsJSON string) string {
	// 1. Argumente vom Modell (JSON-String) in eine Map umwandeln
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return fmt.Sprintf("System error while parsing arguments: %v", err)
	}

	// 2. Den Request an den MCP-Server zusammenbauen
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args

	// 3. Das Tool auf dem Server ausführen lassen
	result, err := mcpClient.CallTool(ctx, req)
	if err != nil {
		return fmt.Sprintf("Error while executing tool: %v", err)
	}

	// 4. Das Ergebnis auswerten (MCP liefert Arrays von Content-Blöcken zurück)
	if len(result.Content) > 0 {
		// Wir extrahieren den ersten Text-Block
		if textContent, ok := mcp.AsTextContent(result.Content[0]); ok {
			return textContent.Text
		}
	}

	return "The tool was executed, but did not return a text response."
}

// getAvailableTools gets the tools from the MCP-Server and converts them to the OpenAI format
func getAvailableTools(ctx context.Context, mcpClient client.MCPClient) ([]interface{}, error) {
	toolList, err := mcpClient.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, err
	}

	var openAITools []interface{}
	for _, t := range toolList.Tools {
		openAITools = append(openAITools, map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  t.InputSchema,
			},
		})
	}
	return openAITools, nil
}

func startTerminalChat(ctx context.Context, serverURL string, mcpClient client.MCPClient, sysPrompt string, availableTools []interface{}) {
	reader := bufio.NewReader(os.Stdin)
	var history []ChatMessage

	if sysPrompt != "" {
		history = append(history, ChatMessage{Role: "system", Content: sysPrompt})
	}

	fmt.Println("\n Chat gestartet. Beenden mit '/exit'.")

	for {
		fmt.Print("\nDu: ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		if input == "/exit" {
			break
		}
		if input == "" {
			continue
		}

		// append user message to history
		history = append(history, ChatMessage{Role: "user", Content: input})

		// call the agent loop
		newHistory, err := sendToModelWithTools(ctx, serverURL, mcpClient, history, availableTools)
		if err != nil {
			fmt.Printf("\n  [Fehler: %v]\n", err)
		} else {
			history = newHistory // update history with the new messages from the model
		}
	}
}
