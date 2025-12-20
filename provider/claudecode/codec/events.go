// Package codec provides encoding and decoding for Claude CLI communication.
package codec

import (
	"encoding/json"
	"errors"
)

// EventType represents the type of event from the Claude CLI.
type EventType string

const (
	// EventTypeSystem is emitted at session initialization.
	EventTypeSystem EventType = "system"
	// EventTypeAssistant contains a complete assistant message.
	EventTypeAssistant EventType = "assistant"
	// EventTypeStreamEvent contains streaming events (deltas, start/stop).
	EventTypeStreamEvent EventType = "stream_event"
	// EventTypeResult is the final result of a request.
	EventTypeResult EventType = "result"
)

// StreamEventType represents the type of streaming event.
type StreamEventType string

const (
	StreamEventTypeMessageStart      StreamEventType = "message_start"
	StreamEventTypeMessageDelta      StreamEventType = "message_delta"
	StreamEventTypeMessageStop       StreamEventType = "message_stop"
	StreamEventTypeContentBlockStart StreamEventType = "content_block_start"
	StreamEventTypeContentBlockDelta StreamEventType = "content_block_delta"
	StreamEventTypeContentBlockStop  StreamEventType = "content_block_stop"
)

// Event represents a parsed event from the Claude CLI NDJSON output.
type Event struct {
	// Common fields
	Type      EventType `json:"type"`
	Subtype   string    `json:"subtype,omitempty"`
	SessionID string    `json:"session_id,omitempty"`
	UUID      string    `json:"uuid,omitempty"`

	// System init fields
	Cwd              string `json:"cwd,omitempty"`
	Model            string `json:"model,omitempty"`
	PermissionMode   string `json:"permissionMode,omitempty"`
	ApiKeySource     string `json:"apiKeySource,omitempty"`
	ClaudeCodeVersion string `json:"claude_code_version,omitempty"`

	// Assistant message
	Message *AssistantMessage `json:"message,omitempty"`

	// Stream event (when Type == EventTypeStreamEvent)
	StreamEvent *StreamEvent `json:"event,omitempty"`

	// Result fields (when Type == EventTypeResult)
	IsError          bool            `json:"is_error,omitempty"`
	Result           string          `json:"result,omitempty"`
	StructuredOutput json.RawMessage `json:"structured_output,omitempty"`
	TotalCostUSD     float64         `json:"total_cost_usd,omitempty"`
	DurationMS       int             `json:"duration_ms,omitempty"`
	DurationAPIMS    int             `json:"duration_api_ms,omitempty"`
	NumTurns         int             `json:"num_turns,omitempty"`
	Usage            *Usage          `json:"usage,omitempty"`
}

// StreamEvent represents a streaming event within an Event.
type StreamEvent struct {
	Type         StreamEventType   `json:"type"`
	Index        int               `json:"index,omitempty"`
	Delta        *Delta            `json:"delta,omitempty"`
	Message      *AssistantMessage `json:"message,omitempty"`
	ContentBlock *ContentBlock     `json:"content_block,omitempty"`
	Usage        *Usage            `json:"usage,omitempty"`
}

// Delta represents a delta update in a streaming event.
type Delta struct {
	Type       string `json:"type,omitempty"`
	Text       string `json:"text,omitempty"`
	StopReason string `json:"stop_reason,omitempty"`
}

// AssistantMessage represents a message from the assistant.
type AssistantMessage struct {
	ID           string          `json:"id,omitempty"`
	Type         string          `json:"type,omitempty"`
	Model        string          `json:"model,omitempty"`
	Role         string          `json:"role,omitempty"`
	Content      []ContentBlock  `json:"content,omitempty"`
	StopReason   string          `json:"stop_reason,omitempty"`
	StopSequence *string         `json:"stop_sequence,omitempty"`
	Usage        *Usage          `json:"usage,omitempty"`
}

// ContentBlock represents a content block in a message.
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	// Tool use fields
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

// Usage represents token usage information.
type Usage struct {
	InputTokens              int `json:"input_tokens,omitempty"`
	OutputTokens             int `json:"output_tokens,omitempty"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}

// ParseEvent parses a single NDJSON line from the Claude CLI.
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
