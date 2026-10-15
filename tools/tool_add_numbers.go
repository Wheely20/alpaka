package tools

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

func init() {
	// 1. Definition
	tool := mcp.NewTool("add_numbers",
		mcp.WithDescription("Adds two numbers. Use this tool when you want to calculate math problems."),
		mcp.WithNumber("a", mcp.Required(), mcp.Description("The first number")),
		mcp.WithNumber("b", mcp.Required(), mcp.Description("The second number")),
	)

	// 2. Registration in the global map
	Register("add_numbers", &tool, addNumbersHandler)
}

// 3. Execution (Handler)
func addNumbersHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, ok := request.Params.Arguments.(map[string]interface{})
	if !ok {
		return mcp.NewToolResultError("Error: Arguments could not be parsed."), nil
	}

	a, okA := args["a"].(float64)
	b, okB := args["b"].(float64)

	if !okA || !okB {
		return mcp.NewToolResultError("Error: Arguments 'a' and 'b' must be valid numbers."), nil
	}

	result := a + b
	resultStr := fmt.Sprintf("The result of %v + %v is %v", a, b, result)
	return mcp.NewToolResultText(resultStr), nil
}
