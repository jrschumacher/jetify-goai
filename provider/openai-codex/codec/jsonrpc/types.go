// Package jsonrpc provides JSON-RPC 2.0 types and client for Codex app-server communication.
package jsonrpc

import (
	"encoding/json"
	"fmt"
)

// Version is the JSON-RPC protocol version.
const Version = "2.0"

// Request represents a JSON-RPC 2.0 request.
type Request struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
	ID      int64  `json:"id"`
}

// NewRequest creates a new JSON-RPC request.
func NewRequest(id int64, method string, params any) *Request {
	return &Request{
		JSONRPC: Version,
		Method:  method,
		Params:  params,
		ID:      id,
	}
}

// Response represents a JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
	ID      *int64          `json:"id"`
}

// Error represents a JSON-RPC 2.0 error.
type Error struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Data != nil {
		return fmt.Sprintf("jsonrpc error %d: %s (data: %s)", e.Code, e.Message, string(e.Data))
	}
	return fmt.Sprintf("jsonrpc error %d: %s", e.Code, e.Message)
}

// Notification represents a JSON-RPC 2.0 notification (request without ID).
// These are server-to-client messages that don't expect a response.
type Notification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Message is a union type that can be either a Response or Notification.
// Used for parsing incoming messages from the server.
type Message struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
	ID      *int64          `json:"id,omitempty"`
}

// IsResponse returns true if this message is a response (has ID).
func (m *Message) IsResponse() bool {
	return m.ID != nil
}

// IsNotification returns true if this message is a notification (has method, no ID).
func (m *Message) IsNotification() bool {
	return m.Method != "" && m.ID == nil
}

// AsResponse converts the message to a Response if it is one.
func (m *Message) AsResponse() *Response {
	if !m.IsResponse() {
		return nil
	}
	return &Response{
		JSONRPC: m.JSONRPC,
		Result:  m.Result,
		Error:   m.Error,
		ID:      m.ID,
	}
}

// AsNotification converts the message to a Notification if it is one.
func (m *Message) AsNotification() *Notification {
	if !m.IsNotification() {
		return nil
	}
	return &Notification{
		JSONRPC: m.JSONRPC,
		Method:  m.Method,
		Params:  m.Params,
	}
}

// Standard JSON-RPC error codes.
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
)

// InitializeParams contains parameters for the initialize request.
type InitializeParams struct {
	ClientInfo ClientInfo `json:"clientInfo"`
}

// ClientInfo identifies the client to the server.
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// InitializeResult contains the server's response to initialize.
type InitializeResult struct {
	UserAgent string `json:"userAgent,omitempty"`
}

// SandboxMode controls which sandbox mode to use when executing model-generated shell commands.
type SandboxMode string

const (
	SandboxReadOnly         SandboxMode = "readOnly"
	SandboxWorkspaceWrite   SandboxMode = "workspaceWrite"
	SandboxDangerFullAccess SandboxMode = "dangerFullAccess"
)

// SandboxPolicy controls sandbox policy details (e.g. network access, writable roots).
//
// The exact fields that apply depend on the Type.
type SandboxPolicy struct {
	Type string `json:"type"` // "readOnly" | "workspaceWrite" | "dangerFullAccess"

	// WorkspaceWriteSandboxPolicy fields
	WritableRoots       []string `json:"writableRoots,omitempty"`
	NetworkAccess       bool     `json:"networkAccess,omitempty"`
	ExcludeSlashTmp     bool     `json:"excludeSlashTmp,omitempty"`
	ExcludeTmpdirEnvVar bool     `json:"excludeTmpdirEnvVar,omitempty"`
}

// ThreadStartParams contains parameters for thread/start request.
type ThreadStartParams struct {
	// Model is the model identifier to use for this thread.
	Model string `json:"model,omitempty"`

	// BaseInstructions sets the base instructions for the thread (similar to a system prompt).
	BaseInstructions string `json:"baseInstructions,omitempty"`

	// DeveloperInstructions sets developer instructions for the thread (optional).
	DeveloperInstructions string `json:"developerInstructions,omitempty"`

	// Cwd sets the working directory for the thread.
	Cwd string `json:"cwd,omitempty"`

	// Sandbox sets the sandbox mode for the thread.
	Sandbox SandboxMode `json:"sandbox,omitempty"`

	// ApprovalPolicy sets the approval policy for the thread.
	ApprovalPolicy ApprovalPolicy `json:"approvalPolicy,omitempty"`

	// Config allows passing arbitrary Codex configuration overrides.
	// This maps to Codex CLI config keys (e.g. config.toml) and is intentionally untyped.
	Config map[string]any `json:"config,omitempty"`
}

// ThreadStartResult contains the result of thread/start.
type ThreadStartResult struct {
	Thread Thread `json:"thread"`

	// Thread configuration as resolved by the server.
	Model          string         `json:"model,omitempty"`
	ModelProvider  string         `json:"modelProvider,omitempty"`
	Cwd            string         `json:"cwd,omitempty"`
	Sandbox        *SandboxPolicy `json:"sandbox,omitempty"`
	ApprovalPolicy ApprovalPolicy `json:"approvalPolicy,omitempty"`
}

// Thread represents a conversation thread.
type Thread struct {
	ID string `json:"id"`
}

// TurnStartParams contains parameters for turn/start request.
type TurnStartParams struct {
	ThreadID       string         `json:"threadId"`
	Input          []Input        `json:"input"`
	ApprovalPolicy ApprovalPolicy `json:"approvalPolicy,omitempty"`

	// Optional per-turn overrides.
	Model         string         `json:"model,omitempty"`
	Cwd           string         `json:"cwd,omitempty"`
	SandboxPolicy *SandboxPolicy `json:"sandboxPolicy,omitempty"`
}

// Input represents a user input item for a turn.
//
// Codex app-server supports multiple input types (text, image url, local image).
type Input struct {
	Type string `json:"type"`

	// Text input
	Text string `json:"text,omitempty"`

	// Remote image input
	URL string `json:"url,omitempty"`

	// Local image input
	Path string `json:"path,omitempty"`
}

// ApprovalPolicy controls how tool executions are approved.
type ApprovalPolicy string

const (
	// ApprovalNever means tools execute without approval.
	ApprovalNever ApprovalPolicy = "never"

	// ApprovalUnlessTrusted means approvals are required unless the current directory is trusted.
	ApprovalUnlessTrusted ApprovalPolicy = "unlessTrusted"
	// ApprovalOnFailure means approvals are requested when something fails.
	ApprovalOnFailure ApprovalPolicy = "onFailure"
	// ApprovalOnRequest means approvals are requested when a tool asks for it.
	ApprovalOnRequest ApprovalPolicy = "onRequest"

	// Deprecated: legacy constants from older protocol versions. Prefer ApprovalOnRequest.
	ApprovalAlways ApprovalPolicy = ApprovalOnRequest
	// Deprecated: legacy constants from older protocol versions. Prefer ApprovalOnRequest.
	ApprovalOnce ApprovalPolicy = ApprovalOnRequest
)

// TurnStartResult contains the result of turn/start.
// The actual response comes via notifications.
type TurnStartResult struct {
	// Turn start acknowledgment - actual content comes via notifications
}
