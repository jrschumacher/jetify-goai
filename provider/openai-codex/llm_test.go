package codex

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.jetify.com/ai/api"
	"go.jetify.com/ai/provider/openai-codex/codec"
	"go.jetify.com/ai/provider/openai-codex/process"
)

func TestLanguageModel_ProviderName(t *testing.T) {
	model := NewLanguageModel("o3")
	assert.Equal(t, ProviderName, model.ProviderName())
}

func TestLanguageModel_ModelID(t *testing.T) {
	model := NewLanguageModel("o4-mini")
	assert.Equal(t, "o4-mini", model.ModelID())
}

func TestLanguageModel_SupportedUrls(t *testing.T) {
	model := NewLanguageModel("o3")
	urls := model.SupportedUrls()

	// Codex CLI doesn't support direct URL loading
	assert.Empty(t, urls)
}

func TestLanguageModel_Generate_TextResponse(t *testing.T) {
	mockProc := process.NewMockProcess()

	// Simulate CLI output
	mockProc.WriteStdoutLine([]byte(`{"type":"thread.started","thread_id":"thread-123"}`))
	mockProc.WriteStdoutLine([]byte(`{"type":"turn.started"}`))
	mockProc.WriteStdoutLine([]byte(`{"type":"item.completed","item":{"id":"item_1","type":"agent_message","text":"Hello world"}}`))
	mockProc.WriteStdoutLine([]byte(`{"type":"turn.completed","usage":{"input_tokens":10,"output_tokens":5}}`))

	model := NewLanguageModel("o3", WithProcess(mockProc))

	prompt := []api.Message{
		&api.UserMessage{Content: []api.ContentBlock{&api.TextBlock{Text: "Say hello"}}},
	}

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

func TestLanguageModel_Generate_WithReasoning(t *testing.T) {
	mockProc := process.NewMockProcess()

	mockProc.WriteStdoutLine([]byte(`{"type":"thread.started","thread_id":"thread-456"}`))
	mockProc.WriteStdoutLine([]byte(`{"type":"turn.started"}`))
	mockProc.WriteStdoutLine([]byte(`{"type":"item.completed","item":{"id":"item_0","type":"reasoning","text":"Let me think about this..."}}`))
	mockProc.WriteStdoutLine([]byte(`{"type":"item.completed","item":{"id":"item_1","type":"agent_message","text":"The answer is 42"}}`))
	mockProc.WriteStdoutLine([]byte(`{"type":"turn.completed","usage":{"input_tokens":100,"output_tokens":20}}`))

	model := NewLanguageModel("o3", WithProcess(mockProc))

	prompt := []api.Message{
		&api.UserMessage{Content: []api.ContentBlock{&api.TextBlock{Text: "What is the meaning of life?"}}},
	}

	resp, err := model.Generate(context.Background(), prompt, api.CallOptions{})
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Verify response content
	require.Len(t, resp.Content, 1)
	textBlock, ok := resp.Content[0].(*api.TextBlock)
	require.True(t, ok)
	assert.Equal(t, "The answer is 42", textBlock.Text)

	// Verify reasoning is in metadata
	metadata := codec.GetMetadata(resp)
	require.NotNil(t, metadata)
	assert.Equal(t, "Let me think about this...", metadata.Reasoning)
}

func TestLanguageModel_Generate_WithSystemPrompt(t *testing.T) {
	mockProc := process.NewMockProcess()

	mockProc.WriteStdoutLine([]byte(`{"type":"thread.started","thread_id":"thread-sys"}`))
	mockProc.WriteStdoutLine([]byte(`{"type":"turn.started"}`))
	mockProc.WriteStdoutLine([]byte(`{"type":"item.completed","item":{"id":"item_1","type":"agent_message","text":"I am a helpful assistant"}}`))
	mockProc.WriteStdoutLine([]byte(`{"type":"turn.completed","usage":{"input_tokens":50,"output_tokens":10}}`))

	model := NewLanguageModel("o3", WithProcess(mockProc))

	prompt := []api.Message{
		&api.SystemMessage{Content: "You are a helpful assistant."},
		&api.UserMessage{Content: []api.ContentBlock{&api.TextBlock{Text: "Who are you?"}}},
	}

	resp, err := model.Generate(context.Background(), prompt, api.CallOptions{})
	require.NoError(t, err)
	require.NotNil(t, resp)
}

func TestLanguageModel_Generate_TurnFailed(t *testing.T) {
	mockProc := process.NewMockProcess()

	mockProc.WriteStdoutLine([]byte(`{"type":"thread.started","thread_id":"thread-err"}`))
	mockProc.WriteStdoutLine([]byte(`{"type":"turn.started"}`))
	mockProc.WriteStdoutLine([]byte(`{"type":"turn.failed","error":{"message":"Model not available"}}`))

	model := NewLanguageModel("o3", WithProcess(mockProc))

	prompt := []api.Message{
		&api.UserMessage{Content: []api.ContentBlock{&api.TextBlock{Text: "Hello"}}},
	}

	_, err := model.Generate(context.Background(), prompt, api.CallOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Model not available")
}

func TestLanguageModel_Generate_Error(t *testing.T) {
	mockProc := process.NewMockProcess()

	mockProc.WriteStdoutLine([]byte(`{"type":"error","message":"Connection lost"}`))

	model := NewLanguageModel("o3", WithProcess(mockProc))

	prompt := []api.Message{
		&api.UserMessage{Content: []api.ContentBlock{&api.TextBlock{Text: "Hello"}}},
	}

	_, err := model.Generate(context.Background(), prompt, api.CallOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Connection lost")
}

func TestLanguageModel_Generate_Usage(t *testing.T) {
	mockProc := process.NewMockProcess()

	mockProc.WriteStdoutLine([]byte(`{"type":"thread.started","thread_id":"thread-usage"}`))
	mockProc.WriteStdoutLine([]byte(`{"type":"turn.started"}`))
	mockProc.WriteStdoutLine([]byte(`{"type":"item.completed","item":{"id":"item_1","type":"agent_message","text":"Hi"}}`))
	mockProc.WriteStdoutLine([]byte(`{"type":"turn.completed","usage":{"input_tokens":100,"output_tokens":50,"cached_input_tokens":25}}`))

	model := NewLanguageModel("o3", WithProcess(mockProc))

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

	mockProc.WriteStdoutLine([]byte(`{"type":"thread.started","thread_id":"thread-info"}`))
	mockProc.WriteStdoutLine([]byte(`{"type":"turn.started"}`))
	mockProc.WriteStdoutLine([]byte(`{"type":"item.completed","item":{"id":"item_1","type":"agent_message","text":"Hi"}}`))
	mockProc.WriteStdoutLine([]byte(`{"type":"turn.completed","usage":{"input_tokens":10,"output_tokens":5}}`))

	model := NewLanguageModel("o3", WithProcess(mockProc))

	prompt := []api.Message{
		&api.UserMessage{Content: []api.ContentBlock{&api.TextBlock{Text: "Hi"}}},
	}

	resp, err := model.Generate(context.Background(), prompt, api.CallOptions{})
	require.NoError(t, err)

	require.NotNil(t, resp.ResponseInfo)
	assert.Equal(t, "thread-info", resp.ResponseInfo.ID)
}

func TestLanguageModel_Generate_MultipleMessages(t *testing.T) {
	mockProc := process.NewMockProcess()

	mockProc.WriteStdoutLine([]byte(`{"type":"thread.started","thread_id":"thread-multi"}`))
	mockProc.WriteStdoutLine([]byte(`{"type":"turn.started"}`))
	mockProc.WriteStdoutLine([]byte(`{"type":"item.completed","item":{"id":"item_1","type":"agent_message","text":"First part. "}}`))
	mockProc.WriteStdoutLine([]byte(`{"type":"item.completed","item":{"id":"item_2","type":"agent_message","text":"Second part."}}`))
	mockProc.WriteStdoutLine([]byte(`{"type":"turn.completed","usage":{"input_tokens":20,"output_tokens":10}}`))

	model := NewLanguageModel("o3", WithProcess(mockProc))

	prompt := []api.Message{
		&api.UserMessage{Content: []api.ContentBlock{&api.TextBlock{Text: "Say something"}}},
	}

	resp, err := model.Generate(context.Background(), prompt, api.CallOptions{})
	require.NoError(t, err)

	require.Len(t, resp.Content, 2)
	assert.Equal(t, "First part. ", resp.Content[0].(*api.TextBlock).Text)
	assert.Equal(t, "Second part.", resp.Content[1].(*api.TextBlock).Text)
}

func TestNewLanguageModel_DefaultOptions(t *testing.T) {
	model := NewLanguageModel("o3")

	assert.Equal(t, "o3", model.ModelID())
	assert.Equal(t, ProviderName, model.ProviderName())
}

func TestNewLanguageModel_CustomOptions(t *testing.T) {
	mockProc := process.NewMockProcess()
	model := NewLanguageModel("o4-mini", WithProcess(mockProc))

	assert.Equal(t, "o4-mini", model.ModelID())
}

func TestLanguageModel_Stream_TextDeltas(t *testing.T) {
	mockProc := process.NewMockProcess()

	// Simulate CLI streaming output
	mockProc.WriteStdoutLine([]byte(`{"type":"thread.started","thread_id":"thread-stream"}`))
	mockProc.WriteStdoutLine([]byte(`{"type":"turn.started"}`))
	mockProc.WriteStdoutLine([]byte(`{"type":"item.completed","item":{"id":"item_1","type":"agent_message","text":"Hello world"}}`))
	mockProc.WriteStdoutLine([]byte(`{"type":"turn.completed","usage":{"input_tokens":10,"output_tokens":5}}`))

	model := NewLanguageModel("o3", WithProcess(mockProc))

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

	// Should have: ResponseMetadataEvent, TextDeltaEvent, FinishEvent
	require.GreaterOrEqual(t, len(events), 3, "expected at least 3 events")

	// First event should be response metadata
	metadata, ok := events[0].(*api.ResponseMetadataEvent)
	require.True(t, ok, "first event should be ResponseMetadataEvent, got %T", events[0])
	assert.Equal(t, "thread-stream", metadata.ID)

	// Should have text delta
	var textDelta *api.TextDeltaEvent
	for _, e := range events {
		if td, ok := e.(*api.TextDeltaEvent); ok {
			textDelta = td
			break
		}
	}
	require.NotNil(t, textDelta, "expected TextDeltaEvent")
	assert.Equal(t, "Hello world", textDelta.TextDelta)

	// Last event should be FinishEvent
	finishEvent, ok := events[len(events)-1].(*api.FinishEvent)
	require.True(t, ok, "last event should be FinishEvent, got %T", events[len(events)-1])
	assert.Equal(t, api.FinishReasonStop, finishEvent.FinishReason)
	assert.Equal(t, 10, finishEvent.Usage.InputTokens)
	assert.Equal(t, 5, finishEvent.Usage.OutputTokens)
}

func TestLanguageModel_Stream_WithReasoning(t *testing.T) {
	mockProc := process.NewMockProcess()

	mockProc.WriteStdoutLine([]byte(`{"type":"thread.started","thread_id":"thread-reason"}`))
	mockProc.WriteStdoutLine([]byte(`{"type":"turn.started"}`))
	mockProc.WriteStdoutLine([]byte(`{"type":"item.completed","item":{"id":"item_0","type":"reasoning","text":"Let me think..."}}`))
	mockProc.WriteStdoutLine([]byte(`{"type":"item.completed","item":{"id":"item_1","type":"agent_message","text":"The answer is 42"}}`))
	mockProc.WriteStdoutLine([]byte(`{"type":"turn.completed","usage":{"input_tokens":50,"output_tokens":20}}`))

	model := NewLanguageModel("o3", WithProcess(mockProc))

	prompt := []api.Message{
		&api.UserMessage{Content: []api.ContentBlock{&api.TextBlock{Text: "Question"}}},
	}

	streamResp, err := model.Stream(context.Background(), prompt, api.CallOptions{})
	require.NoError(t, err)

	var events []api.StreamEvent
	for event := range streamResp.Stream {
		events = append(events, event)
	}

	// Should have reasoning event
	var reasoningEvent *api.ReasoningEvent
	for _, e := range events {
		if re, ok := e.(*api.ReasoningEvent); ok {
			reasoningEvent = re
			break
		}
	}
	require.NotNil(t, reasoningEvent, "expected ReasoningEvent")
	assert.Equal(t, "Let me think...", reasoningEvent.TextDelta)

	// Should have text delta
	var textDelta *api.TextDeltaEvent
	for _, e := range events {
		if td, ok := e.(*api.TextDeltaEvent); ok {
			textDelta = td
			break
		}
	}
	require.NotNil(t, textDelta, "expected TextDeltaEvent")
	assert.Equal(t, "The answer is 42", textDelta.TextDelta)
}

func TestLanguageModel_Stream_Error(t *testing.T) {
	mockProc := process.NewMockProcess()

	mockProc.WriteStdoutLine([]byte(`{"type":"thread.started","thread_id":"thread-err"}`))
	mockProc.WriteStdoutLine([]byte(`{"type":"turn.failed","error":{"message":"API rate limit exceeded"}}`))

	model := NewLanguageModel("o3", WithProcess(mockProc))

	prompt := []api.Message{
		&api.UserMessage{Content: []api.ContentBlock{&api.TextBlock{Text: "Hello"}}},
	}

	streamResp, err := model.Stream(context.Background(), prompt, api.CallOptions{})
	require.NoError(t, err)

	var events []api.StreamEvent
	for event := range streamResp.Stream {
		events = append(events, event)
	}

	// Find the error event
	var errorEvent *api.ErrorEvent
	for _, e := range events {
		if ee, ok := e.(*api.ErrorEvent); ok {
			errorEvent = ee
			break
		}
	}
	require.NotNil(t, errorEvent, "expected ErrorEvent")
	errVal, ok := errorEvent.Err.(error)
	require.True(t, ok, "expected error type")
	assert.Contains(t, errVal.Error(), "API rate limit exceeded")
}

func TestLanguageModel_Stream_ProviderMetadata(t *testing.T) {
	mockProc := process.NewMockProcess()

	mockProc.WriteStdoutLine([]byte(`{"type":"thread.started","thread_id":"thread-meta"}`))
	mockProc.WriteStdoutLine([]byte(`{"type":"turn.started"}`))
	mockProc.WriteStdoutLine([]byte(`{"type":"item.completed","item":{"id":"item_1","type":"agent_message","text":"Hello"}}`))
	mockProc.WriteStdoutLine([]byte(`{"type":"turn.completed","usage":{"input_tokens":10,"output_tokens":5}}`))

	model := NewLanguageModel("o3", WithProcess(mockProc))

	prompt := []api.Message{
		&api.UserMessage{Content: []api.ContentBlock{&api.TextBlock{Text: "Hi"}}},
	}

	streamResp, err := model.Stream(context.Background(), prompt, api.CallOptions{})
	require.NoError(t, err)

	var events []api.StreamEvent
	for event := range streamResp.Stream {
		events = append(events, event)
	}

	// Find the finish event and check metadata
	var finishEvent *api.FinishEvent
	for _, e := range events {
		if fe, ok := e.(*api.FinishEvent); ok {
			finishEvent = fe
			break
		}
	}
	require.NotNil(t, finishEvent, "expected FinishEvent")
	require.NotNil(t, finishEvent.ProviderMetadata)

	metadata := codec.GetMetadata(finishEvent)
	require.NotNil(t, metadata)
	assert.Equal(t, "thread-meta", metadata.ThreadID)
}
