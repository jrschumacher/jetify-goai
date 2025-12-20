package claudecode

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.jetify.com/ai/api"
	"go.jetify.com/ai/provider/claudecode/process"
)

func TestLanguageModel_ProviderName(t *testing.T) {
	model := NewLanguageModel("sonnet")
	assert.Equal(t, ProviderName, model.ProviderName())
}

func TestLanguageModel_ModelID(t *testing.T) {
	model := NewLanguageModel("claude-sonnet-4-5-20250929")
	assert.Equal(t, "claude-sonnet-4-5-20250929", model.ModelID())
}

func TestLanguageModel_SupportedUrls(t *testing.T) {
	model := NewLanguageModel("sonnet")
	urls := model.SupportedUrls()

	// Claude CLI doesn't support direct URL loading
	assert.Empty(t, urls)
}

func TestLanguageModel_Generate_TextResponse(t *testing.T) {
	// Create mock process
	mockProc := process.NewMockProcess()

	// Simulate CLI output
	mockProc.WriteStdout([]byte(`{"type":"system","subtype":"init","session_id":"abc-123","model":"claude-sonnet-4-5-20250929"}` + "\n"))
	mockProc.WriteStdout([]byte(`{"type":"result","subtype":"success","session_id":"abc-123","message":{"id":"msg_123","model":"claude-sonnet-4-5-20250929","role":"assistant","content":[{"type":"text","text":"Hello world"}],"stop_reason":"end_turn"},"usage":{"input_tokens":10,"output_tokens":5}}` + "\n"))

	// Create model with mock process
	model := NewLanguageModel("sonnet", WithProcess(mockProc))

	// Create test prompt
	prompt := []api.Message{
		&api.UserMessage{Content: []api.ContentBlock{&api.TextBlock{Text: "Say hello"}}},
	}

	// Generate response
	resp, err := model.Generate(context.Background(), prompt, api.CallOptions{})
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Verify response
	require.Len(t, resp.Content, 1)
	textBlock, ok := resp.Content[0].(*api.TextBlock)
	require.True(t, ok)
	assert.Equal(t, "Hello world", textBlock.Text)
	assert.Equal(t, api.FinishReasonStop, resp.FinishReason)
}

func TestLanguageModel_Generate_ToolCall(t *testing.T) {
	mockProc := process.NewMockProcess()

	mockProc.WriteStdout([]byte(`{"type":"system","subtype":"init","session_id":"abc-123"}` + "\n"))
	mockProc.WriteStdout([]byte(`{"type":"result","subtype":"success","session_id":"abc-123","message":{"id":"msg_123","role":"assistant","content":[{"type":"tool_use","id":"tool_123","name":"get_weather","input":{"location":"NYC"}}],"stop_reason":"tool_use"},"usage":{"input_tokens":10,"output_tokens":15}}` + "\n"))

	model := NewLanguageModel("sonnet", WithProcess(mockProc))

	prompt := []api.Message{
		&api.UserMessage{Content: []api.ContentBlock{&api.TextBlock{Text: "What's the weather in NYC?"}}},
	}

	resp, err := model.Generate(context.Background(), prompt, api.CallOptions{})
	require.NoError(t, err)
	require.NotNil(t, resp)

	require.Len(t, resp.Content, 1)
	toolCall, ok := resp.Content[0].(*api.ToolCallBlock)
	require.True(t, ok)
	assert.Equal(t, "tool_123", toolCall.ToolCallID)
	assert.Equal(t, "get_weather", toolCall.ToolName)
	assert.Equal(t, api.FinishReasonToolCalls, resp.FinishReason)
}

func TestLanguageModel_Generate_WithSystemPrompt(t *testing.T) {
	mockProc := process.NewMockProcess()

	mockProc.WriteStdout([]byte(`{"type":"system","subtype":"init","session_id":"abc-123"}` + "\n"))
	mockProc.WriteStdout([]byte(`{"type":"result","subtype":"success","session_id":"abc-123","message":{"content":[{"type":"text","text":"I am helpful"}],"stop_reason":"end_turn"}}` + "\n"))

	model := NewLanguageModel("sonnet", WithProcess(mockProc))

	prompt := []api.Message{
		&api.SystemMessage{Content: "You are a helpful assistant."},
		&api.UserMessage{Content: []api.ContentBlock{&api.TextBlock{Text: "Who are you?"}}},
	}

	resp, err := model.Generate(context.Background(), prompt, api.CallOptions{})
	require.NoError(t, err)
	require.NotNil(t, resp)
}

