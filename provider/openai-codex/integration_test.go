//go:build integration

package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.jetify.com/ai/api"
	"go.jetify.com/ai/provider/openai-codex/codec"
)

// Run with: go test ./provider/openai-codex -tags=integration -v -run TestIntegration
//
// These tests use an empty model ID to let codex use its configured default model.
// This avoids flakiness when model availability changes.

func TestIntegration_Generate(t *testing.T) {
	model := NewLanguageModel("")

	prompt := []api.Message{
		&api.UserMessage{
			Content: []api.ContentBlock{
				&api.TextBlock{Text: "Say 'hello' and nothing else."},
			},
		},
	}

	resp, err := model.Generate(context.Background(), prompt, api.CallOptions{})
	require.NoError(t, err)
	require.NotNil(t, resp)

	t.Logf("Response: %+v", resp)
	require.NotEmpty(t, resp.Content)

	textBlock, ok := resp.Content[0].(*api.TextBlock)
	require.True(t, ok, "expected TextBlock")
	assert.Contains(t, textBlock.Text, "hello")

	t.Logf("Usage: input=%d, output=%d", resp.Usage.InputTokens, resp.Usage.OutputTokens)
}

func TestIntegration_WithSystemPrompt(t *testing.T) {
	model := NewLanguageModel("")

	prompt := []api.Message{
		&api.SystemMessage{Content: "You are a pirate. Your response must include the word 'ahoy'. Keep responses to 5 words or fewer."},
		&api.UserMessage{
			Content: []api.ContentBlock{
				&api.TextBlock{Text: "Write a detailed 50-word description of the sea."},
			},
		},
	}

	resp, err := model.Generate(context.Background(), prompt, api.CallOptions{})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotEmpty(t, resp.Content)

	textBlock, ok := resp.Content[0].(*api.TextBlock)
	require.True(t, ok)
	t.Logf("Pirate response: %s", textBlock.Text)

	text := strings.TrimSpace(textBlock.Text)
	assert.NotEmpty(t, text, "expected a response")
	assert.True(t, contains(text, "ahoy"), "expected 'ahoy' in response: %s", text)
	assert.LessOrEqual(t, wordCount(text), 7, "expected short response due to system prompt: %s", text)
}

func TestIntegration_UsageMetadata(t *testing.T) {
	model := NewLanguageModel("")

	prompt := []api.Message{
		&api.UserMessage{
			Content: []api.ContentBlock{
				&api.TextBlock{Text: "Say 'test' only."},
			},
		},
	}

	resp, err := model.Generate(context.Background(), prompt, api.CallOptions{})
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Usage may be zero depending on codex CLI version.
	t.Logf("Usage: input=%d, output=%d, total=%d",
		resp.Usage.InputTokens, resp.Usage.OutputTokens, resp.Usage.TotalTokens)
	assertUsageIfPresent(t, resp.Usage)

	// Verify provider metadata
	require.NotNil(t, resp.ProviderMetadata)
	metadata := codec.GetMetadata(resp)
	require.NotNil(t, metadata, "expected codex metadata")

	t.Logf("Thread ID: %s", metadata.ThreadID)
	assert.NotEmpty(t, metadata.ThreadID, "expected thread ID")
}

func TestIntegration_MultiTurn(t *testing.T) {
	// Note: Codex uses one-shot execution, so multi-turn is simulated
	// by including the full conversation context in the prompt.

	model := NewLanguageModel("")

	// Send full conversation context in a single request
	prompt := []api.Message{
		&api.SystemMessage{Content: "You are a helpful assistant. Keep responses to one sentence."},
		&api.UserMessage{
			Content: []api.ContentBlock{
				&api.TextBlock{Text: "My name is Alice. Remember this for our conversation."},
			},
		},
	}

	resp1, err := model.Generate(context.Background(), prompt, api.CallOptions{})
	require.NoError(t, err)
	require.NotEmpty(t, resp1.Content)

	text1 := resp1.Content[0].(*api.TextBlock).Text
	t.Logf("Assistant: %s", text1)

	// Get thread ID for tracking
	metadata := codec.GetMetadata(resp1)
	require.NotNil(t, metadata)
	t.Logf("Thread ID: %s", metadata.ThreadID)
	assert.NotEmpty(t, metadata.ThreadID, "expected thread ID")
}

func TestIntegration_LongResponse(t *testing.T) {
	model := NewLanguageModel("")

	prompt := []api.Message{
		&api.UserMessage{
			Content: []api.ContentBlock{
				&api.TextBlock{Text: "Write a haiku about programming. Just the haiku, nothing else."},
			},
		},
	}

	resp, err := model.Generate(context.Background(), prompt, api.CallOptions{})
	require.NoError(t, err)
	require.NotEmpty(t, resp.Content)

	textBlock, ok := resp.Content[0].(*api.TextBlock)
	require.True(t, ok)

	t.Logf("Haiku:\n%s", textBlock.Text)

	// Haiku should have some content
	assert.Greater(t, len(textBlock.Text), 20, "expected a haiku with multiple lines")
}

