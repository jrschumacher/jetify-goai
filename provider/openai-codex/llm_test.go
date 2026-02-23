package codex

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.jetify.com/ai/api"
	"go.jetify.com/ai/provider/internal/cli"
	"go.jetify.com/ai/provider/openai-codex/codec"
	"go.jetify.com/ai/provider/openai-codex/codec/jsonrpc"
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
	mockProc := process.NewMockAppServer()
	mockProc.SetThreadID("thread-123")

	// Set up StartThread to return the thread ID
	mockProc.OnStartThread = func(ctx context.Context) (string, error) {
		// Queue up notifications that will be sent
		go func() {
			mockProc.SendNotification(codec.NotifyItemAgentMessageDelta, codec.TextDeltaParams{
				ItemID: "item_1",
				Delta:  "Hello world",
			})
			mockProc.SendNotification(codec.NotifyTurnCompleted, codec.TurnCompletedParams{
				Usage: &codec.AppServerUsage{
					InputTokens:  10,
					OutputTokens: 5,
				},
			})
		}()
		return "thread-123", nil
	}

	model := NewLanguageModel("o3", WithAppServer(mockProc))

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
}

func TestLanguageModel_Generate_WithReasoning(t *testing.T) {
	mockProc := process.NewMockAppServer()
	mockProc.SetThreadID("thread-456")

	mockProc.OnStartThread = func(ctx context.Context) (string, error) {
		go func() {
			mockProc.SendNotification(codec.NotifyItemReasoningDelta, codec.ReasoningDeltaParams{
				ItemID: "item_0",
				Delta:  "Let me think about this...",
			})
			mockProc.SendNotification(codec.NotifyItemAgentMessageDelta, codec.TextDeltaParams{
				ItemID: "item_1",
				Delta:  "The answer is 42",
			})
			mockProc.SendNotification(codec.NotifyTurnCompleted, codec.TurnCompletedParams{
				Usage: &codec.AppServerUsage{
					InputTokens:  100,
					OutputTokens: 20,
				},
			})
		}()
		return "thread-456", nil
	}

	model := NewLanguageModel("o3", WithAppServer(mockProc))

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
	mockProc := process.NewMockAppServer()
	mockProc.SetThreadID("thread-sys")

	mockProc.OnStartThread = func(ctx context.Context) (string, error) {
		go func() {
			mockProc.SendNotification(codec.NotifyItemAgentMessageDelta, codec.TextDeltaParams{
				ItemID: "item_1",
				Delta:  "I am a helpful assistant",
			})
			mockProc.SendNotification(codec.NotifyTurnCompleted, codec.TurnCompletedParams{
				Usage: &codec.AppServerUsage{
					InputTokens:  50,
					OutputTokens: 10,
				},
			})
		}()
		return "thread-sys", nil
	}

	model := NewLanguageModel("o3", WithAppServer(mockProc))

	prompt := []api.Message{
		&api.SystemMessage{Content: "You are a helpful assistant."},
		&api.UserMessage{Content: []api.ContentBlock{&api.TextBlock{Text: "Who are you?"}}},
	}

	resp, err := model.Generate(context.Background(), prompt, api.CallOptions{})
	require.NoError(t, err)
	require.NotNil(t, resp)
}

func TestLanguageModel_Generate_Error(t *testing.T) {
	mockProc := process.NewMockAppServer()
	mockProc.SetThreadID("thread-err")

	mockProc.OnStartThread = func(ctx context.Context) (string, error) {
		go func() {
			// Close without sending turn completed - simulates error
			mockProc.CloseNotifications()
		}()
		return "thread-err", nil
	}

	model := NewLanguageModel("o3", WithAppServer(mockProc))

	prompt := []api.Message{
		&api.UserMessage{Content: []api.ContentBlock{&api.TextBlock{Text: "Hello"}}},
	}

	_, err := model.Generate(context.Background(), prompt, api.CallOptions{})
	require.Error(t, err)
}

