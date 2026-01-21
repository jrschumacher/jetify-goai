package claudecode

import (
	"bufio"
	"context"
	"fmt"
	"iter"

	"go.jetify.com/ai/api"
	"go.jetify.com/ai/provider/anthropic-claudecode/codec"
	"go.jetify.com/ai/provider/anthropic-claudecode/process"
)

// ModelOption is a function type that modifies a LanguageModel.
type ModelOption func(*LanguageModel)

// WithProcess sets a custom process (useful for testing with mocks).
func WithProcess(proc process.Process) ModelOption {
	return func(m *LanguageModel) {
		m.proc = proc
	}
}

// LanguageModel represents a Claude Code language model.
type LanguageModel struct {
	modelID string
	proc    process.Process
}

var _ api.LanguageModel = &LanguageModel{}

// NewLanguageModel creates a new Claude Code language model.
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
// Claude CLI doesn't support direct URL loading, so we return empty.
func (m *LanguageModel) SupportedUrls() []api.SupportedURL {
	return nil
}

// Generate generates a response from the model.
func (m *LanguageModel) Generate(
	ctx context.Context, prompt []api.Message, opts api.CallOptions,
) (*api.Response, error) {
	// Extract system prompt from messages
	systemPrompt, messages := codec.ExtractSystemPrompt(prompt)

	// Get or create process
	proc := m.proc
	ownsProc := false
	if proc == nil {
		procOpts := []process.Option{
			process.WithModel(m.modelID),
		}
		if systemPrompt != "" {
			procOpts = append(procOpts, process.WithSystemPrompt(systemPrompt))
		}
		if opts.Temperature != nil {
			procOpts = append(procOpts, process.WithTemperature(*opts.Temperature))
		}
		proc = process.NewCLIProcess(procOpts...)
		ownsProc = true
	}

	// Ensure cleanup of locally-created process
	if ownsProc {
		defer proc.Stop()
	}

	// Start the process if not running
	if !proc.IsRunning() {
		if err := proc.Start(ctx); err != nil {
			return nil, fmt.Errorf("failed to start CLI process: %w", err)
		}
	}

	// Send messages to the CLI
	stdin := proc.Stdin()
	if stdin == nil {
		return nil, fmt.Errorf("process stdin not available")
	}
	for _, msg := range messages {
		encoded, err := codec.EncodeMessage(msg)
		if err != nil {
			return nil, fmt.Errorf("failed to encode message: %w", err)
		}
		if _, err := stdin.Write(append(encoded, '\n')); err != nil {
			return nil, fmt.Errorf("failed to write to CLI: %w", err)
		}
	}

	// Read events from stdout until we get a result
	stdout := proc.Stdout()
	if stdout == nil {
		return nil, fmt.Errorf("process stdout not available")
	}
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		event, err := codec.ParseEvent(line)
		if err != nil {
			return nil, fmt.Errorf("failed to parse event: %w", err)
		}

		// Handle system init - capture session ID
		if event.Type == codec.EventTypeSystem && event.Subtype == "init" {
			proc.SetSessionID(event.SessionID)
			continue
		}

		// Handle result event
		if event.Type == codec.EventTypeResult {
			return codec.DecodeResponse(event)
		}

		// Skip other events (assistant, stream_event) for non-streaming Generate
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading CLI output: %w", err)
	}

	return nil, fmt.Errorf("CLI exited without returning a result")
}

