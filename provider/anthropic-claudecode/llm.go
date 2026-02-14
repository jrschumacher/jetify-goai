package claudecode

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"iter"
	"log/slog"
	"strings"
	"sync"

	"go.jetify.com/ai/api"
	"go.jetify.com/ai/provider/anthropic-claudecode/codec"
	"go.jetify.com/ai/provider/anthropic-claudecode/process"
	"go.jetify.com/ai/provider/internal/cli"
)

// ModelOption is a function type that modifies a LanguageModel.
type ModelOption func(*LanguageModel)

// WithProcess sets a custom process (useful for testing with mocks).
func WithProcess(proc process.Process) ModelOption {
	return func(m *LanguageModel) {
		m.proc = proc
	}
}

// WithRetryPolicy sets a custom retry policy for handling transient failures.
func WithRetryPolicy(policy *RetryPolicy) ModelOption {
	return func(m *LanguageModel) {
		m.retryPolicy = policy
	}
}

// WithTokenTracker sets a token tracker for monitoring token usage.
func WithTokenTracker(tracker *cli.TokenTracker) ModelOption {
	return func(m *LanguageModel) {
		m.tokenTracker = tracker
	}
}

// WithLogger sets the structured logger for the language model and its processes.
func WithLogger(logger *slog.Logger) ModelOption {
	return func(m *LanguageModel) {
		m.logger = logger
	}
}

// WithWorkDir sets the working directory for the CLI process.
// The CLI will be sandboxed to this directory.
func WithWorkDir(dir string) ModelOption {
	return func(m *LanguageModel) {
		m.workDir = dir
	}
}

// WithAllowedTools sets the list of CLI built-in tools to enable.
// If not called, all built-in tools are disabled (default).
func WithAllowedTools(tools []string) ModelOption {
	return func(m *LanguageModel) {
		m.allowedTools = tools
	}
}

// LanguageModel represents a Claude Code language model.
// It maintains a persistent process for efficient streaming and multi-turn conversations.
type LanguageModel struct {
	mu sync.Mutex

	modelID string
	proc    process.Process
	logger  *slog.Logger

	// cachedKey tracks the config used to start the current process.
	// If options change, the process is restarted.
	cachedKey cli.ConfigKey

	// resumeSessionID is set by RestartProcess to continue a conversation
	// using the CLI's --resume flag on the next process start.
	resumeSessionID string

	// workDir is the working directory for sandboxing the CLI process.
	workDir string

	// allowedTools is the list of CLI built-in tools to enable.
	allowedTools []string

	// retryPolicy defines how operations should be retried on failure.
	retryPolicy *RetryPolicy

	// tokenTracker tracks token usage across the session.
	tokenTracker *cli.TokenTracker
}

var (
	_ api.LanguageModel    = &LanguageModel{}
	_ api.CLILanguageModel = &LanguageModel{}
	_ api.CLITokenTracker  = &LanguageModel{}
	_ api.CLIConfigAware   = &LanguageModel{}
)