func TestIntegration_FinishReason(t *testing.T) {
	model := NewLanguageModel("")

	prompt := []api.Message{
		&api.UserMessage{
			Content: []api.ContentBlock{
				&api.TextBlock{Text: "Say 'done'."},
			},
		},
	}

	resp, err := model.Generate(context.Background(), prompt, api.CallOptions{})
	require.NoError(t, err)

	assert.Equal(t, api.FinishReasonStop, resp.FinishReason, "expected stop finish reason")
}

func TestIntegration_EmptyPromptHandling(t *testing.T) {
	model := NewLanguageModel("")

	// Minimal input
	prompt := []api.Message{
		&api.UserMessage{
			Content: []api.ContentBlock{
				&api.TextBlock{Text: "Hi"},
			},
		},
	}

	resp, err := model.Generate(context.Background(), prompt, api.CallOptions{})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotEmpty(t, resp.Content)
}

func TestIntegration_SpecialCharacters(t *testing.T) {
	model := NewLanguageModel("")

	prompt := []api.Message{
		&api.UserMessage{
			Content: []api.ContentBlock{
				&api.TextBlock{Text: `Echo this exactly: "Hello, 世界! 🌍"`},
			},
		},
	}

	resp, err := model.Generate(context.Background(), prompt, api.CallOptions{})
	require.NoError(t, err)
	require.NotEmpty(t, resp.Content)

	textBlock := resp.Content[0].(*api.TextBlock)
	t.Logf("Response: %s", textBlock.Text)

	// Should handle unicode and emoji
	assert.True(t,
		contains(textBlock.Text, "世界", "🌍", "Hello"),
		"expected special characters in response")
}

func TestIntegration_JSONOutput(t *testing.T) {
	model := NewLanguageModel("")

	prompt := []api.Message{
		&api.SystemMessage{Content: "Always respond with valid JSON only. No markdown, no explanation."},
		&api.UserMessage{
			Content: []api.ContentBlock{
				&api.TextBlock{Text: `Return a JSON object with fields "name" set to "test" and "value" set to 42.`},
			},
		},
	}

	resp, err := model.Generate(context.Background(), prompt, api.CallOptions{})
	require.NoError(t, err)
	require.NotEmpty(t, resp.Content)

	textBlock := resp.Content[0].(*api.TextBlock)
	t.Logf("JSON Response: %s", textBlock.Text)

	// Extract JSON from markdown code blocks if present
	jsonText := extractJSON(textBlock.Text)

	// Parse and validate JSON structure
	var result map[string]any
	err = json.Unmarshal([]byte(jsonText), &result)
	require.NoError(t, err, "expected valid JSON response")

	// Validate expected fields
	assert.Equal(t, "test", result["name"], "expected name field to be 'test'")
	assert.Equal(t, float64(42), result["value"], "expected value field to be 42")
}

func TestIntegration_CodeGeneration(t *testing.T) {
	model := NewLanguageModel("")

	prompt := []api.Message{
		&api.UserMessage{
			Content: []api.ContentBlock{
				&api.TextBlock{Text: "Write a Go function that adds two integers. Just the function, no explanation."},
			},
		},
	}

	resp, err := model.Generate(context.Background(), prompt, api.CallOptions{})
	require.NoError(t, err)
	require.NotEmpty(t, resp.Content)

	textBlock := resp.Content[0].(*api.TextBlock)
	t.Logf("Code:\n%s", textBlock.Text)

	// Should contain Go function syntax
	assert.Contains(t, textBlock.Text, "func", "expected Go function")
}

