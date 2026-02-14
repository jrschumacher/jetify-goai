// Package process provides process management for the Claude CLI.
package process

import (
	"context"
	"io"
	"log/slog"
	"sort"
	"strings"

	"go.jetify.com/ai/provider/internal/cli"
)

// Process represents a Claude CLI process.
// It provides methods to communicate with the CLI via NDJSON.
type Process interface {
	// Start launches the CLI process.
	// Returns an error if the process fails to start.
	Start(ctx context.Context) error

	// Stop gracefully terminates the CLI process.
	// Returns an error if the process fails to stop.
	Stop() error

	// Stdin returns a writer for sending NDJSON messages to the CLI.
	Stdin() io.Writer

	// Stdout returns a reader for receiving NDJSON events from the CLI.
	Stdout() io.Reader

	// Stderr returns a reader for error output from the CLI.
	Stderr() io.Reader

	// Wait blocks until the process exits.
	// Returns an error if the process exited with an error.
	Wait() error

	// IsRunning returns true if the process is currently running.
	IsRunning() bool

	// SessionID returns the session ID from the CLI initialization.
	// Returns empty string if the process hasn't been initialized yet.
	SessionID() string

	// SetSessionID sets the session ID for resume functionality.
	SetSessionID(id string)
}

// Config contains configuration for the CLI process.
type Config struct {
	// Model is the model ID to use (e.g., "sonnet", "opus", "haiku").
	Model string

	// SystemPrompt is the system prompt to use.
	SystemPrompt string

	// WorkDir is the working directory for the CLI process.
	// This is important for sandboxing - the CLI will only have access to this directory.
	WorkDir string

	// Temperature sets the sampling temperature (0.0 - 1.0).
	Temperature *float64

	// JSONSchema is the JSON schema for structured output.
	JSONSchema string

	// ResumeSessionID is the session ID to resume from.
	ResumeSessionID string

	// Verbose enables verbose output from the CLI.
	Verbose bool

	// MaxTurns limits the number of conversation turns.
	MaxTurns int

	// AllowedTools is the list of CLI built-in tools to enable.
	// If nil, all built-in tools are disabled (default).
	// If non-nil, only the listed tools are enabled (e.g., []string{"Read", "Bash"}).
	AllowedTools []string

	// Logger is the structured logger for process lifecycle events.
	// If nil, a discard logger is used.
	Logger *slog.Logger
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

// WithTemperature sets the sampling temperature.
func WithTemperature(temp float64) Option {
	return func(c *Config) {
		c.Temperature = &temp
	}
}

// WithJSONSchema sets the JSON schema for structured output.
func WithJSONSchema(schema string) Option {
	return func(c *Config) {
		c.JSONSchema = schema
	}
}

// WithResumeSession sets the session ID to resume from.
func WithResumeSession(sessionID string) Option {
	return func(c *Config) {
		c.ResumeSessionID = sessionID
	}
}

// WithVerbose enables verbose output.
func WithVerbose(verbose bool) Option {
	return func(c *Config) {
		c.Verbose = verbose
	}
}

// WithMaxTurns sets the maximum number of conversation turns.
func WithMaxTurns(turns int) Option {
	return func(c *Config) {
		c.MaxTurns = turns
	}
}

// WithAllowedTools sets the list of CLI built-in tools to enable.
// If not called, all built-in tools are disabled.
func WithAllowedTools(tools []string) Option {
	return func(c *Config) {
		c.AllowedTools = tools
	}
}

// WithLogger sets the structured logger for process lifecycle events.
func WithLogger(logger *slog.Logger) Option {
	return func(c *Config) {
		c.Logger = logger
	}
}

// NewConfig creates a new Config with the given options.
func NewConfig(opts ...Option) *Config {
	cfg := &Config{
		Verbose: true, // Default to verbose for streaming
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

// ConfigKey returns a comparable key for detecting config changes.
// Implements cli.ConfigComparable interface.
func (c *Config) ConfigKey() cli.ConfigKey {
	var temp float64
	if c.Temperature != nil {
		temp = *c.Temperature
	}

	extra := make(map[string]string)
	if c.WorkDir != "" {
		extra["workdir"] = c.WorkDir
	}
	if c.JSONSchema != "" {
		extra["jsonschema"] = c.JSONSchema
	}
	if c.ResumeSessionID != "" {
		extra["resume"] = c.ResumeSessionID
	}
	if len(c.AllowedTools) > 0 {
		sorted := make([]string, len(c.AllowedTools))
		copy(sorted, c.AllowedTools)
		sort.Strings(sorted)
		extra["tools"] = strings.Join(sorted, ",")
	}

	return cli.ConfigKey{
		Model:        c.Model,
		SystemPrompt: c.SystemPrompt,
		Temperature:  temp,
		HasTemp:      c.Temperature != nil,
		Extra:        extra,
	}
}

// Ensure Config implements cli.ConfigComparable
var _ cli.ConfigComparable = (*Config)(nil)
