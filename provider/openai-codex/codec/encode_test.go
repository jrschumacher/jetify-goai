package codec

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.jetify.com/ai/api"
)

func TestBuildPrompt_Empty(t *testing.T) {
	result := BuildPrompt(nil)
	assert.Equal(t, "", result)

	result = BuildPrompt([]api.Message{})
	assert.Equal(t, "", result)
}

func TestBuildPrompt_SingleUserMessage(t *testing.T) {
	messages := []api.Message{
		&api.UserMessage{
			Content: []api.ContentBlock{
				&api.TextBlock{Text: "Hello, how are you?"},
			},
		},
	}

	result := BuildPrompt(messages)
	assert.Equal(t, "User: Hello, how are you?", result)
}

func TestBuildPrompt_SingleAssistantMessage(t *testing.T) {
	messages := []api.Message{
		&api.AssistantMessage{
			Content: []api.ContentBlock{
				&api.TextBlock{Text: "I'm doing well, thank you!"},
			},
		},
	}

	result := BuildPrompt(messages)
	assert.Equal(t, "Assistant: I'm doing well, thank you!", result)
}

func TestBuildPrompt_Conversation(t *testing.T) {
	messages := []api.Message{
		&api.UserMessage{
			Content: []api.ContentBlock{
				&api.TextBlock{Text: "What is 2+2?"},
			},
		},
		&api.AssistantMessage{
			Content: []api.ContentBlock{
				&api.TextBlock{Text: "4"},
			},
		},
		&api.UserMessage{
			Content: []api.ContentBlock{
				&api.TextBlock{Text: "And 3+3?"},
			},
		},
	}

	result := BuildPrompt(messages)
	expected := "User: What is 2+2?\n\nAssistant: 4\n\nUser: And 3+3?"
	assert.Equal(t, expected, result)
}

func TestBuildPrompt_WithSystemMessage(t *testing.T) {
	messages := []api.Message{
		&api.SystemMessage{Content: "You are a helpful assistant."},
		&api.UserMessage{
			Content: []api.ContentBlock{
				&api.TextBlock{Text: "Hello"},
			},
		},
	}

	result := BuildPrompt(messages)
	expected := "You are a helpful assistant.\n\nUser: Hello"
	assert.Equal(t, expected, result)
}

func TestBuildPrompt_MultipleTextBlocks(t *testing.T) {
	messages := []api.Message{
		&api.UserMessage{
			Content: []api.ContentBlock{
				&api.TextBlock{Text: "First part."},
				&api.TextBlock{Text: "Second part."},
			},
		},
	}

	result := BuildPrompt(messages)
	assert.Equal(t, "User: First part.\nSecond part.", result)
}

func TestBuildPrompt_WithToolCall(t *testing.T) {
	messages := []api.Message{
		&api.AssistantMessage{
			Content: []api.ContentBlock{
				&api.TextBlock{Text: "Let me check that for you."},
				&api.ToolCallBlock{
					ToolCallID: "call_1",
					ToolName:   "search",
					Args:       json.RawMessage(`{"query":"test"}`),
				},
			},
		},
	}

	result := BuildPrompt(messages)
	expected := "Assistant: Let me check that for you.\n[Tool Call: search]"
	assert.Equal(t, expected, result)
}

func TestBuildPrompt_WithToolResults(t *testing.T) {
	messages := []api.Message{
		&api.ToolMessage{
			Content: []api.ToolResultBlock{
				{
					ToolCallID: "call_1",
					ToolName:   "search",
					Content: []api.ContentBlock{
						&api.TextBlock{Text: "Found 3 results"},
					},
				},
			},
		},
	}

	result := BuildPrompt(messages)
	expected := "Tool Results:\n[search]: Found 3 results"
	assert.Equal(t, expected, result)
}

func TestBuildPrompt_WithToolResultError(t *testing.T) {
	messages := []api.Message{
		&api.ToolMessage{
			Content: []api.ToolResultBlock{
				{
					ToolCallID: "call_1",
					ToolName:   "search",
					Result:     "Connection failed",
					IsError:    true,
				},
			},
		},
	}

	result := BuildPrompt(messages)
	expected := "Tool Results:\n[search]: Connection failed (error)"
	assert.Equal(t, expected, result)
}

func TestBuildPrompt_WithImage(t *testing.T) {
	messages := []api.Message{
		&api.UserMessage{
			Content: []api.ContentBlock{
				&api.TextBlock{Text: "What's in this image?"},
				&api.ImageBlock{URL: "https://example.com/image.png"},
			},
		},
	}

	result := BuildPrompt(messages)
	expected := "User: What's in this image?\n[Image]"
	assert.Equal(t, expected, result)
}

