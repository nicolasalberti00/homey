package server

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nicolasalberti00/homey/internal/tools"
)

// registerTools exposes the registry on the server. The schemas, the validation
// and the meaning of a tool stay in the registry: what happens here is only the
// protocol's shape around them, so the same tool can be served to another
// adapter without being rewritten.
func registerTools(server *mcp.Server, registry *tools.Registry) {
	for _, tool := range registry.List() {
		server.AddTool(describe(tool), handle(tool))
	}
}

// describe tells a client what the tool does, what it takes and returns, and
// whether a host should present it as safe.
func describe(tool tools.Tool) *mcp.Tool {
	destructive := tool.Destructive()
	return &mcp.Tool{
		Name:         tool.Name(),
		Description:  tool.Description(),
		InputSchema:  tool.InputSchema(),
		OutputSchema: tool.OutputSchema(),
		Annotations: &mcp.ToolAnnotations{
			Title:           tool.Name(),
			ReadOnlyHint:    tool.Permission() == tools.PermissionRead,
			DestructiveHint: &destructive,
		},
	}
}

// handle moves JSON between the transport and the tool. A failure the caller
// can act on — arguments that do not fit the schema, an entity that does not
// exist — comes back as a tool error, which is what the protocol asks a server
// to do: the model reads it and tries again.
func handle(tool tools.Tool) mcp.ToolHandler {
	return func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		arguments, err := json.Marshal(request.Params.Arguments)
		if err != nil {
			return nil, fmt.Errorf("encoding the arguments of %s: %w", tool.Name(), err)
		}
		output, err := tool.Call(ctx, arguments)
		if err != nil {
			if tools.CallerError(err) {
				return errorResult(err), nil
			}
			return nil, err
		}
		var structured any
		if err := json.Unmarshal(output, &structured); err != nil {
			return nil, fmt.Errorf("decoding the result of %s: %w", tool.Name(), err)
		}
		return &mcp.CallToolResult{
			Content:           []mcp.Content{&mcp.TextContent{Text: string(output)}},
			StructuredContent: structured,
		}, nil
	}
}

// errorResult reports a failure as the result of the call rather than as a
// protocol error, so the message reaches the model that made the call.
func errorResult(err error) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
	}
}
