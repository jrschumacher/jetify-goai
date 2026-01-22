package codec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.jetify.com/ai/api"
)

func TestEventCollector_SimpleResponse(t *testing.T) {
	collector := NewEventCollector()

	events := []*Event{
		{Type: EventTypeThreadStarted, ThreadID: "thread-123"},
		{Type: EventTypeTurnStarted},
		{Type: EventTypeItemCompleted, Item: &Item{ID: "item_1", Type: ItemTypeAgentMessage, Text: "Hello world"}},
		{Type: EventTypeTurnCompleted, Usage: &Usage{InputTokens: 10, OutputTokens: 5}},
	}

	for _, event := range events {
		done, err := collector.ProcessEvent(event)
		require.NoError(t, err)
		if done {
			break
		}
	}

	resp, err := collector.Build()
	require.NoError(t, err)

	require.Len(t, resp.Content, 1)
	textBlock, ok := resp.Content[0].(*api.TextBlock)
	require.True(t, ok)
	assert.Equal(t, "Hello world", textBlock.Text)

	assert.Equal(t, api.FinishReasonStop, resp.FinishReason)
	assert.Equal(t, 10, resp.Usage.InputTokens)
	assert.Equal(t, 5, resp.Usage.OutputTokens)
	assert.Equal(t, 15, resp.Usage.TotalTokens)

	require.NotNil(t, resp.ResponseInfo)
	assert.Equal(t, "thread-123", resp.ResponseInfo.ID)
}

func TestEventCollector_WithReasoning(t *testing.T) {
	collector := NewEventCollector()

	events := []*Event{
		{Type: EventTypeThreadStarted, ThreadID: "thread-456"},
		{Type: EventTypeTurnStarted},
		{Type: EventTypeItemCompleted, Item: &Item{ID: "item_0", Type: ItemTypeReasoning, Text: "Let me think about this..."}},
		{Type: EventTypeItemCompleted, Item: &Item{ID: "item_1", Type: ItemTypeAgentMessage, Text: "The answer is 42"}},
		{Type: EventTypeTurnCompleted, Usage: &Usage{InputTokens: 100, OutputTokens: 20}},
	}

	for _, event := range events {
		collector.ProcessEvent(event)
	}

	resp, err := collector.Build()
	require.NoError(t, err)

	// Only agent_message should be in content
	require.Len(t, resp.Content, 1)
	textBlock := resp.Content[0].(*api.TextBlock)
	assert.Equal(t, "The answer is 42", textBlock.Text)

	// Reasoning should be in metadata
	metadata := GetMetadata(resp)
	require.NotNil(t, metadata)
	assert.Equal(t, "Let me think about this...", metadata.Reasoning)
}

func TestEventCollector_MultipleMessages(t *testing.T) {
	collector := NewEventCollector()

	events := []*Event{
		{Type: EventTypeThreadStarted, ThreadID: "thread-789"},
		{Type: EventTypeTurnStarted},
		{Type: EventTypeItemCompleted, Item: &Item{ID: "item_1", Type: ItemTypeAgentMessage, Text: "First part. "}},
		{Type: EventTypeItemCompleted, Item: &Item{ID: "item_2", Type: ItemTypeAgentMessage, Text: "Second part."}},
		{Type: EventTypeTurnCompleted, Usage: &Usage{InputTokens: 50, OutputTokens: 10}},
	}

	for _, event := range events {
		collector.ProcessEvent(event)
	}

	resp, err := collector.Build()
	require.NoError(t, err)

	require.Len(t, resp.Content, 2)
	assert.Equal(t, "First part. ", resp.Content[0].(*api.TextBlock).Text)
	assert.Equal(t, "Second part.", resp.Content[1].(*api.TextBlock).Text)
}

func TestEventCollector_TurnFailed(t *testing.T) {
	collector := NewEventCollector()

	events := []*Event{
		{Type: EventTypeThreadStarted, ThreadID: "thread-err"},
		{Type: EventTypeTurnStarted},
		{Type: EventTypeTurnFailed, Error: &Error{Message: "Model not supported"}},
	}

	for _, event := range events {
		collector.ProcessEvent(event)
	}

	_, err := collector.Build()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Model not supported")
}

func TestEventCollector_Error(t *testing.T) {
	collector := NewEventCollector()

	events := []*Event{
		{Type: EventTypeError, Message: "Connection failed"},
	}

	for _, event := range events {
		collector.ProcessEvent(event)
	}

	_, err := collector.Build()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Connection failed")
}

func TestEventCollector_CachedTokens(t *testing.T) {
	collector := NewEventCollector()

	events := []*Event{
		{Type: EventTypeThreadStarted, ThreadID: "thread-cache"},
		{Type: EventTypeTurnStarted},
		{Type: EventTypeItemCompleted, Item: &Item{ID: "item_1", Type: ItemTypeAgentMessage, Text: "Response"}},
		{Type: EventTypeTurnCompleted, Usage: &Usage{InputTokens: 100, CachedInputTokens: 50, OutputTokens: 10}},
	}

	for _, event := range events {
		collector.ProcessEvent(event)
	}

	resp, err := collector.Build()
	require.NoError(t, err)

	assert.Equal(t, 50, resp.Usage.CachedInputTokens)
}

func TestEventCollector_NilEvent(t *testing.T) {
	collector := NewEventCollector()
	_, err := collector.ProcessEvent(nil)
	require.Error(t, err)
}

func TestEventCollector_EmptyAgentMessage(t *testing.T) {
	collector := NewEventCollector()

	events := []*Event{
		{Type: EventTypeThreadStarted, ThreadID: "thread-empty"},
		{Type: EventTypeTurnStarted},
		{Type: EventTypeItemCompleted, Item: &Item{ID: "item_1", Type: ItemTypeAgentMessage, Text: ""}},
		{Type: EventTypeTurnCompleted, Usage: &Usage{InputTokens: 10, OutputTokens: 0}},
	}

	for _, event := range events {
		collector.ProcessEvent(event)
	}

	resp, err := collector.Build()
	require.NoError(t, err)

	// Empty messages should not be added
	assert.Len(t, resp.Content, 0)
}

func TestDecodeResponse(t *testing.T) {
	events := []*Event{
		{Type: EventTypeThreadStarted, ThreadID: "thread-decode"},
		{Type: EventTypeTurnStarted},
		{Type: EventTypeItemCompleted, Item: &Item{ID: "item_1", Type: ItemTypeAgentMessage, Text: "Hello"}},
		{Type: EventTypeTurnCompleted, Usage: &Usage{InputTokens: 5, OutputTokens: 2}},
	}

	resp, err := DecodeResponse(events)
	require.NoError(t, err)

	require.Len(t, resp.Content, 1)
	assert.Equal(t, "Hello", resp.Content[0].(*api.TextBlock).Text)
}

func TestEventCollector_ThreadID(t *testing.T) {
	collector := NewEventCollector()

	events := []*Event{
		{Type: EventTypeThreadStarted, ThreadID: "my-thread-id"},
	}

	for _, event := range events {
		collector.ProcessEvent(event)
	}

	assert.Equal(t, "my-thread-id", collector.ThreadID())
}
