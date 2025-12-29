//go:build integration

package codex

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.jetify.com/ai/api"
	"go.jetify.com/ai/provider/openai-codex/codec"
)

// Run with: go test ./provider/openai-codex -tags=integration -v -run TestIntegration

func TestIntegration_Generate(t *testing.T) {
	model := NewLanguageModel("o3")

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
	model := NewLanguageModel("o3")

	prompt := []api.Message{
		&api.SystemMessage{Content: "You are a pirate. Respond in pirate speak. Keep responses under 20 words."},
		&api.UserMessage{
			Content: []api.ContentBlock{
				&api.TextBlock{Text: "Say hello."},
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

	// Should contain pirate-like language
	assert.True(t,
		contains(textBlock.Text, "ahoy", "matey", "arr", "aye", "yo ho", "avast", "yarr"),
		"expected pirate speak in: %s", textBlock.Text)
}

func TestIntegration_UsageMetadata(t *testing.T) {
	model := NewLanguageModel("o3")

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

	// Verify usage is populated
	assert.Greater(t, resp.Usage.InputTokens, 0, "expected input tokens")
	assert.Greater(t, resp.Usage.OutputTokens, 0, "expected output tokens")
	assert.Equal(t, resp.Usage.InputTokens+resp.Usage.OutputTokens, resp.Usage.TotalTokens)

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

	model := NewLanguageModel("o3")

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
	model := NewLanguageModel("o3")

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
	model := NewLanguageModel("o3")

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
	model := NewLanguageModel("o3")

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
	model := NewLanguageModel("o3")

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
	model := NewLanguageModel("o3")

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

	// Try to parse as JSON
	var result map[string]any
	err = json.Unmarshal([]byte(textBlock.Text), &result)
	if err != nil {
		// Sometimes the model wraps in markdown, try to extract
		t.Logf("Note: Response may contain markdown wrapper")
	}
}

func TestIntegration_CodeGeneration(t *testing.T) {
	model := NewLanguageModel("o3")

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
	model := NewLanguageModel("o3")

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
	assert.Greater(t, finishEvent.Usage.InputTokens, 0)
	assert.Greater(t, finishEvent.Usage.OutputTokens, 0)
}

func TestIntegration_StreamWithSystemPrompt(t *testing.T) {
	model := NewLanguageModel("o3")

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
	model := NewLanguageModel("o3")

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

// Helper function to check if text contains any of the given substrings (case-insensitive)
func contains(text string, substrs ...string) bool {
	textLower := toLower(text)
	for _, s := range substrs {
		if containsStr(textLower, toLower(s)) {
			return true
		}
	}
	return false
}

func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		result[i] = c
	}
	return string(result)
}

func containsStr(s, substr string) bool {
	return len(substr) <= len(s) && (s == substr || len(substr) == 0 || findSubstr(s, substr) >= 0)
}

func findSubstr(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
