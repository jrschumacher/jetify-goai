package cli

import (
	"sync"
	"time"

	"go.jetify.com/ai/api"
)

// TokenUsage represents cumulative token usage across a session.
type TokenUsage struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
	CachedTokens int
}

// TokenAlert represents a notification when token thresholds are exceeded.
type TokenAlert struct {
	// Usage is the current token usage.
	Usage TokenUsage
	// Threshold is the threshold that was exceeded.
	Threshold int
	// Timestamp is when the alert was triggered.
	Timestamp time.Time
}

// TokenTracker tracks token usage across a session.
// It provides thread-safe token accounting and optional threshold alerts.
type TokenTracker struct {
	mu sync.RWMutex

	inputTokens  int
	outputTokens int
	cachedTokens int

	// Thresholds and callbacks
	thresholds []thresholdEntry
	startTime  time.Time
}

type thresholdEntry struct {
	threshold int
	callback  func(TokenAlert)
	fired     bool
}

// TokenTrackerOption configures a TokenTracker.
type TokenTrackerOption func(*TokenTracker)

// WithThresholdAlert adds a threshold alert callback.
// The callback is called once when total tokens exceeds the threshold.
func WithThresholdAlert(threshold int, callback func(TokenAlert)) TokenTrackerOption {
	return func(t *TokenTracker) {
		t.thresholds = append(t.thresholds, thresholdEntry{
			threshold: threshold,
			callback:  callback,
		})
	}
}

// NewTokenTracker creates a new token tracker.
func NewTokenTracker(opts ...TokenTrackerOption) *TokenTracker {
	t := &TokenTracker{
		startTime: time.Now(),
	}

	for _, opt := range opts {
		opt(t)
	}

	return t
}

// Add records token usage from a response.
func (t *TokenTracker) Add(usage api.Usage) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.inputTokens += usage.InputTokens
	t.outputTokens += usage.OutputTokens
	t.cachedTokens += usage.CachedInputTokens

	// Check thresholds
	total := t.inputTokens + t.outputTokens
	for i := range t.thresholds {
		entry := &t.thresholds[i]
		if !entry.fired && total >= entry.threshold {
			entry.fired = true
			if entry.callback != nil {
				entry.callback(TokenAlert{
					Usage:     t.usageUnsafe(),
					Threshold: entry.threshold,
					Timestamp: time.Now(),
				})
			}
		}
	}
}

// AddDirect records token usage directly with individual values.
func (t *TokenTracker) AddDirect(input, output, cached int) {
	t.Add(api.Usage{
		InputTokens:      input,
		OutputTokens:     output,
		CachedInputTokens: cached,
	})
}

// Usage returns the current cumulative token usage.
func (t *TokenTracker) Usage() TokenUsage {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.usageUnsafe()
}

// usageUnsafe returns usage without locking (caller must hold lock).
func (t *TokenTracker) usageUnsafe() TokenUsage {
	return TokenUsage{
		InputTokens:  t.inputTokens,
		OutputTokens: t.outputTokens,
		TotalTokens:  t.inputTokens + t.outputTokens,
		CachedTokens: t.cachedTokens,
	}
}

// Reset clears the usage counters and resets threshold alerts.
func (t *TokenTracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.inputTokens = 0
	t.outputTokens = 0
	t.cachedTokens = 0
	t.startTime = time.Now()

	// Reset threshold fired flags
	for i := range t.thresholds {
		t.thresholds[i].fired = false
	}
}

// Duration returns how long the tracker has been running since last reset.
func (t *TokenTracker) Duration() time.Duration {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return time.Since(t.startTime)
}