// NewLanguageModel creates a new Claude Code language model.
func NewLanguageModel(modelID string, opts ...ModelOption) *LanguageModel {
	model := &LanguageModel{
		modelID: modelID,
	}

	for _, opt := range opts {
		opt(model)
	}

	if model.logger == nil {
		model.logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	return model
}

// ProviderName returns the provider name.
func (m *LanguageModel) ProviderName() string {
	return ProviderName
}

// ModelID returns the model ID.
func (m *LanguageModel) ModelID() string {
	return m.modelID
}

// SupportedUrls returns URL patterns supported by the model.
// Claude CLI doesn't support direct URL loading, so we return empty.
func (m *LanguageModel) SupportedUrls() []api.SupportedURL {
	return nil
}

// buildConfig creates a process config from the current options.
func (m *LanguageModel) buildConfig(systemPrompt string, opts api.CallOptions) *process.Config {
	cfg := &process.Config{
		Model:        m.modelID,
		SystemPrompt: systemPrompt,
		Verbose:      true, // Default for streaming
		Logger:       m.logger,
	}
	if opts.Temperature != nil {
		cfg.Temperature = opts.Temperature
	}
	if m.workDir != "" {
		cfg.WorkDir = m.workDir
	}
	if m.allowedTools != nil {
		cfg.AllowedTools = m.allowedTools
	}
	return cfg
}

// ensureProcess ensures a running process with the correct config.
// If the config has changed, the existing process is stopped and a new one started.
func (m *LanguageModel) ensureProcess(ctx context.Context, cfg *process.Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	newKey := cfg.ConfigKey()

	// Check if we need to restart due to config change
	if m.proc != nil && m.proc.IsRunning() {
		if cli.ConfigKeysEqual(m.cachedKey, newKey) {
			m.logger.Debug("reusing existing process")
			return nil // Config unchanged, reuse existing process
		}
		// Config changed, stop the old process
		m.logger.Info("config changed, restarting process")
		m.proc.Stop()
		m.proc = nil
	}

	// Create new process if needed
	if m.proc == nil {
		m.logger.Debug("creating new process", "model", cfg.Model)
		procOpts := []process.Option{
			process.WithModel(cfg.Model),
			process.WithVerbose(cfg.Verbose),
			process.WithLogger(m.logger),
		}
		if cfg.SystemPrompt != "" {
			procOpts = append(procOpts, process.WithSystemPrompt(cfg.SystemPrompt))
		}
		if cfg.Temperature != nil {
			procOpts = append(procOpts, process.WithTemperature(*cfg.Temperature))
		}
		if cfg.WorkDir != "" {
			procOpts = append(procOpts, process.WithWorkDir(cfg.WorkDir))
		}
		if cfg.JSONSchema != "" {
			procOpts = append(procOpts, process.WithJSONSchema(cfg.JSONSchema))
		}
		if cfg.AllowedTools != nil {
			procOpts = append(procOpts, process.WithAllowedTools(cfg.AllowedTools))
		}
		if m.resumeSessionID != "" {
			m.logger.Info("resuming session", "sessionID", m.resumeSessionID)
			procOpts = append(procOpts, process.WithResumeSession(m.resumeSessionID))
			m.resumeSessionID = "" // Clear after use
		}
		m.proc = process.NewCLIProcess(procOpts...)
	}

	// Start the process if not running, with retry logic if configured
	if !m.proc.IsRunning() {
		m.logger.Info("starting process", "model", cfg.Model)
		startFn := func() error {
			if err := m.proc.Start(ctx); err != nil {
				return WrapError(fmt.Errorf("failed to start CLI process: %w", err))
			}
			return nil
		}

		// Use retry policy if configured, otherwise execute once
		var err error
		if m.retryPolicy != nil {
			err = m.retryPolicy.Execute(ctx, startFn)
		} else {
			err = startFn()
		}

		if err != nil {
			return err
		}
	}

	m.cachedKey = newKey
	return nil
}

// drainStderr reads any available stderr content from the process.
// Useful for surfacing actual CLI error messages when stdin/stdout operations fail.
func drainStderr(proc process.Process) string {
	stderr := proc.Stderr()
	if stderr == nil {
		return ""
	}
	data, _ := io.ReadAll(io.LimitReader(stderr, 4096))
	msg := strings.TrimSpace(string(data))
	return msg
}

// Generate generates a response from the model.
func (m *LanguageModel) Generate(
	ctx context.Context, prompt []api.Message, opts api.CallOptions,
) (*api.Response, error) {
	// Extract system prompt from messages
	systemPrompt, messages := codec.ExtractSystemPrompt(prompt)

	// Build config and ensure process
	cfg := m.buildConfig(systemPrompt, opts)
	if err := m.ensureProcess(ctx, cfg); err != nil {
		return nil, err
	}

	m.mu.Lock()
	proc := m.proc
	m.mu.Unlock()

	// Send messages to the CLI
	for _, msg := range messages {
		encoded, err := codec.EncodeMessage(msg)
		if err != nil {
			return nil, WrapError(fmt.Errorf("failed to encode message: %w", err))
		}
		if _, err := proc.Stdin().Write(append(encoded, '\n')); err != nil {
			stderrMsg := drainStderr(proc)
			m.logger.Error("failed to write to CLI stdin", "error", err, "stderr", stderrMsg, "processRunning", proc.IsRunning())
			if stderrMsg != "" {
				return nil, WrapError(fmt.Errorf("CLI process error (stderr: %s): %w", stderrMsg, err))
			}
			return nil, WrapError(fmt.Errorf("failed to write to CLI stdin (process running=%v): %w", proc.IsRunning(), err))
		}
	}

	// Read events from stdout until we get a result
	scanner := bufio.NewScanner(proc.Stdout())
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		event, err := codec.ParseEvent(line)
		if err != nil {
			return nil, WrapError(fmt.Errorf("failed to parse event: %w", err))
		}

		// Handle system init - capture session ID
		if event.Type == codec.EventTypeSystem && event.Subtype == "init" {
			proc.SetSessionID(event.SessionID)
			m.logger.Debug("session initialized", "sessionID", event.SessionID)
			continue
		}

		// Handle result event
		if event.Type == codec.EventTypeResult {
			resp, err := codec.DecodeResponse(event)
			if err != nil {
				return nil, WrapError(err)
			}

			// Track token usage if tracker is configured
			if m.tokenTracker != nil && resp != nil {
				m.tokenTracker.Add(resp.Usage)
			}

			if resp != nil {
				resp.Warnings = append(resp.Warnings, callOptionWarnings(opts)...)
			}

			return resp, nil
		}

		// Skip other events (assistant, stream_event) for non-streaming Generate
	}

	if err := scanner.Err(); err != nil {
		stderrMsg := drainStderr(proc)
		m.logger.Error("error reading CLI output", "error", err, "stderr", stderrMsg)
		if stderrMsg != "" {
			return nil, WrapError(fmt.Errorf("CLI process error (stderr: %s): %w", stderrMsg, err))
		}
		return nil, WrapError(fmt.Errorf("error reading CLI output: %w", err))
	}

	stderrMsg := drainStderr(proc)
	if stderrMsg != "" {
		m.logger.Warn("CLI exited without result", "stderr", stderrMsg)
		return nil, WrapError(fmt.Errorf("CLI exited without returning a result (stderr: %s)", stderrMsg))
	}
	return nil, WrapError(fmt.Errorf("CLI exited without returning a result"))
}