func TestLanguageModel_Generate_Error(t *testing.T) {
	mockProc := process.NewMockProcess()

	mockProc.WriteStdout([]byte(`{"type":"system","subtype":"init","session_id":"abc-123"}` + "\n"))
	mockProc.WriteStdout([]byte(`{"type":"result","subtype":"error","is_error":true,"result":"Something went wrong"}` + "\n"))

	model := NewLanguageModel("sonnet", WithProcess(mockProc))

	prompt := []api.Message{
		&api.UserMessage{Content: []api.ContentBlock{&api.TextBlock{Text: "Hello"}}},
	}

	_, err := model.Generate(context.Background(), prompt, api.CallOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Something went wrong")
}

func TestLanguageModel_Generate_Usage(t *testing.T) {
	mockProc := process.NewMockProcess()

	mockProc.WriteStdout([]byte(`{"type":"system","subtype":"init","session_id":"abc-123"}` + "\n"))
	mockProc.WriteStdout([]byte(`{"type":"result","subtype":"success","session_id":"abc-123","message":{"content":[{"type":"text","text":"Hi"}],"stop_reason":"end_turn"},"usage":{"input_tokens":100,"output_tokens":50,"cache_read_input_tokens":25},"total_cost_usd":0.05}` + "\n"))

	model := NewLanguageModel("sonnet", WithProcess(mockProc))

	prompt := []api.Message{
		&api.UserMessage{Content: []api.ContentBlock{&api.TextBlock{Text: "Hi"}}},
	}

	resp, err := model.Generate(context.Background(), prompt, api.CallOptions{})
	require.NoError(t, err)

	assert.Equal(t, 100, resp.Usage.InputTokens)
	assert.Equal(t, 50, resp.Usage.OutputTokens)
	assert.Equal(t, 150, resp.Usage.TotalTokens)
	assert.Equal(t, 25, resp.Usage.CachedInputTokens)
}

func TestLanguageModel_Generate_ResponseInfo(t *testing.T) {
	mockProc := process.NewMockProcess()

	mockProc.WriteStdout([]byte(`{"type":"system","subtype":"init","session_id":"abc-123"}` + "\n"))
	mockProc.WriteStdout([]byte(`{"type":"result","subtype":"success","session_id":"abc-123","message":{"id":"msg_789","model":"claude-sonnet-4-5-20250929","content":[{"type":"text","text":"Hi"}],"stop_reason":"end_turn"}}` + "\n"))

	model := NewLanguageModel("sonnet", WithProcess(mockProc))

	prompt := []api.Message{
		&api.UserMessage{Content: []api.ContentBlock{&api.TextBlock{Text: "Hi"}}},
	}

	resp, err := model.Generate(context.Background(), prompt, api.CallOptions{})
	require.NoError(t, err)

	require.NotNil(t, resp.ResponseInfo)
	assert.Equal(t, "msg_789", resp.ResponseInfo.ID)
	assert.Equal(t, "claude-sonnet-4-5-20250929", resp.ResponseInfo.ModelID)
}

func TestNewLanguageModel_DefaultOptions(t *testing.T) {
	model := NewLanguageModel("sonnet")

	assert.Equal(t, "sonnet", model.ModelID())
	assert.Equal(t, ProviderName, model.ProviderName())
}

func TestNewLanguageModel_CustomOptions(t *testing.T) {
	mockProc := process.NewMockProcess()
	model := NewLanguageModel("opus", WithProcess(mockProc))

	assert.Equal(t, "opus", model.ModelID())
}

func TestLanguageModel_Stream_TextDeltas(t *testing.T) {
	mockProc := process.NewMockProcess()

	// Simulate CLI streaming output (note: stream event data is in "event" field, not "stream_event")
	mockProc.WriteStdout([]byte(`{"type":"system","subtype":"init","session_id":"abc-123"}` + "\n"))
	mockProc.WriteStdout([]byte(`{"type":"stream_event","event":{"type":"message_start","message":{"id":"msg_123","model":"claude-sonnet-4-5-20250929","role":"assistant"}}}` + "\n"))
	mockProc.WriteStdout([]byte(`{"type":"stream_event","event":{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}}` + "\n"))
	mockProc.WriteStdout([]byte(`{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}}` + "\n"))
	mockProc.WriteStdout([]byte(`{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}}` + "\n"))
	mockProc.WriteStdout([]byte(`{"type":"stream_event","event":{"type":"content_block_stop","index":0}}` + "\n"))
	mockProc.WriteStdout([]byte(`{"type":"stream_event","event":{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":10,"output_tokens":5}}}` + "\n"))
	mockProc.WriteStdout([]byte(`{"type":"result","subtype":"success","session_id":"abc-123","usage":{"input_tokens":10,"output_tokens":5},"total_cost_usd":0.001}` + "\n"))

	model := NewLanguageModel("sonnet", WithProcess(mockProc))

	prompt := []api.Message{
		&api.UserMessage{Content: []api.ContentBlock{&api.TextBlock{Text: "Say hello"}}},
	}

	streamResp, err := model.Stream(context.Background(), prompt, api.CallOptions{})
	require.NoError(t, err)
	require.NotNil(t, streamResp)

	// Collect all events
	var events []api.StreamEvent
	for event := range streamResp.Stream {
		events = append(events, event)
	}

	// Should have: ResponseMetadataEvent, 2 TextDeltaEvents, FinishEvent (message_delta), FinishEvent (result)
	require.GreaterOrEqual(t, len(events), 3, "expected at least 3 events")

	// First event should be response metadata
	metadata, ok := events[0].(*api.ResponseMetadataEvent)
	require.True(t, ok, "first event should be ResponseMetadataEvent, got %T", events[0])
	assert.Equal(t, "msg_123", metadata.ID)

	// Should have text deltas
	var textDeltas []string
	for _, e := range events {
		if td, ok := e.(*api.TextDeltaEvent); ok {
			textDeltas = append(textDeltas, td.TextDelta)
		}
	}
	assert.Equal(t, []string{"Hello", " world"}, textDeltas)

	// Last event should be FinishEvent from result
	finishEvent, ok := events[len(events)-1].(*api.FinishEvent)
	require.True(t, ok, "last event should be FinishEvent, got %T", events[len(events)-1])
	assert.Equal(t, api.FinishReasonStop, finishEvent.FinishReason)
	assert.Equal(t, 10, finishEvent.Usage.InputTokens)
	assert.Equal(t, 5, finishEvent.Usage.OutputTokens)
}

func TestLanguageModel_Stream_ToolCall(t *testing.T) {
	mockProc := process.NewMockProcess()

	mockProc.WriteStdout([]byte(`{"type":"system","subtype":"init","session_id":"abc-123"}` + "\n"))
	mockProc.WriteStdout([]byte(`{"type":"stream_event","event":{"type":"message_start","message":{"id":"msg_123","role":"assistant"}}}` + "\n"))
	mockProc.WriteStdout([]byte(`{"type":"stream_event","event":{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"tool_123","name":"get_weather","input":{}}}}` + "\n"))
	mockProc.WriteStdout([]byte(`{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","text":"{\"location\":"}}}` + "\n"))
	mockProc.WriteStdout([]byte(`{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","text":"\"NYC\"}"}}}` + "\n"))
	mockProc.WriteStdout([]byte(`{"type":"stream_event","event":{"type":"content_block_stop","index":0}}` + "\n"))
	mockProc.WriteStdout([]byte(`{"type":"stream_event","event":{"type":"message_delta","delta":{"stop_reason":"tool_use"}}}` + "\n"))
	mockProc.WriteStdout([]byte(`{"type":"result","subtype":"success","session_id":"abc-123","message":{"content":[{"type":"tool_use","id":"tool_123","name":"get_weather","input":{"location":"NYC"}}],"stop_reason":"tool_use"}}` + "\n"))

	model := NewLanguageModel("sonnet", WithProcess(mockProc))

	prompt := []api.Message{
		&api.UserMessage{Content: []api.ContentBlock{&api.TextBlock{Text: "What's the weather?"}}},
	}

	streamResp, err := model.Stream(context.Background(), prompt, api.CallOptions{})
	require.NoError(t, err)

	var events []api.StreamEvent
	for event := range streamResp.Stream {
		events = append(events, event)
	}

	// Should have tool call event from content_block_start
	var toolCallEvent *api.ToolCallEvent
	for _, e := range events {
		if tc, ok := e.(*api.ToolCallEvent); ok {
			toolCallEvent = tc
			break
		}
	}
	require.NotNil(t, toolCallEvent, "expected ToolCallEvent")
	assert.Equal(t, "tool_123", toolCallEvent.ToolCallID)
	assert.Equal(t, "get_weather", toolCallEvent.ToolName)

	// Last event should be FinishEvent with tool_use reason
	finishEvent, ok := events[len(events)-1].(*api.FinishEvent)
	require.True(t, ok)
	assert.Equal(t, api.FinishReasonToolCalls, finishEvent.FinishReason)
}

func TestLanguageModel_Stream_Error(t *testing.T) {
	mockProc := process.NewMockProcess()

	mockProc.WriteStdout([]byte(`{"type":"system","subtype":"init","session_id":"abc-123"}` + "\n"))
	mockProc.WriteStdout([]byte(`{"type":"result","subtype":"error","is_error":true,"result":"API rate limit exceeded"}` + "\n"))

	model := NewLanguageModel("sonnet", WithProcess(mockProc))

	prompt := []api.Message{
		&api.UserMessage{Content: []api.ContentBlock{&api.TextBlock{Text: "Hello"}}},
	}

	streamResp, err := model.Stream(context.Background(), prompt, api.CallOptions{})
	require.NoError(t, err)

	var events []api.StreamEvent
	for event := range streamResp.Stream {
		events = append(events, event)
	}

	// Should have an error event
	require.Len(t, events, 1)
	errorEvent, ok := events[0].(*api.ErrorEvent)
	require.True(t, ok, "expected ErrorEvent")
	errVal, ok := errorEvent.Err.(error)
	require.True(t, ok, "expected error type")
	assert.Contains(t, errVal.Error(), "API rate limit exceeded")
}
