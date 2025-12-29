// Package codec provides encoding and decoding for Codex CLI communication.
package codec

import (
	"encoding/json"
	"errors"
)

// EventType represents the type of event from the Codex CLI.
type EventType string

const (
	// EventTypeThreadStarted is emitted at session initialization.
	EventTypeThreadStarted EventType = "thread.started"
	// EventTypeTurnStarted is emitted at the beginning of a request/response turn.
	EventTypeTurnStarted EventType = "turn.started"
	// EventTypeTurnCompleted is emitted at the end of a turn with usage metrics.
	EventTypeTurnCompleted EventType = "turn.completed"
	// EventTypeTurnFailed is emitted when a turn fails.
	EventTypeTurnFailed EventType = "turn.failed"
	// EventTypeItemStarted is emitted when an item starts.
	EventTypeItemStarted EventType = "item.started"
	// EventTypeItemUpdated is emitted when an item is updated (streaming).
	EventTypeItemUpdated EventType = "item.updated"
	// EventTypeItemCompleted is emitted when an item is finalized.
	EventTypeItemCompleted EventType = "item.completed"
	// EventTypeError is emitted for unrecoverable errors.
	EventTypeError EventType = "error"
)

// ItemType represents the type of item in an item event.
type ItemType string

const (
	// ItemTypeAgentMessage is a natural language response from the assistant.
	ItemTypeAgentMessage ItemType = "agent_message"
	// ItemTypeReasoning is a summary of the assistant's thinking process.
	ItemTypeReasoning ItemType = "reasoning"
	// ItemTypeCommandExecution is a shell command executed by the assistant.
	ItemTypeCommandExecution ItemType = "command_execution"
	// ItemTypeFileChange is a file modification made by the assistant.
	ItemTypeFileChange ItemType = "file_change"
	// ItemTypeMCPToolCall is a Model Context Protocol tool invocation.
	ItemTypeMCPToolCall ItemType = "mcp_tool_call"
	// ItemTypeWebSearch is a web search operation.
	ItemTypeWebSearch ItemType = "web_search"
	// ItemTypeTodoList is the agent's running plan.
	ItemTypeTodoList ItemType = "todo_list"
)

// Event represents a parsed event from the Codex CLI JSONL output.
type Event struct {
	// Type is the event type (thread.started, item.completed, etc.)
	Type EventType `json:"type"`

	// ThreadID is set for thread.started events.
	ThreadID string `json:"thread_id,omitempty"`

	// Item is set for item.* events.
	Item *Item `json:"item,omitempty"`

	// Usage is set for turn.completed events.
	Usage *Usage `json:"usage,omitempty"`

	// Error is set for turn.failed and error events.
	Error *Error `json:"error,omitempty"`

	// Message is set for error events (alternative to Error struct).
	Message string `json:"message,omitempty"`
}

// Item represents an item in item.* events.
type Item struct {
	// ID is the unique identifier for the item.
	ID string `json:"id"`

	// Type is the item type (agent_message, reasoning, etc.)
	Type ItemType `json:"type"`

	// Text is the content for text-based items (agent_message, reasoning).
	Text string `json:"text,omitempty"`

	// Command is set for command_execution items.
	Command string `json:"command,omitempty"`

	// ExitCode is set for command_execution items.
	ExitCode *int `json:"exit_code,omitempty"`

	// Output is set for command_execution items.
	Output string `json:"output,omitempty"`

	// FilePath is set for file_change items.
	FilePath string `json:"file_path,omitempty"`

	// Diff is set for file_change items.
	Diff string `json:"diff,omitempty"`

	// ToolName is set for mcp_tool_call items.
	ToolName string `json:"tool_name,omitempty"`

	// ToolInput is set for mcp_tool_call items.
	ToolInput json.RawMessage `json:"tool_input,omitempty"`

	// ToolOutput is set for mcp_tool_call items.
	ToolOutput json.RawMessage `json:"tool_output,omitempty"`
}

// Usage represents token usage information.
type Usage struct {
	InputTokens       int `json:"input_tokens,omitempty"`
	CachedInputTokens int `json:"cached_input_tokens,omitempty"`
	OutputTokens      int `json:"output_tokens,omitempty"`
}

// Error represents an error in turn.failed or error events.
type Error struct {
	Message string `json:"message,omitempty"`
	Code    string `json:"code,omitempty"`
}

// ParseEvent parses a single JSONL line from the Codex CLI.
func ParseEvent(data []byte) (*Event, error) {
	if len(data) == 0 {
		return nil, errors.New("empty input")
	}

	var event Event
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, err
	}

	return &event, nil
}