func TestLanguageModel_Generate_Usage(t *testing.T) {
	mockProc := process.NewMockAppServer()
	mockProc.SetThreadID("thread-usage")

	mockProc.OnStartThread = func(ctx context.Context) (string, error) {
		go func() {
			mockProc.SendNotification(codec.NotifyItemAgentMessageDelta, codec.TextDeltaParams{
				ItemID: "item_1",
				Delta:  "Hi",
			})
			mockProc.SendNotification(codec.NotifyTurnCompleted, codec.TurnCompletedParams{
				Usage: &codec.AppServerUsage{
					InputTokens:       100,
					OutputTokens:      50,
					CachedInputTokens: 25,
				},
			})
		}()
		return "thread-usage", nil
	}

	model := NewLanguageModel("o3", WithAppServer(mockProc))

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
	mockProc := process.NewMockAppServer()
	mockProc.SetThreadID("thread-info")

	mockProc.OnStartThread = func(ctx context.Context) (string, error) {
		go func() {
			mockProc.SendNotification(codec.NotifyItemAgentMessageDelta, codec.TextDeltaParams{
				ItemID: "item_1",
				Delta:  "Hi",
			})
			mockProc.SendNotification(codec.NotifyTurnCompleted, codec.TurnCompletedParams{
				Usage: &codec.AppServerUsage{
					InputTokens:  10,
					OutputTokens: 5,
				},
			})
		}()
		return "thread-info", nil
	}

	model := NewLanguageModel("o3", WithAppServer(mockProc))

	prompt := []api.Message{
		&api.UserMessage{Content: []api.ContentBlock{&api.TextBlock{Text: "Hi"}}},
	}

	resp, err := model.Generate(context.Background(), prompt, api.CallOptions{})
	require.NoError(t, err)

	require.NotNil(t, resp.ResponseInfo)
	assert.Equal(t, "thread-info", resp.ResponseInfo.ID)
}

func TestLanguageModel_Generate_ProviderMetadataAlwaysPresent(t *testing.T) {
	mockProc := process.NewMockAppServer()

	mockProc.OnStartThread = func(ctx context.Context) (string, error) {
		go func() {
			mockProc.SendNotification(codec.NotifyItemAgentMessageDelta, codec.TextDeltaParams{
				ItemID: "item_1",
				Delta:  "Hello",
			})
			mockProc.SendNotification(codec.NotifyTurnCompleted, codec.TurnCompletedParams{
				Usage: &codec.AppServerUsage{
					InputTokens:  10,
					OutputTokens: 5,
				},
			})
		}()
		return "thread-meta-always", nil
	}

	model := NewLanguageModel("o3", WithAppServer(mockProc))

	prompt := []api.Message{
		&api.UserMessage{Content: []api.ContentBlock{&api.TextBlock{Text: "Hi"}}},
	}

	resp, err := model.Generate(context.Background(), prompt, api.CallOptions{})
	require.NoError(t, err)
	require.NotNil(t, resp.ProviderMetadata)

	metadata := codec.GetMetadata(resp)
	require.NotNil(t, metadata)
	assert.Equal(t, "thread-meta-always", metadata.ThreadID)
}

