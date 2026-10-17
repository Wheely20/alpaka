package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// formatToolResult converts all content blocks of a tool result into one text for the model.
// Binary content (images, audio, blobs) is described but not included. The result is
// limited to maxChars characters (<= 0 means defaultMCPMaxResultChars).
func formatToolResult(result *mcp.CallToolResult, maxChars int) string {
	if result == nil {
		return "The tool was executed, but did not return a result."
	}
	if maxChars <= 0 {
		maxChars = defaultMCPMaxResultChars
	}

	var parts []string
	for _, content := range result.Content {
		switch c := content.(type) {
		case mcp.TextContent:
			parts = append(parts, c.Text)
		case mcp.ImageContent:
			parts = append(parts, fmt.Sprintf("[Image %s, %d bytes (base64) - not shown]", c.MIMEType, len(c.Data)))
		case mcp.AudioContent:
			parts = append(parts, fmt.Sprintf("[Audio %s, %d bytes (base64) - not shown]", c.MIMEType, len(c.Data)))
		case mcp.EmbeddedResource:
			if text, ok := mcp.AsTextResourceContents(c.Resource); ok {
				parts = append(parts, fmt.Sprintf("[Resource %s]\n%s", text.URI, text.Text))
			} else if blob, ok := mcp.AsBlobResourceContents(c.Resource); ok {
				parts = append(parts, fmt.Sprintf("[Binary resource %s, %s, %d bytes (base64) - not shown]", blob.URI, blob.MIMEType, len(blob.Blob)))
			} else {
				parts = append(parts, "[Unsupported embedded resource]")
			}
		case mcp.ResourceLink:
			parts = append(parts, fmt.Sprintf("[Resource link: %s - %s]", c.URI, c.Name))
		default:
			parts = append(parts, fmt.Sprintf("[Unsupported content type: %T]", content))
		}
	}

	text := strings.Join(parts, "\n\n")

	// Structured content is only used if there is no text, otherwise it is usually a duplicate
	if strings.TrimSpace(text) == "" && result.StructuredContent != nil {
		if data, err := json.Marshal(result.StructuredContent); err == nil {
			text = string(data)
		}
	}
	if strings.TrimSpace(text) == "" {
		text = "The tool was executed, but did not return any content."
	}

	text = truncateRunes(text, maxChars)

	if result.IsError {
		text = "Tool error: " + text
	}
	return text
}

// truncateRunes shortens text to maxChars characters and appends a hint
func truncateRunes(text string, maxChars int) string {
	runes := []rune(text)
	if len(runes) <= maxChars {
		return text
	}
	return string(runes[:maxChars]) + fmt.Sprintf("\n...[truncated %d chars]", len(runes)-maxChars)
}
