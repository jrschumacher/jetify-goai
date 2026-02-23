package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.jetify.com/ai/api"
)

func TestExtractSystemPrompt_Nil(t *testing.T) {
	prompt, remaining := ExtractSystemPrompt(nil, "\n\n")
	assert.Equal(t, "", prompt)
	assert.Nil(t, remaining)
}

func TestExtractSystemPrompt_NoSystem(t *testing.T) {
	msgs := []api.Message{
		&api.UserMessage{Content: []api.ContentBlock{&api.TextBlock{Text: "hello"}}},
	}
	prompt, remaining := ExtractSystemPrompt(msgs, "\n\n")
	assert.Equal(t, "", prompt)
	assert.Len(t, remaining, 1)
}

func TestExtractSystemPrompt_SingleSystem(t *testing.T) {
	msgs := []api.Message{
		&api.SystemMessage{Content: "You are helpful"},
		&api.UserMessage{Content: []api.ContentBlock{&api.TextBlock{Text: "hello"}}},
	}
	prompt, remaining := ExtractSystemPrompt(msgs, "\n\n")
	assert.Equal(t, "You are helpful", prompt)
	assert.Len(t, remaining, 1)
}

func TestExtractSystemPrompt_MultipleSystemWithSeparator(t *testing.T) {
	msgs := []api.Message{
		&api.SystemMessage{Content: "First"},
		&api.SystemMessage{Content: "Second"},
		&api.UserMessage{Content: []api.ContentBlock{&api.TextBlock{Text: "hello"}}},
	}
	prompt, remaining := ExtractSystemPrompt(msgs, "\n\n")
	assert.Equal(t, "First\n\nSecond", prompt)
	assert.Len(t, remaining, 1)
}

func TestExtractSystemPrompt_MultipleSystemNoSeparator(t *testing.T) {
	msgs := []api.Message{
		&api.SystemMessage{Content: "First"},
		&api.SystemMessage{Content: "Second"},
		&api.UserMessage{Content: []api.ContentBlock{&api.TextBlock{Text: "hello"}}},
	}
	prompt, remaining := ExtractSystemPrompt(msgs, "")
	assert.Equal(t, "FirstSecond", prompt)
	assert.Len(t, remaining, 1)
}

func TestExtractSystemPrompt_InterleavedSystem(t *testing.T) {
	msgs := []api.Message{
		&api.SystemMessage{Content: "First"},
		&api.UserMessage{Content: []api.ContentBlock{&api.TextBlock{Text: "hello"}}},
		&api.SystemMessage{Content: "Second"},
	}
	prompt, remaining := ExtractSystemPrompt(msgs, "\n\n")
	assert.Equal(t, "First\n\nSecond", prompt)
	assert.Len(t, remaining, 1)
}