func TestBuildPrompt_WithFile(t *testing.T) {
	messages := []api.Message{
		&api.UserMessage{
			Content: []api.ContentBlock{
				&api.TextBlock{Text: "Analyze this file:"},
				&api.FileBlock{Filename: "data.csv"},
			},
		},
	}

	result := BuildPrompt(messages)
	expected := "User: Analyze this file:\n[File: data.csv]"
	assert.Equal(t, expected, result)
}

func TestBuildPrompt_WithReasoning(t *testing.T) {
	messages := []api.Message{
		&api.AssistantMessage{
			Content: []api.ContentBlock{
				&api.ReasoningBlock{Text: "Let me think about this..."},
				&api.TextBlock{Text: "The answer is 42."},
			},
		},
	}

	result := BuildPrompt(messages)
	expected := "Assistant: <reasoning>Let me think about this...</reasoning>\nThe answer is 42."
	assert.Equal(t, expected, result)
}

func TestExtractSystemPrompt_NoSystem(t *testing.T) {
	messages := []api.Message{
		&api.UserMessage{
			Content: []api.ContentBlock{
				&api.TextBlock{Text: "Hello"},
			},
		},
	}

	systemPrompt, remaining := ExtractSystemPrompt(messages)
	assert.Equal(t, "", systemPrompt)
	assert.Len(t, remaining, 1)
}

func TestExtractSystemPrompt_SingleSystem(t *testing.T) {
	messages := []api.Message{
		&api.SystemMessage{Content: "You are helpful."},
		&api.UserMessage{
			Content: []api.ContentBlock{
				&api.TextBlock{Text: "Hello"},
			},
		},
	}

	systemPrompt, remaining := ExtractSystemPrompt(messages)
	assert.Equal(t, "You are helpful.", systemPrompt)
	assert.Len(t, remaining, 1)
}

func TestExtractSystemPrompt_MultipleSystem(t *testing.T) {
	messages := []api.Message{
		&api.SystemMessage{Content: "First instruction."},
		&api.SystemMessage{Content: "Second instruction."},
		&api.UserMessage{
			Content: []api.ContentBlock{
				&api.TextBlock{Text: "Hello"},
			},
		},
	}

	systemPrompt, remaining := ExtractSystemPrompt(messages)
	assert.Equal(t, "First instruction.\n\nSecond instruction.", systemPrompt)
	assert.Len(t, remaining, 1)
}

func TestExtractSystemPrompt_Nil(t *testing.T) {
	systemPrompt, remaining := ExtractSystemPrompt(nil)
	assert.Equal(t, "", systemPrompt)
	assert.Nil(t, remaining)
}

func TestBuildPromptWithSystemSeparate(t *testing.T) {
	messages := []api.Message{
		&api.SystemMessage{Content: "You are a coding assistant."},
		&api.UserMessage{
			Content: []api.ContentBlock{
				&api.TextBlock{Text: "Write a function"},
			},
		},
		&api.AssistantMessage{
			Content: []api.ContentBlock{
				&api.TextBlock{Text: "Here's the function:"},
			},
		},
	}

	systemPrompt, userPrompt := BuildPromptWithSystemSeparate(messages)
	assert.Equal(t, "You are a coding assistant.", systemPrompt)
	assert.Equal(t, "User: Write a function\n\nAssistant: Here's the function:", userPrompt)
}

func TestBuildPrompt_ComplexConversation(t *testing.T) {
	messages := []api.Message{
		&api.SystemMessage{Content: "You are a helpful coding assistant."},
		&api.UserMessage{
			Content: []api.ContentBlock{
				&api.TextBlock{Text: "Search for Go tutorials"},
			},
		},
		&api.AssistantMessage{
			Content: []api.ContentBlock{
				&api.TextBlock{Text: "I'll search for that."},
				&api.ToolCallBlock{
					ToolCallID: "call_1",
					ToolName:   "web_search",
					Args:       json.RawMessage(`{"query":"Go tutorials"}`),
				},
			},
		},
		&api.ToolMessage{
			Content: []api.ToolResultBlock{
				{
					ToolCallID: "call_1",
					ToolName:   "web_search",
					Content: []api.ContentBlock{
						&api.TextBlock{Text: "Found: Go by Example, Tour of Go"},
					},
				},
			},
		},
		&api.AssistantMessage{
			Content: []api.ContentBlock{
				&api.TextBlock{Text: "I found some great resources!"},
			},
		},
	}

	result := BuildPrompt(messages)
	expected := "You are a helpful coding assistant.\n\nUser: Search for Go tutorials\n\nAssistant: I'll search for that.\n[Tool Call: web_search]\n\nTool Results:\n[web_search]: Found: Go by Example, Tour of Go\n\nAssistant: I found some great resources!"
	assert.Equal(t, expected, result)
}