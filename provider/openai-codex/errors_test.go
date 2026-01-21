package codex

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.jetify.com/ai/api"
	"go.jetify.com/ai/provider/openai-codex/codec/jsonrpc"
	"go.jetify.com/ai/provider/openai-codex/process"
)

func TestClassifyError_QuotaExceeded(t *testing.T) {
	mockProc := process.NewMockAppServer()
	mockProc.SimulateQuotaExceeded()

	model := NewLanguageModel("gpt-5.2-codex-max", WithAppServer(mockProc))

	prompt := []api.Message{
		&api.UserMessage{
			Content: []api.ContentBlock{
				&api.TextBlock{Text: "test"},
			},
		},
	}

	_, err := model.Generate(context.Background(), prompt, api.CallOptions{})

	require.Error(t, err)

	// Check that error was classified
	errInfo, ok := err.(*ErrorInfo)
	require.True(t, ok, "expected ErrorInfo, got %T", err)

	assert.Equal(t, CategoryQuotaExceeded, errInfo.Category)
	assert.Contains(t, strings.ToLower(errInfo.UserMessage), "quota")
	assert.Contains(t, errInfo.SuggestedAction, "platform.openai.com")
	assert.False(t, errInfo.Retryable)
}

func TestClassifyError_RateLimit(t *testing.T) {
	mockProc := process.NewMockAppServer()
	mockProc.SimulateRateLimit()

	model := NewLanguageModel("gpt-5.2-codex-max", WithAppServer(mockProc))

	prompt := []api.Message{
		&api.UserMessage{
			Content: []api.ContentBlock{
				&api.TextBlock{Text: "test"},
			},
		},
	}

	// Set thread ID so StartThread succeeds
	mockProc.SetThreadID("thread-1")
	mockProc.OnStartThread = func(ctx context.Context) (string, error) {
		return "thread-1", nil
	}

	_, err := model.Generate(context.Background(), prompt, api.CallOptions{})

	require.Error(t, err)

	errInfo, ok := err.(*ErrorInfo)
	require.True(t, ok)

	assert.Equal(t, CategoryRateLimited, errInfo.Category)
	assert.Contains(t, strings.ToLower(errInfo.UserMessage), "rate limit")
	assert.True(t, errInfo.Retryable)
	assert.Greater(t, errInfo.RetryAfter, time.Duration(0))
}

func TestClassifyError_AuthenticationFailed(t *testing.T) {
	mockProc := process.NewMockAppServer()
	mockProc.SimulateAuthFailure()

	model := NewLanguageModel("gpt-5.2-codex-max", WithAppServer(mockProc))

	prompt := []api.Message{
		&api.UserMessage{
			Content: []api.ContentBlock{
				&api.TextBlock{Text: "test"},
			},
		},
	}

	_, err := model.Generate(context.Background(), prompt, api.CallOptions{})

	require.Error(t, err)

	errInfo, ok := err.(*ErrorInfo)
	require.True(t, ok)

	assert.Equal(t, CategoryAuthenticationFailed, errInfo.Category)
	assert.Contains(t, errInfo.SuggestedAction, "codex login")
	assert.False(t, errInfo.Retryable)
}

func TestClassifyError_ServiceUnavailable(t *testing.T) {
	mockProc := process.NewMockAppServer()
	mockProc.SimulateServiceUnavailable()

	model := NewLanguageModel("gpt-5.2-codex-max", WithAppServer(mockProc))

	prompt := []api.Message{
		&api.UserMessage{
			Content: []api.ContentBlock{
				&api.TextBlock{Text: "test"},
			},
		},
	}

	_, err := model.Generate(context.Background(), prompt, api.CallOptions{})

	require.Error(t, err)

	errInfo, ok := err.(*ErrorInfo)
	require.True(t, ok)

	assert.Equal(t, CategoryServiceUnavailable, errInfo.Category)
	assert.True(t, errInfo.Retryable)
	assert.Greater(t, errInfo.RetryAfter, time.Duration(0))
}