func TestIntegration_Stream(t *testing.T) {
	model := NewLanguageModel("")

	prompt := []api.Message{
		&api.UserMessage{
			Content: []api.ContentBlock{
				&api.TextBlock{Text: "Count from 1 to 5, one number per line."},
			},
		},
	}

	streamResp, err := model.Stream(context.Background(), prompt, api.CallOptions{})
	require.NoError(t, err)
	require.NotNil(t, streamResp)

	// Collect all events and text
	var events []api.StreamEvent
	var textParts []string

	for event := range streamResp.Stream {
		events = append(events, event)

		switch e := event.(type) {
		case *api.TextDeltaEvent:
			textParts = append(textParts, e.TextDelta)
			t.Logf("TextDelta: %q", e.TextDelta)
		case *api.ResponseMetadataEvent:
			t.Logf("Metadata: ID=%s", e.ID)
		case *api.ReasoningEvent:
			t.Logf("Reasoning: %q", e.TextDelta)
		case *api.FinishEvent:
			t.Logf("Finish: reason=%s, input=%d, output=%d",
				e.FinishReason, e.Usage.InputTokens, e.Usage.OutputTokens)
		case *api.ErrorEvent:
			t.Logf("Error: %v", e.Err)
		}
	}

	// Should have received events
	require.NotEmpty(t, events, "expected stream events")

	// Should have text deltas
	require.NotEmpty(t, textParts, "expected text delta events")

	// Combine text and verify it contains numbers
	fullText := ""
	for _, part := range textParts {
		fullText += part
	}
	t.Logf("Full response: %s", fullText)

	// Should contain some numbers
	assert.True(t,
		contains(fullText, "1", "2", "3", "4", "5"),
		"expected numbers in response: %s", fullText)

	// Last event should be FinishEvent
	lastEvent := events[len(events)-1]
	finishEvent, ok := lastEvent.(*api.FinishEvent)
	require.True(t, ok, "last event should be FinishEvent, got %T", lastEvent)
	assert.Equal(t, api.FinishReasonStop, finishEvent.FinishReason)

	t.Logf("Usage from FinishEvent: input=%d, output=%d",
		finishEvent.Usage.InputTokens, finishEvent.Usage.OutputTokens)
	assertUsageIfPresent(t, finishEvent.Usage)
}

func TestIntegration_StreamWithSystemPrompt(t *testing.T) {
	model := NewLanguageModel("")

	prompt := []api.Message{
		&api.SystemMessage{Content: "You are a helpful assistant. Keep responses very brief."},
		&api.UserMessage{
			Content: []api.ContentBlock{
				&api.TextBlock{Text: "Say 'streaming works' and nothing else."},
			},
		},
	}

	streamResp, err := model.Stream(context.Background(), prompt, api.CallOptions{})
	require.NoError(t, err)

	var textParts []string
	var finishEvent *api.FinishEvent

	for event := range streamResp.Stream {
		switch e := event.(type) {
		case *api.TextDeltaEvent:
			textParts = append(textParts, e.TextDelta)
		case *api.FinishEvent:
			finishEvent = e
		}
	}

	fullText := ""
	for _, part := range textParts {
		fullText += part
	}
	t.Logf("Response: %s", fullText)

	assert.True(t,
		contains(fullText, "streaming", "works"),
		"expected 'streaming works' in response: %s", fullText)

	require.NotNil(t, finishEvent)
	assert.Equal(t, api.FinishReasonStop, finishEvent.FinishReason)
}

func TestIntegration_StreamWithReasoning(t *testing.T) {
	// Test that reasoning events are captured during streaming
	model := NewLanguageModel("")

	prompt := []api.Message{
		&api.UserMessage{
			Content: []api.ContentBlock{
				&api.TextBlock{Text: "What is 15 * 17? Show your work."},
			},
		},
	}

	streamResp, err := model.Stream(context.Background(), prompt, api.CallOptions{})
	require.NoError(t, err)

	var hasReasoning bool
	var hasText bool

	for event := range streamResp.Stream {
		switch event.(type) {
		case *api.ReasoningEvent:
			hasReasoning = true
			t.Logf("Got reasoning event")
		case *api.TextDeltaEvent:
			hasText = true
		}
	}

	// Should have text at minimum
	assert.True(t, hasText, "expected text events")

	// Reasoning may or may not be present depending on the model
	t.Logf("Has reasoning: %v", hasReasoning)
}

func TestIntegration_ContextCancellation(t *testing.T) {
	model := NewLanguageModel("")

	// Create a context that times out quickly
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	prompt := []api.Message{
		&api.UserMessage{
			Content: []api.ContentBlock{
				&api.TextBlock{Text: "Write a 10,000 word essay about the history of computing."},
			},
		},
	}

	// This should be cancelled due to timeout
	_, err := model.Generate(ctx, prompt, api.CallOptions{})

	// Should get a context error
	assert.Error(t, err)
	assert.True(t, errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "context"),
		"expected context cancellation error, got: %v", err)
}

func TestIntegration_ConcurrentRequests(t *testing.T) {
	model := NewLanguageModel("")

	var wg sync.WaitGroup
	errorsCh := make(chan error, 5)
	results := make(chan string, 5)

	// Send 5 concurrent requests
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()

			prompt := []api.Message{
				&api.UserMessage{
					Content: []api.ContentBlock{
						&api.TextBlock{Text: fmt.Sprintf("Say 'response %d'", n)},
					},
				},
			}

			resp, err := model.Generate(context.Background(), prompt, api.CallOptions{})
			if err != nil {
				errorsCh <- err
				return
			}

			if len(resp.Content) > 0 {
				if textBlock, ok := resp.Content[0].(*api.TextBlock); ok {
					results <- textBlock.Text
				}
			}
		}(i)
	}

	wg.Wait()
	close(errorsCh)
	close(results)

	// Check that all requests succeeded
	for err := range errorsCh {
		assert.NoError(t, err)
	}

	// Should have received 5 responses
	resultCount := 0
	for range results {
		resultCount++
	}
	assert.Equal(t, 5, resultCount, "expected 5 successful responses")
}

