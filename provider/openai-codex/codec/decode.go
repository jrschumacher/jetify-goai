package codec

import (
	"errors"
	"fmt"

	"go.jetify.com/ai/api"
)

// EventCollector collects events from a Codex CLI stream and builds an api.Response.
type EventCollector struct {
	threadID  string
	content   []api.ContentBlock
	usage     api.Usage
	reasoning string
	hasError  bool
	errorMsg  string
}

// NewEventCollector creates a new EventCollector.
func NewEventCollector() *EventCollector {
	return &EventCollector{
		content: make([]api.ContentBlock, 0),
	}
}

// ProcessEvent processes a single event and updates the collector state.
// Returns true if this is the final event (turn.completed or error).
func (c *EventCollector) ProcessEvent(event *Event) (bool, error) {
	if event == nil {
		return false, errors.New("nil event provided")
	}

	switch event.Type {
	case EventTypeThreadStarted:
		c.threadID = event.ThreadID
		return false, nil

	case EventTypeTurnStarted:
		// Nothing to do
		return false, nil

	case EventTypeItemStarted, EventTypeItemUpdated:
		// For non-streaming, we only care about completed items
		return false, nil

	case EventTypeItemCompleted:
		if event.Item == nil {
			return false, nil
		}

		switch event.Item.Type {
		case ItemTypeAgentMessage:
			if event.Item.Text != "" {
				c.content = append(c.content, &api.TextBlock{
					Text: event.Item.Text,
				})
			}
		case ItemTypeReasoning:
			c.reasoning = event.Item.Text
		// Other item types are stored in provider metadata
		}
		return false, nil

	case EventTypeTurnCompleted:
		if event.Usage != nil {
			c.usage = api.Usage{
				InputTokens:       event.Usage.InputTokens,
				OutputTokens:      event.Usage.OutputTokens,
				TotalTokens:       event.Usage.InputTokens + event.Usage.OutputTokens,
				CachedInputTokens: event.Usage.CachedInputTokens,
			}
		}
		return true, nil

	case EventTypeTurnFailed:
		c.hasError = true
		if event.Error != nil {
			c.errorMsg = event.Error.Message
		}
		return true, nil

	case EventTypeError:
		c.hasError = true
		c.errorMsg = event.Message
		if event.Error != nil && c.errorMsg == "" {
			c.errorMsg = event.Error.Message
		}
		return true, nil

	default:
		// Unknown event type, ignore
		return false, nil
	}
}

// Build creates an api.Response from the collected events.
func (c *EventCollector) Build() (*api.Response, error) {
	if c.hasError {
		return nil, fmt.Errorf("Codex error: %s", c.errorMsg)
	}

	metadata := &Metadata{
		ThreadID:  c.threadID,
		Reasoning: c.reasoning,
	}

	response := &api.Response{
		Content:      c.content,
		FinishReason: api.FinishReasonStop,
		Usage:        c.usage,
		ResponseInfo: &api.ResponseInfo{
			ID: c.threadID,
		},
		ProviderMetadata: api.NewProviderMetadata(map[string]any{
			ProviderName: metadata,
		}),
	}

	return response, nil
}

// ThreadID returns the captured thread ID.
func (c *EventCollector) ThreadID() string {
	return c.threadID
}

// DecodeResponse is a convenience function that processes a single turn.completed event
// into an api.Response. For full event collection, use EventCollector.
func DecodeResponse(events []*Event) (*api.Response, error) {
	collector := NewEventCollector()

	for _, event := range events {
		done, err := collector.ProcessEvent(event)
		if err != nil {
			return nil, err
		}
		if done {
			break
		}
	}

	return collector.Build()
}