func TestClassifyError_ModelNotAvailable(t *testing.T) {
	mockProc := process.NewMockAppServer()
	mockProc.SimulateModelNotAvailable()

	model := NewLanguageModel("invalid-model", WithAppServer(mockProc))

	prompt := []api.Message{
		&api.UserMessage{
			Content: []api.ContentBlock{
				&api.TextBlock{Text: "test"},
			},
		},
	}

	_, err := model.Generate(context.Background(), prompt, api.CallOptions{})

	require.Error(t, err)

	errInfo, ok := err.(*ErrorInfo)
	require.True(t, ok)

	assert.Equal(t, CategoryModelNotAvailable, errInfo.Category)
	assert.False(t, errInfo.Retryable)
}

func TestRetryPolicy_Success(t *testing.T) {
	mockProc := process.NewMockAppServer()
	mockProc.SimulateSuccess("thread-1", "Hello", 10, 5)

	retryPolicy := NewRetryPolicy(
		WithMaxAttempts(3),
		WithInitialDelay(10*time.Millisecond),
	)

	model := NewLanguageModel("gpt-5.2-codex-max",
		WithAppServer(mockProc),
		WithRetryPolicy(retryPolicy),
	)

	prompt := []api.Message{
		&api.UserMessage{
			Content: []api.ContentBlock{
				&api.TextBlock{Text: "test"},
			},
		},
	}

	resp, err := model.Generate(context.Background(), prompt, api.CallOptions{})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.NotEmpty(t, resp.Content)
}

func TestRetryPolicy_TransientFailure(t *testing.T) {
	mockProc := process.NewMockAppServer()

	// Simulate transient failure on process start, which retries
	attempts := 0
	mockProc.OnStart = func(ctx context.Context) error {
		attempts++
		if attempts <= 2 {
			// Fail first 2 attempts
			return &jsonrpc.Error{
				Code:    503,
				Message: "service_unavailable: Temporary failure",
			}
		}
		// Succeed on 3rd attempt
		return nil
	}

	mockProc.OnStartThread = func(ctx context.Context) (string, error) {
		return "thread-1", nil
	}

	mockProc.OnStartTurn = func(ctx context.Context, threadID string, input []jsonrpc.Input, policy jsonrpc.ApprovalPolicy) error {
		// Send success response
		mockProc.SendNotification("item/agentMessage/delta", map[string]any{
			"itemId": "msg-1",
			"delta":  "Success after retry",
		})
		mockProc.SendNotification("turn/completed", map[string]any{
			"usage": map[string]int{
				"inputTokens":  10,
				"outputTokens": 5,
			},
		})
		go mockProc.CloseNotifications()
		return nil
	}

	retryPolicy := NewRetryPolicy(
		WithMaxAttempts(3),
		WithInitialDelay(10*time.Millisecond),
		WithJitter(false), // Disable jitter for predictable test timing
	)

	model := NewLanguageModel("gpt-5.2-codex-max",
		WithAppServer(mockProc),
		WithRetryPolicy(retryPolicy),
	)

	prompt := []api.Message{
		&api.UserMessage{
			Content: []api.ContentBlock{
				&api.TextBlock{Text: "test"},
			},
		},
	}

	resp, err := model.Generate(context.Background(), prompt, api.CallOptions{})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 3, attempts, "should have retried 2 times before succeeding")
}

func TestRetryPolicy_PermanentFailure(t *testing.T) {
	mockProc := process.NewMockAppServer()
	mockProc.SimulateQuotaExceeded()

	retryPolicy := NewRetryPolicy(
		WithMaxAttempts(3),
		WithInitialDelay(10*time.Millisecond),
	)

	model := NewLanguageModel("gpt-5.2-codex-max",
		WithAppServer(mockProc),
		WithRetryPolicy(retryPolicy),
	)

	prompt := []api.Message{
		&api.UserMessage{
			Content: []api.ContentBlock{
				&api.TextBlock{Text: "test"},
			},
		},
	}

	_, err := model.Generate(context.Background(), prompt, api.CallOptions{})

	require.Error(t, err)

	errInfo, ok := err.(*ErrorInfo)
	require.True(t, ok)

	// Should not retry on quota exceeded (permanent failure)
	assert.Equal(t, CategoryQuotaExceeded, errInfo.Category)
	assert.False(t, errInfo.Retryable)
}