// Stream generates a streaming response from the model.
func (m *LanguageModel) Stream(
	ctx context.Context, prompt []api.Message, opts api.CallOptions,
) (*api.StreamResponse, error) {
	// Extract system prompt from messages
	systemPrompt, messages := codec.ExtractSystemPrompt(prompt)

	// Build config and ensure process
	cfg := m.buildConfig(systemPrompt, opts)
	if err := m.ensureProcess(ctx, cfg); err != nil {
		return nil, err
	}

	m.mu.Lock()
	proc := m.proc
	m.mu.Unlock()

	// Send messages to the CLI
	for _, msg := range messages {
		encoded, err := codec.EncodeMessage(msg)
		if err != nil {
			return nil, WrapError(fmt.Errorf("failed to encode message: %w", err))
		}
		if _, err := proc.Stdin().Write(append(encoded, '\n')); err != nil {
			stderrMsg := drainStderr(proc)
			m.logger.Error("failed to write to CLI stdin", "error", err, "stderr", stderrMsg, "processRunning", proc.IsRunning())
			if stderrMsg != "" {
				return nil, WrapError(fmt.Errorf("CLI process error (stderr: %s): %w", stderrMsg, err))
			}
			return nil, WrapError(fmt.Errorf("failed to write to CLI stdin (process running=%v): %w", proc.IsRunning(), err))
		}
	}

	// Create the stream decoder
	decoder := &streamDecoder{
		proc:         proc,
		logger:       m.logger,
		tokenTracker: m.tokenTracker,
	}

	return &api.StreamResponse{
		Stream: decoder.decodeEvents(),
	}, nil
}

// IsProcessRunning returns true if the underlying CLI process is running.
func (m *LanguageModel) IsProcessRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.proc != nil && m.proc.IsRunning()
}

// RestartProcess stops the current process and starts a new one.
// The new process will be started on the next Generate or Stream call.
func (m *LanguageModel) RestartProcess(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.proc != nil {
		// Save the session ID for --resume on the next process start
		sessionID := m.proc.SessionID()
		m.logger.Info("restarting process", "resumeSessionID", sessionID)
		if err := m.proc.Stop(); err != nil {
			return err
		}
		m.proc = nil
		m.resumeSessionID = sessionID
	}
	return nil
}

// TokenUsage returns cumulative token usage for this model instance.
func (m *LanguageModel) TokenUsage() api.CLITokenUsage {
	if m.tokenTracker == nil {
		return api.CLITokenUsage{}
	}
	u := m.tokenTracker.Usage()
	return api.CLITokenUsage{
		InputTokens:  u.InputTokens,
		OutputTokens: u.OutputTokens,
		TotalTokens:  u.TotalTokens,
		CachedTokens: u.CachedTokens,
	}
}

// ResetTokenUsage clears the cumulative token counters for this model instance.
func (m *LanguageModel) ResetTokenUsage() {
	if m.tokenTracker != nil {
		m.tokenTracker.Reset()
	}
}

// ConfigNeedsRestart returns true if the given call options would require restarting the underlying process.
// Currently, only temperature changes require a restart.
func (m *LanguageModel) ConfigNeedsRestart(opts api.CallOptions) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.proc == nil || !m.proc.IsRunning() {
		return false
	}

	var temp float64
	hasTemp := opts.Temperature != nil
	if hasTemp {
		temp = *opts.Temperature
	}

	return m.cachedKey.Temperature != temp || m.cachedKey.HasTemp != hasTemp
}

