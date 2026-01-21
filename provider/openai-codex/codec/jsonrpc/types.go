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
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  any `json:"params,omitempty"`
	ID      int64       `json:"id"`
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

// ThreadStartParams contains parameters for thread/start request.
type ThreadStartParams struct {
	// Currently empty, but may have options in the future.
}

// ThreadStartResult contains the result of thread/start.
type ThreadStartResult struct {
	Thread Thread `json:"thread"`
	Model  string `json:"model,omitempty"`
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
}

// Input represents an input message for a turn.
type Input struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// ApprovalPolicy controls how tool executions are approved.
type ApprovalPolicy string

const (
	// ApprovalNever means tools execute without approval.
	ApprovalNever ApprovalPolicy = "never"
	// ApprovalAlways means tools always require approval.
	ApprovalAlways ApprovalPolicy = "always"
	// ApprovalOnce means approval is requested once per tool type.
	ApprovalOnce ApprovalPolicy = "once"
)

// TurnStartResult contains the result of turn/start.
// The actual response comes via notifications.
type TurnStartResult struct {
	// Turn start acknowledgment - actual content comes via notifications
}
