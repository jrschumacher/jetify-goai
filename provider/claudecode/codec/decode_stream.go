package codec

import (
	"encoding/json"
	"errors"
	"fmt"

	"go.jetify.com/ai/api"
)

// DecodeStreamEvent converts a Claude Code CLI stream event to an AI SDK StreamEvent.
// Returns nil for events that don't map to SDK stream events (e.g., content_block_stop).
func DecodeStreamEvent(event *Event) (api.StreamEvent, error) {
	if event == nil {
		return nil, errors.New("nil event provided")
	}

	if event.Type != EventTypeStreamEvent {
		return nil, fmt.Errorf("expected stream_event, got %s", event.Type)
	}

	if event.StreamEvent == nil {
		return nil, errors.New("nil stream event")
	}

	switch event.StreamEvent.Type {
	case StreamEventTypeMessageStart:
		return decodeMessageStart(event.StreamEvent), nil

	case StreamEventTypeMessageDelta:
		return decodeMessageDelta(event.StreamEvent), nil

	case StreamEventTypeMessageStop:
		// No SDK event for message_stop
		return nil, nil

	case StreamEventTypeContentBlockStart:
		return decodeContentBlockStart(event.StreamEvent), nil

	case StreamEventTypeContentBlockDelta:
		return decodeContentBlockDelta(event.StreamEvent), nil

	case StreamEventTypeContentBlockStop:
		// No SDK event for content_block_stop
		return nil, nil

	default:
		return nil, nil
	}
}

// decodeMessageStart converts a message_start event to ResponseMetadataEvent.
func decodeMessageStart(se *StreamEvent) api.StreamEvent {
	if se.Message == nil {
		return nil
	}

	return &api.ResponseMetadataEvent{
		ID:      se.Message.ID,
		ModelID: se.Message.Model,
	}
}

// decodeMessageDelta converts a message_delta event to FinishEvent.
func decodeMessageDelta(se *StreamEvent) api.StreamEvent {
	finishEvent := &api.FinishEvent{
		FinishReason: decodeStreamFinishReason(se.Delta),
	}

	if se.Usage != nil {
		finishEvent.Usage = api.Usage{
			InputTokens:       se.Usage.InputTokens,
			OutputTokens:      se.Usage.OutputTokens,
			TotalTokens:       se.Usage.InputTokens + se.Usage.OutputTokens,
			CachedInputTokens: se.Usage.CacheReadInputTokens,
		}
	}

	return finishEvent
}

// decodeStreamFinishReason extracts finish reason from a stream delta.
func decodeStreamFinishReason(delta *Delta) api.FinishReason {
	if delta == nil {
		return api.FinishReasonUnknown
	}

	switch delta.StopReason {
	case "end_turn", "stop_sequence":
		return api.FinishReasonStop
	case "tool_use":
		return api.FinishReasonToolCalls
	case "max_tokens":
		return api.FinishReasonLength
	default:
		return api.FinishReasonUnknown
	}
}

// decodeContentBlockStart handles content_block_start events.
// Returns ToolCallEvent for tool_use blocks, nil for text blocks.
func decodeContentBlockStart(se *StreamEvent) api.StreamEvent {
	if se.ContentBlock == nil {
		return nil
	}

	switch se.ContentBlock.Type {
	case "tool_use":
		args := se.ContentBlock.Input
		if args == nil {
			args = json.RawMessage("{}")
		}
		return &api.ToolCallEvent{
			ToolCallID: se.ContentBlock.ID,
			ToolName:   se.ContentBlock.Name,
			Args:       args,
		}
	case "text":
		// Text blocks are streamed via deltas, no event needed at start
		return nil
	default:
		return nil
	}
}

// decodeContentBlockDelta handles content_block_delta events.
// Returns TextDeltaEvent for text deltas, ToolCallDeltaEvent for input_json_delta.
func decodeContentBlockDelta(se *StreamEvent) api.StreamEvent {
	if se.Delta == nil {
		return nil
	}

	switch se.Delta.Type {
	case "text_delta":
		return &api.TextDeltaEvent{
			TextDelta: se.Delta.Text,
		}
	case "input_json_delta":
		return &api.ToolCallDeltaEvent{
			ArgsDelta: []byte(se.Delta.Text),
		}
	default:
		return nil
	}
}
