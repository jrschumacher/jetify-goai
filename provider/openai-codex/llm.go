package codex

import (
	"bufio"
	"context"
	"fmt"
	"iter"

	"go.jetify.com/ai/api"
	"go.jetify.com/ai/provider/openai-codex/codec"
	"go.jetify.com/ai/provider/openai-codex/process"
)

// ProviderName is the identifier for this provider.
const ProviderName = "openai-codex"

// ModelOption is a function type that modifies a LanguageModel.
type ModelOption func(*LanguageModel)

// WithProcess sets a custom process (useful for testing with mocks).
func WithProcess(proc process.Process) ModelOption {
	return func(m *LanguageModel) {
		m.proc = proc
	}
}

// LanguageModel represents a Codex CLI language model.
type LanguageModel struct {
	modelID string
	proc    process.Process
}

var _ api.LanguageModel = &LanguageModel{}

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

// Generate generates a response from the model.
func (m *LanguageModel) Generate(
	ctx context.Context, prompt []api.Message, opts api.CallOptions,
) (*api.Response, error) {
	// Extract system prompt and build user prompt
	systemPrompt, userPrompt := codec.BuildPromptWithSystemSeparate(prompt)

	// Get or create process
	proc := m.proc
	if proc == nil {
		procOpts := []process.Option{
			process.WithModel(m.modelID),
			process.WithPrompt(userPrompt),
		}
		if systemPrompt != "" {
			procOpts = append(procOpts, process.WithSystemPrompt(systemPrompt))
		}
		proc = process.NewCLIProcess(procOpts...)
	}

	// Start the process
	if !proc.IsRunning() {
		if err := proc.Start(ctx); err != nil {
			return nil, fmt.Errorf("failed to start CLI process: %w", err)
		}
	}

	// Collect events from stdout
	collector := codec.NewEventCollector()
	scanner := bufio.NewScanner(proc.Stdout())

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		event, err := codec.ParseEvent(line)
		if err != nil {
			return nil, fmt.Errorf("failed to parse event: %w", err)
		}

		// Capture thread ID
		if event.Type == codec.EventTypeThreadStarted {
			proc.SetThreadID(event.ThreadID)
		}

		done, err := collector.ProcessEvent(event)
		if err != nil {
			return nil, fmt.Errorf("failed to process event: %w", err)
		}

		if done {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading CLI output: %w", err)
	}

	// Wait for process to finish
	if err := proc.Wait(); err != nil {
		// Only return error if we don't have a response
		resp, buildErr := collector.Build()
		if buildErr != nil {
			return nil, fmt.Errorf("CLI exited with error: %w", err)
		}
		return resp, nil
	}

	return collector.Build()
}

// Stream generates a streaming response from the model.
func (m *LanguageModel) Stream(
	ctx context.Context, prompt []api.Message, opts api.CallOptions,
) (*api.StreamResponse, error) {
	// Extract system prompt and build user prompt
	systemPrompt, userPrompt := codec.BuildPromptWithSystemSeparate(prompt)

	// Get or create process
	proc := m.proc
	if proc == nil {
		procOpts := []process.Option{
			process.WithModel(m.modelID),
			process.WithPrompt(userPrompt),
		}
		if systemPrompt != "" {
			procOpts = append(procOpts, process.WithSystemPrompt(systemPrompt))
		}
		proc = process.NewCLIProcess(procOpts...)
	}

	// Start the process
	if !proc.IsRunning() {
		if err := proc.Start(ctx); err != nil {
			return nil, fmt.Errorf("failed to start CLI process: %w", err)
		}
	}

	// Create the stream decoder
	decoder := &streamDecoder{
		proc: proc,
	}

	return &api.StreamResponse{
		Stream: decoder.decodeEvents(),
	}, nil
}

// streamDecoder maintains state while decoding a stream of Codex CLI events.
type streamDecoder struct {
	proc     process.Process
	threadID string
	usage    api.Usage
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
				if !yield(&api.ErrorEvent{Err: fmt.Errorf("failed to parse event: %w", err)}) {
					return
				}
				continue
			}

			// Convert event to stream event
			streamEvent, err := codec.DecodeStreamEvent(event)
			if err != nil {
				if !yield(&api.ErrorEvent{Err: err}) {
					return
				}
				continue
			}

			// Capture thread ID from metadata events
			if meta, ok := streamEvent.(*api.ResponseMetadataEvent); ok {
				d.threadID = meta.ID
				d.proc.SetThreadID(meta.ID)
			}

			// Skip nil events
			if streamEvent == nil {
				continue
			}

			// Check if this is the final event
			if finish, ok := streamEvent.(*api.FinishEvent); ok {
				// Add provider metadata to finish event
				metadata := &codec.Metadata{
					ThreadID: d.threadID,
				}
				finish.ProviderMetadata = api.NewProviderMetadata(map[string]any{
					codec.ProviderName: metadata,
				})
			}

			if !yield(streamEvent) {
				return
			}
		}

		if err := scanner.Err(); err != nil {
			yield(&api.ErrorEvent{Err: fmt.Errorf("error reading CLI output: %w", err)})
		}
	}
}