// Close stops the underlying process.
func (m *LanguageModel) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.proc != nil {
		return m.proc.Stop()
	}
	return nil
}

// streamDecoder maintains state while decoding a stream of Claude Code CLI events.
type streamDecoder struct {
	proc         process.Process
	logger       *slog.Logger
	sessionID    string
	responseID   string
	modelID      string
	usage        api.Usage
	tokenTracker *cli.TokenTracker
}

// decodeEvents returns an iterator that yields events from the CLI stream.
func (d *streamDecoder) decodeEvents() iter.Seq[api.StreamEvent] {
	return func(yield func(api.StreamEvent) bool) {
		scanner := bufio.NewScanner(d.proc.Stdout())

		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}

			event, err := codec.ParseEvent(line)
			if err != nil {
				if !yield(&api.ErrorEvent{Err: WrapError(fmt.Errorf("failed to parse event: %w", err))}) {
					return
				}
				continue
			}

			// Handle system init - capture session ID
			if event.Type == codec.EventTypeSystem && event.Subtype == "init" {
				d.sessionID = event.SessionID
				d.proc.SetSessionID(event.SessionID)
				d.logger.Debug("stream session initialized", "sessionID", event.SessionID)
				continue
			}

			// Handle result event - this is the final event
			if event.Type == codec.EventTypeResult {
				// Check for errors
				if event.IsError || event.Subtype == "error" {
					yield(&api.ErrorEvent{Err: WrapError(fmt.Errorf("claude code error: %s", event.Result))})
					return
				}

				// Extract usage from result
				if event.Usage != nil {
					d.usage = api.Usage{
						InputTokens:       event.Usage.InputTokens,
						OutputTokens:      event.Usage.OutputTokens,
						TotalTokens:       event.Usage.InputTokens + event.Usage.OutputTokens,
						CachedInputTokens: event.Usage.CacheReadInputTokens,
					}

					// Track token usage if tracker is configured
					if d.tokenTracker != nil {
						d.tokenTracker.Add(d.usage)
					}
				}

				// Create provider metadata
				metadata := codec.Metadata{
					SessionID:        event.SessionID,
					StructuredOutput: event.StructuredOutput,
					CostUSD:          event.TotalCostUSD,
					DurationMS:       event.DurationMS,
					DurationAPIMS:    event.DurationAPIMS,
					NumTurns:         event.NumTurns,
				}
				if event.Usage != nil {
					metadata.Usage = codec.Usage{
						InputTokens:              event.Usage.InputTokens,
						OutputTokens:             event.Usage.OutputTokens,
						CacheCreationInputTokens: event.Usage.CacheCreationInputTokens,
						CacheReadInputTokens:     event.Usage.CacheReadInputTokens,
					}
				}

				// Determine finish reason
				finishReason := api.FinishReasonUnknown
				if event.Message != nil {
					switch event.Message.StopReason {
					case "end_turn", "stop_sequence":
						finishReason = api.FinishReasonStop
					case "tool_use":
						finishReason = api.FinishReasonToolCalls
					case "max_tokens":
						finishReason = api.FinishReasonLength
					}
				} else if event.Subtype == "success" {
					finishReason = api.FinishReasonStop
				}

				// Yield final finish event
				yield(&api.FinishEvent{
					FinishReason: finishReason,
					Usage:        d.usage,
					ProviderMetadata: api.NewProviderMetadata(map[string]any{
						codec.ProviderName: &metadata,
					}),
				})
				return
			}

			// Handle stream events
			if event.Type == codec.EventTypeStreamEvent {
				streamEvent, err := codec.DecodeStreamEvent(event)
				if err != nil {
					if !yield(&api.ErrorEvent{Err: WrapError(err)}) {
						return
					}
					continue
				}

				// Skip nil events (e.g., content_block_stop)
				if streamEvent == nil {
					continue
				}

				// Capture response metadata from message_start
				if metadata, ok := streamEvent.(*api.ResponseMetadataEvent); ok {
					d.responseID = metadata.ID
					d.modelID = metadata.ModelID
				}

				if !yield(streamEvent) {
					return
				}
			}

			// Skip assistant events for streaming (they contain cumulative content)
		}

		if err := scanner.Err(); err != nil {
			stderrMsg := drainStderr(d.proc)
			d.logger.Error("error reading CLI stream", "error", err, "stderr", stderrMsg)
			if stderrMsg != "" {
				yield(&api.ErrorEvent{Err: WrapError(fmt.Errorf("CLI process error (stderr: %s): %w", stderrMsg, err))})
			} else {
				yield(&api.ErrorEvent{Err: WrapError(fmt.Errorf("error reading CLI output: %w", err))})
			}
		}
	}
}
