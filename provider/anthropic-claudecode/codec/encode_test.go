package codec

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.jetify.com/ai/api"
)

func TestEncodeMessage_UserText(t *testing.T) {
	msg := &api.UserMessage{
		Content: []api.ContentBlock{
			&api.TextBlock{Text: "Hello, world!"},
		},
	}

	encoded, err := EncodeMessage(msg)
	require.NoError(t, err)

	var result map[string]any
	err = json.Unmarshal(encoded, &result)
	require.NoError(t, err)

	assert.Equal(t, "user", result["type"])
	message := result["message"].(map[string]any)
	assert.Equal(t, "user", message["role"])
	assert.Equal(t, "Hello, world!", message["content"])
}

func TestEncodeMessage_UserMultipleTextBlocks(t *testing.T) {
	msg := &api.UserMessage{
		Content: []api.ContentBlock{
			&api.TextBlock{Text: "First part. "},
			&api.TextBlock{Text: "Second part."},
		},
	}

	encoded, err := EncodeMessage(msg)
	require.NoError(t, err)

	var result map[string]any
	err = json.Unmarshal(encoded, &result)
	require.NoError(t, err)

	message := result["message"].(map[string]any)
	// Multiple text blocks should be concatenated
	assert.Equal(t, "First part. Second part.", message["content"])
}

func TestEncodeMessage_AssistantText(t *testing.T) {
	msg := &api.AssistantMessage{
		Content: []api.ContentBlock{
			&api.TextBlock{Text: "I can help with that."},
		},
	}

	encoded, err := EncodeMessage(msg)
	require.NoError(t, err)

	var result map[string]any
	err = json.Unmarshal(encoded, &result)
	require.NoError(t, err)

	assert.Equal(t, "assistant", result["type"])
	message := result["message"].(map[string]any)
	assert.Equal(t, "assistant", message["role"])
	assert.Equal(t, "I can help with that.", message["content"])
}

func TestEncodeMessage_AssistantWithToolCall(t *testing.T) {
	msg := &api.AssistantMessage{
		Content: []api.ContentBlock{
			&api.TextBlock{Text: "Let me check the weather."},
			&api.ToolCallBlock{
				ToolCallID: "tool_123",
				ToolName:   "get_weather",
				Args:       json.RawMessage(`{"location":"NYC"}`),
			},
		},
	}

	encoded, err := EncodeMessage(msg)
	require.NoError(t, err)

	var result map[string]any
	err = json.Unmarshal(encoded, &result)
	require.NoError(t, err)

	assert.Equal(t, "assistant", result["type"])
	message := result["message"].(map[string]any)
	content := message["content"].([]any)
	require.Len(t, content, 2)

	// First block is text
	textBlock := content[0].(map[string]any)
	assert.Equal(t, "text", textBlock["type"])
	assert.Equal(t, "Let me check the weather.", textBlock["text"])

	// Second block is tool_use
	toolBlock := content[1].(map[string]any)
	assert.Equal(t, "tool_use", toolBlock["type"])
	assert.Equal(t, "tool_123", toolBlock["id"])
	assert.Equal(t, "get_weather", toolBlock["name"])
}

func TestEncodeMessage_ToolResult(t *testing.T) {
	msg := &api.ToolMessage{
		Content: []api.ToolResultBlock{
			{
				ToolCallID: "tool_123",
				ToolName:   "get_weather",
				Content:    []api.ContentBlock{&api.TextBlock{Text: "72°F, sunny"}},
			},
		},
	}

	encoded, err := EncodeMessage(msg)
	require.NoError(t, err)

	var result map[string]any
	err = json.Unmarshal(encoded, &result)
	require.NoError(t, err)

	assert.Equal(t, "user", result["type"])
	message := result["message"].(map[string]any)
	assert.Equal(t, "user", message["role"])
	content := message["content"].([]any)
	require.Len(t, content, 1)

	toolResult := content[0].(map[string]any)
	assert.Equal(t, "tool_result", toolResult["type"])
	assert.Equal(t, "tool_123", toolResult["tool_use_id"])
	assert.Equal(t, "72°F, sunny", toolResult["content"])
}

func TestEncodeMessage_ToolResultWithError(t *testing.T) {
	msg := &api.ToolMessage{
		Content: []api.ToolResultBlock{
			{
				ToolCallID: "tool_123",
				ToolName:   "get_weather",
				IsError:    true,
				Content:    []api.ContentBlock{&api.TextBlock{Text: "Location not found"}},
			},
		},
	}

	encoded, err := EncodeMessage(msg)
	require.NoError(t, err)

	var result map[string]any
	err = json.Unmarshal(encoded, &result)
	require.NoError(t, err)

	message := result["message"].(map[string]any)
	content := message["content"].([]any)
	toolResult := content[0].(map[string]any)
	assert.Equal(t, true, toolResult["is_error"])
}

func TestEncodeMessage_SystemReturnsError(t *testing.T) {
	msg := &api.SystemMessage{
		Content: "You are a helpful assistant.",
	}

	// System messages should not be encoded as CLI input
	// They are handled separately via --system-prompt flag
	_, err := EncodeMessage(msg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "system messages")
}

func TestEncodeMessage_NilMessage(t *testing.T) {
	_, err := EncodeMessage(nil)
	require.Error(t, err)
}

func TestExtractSystemPrompt(t *testing.T) {
	messages := []api.Message{
		&api.SystemMessage{Content: "You are a helpful assistant."},
		&api.UserMessage{Content: []api.ContentBlock{&api.TextBlock{Text: "Hello"}}},
	}

	systemPrompt, remaining := ExtractSystemPrompt(messages)

	assert.Equal(t, "You are a helpful assistant.", systemPrompt)
	require.Len(t, remaining, 1)
	assert.Equal(t, api.MessageRoleUser, remaining[0].Role())
}

func TestExtractSystemPrompt_NoSystem(t *testing.T) {
	messages := []api.Message{
		&api.UserMessage{Content: []api.ContentBlock{&api.TextBlock{Text: "Hello"}}},
	}

	systemPrompt, remaining := ExtractSystemPrompt(messages)

	assert.Empty(t, systemPrompt)
	require.Len(t, remaining, 1)
}

func TestExtractSystemPrompt_MultipleSystem(t *testing.T) {
	messages := []api.Message{
		&api.SystemMessage{Content: "First system. "},
		&api.SystemMessage{Content: "Second system."},
		&api.UserMessage{Content: []api.ContentBlock{&api.TextBlock{Text: "Hello"}}},
	}

	systemPrompt, remaining := ExtractSystemPrompt(messages)

	// Multiple system messages should be concatenated
	assert.Equal(t, "First system. Second system.", systemPrompt)
	require.Len(t, remaining, 1)
}

func TestExtractSystemPrompt_Empty(t *testing.T) {
	messages := []api.Message{}

	systemPrompt, remaining := ExtractSystemPrompt(messages)

	assert.Empty(t, systemPrompt)
	assert.Empty(t, remaining)
}

func TestExtractSystemPrompt_NilSlice(t *testing.T) {
	systemPrompt, remaining := ExtractSystemPrompt(nil)

	assert.Empty(t, systemPrompt)
	assert.Nil(t, remaining)
}
