package codec

import (
	"errors"
	"fmt"
	"io"
	"iter"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"go.jetify.com/ai/api"
)

// StreamReader is an interface for reading from an SSE stream.
// This abstraction makes testing easier as we can mock this interface.
type StreamReader interface {
	Next() bool
	Current() anthropic.BetaRawMessageStreamEventUnion
	Err() error
}

// sseStreamAdapter wraps the anthropic ssestream.Stream to implement StreamReader.
type sseStreamAdapter struct {
	stream *ssestream.Stream[anthropic.BetaRawMessageStreamEventUnion]
}

func (a *sseStreamAdapter) Next() bool {
	return a.stream.Next()
}

func (a *sseStreamAdapter) Current() anthropic.BetaRawMessageStreamEventUnion {
	return a.stream.Current()
}

func (a *sseStreamAdapter) Err() error {
	return a.stream.Err()
}

// DecodeStream converts an Anthropic SSE stream to our API's StreamResponse.
func DecodeStream(stream *ssestream.Stream[anthropic.BetaRawMessageStreamEventUnion]) (*api.StreamResponse, error) {
	adapter := &sseStreamAdapter{stream: stream}
	decoder := &streamDecoder{}
	return decoder.DecodeStream(adapter)
}

// streamDecoder maintains state while decoding a stream of Anthropic events.
type streamDecoder struct {
	// Map from content block index to tool call information
	ongoingToolCalls map[int64]toolCallInfo

	// Tracking response metadata
	responseID string
	modelID    string

	// Usage statistics
	inputTokens  int64
	outputTokens int64

	// Cache token statistics
	cacheCreationInputTokens int64
	cacheReadInputTokens     int64

	// Flags
	hasToolCalls bool

	// Stop reason for finish event
	stopReason anthropic.BetaStopReason
}

// toolCallInfo tracks information about an ongoing tool call.
type toolCallInfo struct {
	toolName   string
	toolCallID string
}

// DecodeStream processes an Anthropic stream and returns our API stream format.
func (d *streamDecoder) DecodeStream(stream StreamReader) (*api.StreamResponse, error) {
	if d.ongoingToolCalls == nil {
		d.ongoingToolCalls = make(map[int64]toolCallInfo)
	}

	return &api.StreamResponse{
		Stream: d.decodeEvents(stream),
	}, nil
}

// decodeEvents returns an iterator that yields events from the Anthropic stream.
func (d *streamDecoder) decodeEvents(stream StreamReader) iter.Seq[api.StreamEvent] {
	return func(yield func(api.StreamEvent) bool) {
		for stream.Next() {
			event := stream.Current()
			decodedEvents := d.decodeEvent(event)

			for _, decodedEvent := range decodedEvents {
				if decodedEvent != nil {
					if !yield(decodedEvent) {
						return
					}
				}
			}
		}

		// Check for stream errors
		if err := stream.Err(); err != nil && !errors.Is(err, io.EOF) {
			if !yield(&api.ErrorEvent{Err: err}) {
				return
			}
		}

		// Create provider metadata
		metadata := api.NewProviderMetadata(map[string]any{
			"anthropic": &Metadata{
				Usage: Usage{
					InputTokens:             d.inputTokens,
					OutputTokens:            d.outputTokens,
					CacheCreationInputTokens: d.cacheCreationInputTokens,
					CacheReadInputTokens:    d.cacheReadInputTokens,
				},
			},
		})

		// Send the final finish event
		yield(&api.FinishEvent{
			FinishReason: decodeFinishReason(d.stopReason),
			Usage: api.Usage{
				InputTokens:       int(d.inputTokens),
				OutputTokens:      int(d.outputTokens),
				TotalTokens:       int(d.inputTokens + d.outputTokens),
				CachedInputTokens: int(d.cacheReadInputTokens),
			},
			ProviderMetadata: metadata,
		})
	}
}

// decodeEvent translates an Anthropic stream event to our API event format.
// It returns a slice of events since some Anthropic events may produce multiple API events.
func (d *streamDecoder) decodeEvent(event anthropic.BetaRawMessageStreamEventUnion) []api.StreamEvent {
	switch event.Type {
	case "message_start":
		return d.decodeMessageStart(event)
	case "message_delta":
		return d.decodeMessageDelta(event)
	case "message_stop":
		// No event to yield, just marks end of message
		return nil
	case "content_block_start":
		return d.decodeContentBlockStart(event)
	case "content_block_delta":
		return d.decodeContentBlockDelta(event)
	case "content_block_stop":
		return d.decodeContentBlockStop(event)
	default:
		// Ignore unknown event types
		return nil
	}
}

