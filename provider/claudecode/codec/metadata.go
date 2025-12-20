package codec

import (
	"encoding/json"

	"go.jetify.com/ai/api"
)

// ProviderName is the identifier for this provider in metadata.
const ProviderName = "claudecode"

// Metadata contains Claude Code CLI-specific metadata.
type Metadata struct {
	// SessionID is the Claude Code session identifier for conversation continuity.
	SessionID string `json:"session_id,omitempty"`

	// StructuredOutput contains the structured output from the CLI when using --json-schema.
	StructuredOutput json.RawMessage `json:"structured_output,omitempty"`

	// CostUSD is the total cost of the request in USD.
	CostUSD float64 `json:"cost_usd,omitempty"`

	// DurationMS is the total duration of the request in milliseconds.
	DurationMS int `json:"duration_ms,omitempty"`

	// DurationAPIMS is the API-specific duration in milliseconds.
	DurationAPIMS int `json:"duration_api_ms,omitempty"`

	// NumTurns is the number of conversation turns.
	NumTurns int `json:"num_turns,omitempty"`

	// Usage contains Claude Code-specific usage information.
	Usage Usage `json:"usage,omitempty"`
}

// Note: Usage type is defined in events.go and reused here for metadata.

// GetMetadata retrieves Claude Code-specific metadata from a metadata source.
func GetMetadata(source api.MetadataSource) *Metadata {
	return api.GetMetadata[Metadata](ProviderName, source)
}
