package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"sync"

	"go.jetify.com/ai/api"
	"go.jetify.com/ai/provider/internal/cli"
	"go.jetify.com/ai/provider/openai-codex/codec"
	"go.jetify.com/ai/provider/openai-codex/codec/jsonrpc"
	"go.jetify.com/ai/provider/openai-codex/process"
)

// ProviderName is the identifier for this provider.
const ProviderName = "openai-codex"

// ModelOption is a function type that modifies a LanguageModel.
type ModelOption func(*LanguageModel)

// WithAppServer sets a custom app-server process (useful for testing with mocks).
func WithAppServer(proc process.AppServer) ModelOption {
	return func(m *LanguageModel) {
		m.proc = proc
	}
}

// WithCostMonitor sets a cost monitor for tracking and limiting token usage.
func WithCostMonitor(monitor *CostMonitor) ModelOption {
	return func(m *LanguageModel) {
		m.costMonitor = monitor
	}
}

// WithTokenTracker sets a token tracker for monitoring token usage.
func WithTokenTracker(tracker *cli.TokenTracker) ModelOption {
	return func(m *LanguageModel) {
		m.tokenTracker = tracker
	}
}

// WithRetryPolicy sets a custom retry policy for handling transient failures.
func WithRetryPolicy(policy *RetryPolicy) ModelOption {
	return func(m *LanguageModel) {
		m.retryPolicy = policy
	}
}

// LanguageModel represents a Codex CLI language model using app-server mode.
// It maintains a persistent process for efficient streaming and multi-turn conversations.
type LanguageModel struct {
	mu sync.Mutex

	// callMu serializes calls that consume the shared app-server notification stream.
	// The Codex CLI protocol does not provide sufficient correlation fields to safely
	// demultiplex concurrent turns, so a single LanguageModel instance is single-flight.
	callMu sync.Mutex

	modelID string
	proc    process.AppServer

	// cachedKey tracks the config used to start the current process.
	// If options change, the process is restarted.
	cachedKey cli.ConfigKey

	// costMonitor tracks token usage and enforces budget limits.
	costMonitor *CostMonitor

	// tokenTracker tracks token usage across the session.
	tokenTracker *cli.TokenTracker

	// retryPolicy defines how operations should be retried on failure.
	retryPolicy *RetryPolicy
}

var (
	_ api.LanguageModel    = &LanguageModel{}
	_ api.CLILanguageModel = &LanguageModel{}
	_ api.CLITokenTracker  = &LanguageModel{}
	_ api.CLIConfigAware   = &LanguageModel{}
)

