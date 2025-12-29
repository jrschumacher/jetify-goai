// Package process provides process management for the Codex CLI.
package process

import (
	"context"
	"io"
)

// Process represents a Codex CLI process.
// Unlike Claude Code, Codex uses one-shot execution with the prompt as a CLI argument.
type Process interface {
	// Start launches the CLI process with the configured prompt.
	// Returns an error if the process fails to start.
	Start(ctx context.Context) error

	// Stop gracefully terminates the CLI process.
	// Returns an error if the process fails to stop.
	Stop() error

	// Stdin returns a writer for sending input to the CLI.
	// Note: For Codex, stdin is typically not used as the prompt is passed via CLI args.
	Stdin() io.Writer

	// Stdout returns a reader for receiving JSONL events from the CLI.
	Stdout() io.Reader

	// Stderr returns a reader for error output from the CLI.
	Stderr() io.Reader

	// Wait blocks until the process exits.
	// Returns an error if the process exited with an error.
	Wait() error

	// IsRunning returns true if the process is currently running.
	IsRunning() bool

	// ThreadID returns the thread ID from the CLI initialization.
	// Returns empty string if the process hasn't been initialized yet.
	ThreadID() string

	// SetThreadID sets the thread ID for tracking.
	SetThreadID(id string)
}

// Config contains configuration for the CLI process.
type Config struct {
	// Prompt is the prompt to send to the CLI.
	// For multi-turn conversations, this should be the concatenated prompt.
	Prompt string

	// Model is the model ID to use (e.g., "o3", "o4-mini").
	Model string

	// SystemPrompt is the system prompt to use.
	SystemPrompt string

	// WorkDir is the working directory for the CLI process.
	WorkDir string

	// JSONOutput enables JSON output mode.
	// This is required for parsing the event stream.
	JSONOutput bool

	// SkipGitRepoCheck skips the git repository check.
	SkipGitRepoCheck bool

	// FullAutoMode enables full autonomy mode.
	FullAutoMode bool

	// Verbose enables verbose output from the CLI.
	Verbose bool
}

// Option is a function that modifies Config.
type Option func(*Config)

// WithPrompt sets the prompt.
func WithPrompt(prompt string) Option {
	return func(c *Config) {
		c.Prompt = prompt
	}
}

// WithModel sets the model ID.
func WithModel(model string) Option {
	return func(c *Config) {
		c.Model = model
	}
}

// WithSystemPrompt sets the system prompt.
func WithSystemPrompt(prompt string) Option {
	return func(c *Config) {
		c.SystemPrompt = prompt
	}
}

// WithWorkDir sets the working directory.
func WithWorkDir(dir string) Option {
	return func(c *Config) {
		c.WorkDir = dir
	}
}

// WithJSONOutput enables JSON output mode.
func WithJSONOutput(enabled bool) Option {
	return func(c *Config) {
		c.JSONOutput = enabled
	}
}

// WithSkipGitRepoCheck skips the git repository check.
func WithSkipGitRepoCheck(skip bool) Option {
	return func(c *Config) {
		c.SkipGitRepoCheck = skip
	}
}

// WithFullAutoMode enables full autonomy mode.
func WithFullAutoMode(enabled bool) Option {
	return func(c *Config) {
		c.FullAutoMode = enabled
	}
}

// WithVerbose enables verbose output.
func WithVerbose(verbose bool) Option {
	return func(c *Config) {
		c.Verbose = verbose
	}
}

// NewConfig creates a new Config with the given options.
func NewConfig(opts ...Option) *Config {
	cfg := &Config{
		JSONOutput:       true, // Default to JSON for parsing
		SkipGitRepoCheck: true, // Default to skip for SDK usage
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}