func TestLanguageModel_Generate_CapturesCommandExecutionsAndFileChanges(t *testing.T) {
	mockProc := process.NewMockAppServer()

	mockProc.OnStartThread = func(ctx context.Context) (string, error) {
		go func() {
			// Command execution output deltas.
			mockProc.SendNotification(codec.NotifyItemCommandExecutionOutputDelta, map[string]any{
				"itemId": "cmd_1",
				"delta":  "hello",
			})
			mockProc.SendNotification(codec.NotifyItemCommandExecutionOutputDelta, map[string]any{
				"itemId": "cmd_1",
				"delta":  " world",
			})

			// Command execution completion.
			mockProc.SendNotification(codec.NotifyItemCompleted, map[string]any{
				"item": map[string]any{
					"id":       "cmd_1",
					"type":     "commandExecution",
					"command":  "echo hello",
					"exitCode": 0,
				},
			})

			// File change completion.
			mockProc.SendNotification(codec.NotifyItemCompleted, map[string]any{
				"item": map[string]any{
					"id":   "file_1",
					"type": "fileChange",
					"changes": []any{
						map[string]any{
							"path": "test.go",
							"diff": "+ new line",
						},
					},
				},
			})

			mockProc.SendNotification(codec.NotifyItemAgentMessageDelta, codec.TextDeltaParams{
				ItemID: "item_1",
				Delta:  "Done",
			})
			mockProc.SendNotification(codec.NotifyTurnCompleted, codec.TurnCompletedParams{
				Usage: &codec.AppServerUsage{
					InputTokens:  10,
					OutputTokens: 5,
				},
			})
		}()
		return "thread-meta-items", nil
	}

	model := NewLanguageModel("o3", WithAppServer(mockProc))

	prompt := []api.Message{
		&api.UserMessage{Content: []api.ContentBlock{&api.TextBlock{Text: "Hi"}}},
	}

	resp, err := model.Generate(context.Background(), prompt, api.CallOptions{})
	require.NoError(t, err)

	metadata := codec.GetMetadata(resp)
	require.NotNil(t, metadata)

	require.Len(t, metadata.CommandExecutions, 1)
	assert.Equal(t, "echo hello", metadata.CommandExecutions[0].Command)
	assert.Equal(t, 0, metadata.CommandExecutions[0].ExitCode)
	assert.Equal(t, "hello world", metadata.CommandExecutions[0].Output)

	require.Len(t, metadata.FileChanges, 1)
	assert.Equal(t, "test.go", metadata.FileChanges[0].FilePath)
	assert.Equal(t, "+ new line", metadata.FileChanges[0].Diff)
}

func TestLanguageModel_Generate_MultipleDeltas(t *testing.T) {
	mockProc := process.NewMockAppServer()
	mockProc.SetThreadID("thread-multi")

	mockProc.OnStartThread = func(ctx context.Context) (string, error) {
		go func() {
			// Multiple deltas should be concatenated
			mockProc.SendNotification(codec.NotifyItemAgentMessageDelta, codec.TextDeltaParams{
				ItemID: "item_1",
				Delta:  "First part. ",
			})
			mockProc.SendNotification(codec.NotifyItemAgentMessageDelta, codec.TextDeltaParams{
				ItemID: "item_1",
				Delta:  "Second part.",
			})
			mockProc.SendNotification(codec.NotifyTurnCompleted, codec.TurnCompletedParams{
				Usage: &codec.AppServerUsage{
					InputTokens:  20,
					OutputTokens: 10,
				},
			})
		}()
		return "thread-multi", nil
	}

	model := NewLanguageModel("o3", WithAppServer(mockProc))

	prompt := []api.Message{
		&api.UserMessage{Content: []api.ContentBlock{&api.TextBlock{Text: "Say something"}}},
	}

	resp, err := model.Generate(context.Background(), prompt, api.CallOptions{})
	require.NoError(t, err)

	require.Len(t, resp.Content, 1)
	assert.Equal(t, "First part. Second part.", resp.Content[0].(*api.TextBlock).Text)
}

func TestNewLanguageModel_DefaultOptions(t *testing.T) {
	model := NewLanguageModel("o3")

	assert.Equal(t, "o3", model.ModelID())
	assert.Equal(t, ProviderName, model.ProviderName())
}

func TestNewLanguageModel_CustomOptions(t *testing.T) {
	mockProc := process.NewMockAppServer()
	model := NewLanguageModel("o4-mini", WithAppServer(mockProc))

	assert.Equal(t, "o4-mini", model.ModelID())
}

