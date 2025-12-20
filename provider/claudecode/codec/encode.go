package codec

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"go.jetify.com/ai/api"
)

// CLIMessage represents the message format expected by the Claude CLI.
type CLIMessage struct {
	Type    string         `json:"type"`
	Message CLIMessageBody `json:"message"`
}

// CLIMessageBody represents the inner message content.
type CLIMessageBody struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // Can be string or []CLIContentBlock
}

// CLIContentBlock represents a content block in CLI format.
type CLIContentBlock struct {
	Type string `json:"type"`

	// Text block fields
	Text string `json:"text,omitempty"`

	// Tool use fields
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	// Tool result fields
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
}

// EncodeMessage converts an SDK message to CLI input format.
// Returns the JSON bytes ready to be written to the CLI stdin.
func EncodeMessage(msg api.Message) ([]byte, error) {
	if msg == nil {
		return nil, errors.New("nil message provided")
	}

	switch m := msg.(type) {
	case *api.UserMessage:
		return encodeUserMessage(m)
	case *api.AssistantMessage:
		return encodeAssistantMessage(m)
	case *api.ToolMessage:
		return encodeToolMessage(m)
	case *api.SystemMessage:
		return nil, errors.New("system messages should be handled via --system-prompt flag, not encoded as CLI input")
	default:
		return nil, fmt.Errorf("unsupported message type: %T", msg)
	}
}

// encodeUserMessage converts a user message to CLI format.
func encodeUserMessage(msg *api.UserMessage) ([]byte, error) {
	// For simple text-only messages, use string content
	if isTextOnly(msg.Content) {
		text := extractText(msg.Content)
		cliMsg := CLIMessage{
			Type: "user",
			Message: CLIMessageBody{
				Role:    "user",
				Content: text,
			},
		}
		return json.Marshal(cliMsg)
	}

	// For complex content, use content blocks
	blocks, err := encodeContentBlocks(msg.Content)
	if err != nil {
		return nil, err
	}

	cliMsg := CLIMessage{
		Type: "user",
		Message: CLIMessageBody{
			Role:    "user",
			Content: blocks,
		},
	}
	return json.Marshal(cliMsg)
}

// encodeAssistantMessage converts an assistant message to CLI format.
func encodeAssistantMessage(msg *api.AssistantMessage) ([]byte, error) {
	// Check if message has tool calls
	hasToolCalls := false
	for _, block := range msg.Content {
		if _, ok := block.(*api.ToolCallBlock); ok {
			hasToolCalls = true
			break
		}
	}

	// For simple text-only messages, use string content
	if !hasToolCalls && isTextOnly(msg.Content) {
		text := extractText(msg.Content)
		cliMsg := CLIMessage{
			Type: "assistant",
			Message: CLIMessageBody{
				Role:    "assistant",
				Content: text,
			},
		}
		return json.Marshal(cliMsg)
	}

	// For messages with tool calls, use content blocks
	blocks, err := encodeContentBlocks(msg.Content)
	if err != nil {
		return nil, err
	}

	cliMsg := CLIMessage{
		Type: "assistant",
		Message: CLIMessageBody{
			Role:    "assistant",
			Content: blocks,
		},
	}
	return json.Marshal(cliMsg)
}

// encodeToolMessage converts a tool message to CLI format.
// Tool results are sent as user messages with tool_result content blocks.
func encodeToolMessage(msg *api.ToolMessage) ([]byte, error) {
	blocks := make([]CLIContentBlock, 0, len(msg.Content))

	for _, result := range msg.Content {
		content := extractToolResultContent(result.Content)
		block := CLIContentBlock{
			Type:      "tool_result",
			ToolUseID: result.ToolCallID,
			Content:   content,
			IsError:   result.IsError,
		}
		blocks = append(blocks, block)
	}

	cliMsg := CLIMessage{
		Type: "user",
		Message: CLIMessageBody{
			Role:    "user",
			Content: blocks,
		},
	}
	return json.Marshal(cliMsg)
}

// encodeContentBlocks converts SDK content blocks to CLI format.
func encodeContentBlocks(blocks []api.ContentBlock) ([]CLIContentBlock, error) {
	result := make([]CLIContentBlock, 0, len(blocks))

	for _, block := range blocks {
		switch b := block.(type) {
		case *api.TextBlock:
			result = append(result, CLIContentBlock{
				Type: "text",
				Text: b.Text,
			})
		case *api.ToolCallBlock:
			result = append(result, CLIContentBlock{
				Type:  "tool_use",
				ID:    b.ToolCallID,
				Name:  b.ToolName,
				Input: b.Args,
			})
		default:
			// Skip unsupported block types
			continue
		}
	}

	return result, nil
}

// isTextOnly checks if content contains only text blocks.
func isTextOnly(blocks []api.ContentBlock) bool {
	for _, block := range blocks {
		if _, ok := block.(*api.TextBlock); !ok {
			return false
		}
	}
	return true
}

// extractText concatenates all text blocks into a single string.
func extractText(blocks []api.ContentBlock) string {
	var sb strings.Builder
	for _, block := range blocks {
		if tb, ok := block.(*api.TextBlock); ok {
			sb.WriteString(tb.Text)
		}
	}
	return sb.String()
}

// extractToolResultContent extracts text content from tool result blocks.
func extractToolResultContent(blocks []api.ContentBlock) string {
	var sb strings.Builder
	for _, block := range blocks {
		if tb, ok := block.(*api.TextBlock); ok {
			sb.WriteString(tb.Text)
		}
	}
	return sb.String()
}

// ExtractSystemPrompt extracts system messages from the message slice
// and returns the concatenated system prompt and the remaining messages.
func ExtractSystemPrompt(messages []api.Message) (systemPrompt string, remaining []api.Message) {
	if messages == nil {
		return "", nil
	}

	var sb strings.Builder
	remaining = make([]api.Message, 0, len(messages))

	for _, msg := range messages {
		if sm, ok := msg.(*api.SystemMessage); ok {
			sb.WriteString(sm.Content)
		} else {
			remaining = append(remaining, msg)
		}
	}

	return sb.String(), remaining
}
