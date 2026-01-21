package codec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.jetify.com/ai/api"
)

func TestDecodeStreamEvent_TextDelta(t *testing.T) {
	event := &Event{
		Type:      EventTypeStreamEvent,
		SessionID: "abc-123",
		StreamEvent: &StreamEvent{
			Type:  StreamEventTypeContentBlockDelta,
			Index: 0,
			Delta: &Delta{
				Type: "text_delta",
				Text: "Hello",
			},
		},
	}

	streamEvent, err := DecodeStreamEvent(event)
	require.NoError(t, err)
	require.NotNil(t, streamEvent)

	textDelta, ok := streamEvent.(*api.TextDeltaEvent)
	require.True(t, ok, "expected TextDeltaEvent")
	assert.Equal(t, "Hello", textDelta.TextDelta)
}

func TestDecodeStreamEvent_MessageStart(t *testing.T) {
	event := &Event{
		Type:      EventTypeStreamEvent,
		SessionID: "abc-123",
		StreamEvent: &StreamEvent{
			Type: StreamEventTypeMessageStart,
			Message: &AssistantMessage{
				ID:    "msg_123",
				Model: "claude-sonnet-4-5-20250929",
				Role:  "assistant",
			},
		},
	}

	streamEvent, err := DecodeStreamEvent(event)
	require.NoError(t, err)
	require.NotNil(t, streamEvent)

	metadata, ok := streamEvent.(*api.ResponseMetadataEvent)
	require.True(t, ok, "expected ResponseMetadataEvent")
	assert.Equal(t, "msg_123", metadata.ID)
	assert.Equal(t, "claude-sonnet-4-5-20250929", metadata.ModelID)
}

func TestDecodeStreamEvent_MessageDelta(t *testing.T) {
	event := &Event{
		Type:      EventTypeStreamEvent,
		SessionID: "abc-123",
		StreamEvent: &StreamEvent{
			Type: StreamEventTypeMessageDelta,
			Delta: &Delta{
				StopReason: "end_turn",
			},
			Usage: &Usage{
				InputTokens:  10,
				OutputTokens: 5,
			},
		},
	}

	streamEvent, err := DecodeStreamEvent(event)
	require.NoError(t, err)
	require.NotNil(t, streamEvent)

	finishEvent, ok := streamEvent.(*api.FinishEvent)
	require.True(t, ok, "expected FinishEvent")
	assert.Equal(t, api.FinishReasonStop, finishEvent.FinishReason)
	assert.Equal(t, 10, finishEvent.Usage.InputTokens)
	assert.Equal(t, 5, finishEvent.Usage.OutputTokens)
}

func TestDecodeStreamEvent_ToolCall(t *testing.T) {
	event := &Event{
		Type:      EventTypeStreamEvent,
		SessionID: "abc-123",
		StreamEvent: &StreamEvent{
			Type:  StreamEventTypeContentBlockStart,
			Index: 0,
			ContentBlock: &ContentBlock{
				Type:  "tool_use",
				ID:    "tool_123",
				Name:  "get_weather",
				Input: []byte(`{"location":"NYC"}`),
			},
		},
	}

	streamEvent, err := DecodeStreamEvent(event)
	require.NoError(t, err)
	require.NotNil(t, streamEvent)

	toolCall, ok := streamEvent.(*api.ToolCallEvent)
	require.True(t, ok, "expected ToolCallEvent")
	assert.Equal(t, "tool_123", toolCall.ToolCallID)
	assert.Equal(t, "get_weather", toolCall.ToolName)
	assert.JSONEq(t, `{"location":"NYC"}`, string(toolCall.Args))
}

func TestDecodeStreamEvent_ToolCallDelta(t *testing.T) {
	event := &Event{
		Type:      EventTypeStreamEvent,
		SessionID: "abc-123",
		StreamEvent: &StreamEvent{
			Type:  StreamEventTypeContentBlockDelta,
			Index: 0,
			Delta: &Delta{
				Type: "input_json_delta",
				Text: `"location":`,
			},
		},
	}

	streamEvent, err := DecodeStreamEvent(event)
	require.NoError(t, err)
	require.NotNil(t, streamEvent)

	toolDelta, ok := streamEvent.(*api.ToolCallDeltaEvent)
	require.True(t, ok, "expected ToolCallDeltaEvent")
	assert.Equal(t, `"location":`, string(toolDelta.ArgsDelta))
}

func TestDecodeStreamEvent_MessageStop(t *testing.T) {
	event := &Event{
		Type:      EventTypeStreamEvent,
		SessionID: "abc-123",
		StreamEvent: &StreamEvent{
			Type: StreamEventTypeMessageStop,
		},
	}

	// message_stop should return nil (no event to emit)
	streamEvent, err := DecodeStreamEvent(event)
	require.NoError(t, err)
	assert.Nil(t, streamEvent)
}

func TestDecodeStreamEvent_ContentBlockStart_Text(t *testing.T) {
	event := &Event{
		Type:      EventTypeStreamEvent,
		SessionID: "abc-123",
		StreamEvent: &StreamEvent{
			Type:  StreamEventTypeContentBlockStart,
			Index: 0,
			ContentBlock: &ContentBlock{
				Type: "text",
				Text: "",
			},
		},
	}

	// text content_block_start should return nil (wait for deltas)
	streamEvent, err := DecodeStreamEvent(event)
	require.NoError(t, err)
	assert.Nil(t, streamEvent)
}

func TestDecodeStreamEvent_ContentBlockStop(t *testing.T) {
	event := &Event{
		Type:      EventTypeStreamEvent,
		SessionID: "abc-123",
		StreamEvent: &StreamEvent{
			Type:  StreamEventTypeContentBlockStop,
			Index: 0,
		},
	}

	// content_block_stop should return nil (no event to emit)
	streamEvent, err := DecodeStreamEvent(event)
	require.NoError(t, err)
	assert.Nil(t, streamEvent)
}

func TestDecodeStreamEvent_NilEvent(t *testing.T) {
	_, err := DecodeStreamEvent(nil)
	require.Error(t, err)
}

func TestDecodeStreamEvent_WrongEventType(t *testing.T) {
	event := &Event{
		Type: EventTypeResult,
	}

	_, err := DecodeStreamEvent(event)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected stream_event")
}

func TestDecodeStreamEvent_NilStreamEvent(t *testing.T) {
	event := &Event{
		Type:        EventTypeStreamEvent,
		StreamEvent: nil,
	}

	_, err := DecodeStreamEvent(event)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil stream event")
}

func TestDecodeStreamEvent_FinishReasons(t *testing.T) {
	tests := []struct {
		name       string
		stopReason string
		expected   api.FinishReason
	}{
		{"end_turn", "end_turn", api.FinishReasonStop},
		{"stop_sequence", "stop_sequence", api.FinishReasonStop},
		{"tool_use", "tool_use", api.FinishReasonToolCalls},
		{"max_tokens", "max_tokens", api.FinishReasonLength},
		{"unknown", "something_else", api.FinishReasonUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &Event{
				Type: EventTypeStreamEvent,
				StreamEvent: &StreamEvent{
					Type: StreamEventTypeMessageDelta,
					Delta: &Delta{
						StopReason: tt.stopReason,
					},
				},
			}

			streamEvent, err := DecodeStreamEvent(event)
			require.NoError(t, err)
			finishEvent, ok := streamEvent.(*api.FinishEvent)
			require.True(t, ok)
			assert.Equal(t, tt.expected, finishEvent.FinishReason)
		})
	}
}