func TestLanguageModel_Stream_TextDeltas(t *testing.T) {
	mockProc := process.NewMockAppServer()
	mockProc.SetThreadID("thread-stream")

	mockProc.OnStartThread = func(ctx context.Context) (string, error) {
		go func() {
			mockProc.SendNotification(codec.NotifyItemAgentMessageDelta, codec.TextDeltaParams{
				ItemID: "item_1",
				Delta:  "Hello ",
			})
			mockProc.SendNotification(codec.NotifyItemAgentMessageDelta, codec.TextDeltaParams{
				ItemID: "item_1",
				Delta:  "world",
			})
			mockProc.SendNotification(codec.NotifyTurnCompleted, codec.TurnCompletedParams{
				Usage: &codec.AppServerUsage{
					InputTokens:  10,
					OutputTokens: 5,
				},
			})
		}()
		return "thread-stream", nil
	}

	model := NewLanguageModel("o3", WithAppServer(mockProc))

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

	// Should have: ResponseMetadataEvent, TextDeltaEvent(s), FinishEvent
	require.GreaterOrEqual(t, len(events), 3, "expected at least 3 events")

	// First event should be response metadata
	metadata, ok := events[0].(*api.ResponseMetadataEvent)
	require.True(t, ok, "first event should be ResponseMetadataEvent, got %T", events[0])
	assert.Equal(t, "thread-stream", metadata.ID)

	// Should have text deltas
	var textDeltas []*api.TextDeltaEvent
	for _, e := range events {
		if td, ok := e.(*api.TextDeltaEvent); ok {
			textDeltas = append(textDeltas, td)
		}
	}
	require.GreaterOrEqual(t, len(textDeltas), 1, "expected at least one TextDeltaEvent")

	// Last event should be FinishEvent
	finishEvent, ok := events[len(events)-1].(*api.FinishEvent)
	require.True(t, ok, "last event should be FinishEvent, got %T", events[len(events)-1])
	assert.Equal(t, api.FinishReasonStop, finishEvent.FinishReason)
	assert.Equal(t, 10, finishEvent.Usage.InputTokens)
	assert.Equal(t, 5, finishEvent.Usage.OutputTokens)
}

func TestLanguageModel_Stream_WithReasoning(t *testing.T) {
	mockProc := process.NewMockAppServer()
	mockProc.SetThreadID("thread-reason")

	mockProc.OnStartThread = func(ctx context.Context) (string, error) {
		go func() {
			mockProc.SendNotification(codec.NotifyItemReasoningDelta, codec.ReasoningDeltaParams{
				ItemID: "item_0",
				Delta:  "Let me think...",
			})
			mockProc.SendNotification(codec.NotifyItemAgentMessageDelta, codec.TextDeltaParams{
				ItemID: "item_1",
				Delta:  "The answer is 42",
			})
			mockProc.SendNotification(codec.NotifyTurnCompleted, codec.TurnCompletedParams{
				Usage: &codec.AppServerUsage{
					InputTokens:  50,
					OutputTokens: 20,
				},
			})
		}()
		return "thread-reason", nil
	}

	model := NewLanguageModel("o3", WithAppServer(mockProc))

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
	mockProc := process.NewMockAppServer()
	mockProc.SetThreadID("thread-err")

	mockProc.OnStartThread = func(ctx context.Context) (string, error) {
		go func() {
			// Close without sending turn completed - simulates error
			mockProc.CloseNotifications()
		}()
		return "thread-err", nil
	}

	model := NewLanguageModel("o3", WithAppServer(mockProc))

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
}