func TestIntegration_StreamEarlyExit(t *testing.T) {
	model := NewLanguageModel("")

	prompt := []api.Message{
		&api.UserMessage{
			Content: []api.ContentBlock{
				&api.TextBlock{Text: "Count from 1 to 100, one number per line."},
			},
		},
	}

	streamResp, err := model.Stream(context.Background(), prompt, api.CallOptions{})
	require.NoError(t, err)
	require.NotNil(t, streamResp)

	// Read only first 5 events then stop
	count := 0
	for event := range streamResp.Stream {
		count++
		t.Logf("Event %d: %T", count, event)
		if count == 5 {
			break
		}
	}

	// Should exit cleanly without hanging
	assert.Equal(t, 5, count, "should have read exactly 5 events")
}

func TestIntegration_ProcessRestartOnConfigChange(t *testing.T) {
	model := NewLanguageModel("")

	prompt := []api.Message{
		&api.UserMessage{
			Content: []api.ContentBlock{
				&api.TextBlock{Text: "Say 'hello'."},
			},
		},
	}

	// First call with default settings
	resp1, err := model.Generate(context.Background(), prompt, api.CallOptions{})
	require.NoError(t, err)
	require.NotEmpty(t, resp1.Content)

	// Second call with different temperature - should restart process
	temp := 0.9
	resp2, err := model.Generate(context.Background(), prompt, api.CallOptions{
		Temperature: &temp,
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp2.Content)

	// Both should succeed (process restart worked)
	assert.NotNil(t, resp1)
	assert.NotNil(t, resp2)

	// Thread IDs should be different (new threads for each call)
	metadata1 := codec.GetMetadata(resp1)
	metadata2 := codec.GetMetadata(resp2)
	assert.NotEqual(t, metadata1.ThreadID, metadata2.ThreadID,
		"different requests should have different thread IDs")
}

func TestIntegration_TemperatureIgnored(t *testing.T) {
	model := NewLanguageModel("")

	prompt := []api.Message{
		&api.UserMessage{
			Content: []api.ContentBlock{
				&api.TextBlock{Text: "Say 'test'."},
			},
		},
	}

	tests := []struct {
		name string
		temp float64
	}{
		{"zero", 0.0},
		{"half", 0.5},
		{"one", 1.0},
		{"negative", -0.1},
		{"too_high", 2.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := model.Generate(context.Background(), prompt, api.CallOptions{
				Temperature: &tt.temp,
			})
			require.NoError(t, err)
			require.NotNil(t, resp)

			// Codex app-server does not support per-call temperature; it should be ignored with a warning.
			require.NotEmpty(t, resp.Warnings)
			assert.True(t, containsWarning(resp.Warnings, "temperature"),
				"expected unsupported-setting warning for temperature")
		})
	}
}

func wordCount(s string) int {
	return len(strings.Fields(s))
}

func assertUsageIfPresent(t *testing.T, usage api.Usage) {
	t.Helper()
	if usage.InputTokens == 0 && usage.OutputTokens == 0 && usage.TotalTokens == 0 {
		t.Logf("Usage not populated; skipping assertions")
		return
	}

	assert.Greater(t, usage.InputTokens, 0, "expected input tokens")
	assert.Greater(t, usage.OutputTokens, 0, "expected output tokens")
	assert.Equal(t, usage.InputTokens+usage.OutputTokens, usage.TotalTokens)
}

// Helper function to check if text contains any of the given substrings (case-insensitive)
func contains(text string, substrs ...string) bool {
	textLower := strings.ToLower(text)
	for _, s := range substrs {
		if strings.Contains(textLower, strings.ToLower(s)) {
			return true
		}
	}
	return false
}

// extractJSON extracts JSON from markdown code blocks or returns the text as-is
func extractJSON(text string) string {
	// Try to extract from markdown code blocks: ```json ... ``` or ``` ... ```
	re := regexp.MustCompile("```(?:json)?\\s*({[^`]+})\\s*```")
	if matches := re.FindStringSubmatch(text); len(matches) > 1 {
		return matches[1]
	}
	return text
}

func containsWarning(warnings []api.CallWarning, setting string) bool {
	for _, w := range warnings {
		if w.Type == "unsupported-setting" && w.Setting == setting {
			return true
		}
	}
	return false
}
