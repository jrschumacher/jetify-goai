package codec

import (
	"encoding/json"
	"errors"
	"fmt"

	"go.jetify.com/ai/api"
	"go.jetify.com/ai/provider/openai-codex/codec/jsonrpc"
)

// App-server notification method constants.
const (
	NotifyItemAgentMessageDelta  = "item/agentMessage/delta"
	NotifyItemReasoningDelta     = "item/reasoning/summaryTextDelta"
	NotifyItemStarted            = "item/started"
	NotifyItemCompleted          = "item/completed"
	NotifyTurnStarted            = "turn/started"
	NotifyTurnCompleted          = "turn/completed"
	NotifyThreadStarted          = "thread/started"
)

// TextDeltaParams contains parameters for item/agentMessage/delta notifications.
type TextDeltaParams struct {
	ItemID string `json:"itemId"`
	Delta  string `json:"delta"`
}

// ReasoningDeltaParams contains parameters for item/reasoning/summaryTextDelta notifications.
type ReasoningDeltaParams struct {
	ItemID       string `json:"itemId"`
	Delta        string `json:"delta"`
	SummaryIndex int    `json:"summaryIndex"`
}

// ItemStartedParams contains parameters for item/started notifications.
type ItemStartedParams struct {
	Item AppServerItem `json:"item"`
}

// ItemCompletedParams contains parameters for item/completed notifications.
type ItemCompletedParams struct {
	Item AppServerItem `json:"item"`
}

// AppServerItem represents an item in app-server notifications.
type AppServerItem struct {
	ID   string `json:"id"`
	Type string `json:"type"` // "agentMessage", "reasoning", etc.
	Text string `json:"text,omitempty"`
}

// TurnStartedParams contains parameters for turn/started notifications.
type TurnStartedParams struct {
	Turn *AppServerTurn `json:"turn,omitempty"`
}

// TurnCompletedParams contains parameters for turn/completed notifications.
type TurnCompletedParams struct {
	Turn  *AppServerTurn   `json:"turn,omitempty"`
	Usage *AppServerUsage  `json:"usage,omitempty"`
}

// AppServerTurn represents a turn in app-server notifications.
type AppServerTurn struct {
	ID       string `json:"id,omitempty"`
	ThreadID string `json:"threadId,omitempty"`
}

// AppServerUsage represents token usage in app-server notifications.
type AppServerUsage struct {
	InputTokens       int `json:"inputTokens,omitempty"`
	OutputTokens      int `json:"outputTokens,omitempty"`
	CachedInputTokens int `json:"cachedInputTokens,omitempty"`
}

// ThreadStartedParams contains parameters for thread/started notifications.
type ThreadStartedParams struct {
	Thread AppServerThread `json:"thread"`
}

// AppServerThread represents a thread in app-server notifications.
type AppServerThread struct {
	ID string `json:"id"`
}

// DecodeNotification converts a JSON-RPC notification to an AI SDK StreamEvent.
// Returns nil for notifications that don't map to SDK stream events.
func DecodeNotification(notif *jsonrpc.Notification) (api.StreamEvent, error) {
	if notif == nil {
		return nil, errors.New("nil notification")
	}

	switch notif.Method {
	case NotifyItemAgentMessageDelta:
		return decodeTextDelta(notif.Params)

	case NotifyItemReasoningDelta:
		return decodeReasoningDelta(notif.Params)

	case NotifyItemStarted:
		return decodeItemStarted(notif.Params)

	case NotifyItemCompleted:
		return decodeAppServerItemCompleted(notif.Params)

	case NotifyTurnStarted:
		// Turn started doesn't map to a SDK event
		return nil, nil

	case NotifyTurnCompleted:
		return decodeTurnCompletedNotif(notif.Params)

	case NotifyThreadStarted:
		return decodeThreadStarted(notif.Params)

	default:
		// Unknown notification type, ignore
		return nil, nil
	}
}

// decodeTextDelta handles item/agentMessage/delta notifications.
func decodeTextDelta(params json.RawMessage) (api.StreamEvent, error) {
	var p TextDeltaParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("failed to parse text delta params: %w", err)
	}

	if p.Delta == "" {
		return nil, nil
	}

	return &api.TextDeltaEvent{
		TextDelta: p.Delta,
	}, nil
}

// decodeReasoningDelta handles item/reasoning/summaryTextDelta notifications.
func decodeReasoningDelta(params json.RawMessage) (api.StreamEvent, error) {
	var p ReasoningDeltaParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("failed to parse reasoning delta params: %w", err)
	}

	if p.Delta == "" {
		return nil, nil
	}

	return &api.ReasoningEvent{
		TextDelta: p.Delta,
	}, nil
}

// decodeItemStarted handles item/started notifications.
func decodeItemStarted(params json.RawMessage) (api.StreamEvent, error) {
	var p ItemStartedParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("failed to parse item started params: %w", err)
	}

	// Item started doesn't produce a direct SDK event, but we track it
	// The actual content comes via delta events or item/completed
	return nil, nil
}

// decodeAppServerItemCompleted handles item/completed notifications.
func decodeAppServerItemCompleted(params json.RawMessage) (api.StreamEvent, error) {
	var p ItemCompletedParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("failed to parse item completed params: %w", err)
	}

	// For streaming, we've already sent deltas, so item/completed
	// is mainly for confirmation. However, if there's text we haven't
	// sent (non-streaming case), we could send it here.
	// For now, return nil as deltas handle the content.
	return nil, nil
}

// decodeTurnCompletedNotif handles turn/completed notifications.
func decodeTurnCompletedNotif(params json.RawMessage) (api.StreamEvent, error) {
	var p TurnCompletedParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("failed to parse turn completed params: %w", err)
	}

	finishEvent := &api.FinishEvent{
		FinishReason: api.FinishReasonStop,
	}

	if p.Usage != nil {
		finishEvent.Usage = api.Usage{
			InputTokens:       p.Usage.InputTokens,
			OutputTokens:      p.Usage.OutputTokens,
			TotalTokens:       p.Usage.InputTokens + p.Usage.OutputTokens,
			CachedInputTokens: p.Usage.CachedInputTokens,
		}
	}

	return finishEvent, nil
}

// decodeThreadStarted handles thread/started notifications.
func decodeThreadStarted(params json.RawMessage) (api.StreamEvent, error) {
	var p ThreadStartedParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("failed to parse thread started params: %w", err)
	}

	return &api.ResponseMetadataEvent{
		ID: p.Thread.ID,
	}, nil
}
