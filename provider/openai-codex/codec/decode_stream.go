package codec

import (
	"errors"
	"fmt"

	"go.jetify.com/ai/api"
)

// DecodeStreamEvent converts a Codex CLI event to an AI SDK StreamEvent.
// Returns nil for events that don't map to SDK stream events.
func DecodeStreamEvent(event *Event) (api.StreamEvent, error) {
	if event == nil {
		return nil, errors.New("nil event provided")
	}

	switch event.Type {
	case EventTypeThreadStarted:
		return &api.ResponseMetadataEvent{
			ID: event.ThreadID,
		}, nil

	case EventTypeTurnStarted:
		// No SDK event for turn_started
		return nil, nil

	case EventTypeItemStarted, EventTypeItemUpdated:
		// For non-streaming, we only care about completed items
		// For streaming, item.updated could provide progressive text
		if event.Item != nil && event.Item.Type == ItemTypeAgentMessage && event.Item.Text != "" {
			return &api.TextDeltaEvent{
				TextDelta: event.Item.Text,
			}, nil
		}
		return nil, nil

	case EventTypeItemCompleted:
		return decodeItemCompleted(event.Item)

	case EventTypeTurnCompleted:
		return decodeTurnCompleted(event.Usage), nil

	case EventTypeTurnFailed:
		return decodeTurnFailed(event.Error), nil

	case EventTypeError:
		return decodeError(event), nil

	default:
		// Unknown event type, ignore
		return nil, nil
	}
}

// decodeItemCompleted converts an item.completed event to the appropriate StreamEvent.
func decodeItemCompleted(item *Item) (api.StreamEvent, error) {
	if item == nil {
		return nil, nil
	}

	switch item.Type {
	case ItemTypeAgentMessage:
		if item.Text == "" {
			return nil, nil
		}
		return &api.TextDeltaEvent{
			TextDelta: item.Text,
		}, nil

	case ItemTypeReasoning:
		if item.Text == "" {
			return nil, nil
		}
		return &api.ReasoningEvent{
			TextDelta: item.Text,
		}, nil

	case ItemTypeMCPToolCall:
		// MCP tool calls could map to ToolCallEvent
		if item.ToolName != "" {
			args := item.ToolInput
			if args == nil {
				args = []byte("{}")
			}
			return &api.ToolCallEvent{
				ToolCallID: item.ID,
				ToolName:   item.ToolName,
				Args:       args,
			}, nil
		}
		return nil, nil

	default:
		// Other item types (command_execution, file_change, etc.)
		// don't have direct SDK stream event mappings
		return nil, nil
	}
}

// decodeTurnCompleted converts a turn.completed event to a FinishEvent.
func decodeTurnCompleted(usage *Usage) api.StreamEvent {
	finishEvent := &api.FinishEvent{
		FinishReason: api.FinishReasonStop,
	}

	if usage != nil {
		finishEvent.Usage = api.Usage{
			InputTokens:       usage.InputTokens,
			OutputTokens:      usage.OutputTokens,
			TotalTokens:       usage.InputTokens + usage.OutputTokens,
			CachedInputTokens: usage.CachedInputTokens,
		}
	}

	return finishEvent
}

// decodeTurnFailed converts a turn.failed event to an ErrorEvent.
func decodeTurnFailed(err *Error) api.StreamEvent {
	if err == nil {
		return &api.ErrorEvent{
			Err: errors.New("turn failed"),
		}
	}

	return &api.ErrorEvent{
		Err: fmt.Errorf("turn failed: %s", err.Message),
	}
}

// decodeError converts an error event to an ErrorEvent.
func decodeError(event *Event) api.StreamEvent {
	msg := event.Message
	if msg == "" && event.Error != nil {
		msg = event.Error.Message
	}
	if msg == "" {
		msg = "unknown error"
	}

	return &api.ErrorEvent{
		Err: errors.New(msg),
	}
}