// decodeMessageStart handles message_start events
func (d *streamDecoder) decodeMessageStart(event anthropic.BetaRawMessageStreamEventUnion) []api.StreamEvent {
	startEvent := event.AsMessageStart()
	msg := startEvent.Message

	d.responseID = msg.ID
	d.modelID = string(msg.Model)

	// Capture initial usage if provided
	d.inputTokens = msg.Usage.InputTokens
	d.outputTokens = msg.Usage.OutputTokens
	d.cacheCreationInputTokens = msg.Usage.CacheCreationInputTokens
	d.cacheReadInputTokens = msg.Usage.CacheReadInputTokens

	return []api.StreamEvent{
		&api.ResponseMetadataEvent{
			ID:      msg.ID,
			ModelID: string(msg.Model),
		},
	}
}

// decodeMessageDelta handles message_delta events
func (d *streamDecoder) decodeMessageDelta(event anthropic.BetaRawMessageStreamEventUnion) []api.StreamEvent {
	deltaEvent := event.AsMessageDelta()

	// Update stop reason - Delta is directly accessible as BetaRawMessageDeltaEventDelta
	d.stopReason = deltaEvent.Delta.StopReason

	// Update usage statistics
	d.outputTokens = deltaEvent.Usage.OutputTokens

	return nil
}

// decodeContentBlockStart handles content_block_start events
func (d *streamDecoder) decodeContentBlockStart(event anthropic.BetaRawMessageStreamEventUnion) []api.StreamEvent {
	startEvent := event.AsContentBlockStart()
	block := startEvent.ContentBlock

	switch block.Type {
	case "tool_use":
		// Store tool call info for later deltas
		d.ongoingToolCalls[startEvent.Index] = toolCallInfo{
			toolName:   block.Name,
			toolCallID: block.ID,
		}
		d.hasToolCalls = true

		// Return initial tool call delta with empty args
		return []api.StreamEvent{
			&api.ToolCallDeltaEvent{
				ToolCallID: block.ID,
				ToolName:   block.Name,
				ArgsDelta:  []byte{},
			},
		}
	case "thinking":
		// Thinking blocks will send deltas, no initial event needed
		return nil
	case "text":
		// Text blocks will send deltas, no initial event needed
		return nil
	default:
		return nil
	}
}

// decodeContentBlockDelta handles content_block_delta events
func (d *streamDecoder) decodeContentBlockDelta(event anthropic.BetaRawMessageStreamEventUnion) []api.StreamEvent {
	deltaEvent := event.AsContentBlockDelta()
	delta := deltaEvent.Delta

	switch delta.Type {
	case "text_delta":
		return []api.StreamEvent{
			&api.TextDeltaEvent{
				TextDelta: delta.Text,
			},
		}
	case "thinking_delta":
		return []api.StreamEvent{
			&api.ReasoningEvent{
				TextDelta: delta.Thinking,
			},
		}
	case "signature_delta":
		return []api.StreamEvent{
			&api.ReasoningSignatureEvent{
				Signature: delta.Signature,
			},
		}
	case "input_json_delta":
		// Tool call argument delta
		toolCall, exists := d.ongoingToolCalls[deltaEvent.Index]
		if !exists {
			return []api.StreamEvent{
				&api.ErrorEvent{Err: fmt.Errorf("received input_json_delta for unknown content block index: %d", deltaEvent.Index)},
			}
		}
		return []api.StreamEvent{
			&api.ToolCallDeltaEvent{
				ToolCallID: toolCall.toolCallID,
				ToolName:   toolCall.toolName,
				ArgsDelta:  []byte(delta.PartialJSON),
			},
		}
	default:
		return nil
	}
}

// decodeContentBlockStop handles content_block_stop events
func (d *streamDecoder) decodeContentBlockStop(event anthropic.BetaRawMessageStreamEventUnion) []api.StreamEvent {
	stopEvent := event.AsContentBlockStop()

	// Check if this was a tool call block
	toolCall, exists := d.ongoingToolCalls[stopEvent.Index]
	if exists {
		delete(d.ongoingToolCalls, stopEvent.Index)
		// We don't emit a final ToolCallEvent here because we've been streaming
		// the arguments via ToolCallDeltaEvent. The consumer should reconstruct
		// the full args from the deltas.
		_ = toolCall // Used for tracking, complete event could be added if needed
	}

	return nil
}

// DecodeStreamForTest allows testing with a mock stream reader.
// This is exported for testing purposes only.
func DecodeStreamForTest(stream StreamReader) (*api.StreamResponse, error) {
	decoder := &streamDecoder{}
	return decoder.DecodeStream(stream)
}
