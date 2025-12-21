package codec

import (
	"encoding/json"
	"errors"
	"fmt"

	"go.jetify.com/ai/api"
)

// DecodeResponse converts a Claude Code CLI result event to an AI SDK Response.
func DecodeResponse(event *Event) (*api.Response, error) {
	if event == nil {
		return nil, errors.New("nil event provided")
	}

	if event.Type != EventTypeResult {
		return nil, fmt.Errorf("expected result event, got %s", event.Type)
	}

	// Check for errors
	if event.IsError || event.Subtype == "error" {
		return nil, fmt.Errorf("claude code error: %s", event.Result)
	}

	response := &api.Response{
		FinishReason:     decodeFinishReason(event),
		Usage:            decodeUsage(event),
		ResponseInfo:     decodeResponseInfo(event),
		ProviderMetadata: decodeProviderMetadata(event),
		Content:          decodeContent(event),
	}

	return response, nil
}

// decodeFinishReason extracts the finish reason from the event.
func decodeFinishReason(event *Event) api.FinishReason {
	// Check message stop reason first
	if event.Message != nil {
		switch event.Message.StopReason {
		case "end_turn", "stop_sequence":
			return api.FinishReasonStop
		case "tool_use":
			return api.FinishReasonToolCalls
		case "max_tokens":
			return api.FinishReasonLength
		default:
			// Message exists but stop reason is unknown/empty
			return api.FinishReasonUnknown
		}
	}

	// For successful results without a message, assume stop
	if event.Subtype == "success" {
		return api.FinishReasonStop
	}

	return api.FinishReasonUnknown
}

// decodeUsage extracts token usage information from the event.
func decodeUsage(event *Event) api.Usage {
	if event.Usage == nil {
		return api.Usage{}
	}

	return api.Usage{
		InputTokens:       event.Usage.InputTokens,
		OutputTokens:      event.Usage.OutputTokens,
		TotalTokens:       event.Usage.InputTokens + event.Usage.OutputTokens,
		CachedInputTokens: event.Usage.CacheReadInputTokens,
	}
}

// decodeResponseInfo extracts response metadata from the event.
func decodeResponseInfo(event *Event) *api.ResponseInfo {
	if event.Message == nil {
		return nil
	}

	return &api.ResponseInfo{
		ID:      event.Message.ID,
		ModelID: event.Message.Model,
	}
}

// decodeProviderMetadata extracts Claude Code-specific metadata.
func decodeProviderMetadata(event *Event) *api.ProviderMetadata {
	metadata := &Metadata{
		SessionID:        event.SessionID,
		StructuredOutput: event.StructuredOutput,
		CostUSD:          event.TotalCostUSD,
		DurationMS:       event.DurationMS,
		DurationAPIMS:    event.DurationAPIMS,
		NumTurns:         event.NumTurns,
	}

	if event.Usage != nil {
		metadata.Usage = Usage{
			InputTokens:              event.Usage.InputTokens,
			OutputTokens:             event.Usage.OutputTokens,
			CacheCreationInputTokens: event.Usage.CacheCreationInputTokens,
			CacheReadInputTokens:     event.Usage.CacheReadInputTokens,
		}
	}

	return api.NewProviderMetadata(map[string]any{
		ProviderName: metadata,
	})
}

// decodeContent processes the content blocks from the message.
func decodeContent(event *Event) []api.ContentBlock {
	content := make([]api.ContentBlock, 0)

	// If message has content blocks, use those
	if event.Message != nil && event.Message.Content != nil {
		for _, block := range event.Message.Content {
			switch block.Type {
			case "text":
				if block.Text != "" {
					content = append(content, &api.TextBlock{
						Text: block.Text,
					})
				}
			case "tool_use":
				content = append(content, decodeToolUse(block))
			}
		}
		return content
	}

	// Fall back to the result field for plain text responses
	if event.Result != "" {
		content = append(content, &api.TextBlock{
			Text: event.Result,
		})
	}

	return content
}

// decodeToolUse converts a CLI tool use block to an AI SDK ToolCallBlock.
func decodeToolUse(block ContentBlock) *api.ToolCallBlock {
	args := block.Input
	if args == nil {
		args = json.RawMessage("{}")
	}

	return &api.ToolCallBlock{
		ToolCallID: block.ID,
		ToolName:   block.Name,
		Args:       args,
	}
}
