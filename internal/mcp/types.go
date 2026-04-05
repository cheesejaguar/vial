// Package mcp implements a Model Context Protocol (MCP) server that exposes
// Vial's vault over JSON-RPC 2.0 on stdin/stdout. AI assistants and editors
// that speak MCP can use this server to look up, search, and (optionally)
// mutate secrets without any additional network infrastructure.
//
// # Transport
//
// The server reads newline-delimited JSON-RPC 2.0 messages from stdin and
// writes responses to stdout. Each request and response is a single line.
// This is the standard MCP stdio transport.
//
// # Security model
//
// The server is read-only by default. Mutation tools (vault_set,
// vault_remove) are only registered when the --allow-writes flag is passed
// to the `vial serve` command. Every read of a secret value is logged to the
// audit log with the "via MCP" tag so that access is traceable.
//
// The vault must already be unlocked before the server handles any
// vault_get or vault_list requests. Clients receive a clear error message
// if they attempt to access a locked vault.
//
// # Protocol version
//
// This implementation targets MCP protocol version 2024-11-05.
package mcp

// JSONRPCRequest is a JSON-RPC 2.0 request or notification. When ID is nil
// or absent the message is a notification and no response should be sent.
type JSONRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`          // must be "2.0"
	ID      interface{} `json:"id,omitempty"`     // string, number, or null; absent for notifications
	Method  string      `json:"method"`           // e.g. "initialize", "tools/call"
	Params  interface{} `json:"params,omitempty"` // method-specific parameters; may be absent
}

// JSONRPCResponse is a JSON-RPC 2.0 response. Exactly one of Result or Error
// is non-nil; both being nil or both being non-nil is invalid per the spec.
type JSONRPCResponse struct {
	JSONRPC string        `json:"jsonrpc"`        // always "2.0"
	ID      interface{}   `json:"id,omitempty"`   // echoes the request ID; null for notifications
	Result  interface{}   `json:"result,omitempty"` // success payload
	Error   *JSONRPCError `json:"error,omitempty"`  // failure payload
}

// JSONRPCError carries a structured error in a JSON-RPC 2.0 response.
// Standard error codes defined by the spec:
//   - -32700: Parse error (invalid JSON)
//   - -32600: Invalid Request
//   - -32601: Method not found
//   - -32602: Invalid params
//   - -32603: Internal error
type JSONRPCError struct {
	Code    int         `json:"code"`            // numeric error code
	Message string      `json:"message"`         // short human-readable description
	Data    interface{} `json:"data,omitempty"`  // optional additional detail (e.g. error string)
}

// InitializeParams carries the client's capabilities sent in the "initialize"
// request. The server uses ProtocolVersion to verify compatibility, but
// currently accepts any version.
type InitializeParams struct {
	ProtocolVersion string     `json:"protocolVersion"` // MCP protocol version requested by client
	ClientInfo      ClientInfo `json:"clientInfo"`
	Capabilities    struct{}   `json:"capabilities"` // client capabilities (currently unused)
}

// ClientInfo identifies the MCP client (e.g. an editor extension or AI agent).
// Used for logging and diagnostics only; the server does not gate access on
// client identity.
type ClientInfo struct {
	Name    string `json:"name"`    // human-readable client name
	Version string `json:"version"` // client version string
}

// InitializeResult is the server's response to the "initialize" request. It
// advertises the server's protocol version and supported capabilities so the
// client knows which methods are available.
type InitializeResult struct {
	ProtocolVersion string       `json:"protocolVersion"` // MCP version supported by this server
	ServerInfo      ServerInfo   `json:"serverInfo"`
	Capabilities    Capabilities `json:"capabilities"`
}

// ServerInfo identifies this MCP server to connecting clients.
type ServerInfo struct {
	Name    string `json:"name"`    // "vial"
	Version string `json:"version"` // semantic version of the vial binary
}

// Capabilities describes the MCP feature set supported by this server.
// Tools is non-nil when the server exposes callable tools (always the case
// for Vial).
type Capabilities struct {
	Tools *ToolsCapability `json:"tools,omitempty"` // non-nil when tools are available
}

// ToolsCapability signals that the server supports the tools/* method family.
// ListChanged would be set to true if the server could push tool-list change
// notifications; Vial's tool list is static so it remains false.
type ToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"` // true if tool list can change at runtime
}

// ToolDefinition describes a single callable tool exposed by the server.
// InputSchema is a JSON Schema object (as a generic map) that MCP clients use
// to validate arguments and render input forms.
type ToolDefinition struct {
	Name        string                 `json:"name"`        // unique tool identifier, e.g. "vault_get"
	Description string                 `json:"description"` // shown to the AI/user to explain what the tool does
	InputSchema map[string]interface{} `json:"inputSchema"` // JSON Schema describing accepted arguments
}

// ToolsListResult is the response payload for a "tools/list" request.
type ToolsListResult struct {
	Tools []ToolDefinition `json:"tools"`
}

// CallToolParams carries the tool name and arguments for a "tools/call"
// request. Arguments is a free-form map whose shape is defined by the tool's
// InputSchema.
type CallToolParams struct {
	Name      string                 `json:"name"`                // tool identifier
	Arguments map[string]interface{} `json:"arguments,omitempty"` // tool-specific input; may be absent for no-arg tools
}

// CallToolResult is the response payload for a "tools/call" request. Content
// is a slice of [ContentBlock] values; Vial always returns exactly one text
// block. IsError distinguishes a tool-level failure (IsError == true, Content
// carries the error message) from a protocol-level error (JSONRPCError).
type CallToolResult struct {
	Content []ContentBlock `json:"content"`         // one or more response blocks
	IsError bool           `json:"isError,omitempty"` // true when the tool itself failed
}

// ContentBlock is a typed content item inside a [CallToolResult]. Vial only
// produces "text" type blocks; other types (e.g. "image") are reserved for
// future use.
type ContentBlock struct {
	Type string `json:"type"` // content type, e.g. "text"
	Text string `json:"text"` // the content string; present when Type == "text"
}
