package api

import "context"

// CLILanguageModel extends LanguageModel with CLI-specific capabilities.
// This interface is implemented by language models that wrap CLI tools
// (such as Claude Code CLI or OpenAI Codex CLI) rather than direct API calls.
//
// CLI-based models have unique characteristics:
//   - They maintain a persistent subprocess for efficient multi-turn conversations
//   - Configuration changes may require process restart
//   - Token usage is tracked per session rather than per request
//   - They don't support URL loading natively
type CLILanguageModel interface {
	LanguageModel

	// IsProcessRunning returns true if the underlying CLI process is running.
	IsProcessRunning() bool

	// RestartProcess stops the current process and starts a new one.
	// This is useful when configuration changes require a fresh process.
	RestartProcess(ctx context.Context) error

	// Close stops the underlying CLI process and releases resources.
	// After Close is called, the model should not be used.
	Close() error
}

// CLITokenUsage represents cumulative token usage across a CLI session.
// CLI-based models track tokens differently than API-based models because
// they maintain a persistent session and subscriptions don't expose costs.
type CLITokenUsage struct {
	// InputTokens is the cumulative number of input tokens used in the session.
	InputTokens int
	// OutputTokens is the cumulative number of output tokens generated in the session.
	OutputTokens int
	// TotalTokens is the sum of input and output tokens.
	TotalTokens int
	// CachedTokens is the number of tokens that were served from cache.
	CachedTokens int
}

// CLITokenTracker is an optional interface that CLI models can implement
// to provide session-level token tracking.
type CLITokenTracker interface {
	// TokenUsage returns the cumulative token usage for the session.
	TokenUsage() CLITokenUsage
	// ResetTokenUsage clears the token counters.
	ResetTokenUsage()
}

// CLIConfigAware is an optional interface that CLI models can implement
// to support configuration change detection.
type CLIConfigAware interface {
	// ConfigNeedsRestart returns true if the given options would require
	// restarting the underlying process (e.g., model change, system prompt change).
	ConfigNeedsRestart(opts CallOptions) bool
}