func TestCostMonitor_TrackUsage(t *testing.T) {
	mockProc := process.NewMockAppServer()
	mockProc.SimulateSuccess("thread-1", "Test response", 100, 50)

	monitor := NewCostMonitor(200) // 200 token budget

	model := NewLanguageModel("gpt-5.2-codex-max",
		WithAppServer(mockProc),
		WithCostMonitor(monitor),
	)

	prompt := []api.Message{
		&api.UserMessage{
			Content: []api.ContentBlock{
				&api.TextBlock{Text: "test"},
			},
		},
	}

	resp, err := model.Generate(context.Background(), prompt, api.CallOptions{})

	require.NoError(t, err)
	require.NotNil(t, resp)

	// Check usage tracking
	stats := monitor.Usage()
	assert.Equal(t, 150, stats.Used) // 100 input + 50 output
	assert.Equal(t, 200, stats.Budget)
	assert.Equal(t, 50, stats.Remaining)
	assert.Equal(t, 0.75, stats.Percent)
}

func TestCostMonitor_BudgetExceeded(t *testing.T) {
	mockProc := process.NewMockAppServer()
	mockProc.SimulateSuccess("thread-1", "Test response", 100, 101) // 201 tokens total

	monitor := NewCostMonitor(200) // 200 token budget

	model := NewLanguageModel("gpt-5.2-codex-max",
		WithAppServer(mockProc),
		WithCostMonitor(monitor),
	)

	prompt := []api.Message{
		&api.UserMessage{
			Content: []api.ContentBlock{
				&api.TextBlock{Text: "test"},
			},
		},
	}

	_, err := model.Generate(context.Background(), prompt, api.CallOptions{})

	require.Error(t, err)

	errInfo, ok := err.(*ErrorInfo)
	require.True(t, ok)

	assert.Equal(t, CategoryQuotaExceeded, errInfo.Category)
	assert.Contains(t, errInfo.UserMessage, "budget exceeded")
}

func TestCostMonitor_WarningAlert(t *testing.T) {
	mockProc := process.NewMockAppServer()
	mockProc.SimulateSuccess("thread-1", "Test response", 85, 0) // 85 tokens = 85% of budget

	var alertTriggered bool
	var alert CostAlert

	monitor := NewCostMonitor(100,
		WithWarningThreshold(0.8), // 80% threshold
		WithAlertCallback(func(a CostAlert) {
			alertTriggered = true
			alert = a
		}),
	)

	model := NewLanguageModel("gpt-5.2-codex-max",
		WithAppServer(mockProc),
		WithCostMonitor(monitor),
	)

	prompt := []api.Message{
		&api.UserMessage{
			Content: []api.ContentBlock{
				&api.TextBlock{Text: "test"},
			},
		},
	}

	resp, err := model.Generate(context.Background(), prompt, api.CallOptions{})

	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.True(t, alertTriggered, "warning alert should have been triggered")
	assert.Equal(t, AlertTypeWarning, alert.AlertType)
	assert.Equal(t, 85, alert.Used)
	assert.Equal(t, 100, alert.Budget)
}

func TestCostMonitor_CriticalAlert(t *testing.T) {
	mockProc := process.NewMockAppServer()
	mockProc.SimulateSuccess("thread-1", "Test response", 50, 51) // 101 tokens > budget

	var alertTriggered bool
	var alert CostAlert

	monitor := NewCostMonitor(100,
		WithAlertCallback(func(a CostAlert) {
			alertTriggered = true
			alert = a
		}),
	)

	model := NewLanguageModel("gpt-5.2-codex-max",
		WithAppServer(mockProc),
		WithCostMonitor(monitor),
	)

	prompt := []api.Message{
		&api.UserMessage{
			Content: []api.ContentBlock{
				&api.TextBlock{Text: "test"},
			},
		},
	}

	_, err := model.Generate(context.Background(), prompt, api.CallOptions{})

	require.Error(t, err)

	assert.True(t, alertTriggered, "critical alert should have been triggered")
	assert.Equal(t, AlertTypeCritical, alert.AlertType)
}
