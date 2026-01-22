package cli

import (
	"context"
	"iter"
	"sync"

	"go.jetify.com/ai/api"
)

// ToolCallState tracks the state of an ongoing tool call during streaming.
type ToolCallState struct {
	ID        string
	Name      string
	Arguments string
}

// StreamState provides common state tracking for stream decoding.
// Providers can embed this to share state management logic.
type StreamState struct {
	mu sync.Mutex

	ResponseID   string
	ModelID      string
	InputTokens  int
	OutputTokens int
	FinishReason api.FinishReason

	// OngoingToolCalls tracks tool calls being assembled from deltas.
	// The key is the tool call index.
	OngoingToolCalls map[int]*ToolCallState
}

// NewStreamState creates a new stream state.
func NewStreamState() *StreamState {
	return &StreamState{
		OngoingToolCalls: make(map[int]*ToolCallState),
	}
}

// SetResponseMetadata sets the response ID and model ID.
func (s *StreamState) SetResponseMetadata(id, modelID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ResponseID = id
	s.ModelID = modelID
}

// SetUsage sets the token usage.
func (s *StreamState) SetUsage(input, output int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.InputTokens = input
	s.OutputTokens = output
}

// SetFinishReason sets the finish reason.
func (s *StreamState) SetFinishReason(reason api.FinishReason) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.FinishReason = reason
}

// GetToolCall returns the tool call state for the given index, creating it if needed.
func (s *StreamState) GetToolCall(index int) *ToolCallState {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.OngoingToolCalls[index] == nil {
		s.OngoingToolCalls[index] = &ToolCallState{}
	}
	return s.OngoingToolCalls[index]
}

// AppendToolCallArguments appends to the arguments of a tool call.
func (s *StreamState) AppendToolCallArguments(index int, args string) {
	tc := s.GetToolCall(index)
	s.mu.Lock()
	defer s.mu.Unlock()
	tc.Arguments += args
}

// GetSnapshot returns a snapshot of the current state.
func (s *StreamState) GetSnapshot() StreamStateSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return StreamStateSnapshot{
		ResponseID:   s.ResponseID,
		ModelID:      s.ModelID,
		InputTokens:  s.InputTokens,
		OutputTokens: s.OutputTokens,
		FinishReason: s.FinishReason,
	}
}

// StreamStateSnapshot is an immutable snapshot of stream state.
type StreamStateSnapshot struct {
	ResponseID   string
	ModelID      string
	InputTokens  int
	OutputTokens int
	FinishReason api.FinishReason
}

// CreateFinishEvent creates a FinishEvent from the current state.
func (s *StreamState) CreateFinishEvent(providerMetadata *api.ProviderMetadata) *api.FinishEvent {
	snapshot := s.GetSnapshot()
	return &api.FinishEvent{
		FinishReason: snapshot.FinishReason,
		Usage: api.Usage{
			InputTokens:  snapshot.InputTokens,
			OutputTokens: snapshot.OutputTokens,
			TotalTokens:  snapshot.InputTokens + snapshot.OutputTokens,
		},
		ProviderMetadata: providerMetadata,
	}
}

// StreamIteratorFunc is a function type for creating stream iterators.
// This allows providers to implement their own iteration logic while sharing common patterns.
type StreamIteratorFunc func(yield func(api.StreamEvent) bool)

// WrapStreamIterator wraps a StreamIteratorFunc into an iter.Seq.
func WrapStreamIterator(fn StreamIteratorFunc) iter.Seq[api.StreamEvent] {
	return iter.Seq[api.StreamEvent](fn)
}

// ChannelToStreamIterator creates a stream iterator from a channel of events.
// This is useful for providers like Codex that receive events via channels.
// The decode function converts provider-specific events to api.StreamEvent.
// It returns nil to skip an event, or an error event on decode failure.
func ChannelToStreamIterator[E any](
	ctx context.Context,
	events <-chan E,
	decode func(E) (api.StreamEvent, error),
	onFinish func() *api.FinishEvent,
) iter.Seq[api.StreamEvent] {
	return func(yield func(api.StreamEvent) bool) {
		for {
			select {
			case <-ctx.Done():
				yield(&api.ErrorEvent{Err: ctx.Err()})
				return

			case event, ok := <-events:
				if !ok {
					// Channel closed - emit finish event if provided
					if onFinish != nil {
						if finish := onFinish(); finish != nil {
							yield(finish)
						}
					}
					return
				}

				decoded, err := decode(event)
				if err != nil {
					if !yield(&api.ErrorEvent{Err: err}) {
						return
					}
					continue
				}

				// Skip nil events
				if decoded == nil {
					continue
				}

				// Check if this is a finish event
				if _, isFinish := decoded.(*api.FinishEvent); isFinish {
					yield(decoded)
					return
				}

				if !yield(decoded) {
					return
				}
			}
		}
	}
}
