package claudecode

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.jetify.com/ai/api"
	"go.jetify.com/ai/provider/anthropic-claudecode/process"
	"go.jetify.com/ai/provider/internal/cli"
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

func TestLanguageModel_TokenUsageTracker(t *testing.T) {
	mockProc := process.NewMockProcess()

	mockProc.WriteStdout([]byte(`{"type":"system","subtype":"init","session_id":"abc-123"}` + "\n"))
	mockProc.WriteStdout([]byte(`{"type":"result","subtype":"success","session_id":"abc-123","message":{"content":[{"type":"text","text":"Hi"}],"stop_reason":"end_turn"},"usage":{"input_tokens":10,"output_tokens":5,"cache_read_input_tokens":2}}` + "\n"))

	tracker := cli.NewTokenTracker()
	model := NewLanguageModel("sonnet", WithProcess(mockProc), WithTokenTracker(tracker))

	prompt := []api.Message{
		&api.UserMessage{Content: []api.ContentBlock{&api.TextBlock{Text: "Hi"}}},
	}

	_, err := model.Generate(context.Background(), prompt, api.CallOptions{})
	require.NoError(t, err)

	u := model.TokenUsage()
	assert.Equal(t, 10, u.InputTokens)
	assert.Equal(t, 5, u.OutputTokens)
	assert.Equal(t, 15, u.TotalTokens)
	assert.Equal(t, 2, u.CachedTokens)

	model.ResetTokenUsage()
	u = model.TokenUsage()
	assert.Equal(t, 0, u.TotalTokens)
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

// --- Lifecycle tests ---

func TestLanguageModel_IsProcessRunning_NoProcess(t *testing.T) {
	model := NewLanguageModel("sonnet")
	assert.False(t, model.IsProcessRunning())
}

func TestLanguageModel_IsProcessRunning_WithProcess(t *testing.T) {
	mockProc := process.NewMockProcess()
	_ = mockProc.Start(context.Background())

	model := NewLanguageModel("sonnet", WithProcess(mockProc))
	assert.True(t, model.IsProcessRunning())
}

func TestLanguageModel_Close_NoProcess(t *testing.T) {
	model := NewLanguageModel("sonnet")
	err := model.Close()
	assert.NoError(t, err)
}

func TestLanguageModel_Close_StopsProcess(t *testing.T) {
	mockProc := process.NewMockProcess()
	_ = mockProc.Start(context.Background())

	model := NewLanguageModel("sonnet", WithProcess(mockProc))
	assert.True(t, model.IsProcessRunning())

	err := model.Close()
	assert.NoError(t, err)
	assert.False(t, mockProc.IsRunning())
}

func TestLanguageModel_RestartProcess_NoProcess(t *testing.T) {
	model := NewLanguageModel("sonnet")
	err := model.RestartProcess(context.Background())
	assert.NoError(t, err)
}

func TestLanguageModel_RestartProcess_SavesSessionID(t *testing.T) {
	mockProc := process.NewMockProcess()
	_ = mockProc.Start(context.Background())
	mockProc.SetSessionID("sess-to-resume")

	model := NewLanguageModel("sonnet", WithProcess(mockProc))

	err := model.RestartProcess(context.Background())
	require.NoError(t, err)

	// Process should be stopped and nil'd
	assert.False(t, mockProc.IsRunning())
	assert.Nil(t, model.proc)

	// resumeSessionID should be saved for next start
	assert.Equal(t, "sess-to-resume", model.resumeSessionID)
}

func TestLanguageModel_ConfigNeedsRestart_NoProcess(t *testing.T) {
	model := NewLanguageModel("sonnet")

	temp := 0.5
	assert.False(t, model.ConfigNeedsRestart(api.CallOptions{Temperature: &temp}))
}

func TestLanguageModel_ConfigNeedsRestart_SameConfig(t *testing.T) {
	mockProc := process.NewMockProcess()
	_ = mockProc.Start(context.Background())

	model := NewLanguageModel("sonnet", WithProcess(mockProc))
	// cachedKey has no temperature set (default zero, HasTemp=false)

	// No temperature in call options either
	assert.False(t, model.ConfigNeedsRestart(api.CallOptions{}))
}

func TestLanguageModel_ConfigNeedsRestart_TemperatureChanged(t *testing.T) {
	mockProc := process.NewMockProcess()
	_ = mockProc.Start(context.Background())

	model := NewLanguageModel("sonnet", WithProcess(mockProc))
	// cachedKey has no temperature (HasTemp=false, Temperature=0)

	// Now request a temperature change
	temp := 0.8
	assert.True(t, model.ConfigNeedsRestart(api.CallOptions{Temperature: &temp}))
}

func TestLanguageModel_ConfigNeedsRestart_TemperatureRemoved(t *testing.T) {
	mockProc := process.NewMockProcess()
	_ = mockProc.Start(context.Background())

	model := NewLanguageModel("sonnet", WithProcess(mockProc))
	// Set cachedKey to have temperature
	model.cachedKey.Temperature = 0.5
	model.cachedKey.HasTemp = true

	// Call without temperature
	assert.True(t, model.ConfigNeedsRestart(api.CallOptions{}))
}

// --- drainStderr tests ---

func TestDrainStderr_NilStderr(t *testing.T) {
	mockProc := process.NewMockProcess()
	// Don't write anything to stderr, but the buffer exists
	result := drainStderr(mockProc)
	assert.Empty(t, result)
}

func TestDrainStderr_WithContent(t *testing.T) {
	mockProc := process.NewMockProcess()
	mockProc.WriteStderr([]byte("  error: something failed  \n"))

	result := drainStderr(mockProc)
	assert.Equal(t, "error: something failed", result)
}

func TestDrainStderr_TruncatesLargeContent(t *testing.T) {
	mockProc := process.NewMockProcess()
	// Write more than 4096 bytes
	large := make([]byte, 8192)
	for i := range large {
		large[i] = 'x'
	}
	mockProc.WriteStderr(large)

	result := drainStderr(mockProc)
	assert.Len(t, result, 4096)
}

// --- Generate edge cases ---

func TestLanguageModel_Generate_NoResultEvent(t *testing.T) {
	// Clean EOF without result event
	mockProc := process.NewMockProcess()
	mockProc.WriteStdout([]byte(`{"type":"system","subtype":"init","session_id":"abc-123"}` + "\n"))
	// No result event - stdout ends here

	model := NewLanguageModel("sonnet", WithProcess(mockProc))

	prompt := []api.Message{
		&api.UserMessage{Content: []api.ContentBlock{&api.TextBlock{Text: "Hello"}}},
	}

	_, err := model.Generate(context.Background(), prompt, api.CallOptions{})
	require.Error(t, err)
	// Error gets classified by WrapError - the raw message contains "cli" which
	// triggers base classifier's process failure category
	assert.Contains(t, err.Error(), "CLI")
}

func TestLanguageModel_Generate_NoResultEvent_WithStderr(t *testing.T) {
	// Clean EOF without result, but with stderr content
	mockProc := process.NewMockProcess()
	mockProc.WriteStdout([]byte(`{"type":"system","subtype":"init","session_id":"abc-123"}` + "\n"))
	mockProc.WriteStderr([]byte("process crashed unexpectedly"))

	model := NewLanguageModel("sonnet", WithProcess(mockProc))

	prompt := []api.Message{
		&api.UserMessage{Content: []api.ContentBlock{&api.TextBlock{Text: "Hello"}}},
	}

	_, err := model.Generate(context.Background(), prompt, api.CallOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "process crashed unexpectedly")
}

func TestLanguageModel_Generate_InvalidJSON(t *testing.T) {
	mockProc := process.NewMockProcess()
	mockProc.WriteStdout([]byte(`{"type":"system","subtype":"init","session_id":"abc-123"}` + "\n"))
	mockProc.WriteStdout([]byte(`not valid json` + "\n"))

	model := NewLanguageModel("sonnet", WithProcess(mockProc))

	prompt := []api.Message{
		&api.UserMessage{Content: []api.ContentBlock{&api.TextBlock{Text: "Hello"}}},
	}

	_, err := model.Generate(context.Background(), prompt, api.CallOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse event")
}

func TestLanguageModel_Generate_SkipsUnknownEvents(t *testing.T) {
	mockProc := process.NewMockProcess()
	mockProc.WriteStdout([]byte(`{"type":"system","subtype":"init","session_id":"abc-123"}` + "\n"))
	// Unknown event types should be skipped
	mockProc.WriteStdout([]byte(`{"type":"assistant","content":[{"type":"text","text":"partial"}]}` + "\n"))
	mockProc.WriteStdout([]byte(`{"type":"result","subtype":"success","session_id":"abc-123","message":{"content":[{"type":"text","text":"final"}],"stop_reason":"end_turn"}}` + "\n"))

	model := NewLanguageModel("sonnet", WithProcess(mockProc))

	prompt := []api.Message{
		&api.UserMessage{Content: []api.ContentBlock{&api.TextBlock{Text: "Hi"}}},
	}

	resp, err := model.Generate(context.Background(), prompt, api.CallOptions{})
	require.NoError(t, err)
	require.NotNil(t, resp)

	textBlock, ok := resp.Content[0].(*api.TextBlock)
	require.True(t, ok)
	assert.Equal(t, "final", textBlock.Text)
}

func TestLanguageModel_Generate_EmptyLines(t *testing.T) {
	mockProc := process.NewMockProcess()
	mockProc.WriteStdout([]byte(`{"type":"system","subtype":"init","session_id":"abc-123"}` + "\n"))
	mockProc.WriteStdout([]byte("\n"))
	mockProc.WriteStdout([]byte("\n"))
	mockProc.WriteStdout([]byte(`{"type":"result","subtype":"success","session_id":"abc-123","message":{"content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn"}}` + "\n"))

	model := NewLanguageModel("sonnet", WithProcess(mockProc))

	prompt := []api.Message{
		&api.UserMessage{Content: []api.ContentBlock{&api.TextBlock{Text: "Hi"}}},
	}

	resp, err := model.Generate(context.Background(), prompt, api.CallOptions{})
	require.NoError(t, err)
	require.NotNil(t, resp)
}

// --- Stream edge cases ---

func TestLanguageModel_Stream_CleanEOFWithoutResult(t *testing.T) {
	// Stream ends without a result event - should yield an error event
	mockProc := process.NewMockProcess()
	mockProc.WriteStdout([]byte(`{"type":"system","subtype":"init","session_id":"abc-123"}` + "\n"))
	mockProc.WriteStdout([]byte(`{"type":"stream_event","event":{"type":"message_start","message":{"id":"msg_123","role":"assistant"}}}` + "\n"))
	// EOF - no result event

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

	// Should have metadata event from message_start, but no finish event
	// The stream silently terminates on clean EOF (finding #4)
	hasFinish := false
	for _, e := range events {
		if _, ok := e.(*api.FinishEvent); ok {
			hasFinish = true
		}
	}
	// This documents the current behavior: clean EOF without result does NOT emit FinishEvent
	assert.False(t, hasFinish, "clean EOF without result should not emit FinishEvent (known issue)")
}

func TestLanguageModel_Stream_InvalidJSON(t *testing.T) {
	mockProc := process.NewMockProcess()
	mockProc.WriteStdout([]byte(`{"type":"system","subtype":"init","session_id":"abc-123"}` + "\n"))
	mockProc.WriteStdout([]byte(`not json at all` + "\n"))
	mockProc.WriteStdout([]byte(`{"type":"result","subtype":"success","session_id":"abc-123","usage":{"input_tokens":1,"output_tokens":1}}` + "\n"))

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

	// Should have an error event for the bad JSON followed by the finish event
	hasError := false
	hasFinish := false
	for _, e := range events {
		if _, ok := e.(*api.ErrorEvent); ok {
			hasError = true
		}
		if _, ok := e.(*api.FinishEvent); ok {
			hasFinish = true
		}
	}
	assert.True(t, hasError, "expected error event for invalid JSON")
	assert.True(t, hasFinish, "expected finish event after error recovery")
}

// --- Token tracker integration ---

func TestLanguageModel_TokenUsage_NoTracker(t *testing.T) {
	model := NewLanguageModel("sonnet")

	u := model.TokenUsage()
	assert.Equal(t, 0, u.InputTokens)
	assert.Equal(t, 0, u.OutputTokens)
	assert.Equal(t, 0, u.TotalTokens)
}

func TestLanguageModel_ResetTokenUsage_NoTracker(t *testing.T) {
	model := NewLanguageModel("sonnet")
	// Should not panic
	model.ResetTokenUsage()
}

// --- Warnings tests ---

func TestCallOptionWarnings(t *testing.T) {
	opts := api.CallOptions{
		MaxOutputTokens: 100,
		TopP:            0.9,
		TopK:            50,
		Seed:            42,
	}

	warnings := callOptionWarnings(opts)
	assert.Len(t, warnings, 4)

	var settings []string
	for _, w := range warnings {
		settings = append(settings, w.Setting)
		assert.Equal(t, "unsupported-setting", w.Type)
	}
	assert.Contains(t, settings, "max_output_tokens")
	assert.Contains(t, settings, "top_p")
	assert.Contains(t, settings, "top_k")
	assert.Contains(t, settings, "seed")
}

func TestCallOptionWarnings_NoWarnings(t *testing.T) {
	opts := api.CallOptions{}
	warnings := callOptionWarnings(opts)
	assert.Empty(t, warnings)
}

func TestCallOptionWarnings_TemperatureSupported(t *testing.T) {
	temp := 0.5
	opts := api.CallOptions{Temperature: &temp}
	warnings := callOptionWarnings(opts)
	assert.Empty(t, warnings, "temperature should be supported, no warning")
}

// --- ensureProcess tests ---

func TestLanguageModel_EnsureProcess_ReusesExisting(t *testing.T) {
	mockProc := process.NewMockProcess()
	_ = mockProc.Start(context.Background())

	model := NewLanguageModel("sonnet", WithProcess(mockProc))
	// Set cachedKey to match what buildConfig would produce
	cfg := model.buildConfig("", api.CallOptions{})
	model.cachedKey = cfg.ConfigKey()

	// ensureProcess should reuse the existing process
	err := model.ensureProcess(context.Background(), cfg)
	require.NoError(t, err)
	assert.True(t, mockProc.IsRunning())
}

func TestLanguageModel_WithWorkDir(t *testing.T) {
	model := NewLanguageModel("sonnet", WithWorkDir("/tmp/sandbox"))
	assert.Equal(t, "/tmp/sandbox", model.workDir)
}

func TestLanguageModel_WithAllowedTools(t *testing.T) {
	model := NewLanguageModel("sonnet", WithAllowedTools([]string{"Read", "Bash"}))
	assert.Equal(t, []string{"Read", "Bash"}, model.allowedTools)
}

func TestLanguageModel_BuildConfig_WithWorkDir(t *testing.T) {
	model := NewLanguageModel("sonnet", WithWorkDir("/tmp/test"))
	cfg := model.buildConfig("system", api.CallOptions{})
	assert.Equal(t, "/tmp/test", cfg.WorkDir)
}

func TestLanguageModel_BuildConfig_WithAllowedTools(t *testing.T) {
	tools := []string{"Read", "Bash"}
	model := NewLanguageModel("sonnet", WithAllowedTools(tools))
	cfg := model.buildConfig("system", api.CallOptions{})
	assert.Equal(t, tools, cfg.AllowedTools)
}

func TestLanguageModel_BuildConfig_NoWorkDir(t *testing.T) {
	model := NewLanguageModel("sonnet")
	cfg := model.buildConfig("system", api.CallOptions{})
	assert.Empty(t, cfg.WorkDir)
}

func TestLanguageModel_BuildConfig_NoAllowedTools(t *testing.T) {
	model := NewLanguageModel("sonnet")
	cfg := model.buildConfig("system", api.CallOptions{})
	assert.Nil(t, cfg.AllowedTools)
}
