package tools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
)

// ToolDef represents a tool definition with its corresponding handler function
type ToolDef struct {
	Tool    *mcp.Tool
	Handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)
}

// toolRegistry saves all available tools
var Registry = make(map[string]ToolDef)

// registerTool is called by individual tool files in their init() function
func Register(name string, tool *mcp.Tool, handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)) {
	Registry[name] = ToolDef{
		Tool:    tool,
		Handler: handler,
	}
}
