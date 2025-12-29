package codec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.jetify.com/ai/api"
)

func TestDecodeStreamEvent_ThreadStarted(t *testing.T) {
	event := &Event{
		Type:     EventTypeThreadStarted,
		ThreadID: "thread-123",
	}

	streamEvent, err := DecodeStreamEvent(event)
	require.NoError(t, err)
	require.NotNil(t, streamEvent)

	metaEvent, ok := streamEvent.(*api.ResponseMetadataEvent)
	require.True(t, ok)
	assert.Equal(t, "thread-123", metaEvent.ID)
}

func TestDecodeStreamEvent_TurnStarted(t *testing.T) {
	event := &Event{Type: EventTypeTurnStarted}

	streamEvent, err := DecodeStreamEvent(event)
	require.NoError(t, err)
	assert.Nil(t, streamEvent) // No SDK event for turn_started
}

func TestDecodeStreamEvent_ItemCompleted_AgentMessage(t *testing.T) {
	event := &Event{
		Type: EventTypeItemCompleted,
		Item: &Item{
			ID:   "item_1",
			Type: ItemTypeAgentMessage,
			Text: "Hello, world!",
		},
	}

	streamEvent, err := DecodeStreamEvent(event)
	require.NoError(t, err)
	require.NotNil(t, streamEvent)

	textDelta, ok := streamEvent.(*api.TextDeltaEvent)
	require.True(t, ok)
	assert.Equal(t, "Hello, world!", textDelta.TextDelta)
}

func TestDecodeStreamEvent_ItemCompleted_EmptyAgentMessage(t *testing.T) {
	event := &Event{
		Type: EventTypeItemCompleted,
		Item: &Item{
			ID:   "item_1",
			Type: ItemTypeAgentMessage,
			Text: "",
		},
	}

	streamEvent, err := DecodeStreamEvent(event)
	require.NoError(t, err)
	assert.Nil(t, streamEvent) // Empty messages should be ignored
}

func TestDecodeStreamEvent_ItemCompleted_Reasoning(t *testing.T) {
	event := &Event{
		Type: EventTypeItemCompleted,
		Item: &Item{
			ID:   "item_0",
			Type: ItemTypeReasoning,
			Text: "Let me think about this...",
		},
	}

	streamEvent, err := DecodeStreamEvent(event)
	require.NoError(t, err)
	require.NotNil(t, streamEvent)

	reasoning, ok := streamEvent.(*api.ReasoningEvent)
	require.True(t, ok)
	assert.Equal(t, "Let me think about this...", reasoning.TextDelta)
}

func TestDecodeStreamEvent_ItemCompleted_MCPToolCall(t *testing.T) {
	event := &Event{
		Type: EventTypeItemCompleted,
		Item: &Item{
			ID:        "item_2",
			Type:      ItemTypeMCPToolCall,
			ToolName:  "read_file",
			ToolInput: []byte(`{"path":"/tmp/test.txt"}`),
		},
	}

	streamEvent, err := DecodeStreamEvent(event)
	require.NoError(t, err)
	require.NotNil(t, streamEvent)

	toolCall, ok := streamEvent.(*api.ToolCallEvent)
	require.True(t, ok)
	assert.Equal(t, "item_2", toolCall.ToolCallID)
	assert.Equal(t, "read_file", toolCall.ToolName)
	assert.JSONEq(t, `{"path":"/tmp/test.txt"}`, string(toolCall.Args))
}

func TestDecodeStreamEvent_ItemCompleted_MCPToolCallNoInput(t *testing.T) {
	event := &Event{
		Type: EventTypeItemCompleted,
		Item: &Item{
			ID:       "item_2",
			Type:     ItemTypeMCPToolCall,
			ToolName: "list_files",
		},
	}

	streamEvent, err := DecodeStreamEvent(event)
	require.NoError(t, err)
	require.NotNil(t, streamEvent)

	toolCall, ok := streamEvent.(*api.ToolCallEvent)
	require.True(t, ok)
	assert.Equal(t, "{}", string(toolCall.Args))
}

func TestDecodeStreamEvent_ItemCompleted_CommandExecution(t *testing.T) {
	exitCode := 0
	event := &Event{
		Type: EventTypeItemCompleted,
		Item: &Item{
			ID:       "item_3",
			Type:     ItemTypeCommandExecution,
			Command:  "ls -la",
			ExitCode: &exitCode,
			Output:   "total 0",
		},
	}

	streamEvent, err := DecodeStreamEvent(event)
	require.NoError(t, err)
	assert.Nil(t, streamEvent) // No direct SDK mapping for command_execution
}

func TestDecodeStreamEvent_ItemCompleted_NilItem(t *testing.T) {
	event := &Event{
		Type: EventTypeItemCompleted,
		Item: nil,
	}

	streamEvent, err := DecodeStreamEvent(event)
	require.NoError(t, err)
	assert.Nil(t, streamEvent)
}

