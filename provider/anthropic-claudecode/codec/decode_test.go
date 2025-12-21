package codec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.jetify.com/ai/api"
)

func TestDecodeResponse_TextContent(t *testing.T) {
	event := &Event{
		Type:      EventTypeResult,
		Subtype:   "success",
		SessionID: "abc-123",
		Result:    "Hello world",
		Message: &AssistantMessage{
			ID:    "msg_123",
			Model: "claude-sonnet-4-5-20250929",
			Role:  "assistant",
			Content: []ContentBlock{
				{Type: "text", Text: "Hello world"},
			},
			StopReason: "end_turn",
		},
		Usage: &Usage{
			InputTokens:  10,
			OutputTokens: 5,
		},
	}

	resp, err := DecodeResponse(event)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Check content
	require.Len(t, resp.Content, 1)
	textBlock, ok := resp.Content[0].(*api.TextBlock)
	require.True(t, ok, "expected TextBlock")
	assert.Equal(t, "Hello world", textBlock.Text)

	// Check finish reason
	assert.Equal(t, api.FinishReasonStop, resp.FinishReason)

	// Check usage
	assert.Equal(t, 10, resp.Usage.InputTokens)
	assert.Equal(t, 5, resp.Usage.OutputTokens)
	assert.Equal(t, 15, resp.Usage.TotalTokens)
}

func TestDecodeResponse_MultipleTextBlocks(t *testing.T) {
	event := &Event{
		Type:    EventTypeResult,
		Subtype: "success",
		Message: &AssistantMessage{
			Content: []ContentBlock{
				{Type: "text", Text: "First"},
				{Type: "text", Text: "Second"},
			},
			StopReason: "end_turn",
		},
	}

	resp, err := DecodeResponse(event)
	require.NoError(t, err)
	require.Len(t, resp.Content, 2)

	block1, ok := resp.Content[0].(*api.TextBlock)
	require.True(t, ok)
	assert.Equal(t, "First", block1.Text)

	block2, ok := resp.Content[1].(*api.TextBlock)
	require.True(t, ok)
	assert.Equal(t, "Second", block2.Text)
}

func TestDecodeResponse_ToolUse(t *testing.T) {
	event := &Event{
		Type:    EventTypeResult,
		Subtype: "success",
		Message: &AssistantMessage{
			Content: []ContentBlock{
				{
					Type:  "tool_use",
					ID:    "tool_123",
					Name:  "get_weather",
					Input: []byte(`{"location":"NYC"}`),
				},
			},
			StopReason: "tool_use",
		},
	}

	resp, err := DecodeResponse(event)
	require.NoError(t, err)
	require.Len(t, resp.Content, 1)

	toolCall, ok := resp.Content[0].(*api.ToolCallBlock)
	require.True(t, ok, "expected ToolCallBlock")
	assert.Equal(t, "tool_123", toolCall.ToolCallID)
	assert.Equal(t, "get_weather", toolCall.ToolName)
	assert.JSONEq(t, `{"location":"NYC"}`, string(toolCall.Args))
	assert.Equal(t, api.FinishReasonToolCalls, resp.FinishReason)
}

func TestDecodeResponse_StructuredOutput(t *testing.T) {
	event := &Event{
		Type:             EventTypeResult,
		Subtype:          "success",
		StructuredOutput: []byte(`{"answer":"42"}`),
		Message: &AssistantMessage{
			Content:    []ContentBlock{},
			StopReason: "end_turn",
		},
	}

	resp, err := DecodeResponse(event)
	require.NoError(t, err)

	// Structured output should be in provider metadata
	require.NotNil(t, resp.ProviderMetadata)
	metadata := GetMetadata(resp)
	require.NotNil(t, metadata)
	assert.JSONEq(t, `{"answer":"42"}`, string(metadata.StructuredOutput))
}

func TestDecodeResponse_Usage(t *testing.T) {
	event := &Event{
		Type:    EventTypeResult,
		Subtype: "success",
		Message: &AssistantMessage{
			Content:    []ContentBlock{{Type: "text", Text: "Hi"}},
			StopReason: "end_turn",
		},
		Usage: &Usage{
			InputTokens:              100,
			OutputTokens:             50,
			CacheCreationInputTokens: 1000,
			CacheReadInputTokens:     500,
		},
	}

	resp, err := DecodeResponse(event)
	require.NoError(t, err)

	assert.Equal(t, 100, resp.Usage.InputTokens)
	assert.Equal(t, 50, resp.Usage.OutputTokens)
	assert.Equal(t, 150, resp.Usage.TotalTokens)
	assert.Equal(t, 500, resp.Usage.CachedInputTokens)
}

func TestDecodeResponse_FinishReasons(t *testing.T) {
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
		{"empty", "", api.FinishReasonUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &Event{
				Type:    EventTypeResult,
				Subtype: "success",
				Message: &AssistantMessage{
					Content:    []ContentBlock{{Type: "text", Text: "Hi"}},
					StopReason: tt.stopReason,
				},
			}

			resp, err := DecodeResponse(event)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, resp.FinishReason)
		})
	}
}

func TestDecodeResponse_Error(t *testing.T) {
	event := &Event{
		Type:    EventTypeResult,
		Subtype: "error",
		IsError: true,
		Result:  "Something went wrong",
	}

	_, err := DecodeResponse(event)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Something went wrong")
}

func TestDecodeResponse_NilEvent(t *testing.T) {
	_, err := DecodeResponse(nil)
	require.Error(t, err)
}

func TestDecodeResponse_WrongEventType(t *testing.T) {
	event := &Event{
		Type: EventTypeSystem,
	}

	_, err := DecodeResponse(event)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected result event")
}

func TestDecodeResponse_ResponseInfo(t *testing.T) {
	event := &Event{
		Type:      EventTypeResult,
		Subtype:   "success",
		SessionID: "session-456",
		Message: &AssistantMessage{
			ID:         "msg_789",
			Model:      "claude-sonnet-4-5-20250929",
			Content:    []ContentBlock{{Type: "text", Text: "Hi"}},
			StopReason: "end_turn",
		},
	}

	resp, err := DecodeResponse(event)
	require.NoError(t, err)
	require.NotNil(t, resp.ResponseInfo)
	assert.Equal(t, "msg_789", resp.ResponseInfo.ID)
	assert.Equal(t, "claude-sonnet-4-5-20250929", resp.ResponseInfo.ModelID)
}

func TestDecodeResponse_CostInMetadata(t *testing.T) {
	event := &Event{
		Type:         EventTypeResult,
		Subtype:      "success",
		TotalCostUSD: 0.0962,
		Message: &AssistantMessage{
			Content:    []ContentBlock{{Type: "text", Text: "Hi"}},
			StopReason: "end_turn",
		},
	}

	resp, err := DecodeResponse(event)
	require.NoError(t, err)
	require.NotNil(t, resp.ProviderMetadata)

	metadata := GetMetadata(resp)
	require.NotNil(t, metadata)
	assert.InDelta(t, 0.0962, metadata.CostUSD, 0.0001)
}
