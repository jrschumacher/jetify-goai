// Package process provides process management for the Codex CLI.
package process

import (
	"context"
	"io"

	"go.jetify.com/ai/provider/openai-codex/codec/jsonrpc"
)

// Process represents a Codex CLI process.
// The primary implementation is AppServerProcess which uses the persistent
// codex app-server mode for streaming and multi-turn conversations.
type Process interface {
	// Start launches the CLI process.
	// Returns an error if the process fails to start.
	Start(ctx context.Context) error

	// Stop gracefully terminates the CLI process.
	// Returns an error if the process fails to stop.
	Stop() error

	// Stdin returns a writer for sending input to the CLI.
	Stdin() io.Writer

	// Stdout returns a reader for receiving output from the CLI.
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

// AppServer extends Process with app-server specific methods.
type AppServer interface {
	Process

	// Initialize sends the initialize RPC to the server.
	Initialize(ctx context.Context) error

	// StartThread creates a new conversation thread.
	StartThread(ctx context.Context) (string, error)

	// StartTurn begins a new turn in the conversation.
	StartTurn(ctx context.Context, threadID string, input []jsonrpc.Input, policy jsonrpc.ApprovalPolicy) error

	// Notifications returns the channel for receiving streaming notifications.
	Notifications() <-chan *jsonrpc.Notification

	// Client returns the JSON-RPC client for direct access if needed.
	Client() *jsonrpc.Client
}

// Config contains configuration for the CLI process.
type Config struct {
	// Model is the model ID to use (e.g., "o3", "o4-mini", "gpt-5.1-codex-max").
	Model string

	// SystemPrompt is the system prompt to use.
	SystemPrompt string

	// WorkDir is the working directory for the CLI process.
	WorkDir string

	// Temperature controls randomness in the model's output.
	Temperature *float64
}

// Option is a function that modifies Config.
type Option func(*Config)

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

// WithTemperature sets the temperature for the model.
func WithTemperature(temp float64) Option {
	return func(c *Config) {
		c.Temperature = &temp
	}
}

// NewConfig creates a new Config with the given options.
func NewConfig(opts ...Option) *Config {
	cfg := &Config{}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

// ConfigKey returns a comparable key for the config.
// Used to detect when config changes require process restart.
func (c *Config) ConfigKey() ConfigKey {
	var temp float64
	if c.Temperature != nil {
		temp = *c.Temperature
	}
	return ConfigKey{
		Model:        c.Model,
		SystemPrompt: c.SystemPrompt,
		Temperature:  temp,
		HasTemp:      c.Temperature != nil,
	}
}

// ConfigKey is a comparable struct for detecting config changes.
type ConfigKey struct {
	Model        string
	SystemPrompt string
	Temperature  float64
	HasTemp      bool
}
