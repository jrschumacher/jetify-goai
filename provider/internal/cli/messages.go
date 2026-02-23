package cli

import (
	"strings"

	"go.jetify.com/ai/api"
)

// ExtractSystemPrompt extracts system messages from the message slice
// and returns the concatenated system prompt and the remaining messages.
// The separator is placed between multiple system messages.
func ExtractSystemPrompt(messages []api.Message, separator string) (string, []api.Message) {
	if messages == nil {
		return "", nil
	}

	var sb strings.Builder
	remaining := make([]api.Message, 0, len(messages))

	for _, msg := range messages {
		if sm, ok := msg.(*api.SystemMessage); ok {
			if separator != "" && sb.Len() > 0 {
				sb.WriteString(separator)
			}
			sb.WriteString(sm.Content)
		} else {
			remaining = append(remaining, msg)
		}
	}

	return sb.String(), remaining
}