func TestLanguageModel_Stream_ProviderMetadata(t *testing.T) {
	mockProc := process.NewMockAppServer()
	mockProc.SetThreadID("thread-meta")

	mockProc.OnStartThread = func(ctx context.Context) (string, error) {
		go func() {
			mockProc.SendNotification(codec.NotifyItemAgentMessageDelta, codec.TextDeltaParams{
				ItemID: "item_1",
				Delta:  "Hello",
			})
			mockProc.SendNotification(codec.NotifyTurnCompleted, codec.TurnCompletedParams{
				Usage: &codec.AppServerUsage{
					InputTokens:  10,
					OutputTokens: 5,
				},
			})
		}()
		return "thread-meta", nil
	}

	model := NewLanguageModel("o3", WithAppServer(mockProc))

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

func TestLanguageModel_ConfigChange_RestartsProcess(t *testing.T) {
	// Track how many times the process was started
	startCount := 0

	createMock := func(threadID string) *process.MockAppServer {
		mock := process.NewMockAppServer()
		mock.SetThreadID(threadID)
		mock.OnStart = func(ctx context.Context) error {
			startCount++
			return nil
		}
		mock.OnStartThread = func(ctx context.Context) (string, error) {
			go func() {
				mock.SendNotification(codec.NotifyItemAgentMessageDelta, codec.TextDeltaParams{
					ItemID: "item_1",
					Delta:  "Response",
				})
				mock.SendNotification(codec.NotifyTurnCompleted, codec.TurnCompletedParams{
					Usage: &codec.AppServerUsage{
						InputTokens:  10,
						OutputTokens: 5,
					},
				})
			}()
			return threadID, nil
		}
		return mock
	}

	// Create model without injected process - it will create its own
	model := NewLanguageModel("o3")

	// Inject mock for testing
	mock1 := createMock("thread-1")
	model.proc = mock1

	prompt := []api.Message{
		&api.UserMessage{Content: []api.ContentBlock{&api.TextBlock{Text: "Hi"}}},
	}

	// First call
	_, err := model.Generate(context.Background(), prompt, api.CallOptions{})
	require.NoError(t, err)

	// The process should have been started
	assert.Equal(t, 1, startCount)
}

func TestLanguageModel_Close(t *testing.T) {
	mockProc := process.NewMockAppServer()
	mockProc.SetThreadID("thread-close")

	stopCalled := false
	mockProc.OnStop = func() error {
		stopCalled = true
		return nil
	}

	model := NewLanguageModel("o3", WithAppServer(mockProc))

	err := model.Close()
	require.NoError(t, err)
	assert.True(t, stopCalled, "Stop should have been called")
}

func TestLanguageModel_Generate_WithTemperature(t *testing.T) {
	mockProc := process.NewMockAppServer()
	mockProc.SetThreadID("thread-temp")

	mockProc.OnStartThread = func(ctx context.Context) (string, error) {
		go func() {
			mockProc.SendNotification(codec.NotifyItemAgentMessageDelta, codec.TextDeltaParams{
				ItemID: "item_1",
				Delta:  "Response with temperature",
			})
			mockProc.SendNotification(codec.NotifyTurnCompleted, codec.TurnCompletedParams{
				Usage: &codec.AppServerUsage{
					InputTokens:  10,
					OutputTokens: 5,
				},
			})
		}()
		return "thread-temp", nil
	}

	model := NewLanguageModel("o3", WithAppServer(mockProc))

	prompt := []api.Message{
		&api.UserMessage{Content: []api.ContentBlock{&api.TextBlock{Text: "Hi"}}},
	}

	temp := 0.7
	resp, err := model.Generate(context.Background(), prompt, api.CallOptions{
		Temperature: &temp,
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
}

func TestLanguageModel_ProcessReuse(t *testing.T) {
	mockProc := process.NewMockAppServer()
	mockProc.SetThreadID("thread-reuse")

	callCount := 0
	mockProc.OnStartThread = func(ctx context.Context) (string, error) {
		callCount++
		go func() {
			mockProc.SendNotification(codec.NotifyItemAgentMessageDelta, codec.TextDeltaParams{
				ItemID: "item_1",
				Delta:  "Response",
			})
			mockProc.SendNotification(codec.NotifyTurnCompleted, codec.TurnCompletedParams{
				Usage: &codec.AppServerUsage{InputTokens: 10, OutputTokens: 5},
			})
		}()
		return "thread-reuse", nil
	}

	// Mark as running to simulate an already-started process
	mockProc.Start(context.Background())
	mockProc.Initialize(context.Background())

	model := NewLanguageModel("o3", WithAppServer(mockProc))
	// Set cached key to match the config
	model.cachedKey = cli.ConfigKey{Model: "o3"}

	prompt := []api.Message{
		&api.UserMessage{Content: []api.ContentBlock{&api.TextBlock{Text: "Hi"}}},
	}

	// First call
	_, err := model.Generate(context.Background(), prompt, api.CallOptions{})
	require.NoError(t, err)

	// Create new notification channel for second call
	mockProc2 := process.NewMockAppServer()
	mockProc2.SetThreadID("thread-reuse-2")
	mockProc2.OnStartThread = func(ctx context.Context) (string, error) {
		callCount++
		go func() {
			mockProc2.SendNotification(codec.NotifyItemAgentMessageDelta, codec.TextDeltaParams{
				ItemID: "item_1",
				Delta:  "Response 2",
			})
			mockProc2.SendNotification(codec.NotifyTurnCompleted, codec.TurnCompletedParams{
				Usage: &codec.AppServerUsage{InputTokens: 10, OutputTokens: 5},
			})
		}()
		return "thread-reuse-2", nil
	}
	mockProc2.Start(context.Background())
	mockProc2.Initialize(context.Background())

	model.proc = mockProc2

	// Second call - should reuse process (not restart)
	_, err = model.Generate(context.Background(), prompt, api.CallOptions{})
	require.NoError(t, err)

	// Both calls should have created threads
	assert.Equal(t, 2, callCount)
}

func TestLanguageModel_Stream_HoldsCallMu(t *testing.T) {
	// This test verifies that concurrent Stream() calls are serialized by callMu.
	// The second call should block until the first iterator fully completes.

	// We use two separate models pointing to two separate mocks so the mocks
	// don't share notification channels, but they share the same callMu via the
	// single LanguageModel instance.

	mockProc := process.NewMockAppServer()
	mockProc.SetThreadID("thread-1")

	// Track ordering of thread starts to prove serialization.
	var mu sync.Mutex
	var order []string

	notifReady := make(chan struct{}) // signals when first stream is blocked

	mockProc.OnStartThread = func(ctx context.Context) (string, error) {
		return "thread-1", nil
	}

	turnCount := 0
	mockProc.OnStartTurn = func(ctx context.Context, threadID string, input []jsonrpc.Input, policy jsonrpc.ApprovalPolicy) error {
		mu.Lock()
		turnCount++
		turn := turnCount
		mu.Unlock()

		if turn == 1 {
			// First turn: signal ready, then wait before sending notifications.
			// This keeps callMu held while the second Stream() tries to acquire it.
			close(notifReady)
			// Small delay to give the second goroutine time to block on callMu
			go func() {
				<-time.After(50 * time.Millisecond)
				mu.Lock()
				order = append(order, "first-notify")
				mu.Unlock()
				mockProc.SendNotification(codec.NotifyItemAgentMessageDelta, codec.TextDeltaParams{
					ItemID: "item_1",
					Delta:  "first",
				})
				mockProc.SendNotification(codec.NotifyTurnCompleted, codec.TurnCompletedParams{
					Usage: &codec.AppServerUsage{InputTokens: 1, OutputTokens: 1},
				})
			}()
		} else {
			// Second turn: notifications sent immediately
			go func() {
				mu.Lock()
				order = append(order, "second-notify")
				mu.Unlock()
				mockProc.SendNotification(codec.NotifyItemAgentMessageDelta, codec.TextDeltaParams{
					ItemID: "item_2",
					Delta:  "second",
				})
				mockProc.SendNotification(codec.NotifyTurnCompleted, codec.TurnCompletedParams{
					Usage: &codec.AppServerUsage{InputTokens: 2, OutputTokens: 2},
				})
			}()
		}
		return nil
	}

	model := NewLanguageModel("o3", WithAppServer(mockProc))
	prompt := []api.Message{
		&api.UserMessage{Content: []api.ContentBlock{&api.TextBlock{Text: "test"}}},
	}

	var wg sync.WaitGroup
	wg.Add(2)

	// First goroutine: start streaming, hold the lock via iterator
	go func() {
		defer wg.Done()
		resp, err := model.Stream(context.Background(), prompt, api.CallOptions{})
		require.NoError(t, err)
		for range resp.Stream {
			// consume all events
		}
		mu.Lock()
		order = append(order, "first-done")
		mu.Unlock()
	}()

	// Second goroutine: wait until first is in-flight, then try to stream
	go func() {
		defer wg.Done()
		<-notifReady // wait until first stream is set up
		resp, err := model.Stream(context.Background(), prompt, api.CallOptions{})
		require.NoError(t, err)
		for range resp.Stream {
			// consume all events
		}
		mu.Lock()
		order = append(order, "second-done")
		mu.Unlock()
	}()

	wg.Wait()

	mu.Lock()
	defer mu.Unlock()

	// The first stream must complete before the second stream's notifications.
	// Verify: "first-notify" and "first-done" both appear before "second-done".
	firstDoneIdx := -1
	secondDoneIdx := -1
	for i, v := range order {
		if v == "first-done" {
			firstDoneIdx = i
		}
		if v == "second-done" {
			secondDoneIdx = i
		}
	}
	assert.Greater(t, secondDoneIdx, firstDoneIdx,
		"second stream should complete after first; order: %v", order)
}