// Stream generates a streaming response from the model.
func (m *LanguageModel) Stream(
	ctx context.Context, prompt []api.Message, opts api.CallOptions,
) (*api.StreamResponse, error) {
	// Extract system prompt from messages
	systemPrompt, messages := codec.ExtractSystemPrompt(prompt)

	// Get or create process
	proc := m.proc
	ownsProc := false
	if proc == nil {
		procOpts := []process.Option{
			process.WithModel(m.modelID),
		}
		if systemPrompt != "" {
			procOpts = append(procOpts, process.WithSystemPrompt(systemPrompt))
		}
		if opts.Temperature != nil {
			procOpts = append(procOpts, process.WithTemperature(*opts.Temperature))
		}
		proc = process.NewCLIProcess(procOpts...)
		ownsProc = true
	}

	// Start the process if not running
	if !proc.IsRunning() {
		if err := proc.Start(ctx); err != nil {
			if ownsProc {
				proc.Stop()
			}
			return nil, fmt.Errorf("failed to start CLI process: %w", err)
		}
	}

	// Send messages to the CLI
	stdin := proc.Stdin()
	if stdin == nil {
		if ownsProc {
			proc.Stop()
		}
		return nil, fmt.Errorf("process stdin not available")
	}
	for _, msg := range messages {
		encoded, err := codec.EncodeMessage(msg)
		if err != nil {
			if ownsProc {
				proc.Stop()
			}
			return nil, fmt.Errorf("failed to encode message: %w", err)
		}
		if _, err := stdin.Write(append(encoded, '\n')); err != nil {
			if ownsProc {
				proc.Stop()
			}
			return nil, fmt.Errorf("failed to write to CLI: %w", err)
		}
	}

	// Create the stream decoder
	decoder := &streamDecoder{
		ctx:      ctx,
		proc:     proc,
		ownsProc: ownsProc,
	}

	return &api.StreamResponse{
		Stream: decoder.decodeEvents(),
	}, nil
}

// streamDecoder maintains state while decoding a stream of Claude Code CLI events.
type streamDecoder struct {
	ctx        context.Context
	proc       process.Process
	ownsProc   bool // true if we should stop the process when done
	sessionID  string
	responseID string
	modelID    string
	usage      api.Usage
}

// decodeEvents returns an iterator that yields events from the CLI stream.
func (d *streamDecoder) decodeEvents() iter.Seq[api.StreamEvent] {
	return func(yield func(api.StreamEvent) bool) {
		// Ensure cleanup when iterator finishes (either normally or early exit)
		if d.ownsProc {
			defer d.proc.Stop()
		}

		// Check for context cancellation before starting
		if err := d.ctx.Err(); err != nil {
			yield(&api.ErrorEvent{Err: fmt.Errorf("context cancelled: %w", err)})
			return
		}

		stdout := d.proc.Stdout()
		if stdout == nil {
			yield(&api.ErrorEvent{Err: fmt.Errorf("process stdout not available")})
			return
		}
		scanner := bufio.NewScanner(stdout)

		for scanner.Scan() {
			// Check for context cancellation on each iteration
			if err := d.ctx.Err(); err != nil {
				yield(&api.ErrorEvent{Err: fmt.Errorf("context cancelled: %w", err)})
				return
			}
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}

			event, err := codec.ParseEvent(line)
			if err != nil {
				if !yield(&api.ErrorEvent{Err: fmt.Errorf("failed to parse event: %w", err)}) {
					return
				}
				continue
			}

			// Handle system init - capture session ID
			if event.Type == codec.EventTypeSystem && event.Subtype == "init" {
				d.sessionID = event.SessionID
				d.proc.SetSessionID(event.SessionID)
				continue
			}

			// Handle result event - this is the final event
			if event.Type == codec.EventTypeResult {
				// Check for errors
				if event.IsError || event.Subtype == "error" {
					yield(&api.ErrorEvent{Err: fmt.Errorf("claude code error: %s", event.Result)})
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
					if !yield(&api.ErrorEvent{Err: err}) {
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
			// Check if this was due to context cancellation
			if ctxErr := d.ctx.Err(); ctxErr != nil {
				yield(&api.ErrorEvent{Err: fmt.Errorf("context cancelled: %w", ctxErr)})
			} else {
				yield(&api.ErrorEvent{Err: fmt.Errorf("error reading CLI output: %w", err)})
			}
		}
	}
}
