package codec

import (
	"fmt"
	"strings"

	"go.jetify.com/ai/api"
	"go.jetify.com/ai/provider/internal/cli"
)

// BuildPrompt concatenates messages into a single prompt string for the Codex CLI.
// The Codex CLI takes a prompt as a CLI argument rather than NDJSON via stdin.
//
// Format:
//
//	User: First message
//	Assistant: Response
//	User: Follow-up question
//
// System messages are prepended at the beginning.
func BuildPrompt(messages []api.Message) string {
	if len(messages) == 0 {
		return ""
	}

	var sb strings.Builder

	for i, msg := range messages {
		if i > 0 {
			sb.WriteString("\n\n")
		}

		switch m := msg.(type) {
		case *api.SystemMessage:
			sb.WriteString(m.Content)
		case *api.UserMessage:
			sb.WriteString("User: ")
			sb.WriteString(extractTextFromContent(m.Content))
		case *api.AssistantMessage:
			sb.WriteString("Assistant: ")
			sb.WriteString(extractTextFromContent(m.Content))
		case *api.ToolMessage:
			sb.WriteString("Tool Results:\n")
			sb.WriteString(extractToolResults(m.Content))
		}
	}

	return sb.String()
}

// ExtractSystemPrompt extracts system messages from the message slice
// and returns the concatenated system prompt and the remaining messages.
func ExtractSystemPrompt(messages []api.Message) (string, []api.Message) {
	return cli.ExtractSystemPrompt(messages, "\n\n")
}

// BuildPromptWithSystemSeparate builds a prompt from messages,
// keeping the system prompt separate for use with --system-prompt flag.
func BuildPromptWithSystemSeparate(messages []api.Message) (systemPrompt, userPrompt string) {
	systemPrompt, remaining := ExtractSystemPrompt(messages)
	userPrompt = BuildPrompt(remaining)
	return systemPrompt, userPrompt
}

// extractTextFromContent extracts text from content blocks.
func extractTextFromContent(blocks []api.ContentBlock) string {
	var sb strings.Builder

	for i, block := range blocks {
		if i > 0 {
			sb.WriteString("\n")
		}

		switch b := block.(type) {
		case *api.TextBlock:
			sb.WriteString(b.Text)
		case *api.ToolCallBlock:
			// Format tool calls as readable text
			sb.WriteString(fmt.Sprintf("[Tool Call: %s]", b.ToolName))
		case *api.ImageBlock:
			sb.WriteString("[Image]")
		case *api.FileBlock:
			if b.Filename != "" {
				sb.WriteString(fmt.Sprintf("[File: %s]", b.Filename))
			} else {
				sb.WriteString("[File]")
			}
		case *api.ReasoningBlock:
			// Include reasoning if present
			if b.Text != "" {
				sb.WriteString(fmt.Sprintf("<reasoning>%s</reasoning>", b.Text))
			}
		}
	}

	return sb.String()
}

// extractToolResults extracts and formats tool results.
func extractToolResults(results []api.ToolResultBlock) string {
	var sb strings.Builder

	for i, result := range results {
		if i > 0 {
			sb.WriteString("\n")
		}

		sb.WriteString(fmt.Sprintf("[%s]: ", result.ToolName))

		// Try to extract content from blocks first
		if len(result.Content) > 0 {
			sb.WriteString(extractTextFromContent(result.Content))
		} else if result.Result != nil {
			// Fall back to Result field
			switch v := result.Result.(type) {
			case string:
				sb.WriteString(v)
			case error:
				sb.WriteString(v.Error())
			default:
				sb.WriteString(fmt.Sprintf("%v", v))
			}
		}

		if result.IsError {
			sb.WriteString(" (error)")
		}
	}

	return sb.String()
}
