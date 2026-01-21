package codex

import (
	"fmt"
	"sync"
	"time"

	"go.jetify.com/ai/api"
)

// CostAlert represents a notification when cost thresholds are exceeded.
type CostAlert struct {
	// Used is the number of tokens used so far.
	Used int
	// Budget is the total token budget.
	Budget int
	// Percent is the percentage of budget used (0.0-1.0).
	Percent float64
	// Timestamp is when the alert was triggered.
	Timestamp time.Time
	// AlertType indicates the severity of the alert.
	AlertType AlertType
}

// AlertType indicates the severity of a cost alert.
type AlertType string

const (
	// AlertTypeWarning indicates approaching budget limit.
	AlertTypeWarning AlertType = "warning"
	// AlertTypeCritical indicates budget limit reached or exceeded.
	AlertTypeCritical AlertType = "critical"
)

// CostMonitor tracks token usage and enforces budget limits.
type CostMonitor struct {
	mu sync.RWMutex

	// budget is the maximum number of tokens allowed.
	budget int
	// used is the cumulative number of tokens consumed.
	used int
	// inputTokens tracks input tokens separately.
	inputTokens int
	// outputTokens tracks output tokens separately.
	outputTokens int

	// warningThreshold triggers a warning alert (0.0-1.0).
	warningThreshold float64
	// warningFired tracks if warning has been sent to avoid spam.
	warningFired bool

	// onAlert is called when thresholds are exceeded.
	onAlert func(CostAlert)

	// history tracks usage over time.
	history []UsageRecord

	// startTime marks when monitoring started.
	startTime time.Time
}

// UsageRecord represents a single usage event for history tracking.
type UsageRecord struct {
	Timestamp    time.Time
	InputTokens  int
	OutputTokens int
	TotalTokens  int
}

// CostMonitorOption configures a CostMonitor.
type CostMonitorOption func(*CostMonitor)

// WithWarningThreshold sets the threshold for warning alerts (0.0-1.0).
// Default is 0.8 (80% of budget).
func WithWarningThreshold(threshold float64) CostMonitorOption {
	return func(m *CostMonitor) {
		if threshold > 0.0 && threshold < 1.0 {
			m.warningThreshold = threshold
		}
	}
}

// WithAlertCallback sets a callback function to be called when alerts are triggered.
func WithAlertCallback(callback func(CostAlert)) CostMonitorOption {
	return func(m *CostMonitor) {
		m.onAlert = callback
	}
}

// NewCostMonitor creates a new cost monitor with the specified token budget.
func NewCostMonitor(budget int, opts ...CostMonitorOption) *CostMonitor {
	m := &CostMonitor{
		budget:           budget,
		warningThreshold: 0.8, // Default: warn at 80%
		startTime:        time.Now(),
		history:          make([]UsageRecord, 0, 100),
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
}

// TrackUsage records token usage and checks against budget limits.
// Returns an error if the budget is exceeded.
func (m *CostMonitor) TrackUsage(usage api.Usage) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Update cumulative usage
	m.used += usage.TotalTokens
	m.inputTokens += usage.InputTokens
	m.outputTokens += usage.OutputTokens

	// Record in history
	m.history = append(m.history, UsageRecord{
		Timestamp:    time.Now(),
		InputTokens:  usage.InputTokens,
		OutputTokens: usage.OutputTokens,
		TotalTokens:  usage.TotalTokens,
	})

	// Check warning threshold
	percent := m.percentUsed()
	if percent >= m.warningThreshold && !m.warningFired {
		m.warningFired = true
		if m.onAlert != nil {
			m.onAlert(CostAlert{
				Used:      m.used,
				Budget:    m.budget,
				Percent:   percent,
				Timestamp: time.Now(),
				AlertType: AlertTypeWarning,
			})
		}
	}

	// Check budget exceeded
	if m.used >= m.budget {
		if m.onAlert != nil {
			m.onAlert(CostAlert{
				Used:      m.used,
				Budget:    m.budget,
				Percent:   percent,
				Timestamp: time.Now(),
				AlertType: AlertTypeCritical,
			})
		}
		return &ErrorInfo{
			Category:        CategoryQuotaExceeded,
			UserMessage:     fmt.Sprintf("Token budget exceeded: used %d/%d tokens", m.used, m.budget),
			SuggestedAction: "Increase budget or reduce usage",
			Retryable:       false,
		}
	}

	return nil
}

// Usage returns current usage statistics.
func (m *CostMonitor) Usage() UsageStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return UsageStats{
		Used:         m.used,
		Budget:       m.budget,
		Remaining:    m.budget - m.used,
		InputTokens:  m.inputTokens,
		OutputTokens: m.outputTokens,
		Percent:      m.percentUsed(),
		Duration:     time.Since(m.startTime),
	}
}

// UsageStats provides detailed usage statistics.
type UsageStats struct {
	Used         int
	Budget       int
	Remaining    int
	InputTokens  int
	OutputTokens int
	Percent      float64
	Duration     time.Duration
}

// percentUsed calculates the percentage of budget used (0.0-1.0).
func (m *CostMonitor) percentUsed() float64 {
	if m.budget == 0 {
		return 0.0
	}
	return float64(m.used) / float64(m.budget)
}

// Reset resets the usage counter while keeping the budget.
func (m *CostMonitor) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.used = 0
	m.inputTokens = 0
	m.outputTokens = 0
	m.warningFired = false
	m.history = make([]UsageRecord, 0, 100)
	m.startTime = time.Now()
}

// History returns the usage history.
func (m *CostMonitor) History() []UsageRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy to prevent external modification
	historyCopy := make([]UsageRecord, len(m.history))
	copy(historyCopy, m.history)
	return historyCopy
}

// RemainingBudget returns the number of tokens remaining in the budget.
func (m *CostMonitor) RemainingBudget() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	remaining := m.budget - m.used
	if remaining < 0 {
		return 0
	}
	return remaining
}

// CanAfford checks if the budget can accommodate the estimated tokens.
func (m *CostMonitor) CanAfford(estimatedTokens int) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return (m.used + estimatedTokens) <= m.budget
}