// NewLanguageModel creates a new Codex CLI language model.
func NewLanguageModel(modelID string, opts ...ModelOption) *LanguageModel {
	model := &LanguageModel{
		modelID: modelID,
	}

	for _, opt := range opts {
		opt(model)
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
// Codex CLI doesn't support direct URL loading, so we return empty.
func (m *LanguageModel) SupportedUrls() []api.SupportedURL {
	return nil
}

// buildConfig creates a process config from the current options.
func (m *LanguageModel) buildConfig(systemPrompt string, opts api.CallOptions) *process.Config {
	cfg := &process.Config{
		Model:        m.modelID,
		SystemPrompt: systemPrompt,
	}
	return cfg
}

// ensureProcess ensures a running app-server process with the correct config.
// If the config has changed, the existing process is stopped and a new one started.
func (m *LanguageModel) ensureProcess(ctx context.Context, cfg *process.Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	newKey := cfg.ConfigKey()

	// Check if we need to restart due to config change
	if m.proc != nil && m.proc.IsRunning() {
		if cli.ConfigKeysEqual(m.cachedKey, newKey) {
			return nil // Config unchanged, reuse existing process
		}
		// Config changed, stop the old process
		m.proc.Stop()
		m.proc = nil
	}

	// Create new process if needed
	if m.proc == nil {
		procOpts := []process.Option{
			process.WithModel(cfg.Model),
		}
		if cfg.SystemPrompt != "" {
			procOpts = append(procOpts, process.WithSystemPrompt(cfg.SystemPrompt))
		}
		if cfg.Temperature != nil {
			procOpts = append(procOpts, process.WithTemperature(*cfg.Temperature))
		}
		m.proc = process.NewAppServerProcess(procOpts...)
	}

	// Start the process if not running, with retry logic if configured
	if !m.proc.IsRunning() {
		startFn := func() error {
			if err := m.proc.Start(ctx); err != nil {
				return WrapError(fmt.Errorf("failed to start app-server: %w", err))
			}

			// Initialize the connection
			if err := m.proc.Initialize(ctx); err != nil {
				m.proc.Stop()
				return WrapError(fmt.Errorf("failed to initialize app-server: %w", err))
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

// Generate generates a response from the model.
func (m *LanguageModel) Generate(
	ctx context.Context, prompt []api.Message, opts api.CallOptions,
) (*api.Response, error) {
	m.callMu.Lock()
	defer m.callMu.Unlock()

	// Extract system prompt and build user prompt
	systemPrompt, userPrompt := codec.BuildPromptWithSystemSeparate(prompt)

	// Build config and ensure process
	cfg := m.buildConfig(systemPrompt, opts)
	if err := m.ensureProcess(ctx, cfg); err != nil {
		return nil, err
	}

	m.mu.Lock()
	proc := m.proc
	m.mu.Unlock()

	// Start a new thread for this request
	threadID, err := proc.StartThread(ctx)
	if err != nil {
		return nil, WrapError(fmt.Errorf("failed to start thread: %w", err))
	}

	// Build input for the turn
	input := []jsonrpc.Input{
		{Type: "text", Text: userPrompt},
	}

	// Start the turn
	if err := proc.StartTurn(ctx, threadID, input, jsonrpc.ApprovalNever); err != nil {
		return nil, WrapError(fmt.Errorf("failed to start turn: %w", err))
	}

	// Collect response from notifications
	var content []api.ContentBlock
	var textBlock *api.TextBlock
	var usage api.Usage
	var reasoning string
	var commandExecutions []codec.CommandExecution
	var fileChanges []codec.FileChange
	commandOutputByItemID := make(map[string]string)

	notifications := proc.Notifications()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()

		case notif, ok := <-notifications:
			if !ok {
				// Channel closed unexpectedly
				return nil, fmt.Errorf("notification channel closed unexpectedly")
			}

			observeNotification(notif, commandOutputByItemID, &commandExecutions, &fileChanges)

			event, err := codec.DecodeNotification(notif)
			if err != nil {
				return nil, fmt.Errorf("failed to decode notification: %w", err)
			}

			// Skip nil events
			if event == nil {
				continue
			}

			switch e := event.(type) {
			case *api.TextDeltaEvent:
				// Accumulate text content
				if textBlock == nil {
					textBlock = &api.TextBlock{Text: e.TextDelta}
					content = append(content, textBlock)
				} else {
					textBlock.Text += e.TextDelta
				}

			case *api.ReasoningEvent:
				reasoning += e.TextDelta

			case *api.FinishEvent:
				usage = e.Usage

				// Track usage if cost monitor is configured
				if m.costMonitor != nil {
					if err := m.costMonitor.TrackUsage(usage); err != nil {
						return nil, err
					}
				}

				// Track token usage if tracker is configured
				if m.tokenTracker != nil {
					m.tokenTracker.Add(usage)
				}

				// Build and return the response
				metadata := &codec.Metadata{
					ThreadID:          threadID,
					Reasoning:         reasoning,
					CommandExecutions: commandExecutions,
					FileChanges:       fileChanges,
				}
				resp := &api.Response{
					Content:      content,
					FinishReason: e.FinishReason,
					Usage:        usage,
					ResponseInfo: &api.ResponseInfo{
						ID: threadID,
					},
					ProviderMetadata: api.NewProviderMetadata(map[string]any{
						codec.ProviderName: metadata,
					}),
				}
				resp.Warnings = append(resp.Warnings, callOptionWarnings(opts)...)

				return resp, nil

			case *api.ErrorEvent:
				if err, ok := e.Err.(error); ok {
					return nil, WrapError(err)
				}
				return nil, WrapError(fmt.Errorf("%v", e.Err))
			}
		}
	}
}

// Stream generates a streaming response from the model.
func (m *LanguageModel) Stream(
	ctx context.Context, prompt []api.Message, opts api.CallOptions,
) (*api.StreamResponse, error) {
	// Extract system prompt and build user prompt
	systemPrompt, userPrompt := codec.BuildPromptWithSystemSeparate(prompt)

	// Build config and ensure process
	cfg := m.buildConfig(systemPrompt, opts)
	if err := m.ensureProcess(ctx, cfg); err != nil {
		return nil, err
	}

	m.mu.Lock()
	proc := m.proc
	m.mu.Unlock()

	// Start a new thread for this request
	threadID, err := proc.StartThread(ctx)
	if err != nil {
		return nil, WrapError(fmt.Errorf("failed to start thread: %w", err))
	}

	// Build input for the turn
	input := []jsonrpc.Input{
		{Type: "text", Text: userPrompt},
	}

	// Start the turn
	if err := proc.StartTurn(ctx, threadID, input, jsonrpc.ApprovalNever); err != nil {
		return nil, WrapError(fmt.Errorf("failed to start turn: %w", err))
	}

	// Create the stream decoder
	decoder := &streamDecoder{
		ctx:          ctx,
		proc:         proc,
		threadID:     threadID,
		costMonitor:  m.costMonitor,
		tokenTracker: m.tokenTracker,
	}

	return &api.StreamResponse{
		Stream: decoder.decodeEvents(),
	}, nil
}

// streamDecoder maintains state while decoding a stream of app-server notifications.
type streamDecoder struct {
	ctx          context.Context
	proc         process.AppServer
	threadID     string
	reasoning    string
	costMonitor  *CostMonitor
	tokenTracker *cli.TokenTracker

	commandExecutions     []codec.CommandExecution
	fileChanges           []codec.FileChange
	commandOutputByItemID map[string]string
}

// decodeEvents returns an iterator that yields events from the notification stream.
func (d *streamDecoder) decodeEvents() iter.Seq[api.StreamEvent] {
	return func(yield func(api.StreamEvent) bool) {
		// Emit initial metadata event
		if !yield(&api.ResponseMetadataEvent{ID: d.threadID}) {
			return
		}

		notifications := d.proc.Notifications()
		if d.commandOutputByItemID == nil {
			d.commandOutputByItemID = make(map[string]string)
		}

		for {
			select {
			case <-d.ctx.Done():
				yield(&api.ErrorEvent{Err: d.ctx.Err()})
				return

			case notif, ok := <-notifications:
				if !ok {
					// Channel closed - emit error if we haven't finished properly
					yield(&api.ErrorEvent{Err: fmt.Errorf("notification stream ended unexpectedly")})
					return
				}

				observeNotification(notif, d.commandOutputByItemID, &d.commandExecutions, &d.fileChanges)

				event, err := codec.DecodeNotification(notif)
				if err != nil {
					if !yield(&api.ErrorEvent{Err: fmt.Errorf("failed to decode notification: %w", err)}) {
						return
					}
					continue
				}

				// Skip nil events
				if event == nil {
					continue
				}

				// Track reasoning for final metadata
				if re, ok := event.(*api.ReasoningEvent); ok {
					d.reasoning += re.TextDelta
				}

				// Enhance finish event with provider metadata
				if finish, ok := event.(*api.FinishEvent); ok {
					// Track usage if cost monitor is configured
					if d.costMonitor != nil {
						if err := d.costMonitor.TrackUsage(finish.Usage); err != nil {
							yield(&api.ErrorEvent{Err: err})
							return
						}
					}

					// Track token usage if tracker is configured
					if d.tokenTracker != nil {
						d.tokenTracker.Add(finish.Usage)
					}

					metadata := &codec.Metadata{
						ThreadID:          d.threadID,
						Reasoning:         d.reasoning,
						CommandExecutions: d.commandExecutions,
						FileChanges:       d.fileChanges,
					}
					finish.ProviderMetadata = api.NewProviderMetadata(map[string]any{
						codec.ProviderName: metadata,
					})
					yield(finish)
					return
				}

				// Wrap error events with classification
				if errEvent, ok := event.(*api.ErrorEvent); ok {
					if err, ok := errEvent.Err.(error); ok {
						errEvent.Err = WrapError(err)
					}
				}

				if !yield(event) {
					return
				}
			}
		}
	}
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
//
// Codex app-server configuration is currently process-scoped and does not support per-call tuning via CallOptions.
func (m *LanguageModel) ConfigNeedsRestart(opts api.CallOptions) bool {
	_ = opts
	return false
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
		if err := m.proc.Stop(); err != nil {
			return err
		}
		m.proc = nil
	}
	return nil
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

type itemCompletedNotification struct {
	Item struct {
		ID   string `json:"id"`
		Type string `json:"type"`

		// commandExecution
		Command  string `json:"command,omitempty"`
		ExitCode *int   `json:"exitCode,omitempty"`

		// fileChange
		Changes []struct {
			Path string `json:"path"`
			Diff string `json:"diff"`
		} `json:"changes,omitempty"`
	} `json:"item"`
}

type commandExecutionOutputDeltaNotification struct {
	ItemID string `json:"itemId"`
	Delta  string `json:"delta"`
}

func observeNotification(
	notif *jsonrpc.Notification,
	commandOutputByItemID map[string]string,
	commandExecutions *[]codec.CommandExecution,
	fileChanges *[]codec.FileChange,
) {
	if notif == nil {
		return
	}

	switch notif.Method {
	case codec.NotifyItemCommandExecutionOutputDelta:
		var p commandExecutionOutputDeltaNotification
		if err := json.Unmarshal(notif.Params, &p); err != nil {
			return
		}
		if p.ItemID == "" || p.Delta == "" {
			return
		}
		commandOutputByItemID[p.ItemID] += p.Delta

	case codec.NotifyItemCompleted:
		var p itemCompletedNotification
		if err := json.Unmarshal(notif.Params, &p); err != nil {
			return
		}
		if p.Item.ID == "" || p.Item.Type == "" {
			return
		}

		switch p.Item.Type {
		case "commandExecution":
			exec := codec.CommandExecution{
				Command: p.Item.Command,
			}
			if p.Item.ExitCode != nil {
				exec.ExitCode = *p.Item.ExitCode
			}
			if output, ok := commandOutputByItemID[p.Item.ID]; ok {
				exec.Output = output
				delete(commandOutputByItemID, p.Item.ID)
			}
			*commandExecutions = append(*commandExecutions, exec)

		case "fileChange":
			for _, change := range p.Item.Changes {
				*fileChanges = append(*fileChanges, codec.FileChange{
					FilePath: change.Path,
					Diff:     change.Diff,
				})
			}
		}
	}
}