func TestDecodeStreamEvent_TurnCompleted(t *testing.T) {
	event := &Event{
		Type: EventTypeTurnCompleted,
		Usage: &Usage{
			InputTokens:       100,
			CachedInputTokens: 50,
			OutputTokens:      20,
		},
	}

	streamEvent, err := DecodeStreamEvent(event)
	require.NoError(t, err)
	require.NotNil(t, streamEvent)

	finish, ok := streamEvent.(*api.FinishEvent)
	require.True(t, ok)
	assert.Equal(t, api.FinishReasonStop, finish.FinishReason)
	assert.Equal(t, 100, finish.Usage.InputTokens)
	assert.Equal(t, 20, finish.Usage.OutputTokens)
	assert.Equal(t, 120, finish.Usage.TotalTokens)
	assert.Equal(t, 50, finish.Usage.CachedInputTokens)
}

func TestDecodeStreamEvent_TurnCompletedNoUsage(t *testing.T) {
	event := &Event{
		Type:  EventTypeTurnCompleted,
		Usage: nil,
	}

	streamEvent, err := DecodeStreamEvent(event)
	require.NoError(t, err)
	require.NotNil(t, streamEvent)

	finish, ok := streamEvent.(*api.FinishEvent)
	require.True(t, ok)
	assert.Equal(t, api.FinishReasonStop, finish.FinishReason)
	assert.Equal(t, 0, finish.Usage.InputTokens)
}

func TestDecodeStreamEvent_TurnFailed(t *testing.T) {
	event := &Event{
		Type:  EventTypeTurnFailed,
		Error: &Error{Message: "Model not available"},
	}

	streamEvent, err := DecodeStreamEvent(event)
	require.NoError(t, err)
	require.NotNil(t, streamEvent)

	errorEvent, ok := streamEvent.(*api.ErrorEvent)
	require.True(t, ok)
	assert.Contains(t, errorEvent.Error(), "Model not available")
}

func TestDecodeStreamEvent_TurnFailedNoError(t *testing.T) {
	event := &Event{
		Type:  EventTypeTurnFailed,
		Error: nil,
	}

	streamEvent, err := DecodeStreamEvent(event)
	require.NoError(t, err)
	require.NotNil(t, streamEvent)

	errorEvent, ok := streamEvent.(*api.ErrorEvent)
	require.True(t, ok)
	assert.Contains(t, errorEvent.Error(), "turn failed")
}

func TestDecodeStreamEvent_Error(t *testing.T) {
	event := &Event{
		Type:    EventTypeError,
		Message: "Connection lost",
	}

	streamEvent, err := DecodeStreamEvent(event)
	require.NoError(t, err)
	require.NotNil(t, streamEvent)

	errorEvent, ok := streamEvent.(*api.ErrorEvent)
	require.True(t, ok)
	assert.Contains(t, errorEvent.Error(), "Connection lost")
}

func TestDecodeStreamEvent_ErrorWithErrorField(t *testing.T) {
	event := &Event{
		Type:  EventTypeError,
		Error: &Error{Message: "Rate limit exceeded"},
	}

	streamEvent, err := DecodeStreamEvent(event)
	require.NoError(t, err)
	require.NotNil(t, streamEvent)

	errorEvent, ok := streamEvent.(*api.ErrorEvent)
	require.True(t, ok)
	assert.Contains(t, errorEvent.Error(), "Rate limit exceeded")
}

func TestDecodeStreamEvent_NilEvent(t *testing.T) {
	_, err := DecodeStreamEvent(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil event")
}

func TestDecodeStreamEvent_UnknownEventType(t *testing.T) {
	event := &Event{Type: "unknown.event"}

	streamEvent, err := DecodeStreamEvent(event)
	require.NoError(t, err)
	assert.Nil(t, streamEvent) // Unknown events should be ignored
}

func TestDecodeStreamEvent_ItemUpdated_AgentMessage(t *testing.T) {
	event := &Event{
		Type: EventTypeItemUpdated,
		Item: &Item{
			ID:   "item_1",
			Type: ItemTypeAgentMessage,
			Text: "Partial response...",
		},
	}

	streamEvent, err := DecodeStreamEvent(event)
	require.NoError(t, err)
	require.NotNil(t, streamEvent)

	textDelta, ok := streamEvent.(*api.TextDeltaEvent)
	require.True(t, ok)
	assert.Equal(t, "Partial response...", textDelta.TextDelta)
}

func TestDecodeStreamEvent_ItemStarted(t *testing.T) {
	event := &Event{
		Type: EventTypeItemStarted,
		Item: &Item{
			ID:   "item_1",
			Type: ItemTypeAgentMessage,
		},
	}

	streamEvent, err := DecodeStreamEvent(event)
	require.NoError(t, err)
	assert.Nil(t, streamEvent) // No text to emit
}
