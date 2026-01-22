package cli

import (
	"context"
	"sync"
)

// ConfigKey is a comparable struct for detecting configuration changes.
// When configuration changes, CLI-based providers may need to restart their process.
type ConfigKey struct {
	Model        string
	SystemPrompt string
	Temperature  float64
	HasTemp      bool
	// Extra contains provider-specific configuration that affects process lifecycle.
	// Keys and values should be consistent within a provider.
	Extra map[string]string
}

// ConfigComparable is implemented by config structs that can generate a comparable key.
type ConfigComparable interface {
	// ConfigKey returns a comparable key for detecting config changes.
	ConfigKey() ConfigKey
}

// Process represents a CLI process that can be managed.
type Process interface {
	// Start launches the process.
	Start(ctx context.Context) error
	// Stop terminates the process.
	Stop() error
	// IsRunning returns true if the process is running.
	IsRunning() bool
}

// ProcessManager handles config-aware process lifecycle management.
// It tracks the current config and restarts the process when config changes.
type ProcessManager[P Process] struct {
	mu        sync.Mutex
	proc      P
	cachedKey ConfigKey
	// zero is used to check if proc is set
	zero P
}

// NewProcessManager creates a new process manager.
func NewProcessManager[P Process]() *ProcessManager[P] {
	return &ProcessManager[P]{}
}

// GetProcess returns the current process, or nil if none exists.
func (m *ProcessManager[P]) GetProcess() P {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.proc
}

// GetCachedKey returns the current cached config key.
func (m *ProcessManager[P]) GetCachedKey() ConfigKey {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cachedKey
}

// NeedsRestart checks if the process needs to be restarted due to config changes.
// Returns true if the config has changed or if there is no running process.
func (m *ProcessManager[P]) NeedsRestart(cfg ConfigComparable) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	newKey := cfg.ConfigKey()

	// Need restart if no process or config changed
	if any(m.proc) == any(m.zero) {
		return true
	}

	if !m.proc.IsRunning() {
		return true
	}

	return !ConfigKeysEqual(m.cachedKey, newKey)
}

// GetOrCreateProcess returns the current process if it matches the config,
// otherwise creates a new one using the provided factory function.
// If the config has changed, the old process is stopped first.
func (m *ProcessManager[P]) GetOrCreateProcess(
	ctx context.Context,
	cfg ConfigComparable,
	create func() (P, error),
	initialize func(ctx context.Context, p P) error,
) (P, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	newKey := cfg.ConfigKey()

	// Check if we can reuse the existing process
	if any(m.proc) != any(m.zero) && m.proc.IsRunning() {
		if ConfigKeysEqual(m.cachedKey, newKey) {
			return m.proc, nil // Config unchanged, reuse existing process
		}
		// Config changed, stop the old process
		m.proc.Stop()
	}

	// Create new process
	proc, err := create()
	if err != nil {
		return m.zero, err
	}

	// Start the process
	if err := proc.Start(ctx); err != nil {
		return m.zero, err
	}

	// Initialize if provided
	if initialize != nil {
		if err := initialize(ctx, proc); err != nil {
			proc.Stop()
			return m.zero, err
		}
	}

	m.proc = proc
	m.cachedKey = newKey
	return proc, nil
}

// SetProcess manually sets the process and config key.
// This is useful when the process is created externally (e.g., for testing).
func (m *ProcessManager[P]) SetProcess(proc P, key ConfigKey) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.proc = proc
	m.cachedKey = key
}

// Stop stops the current process if running.
func (m *ProcessManager[P]) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if any(m.proc) != any(m.zero) {
		return m.proc.Stop()
	}
	return nil
}

// ConfigKeysEqual compares two ConfigKey structs for equality.
// This handles the Extra map comparison properly.
func ConfigKeysEqual(a, b ConfigKey) bool {
	if a.Model != b.Model ||
		a.SystemPrompt != b.SystemPrompt ||
		a.Temperature != b.Temperature ||
		a.HasTemp != b.HasTemp {
		return false
	}

	// Compare Extra maps
	if len(a.Extra) != len(b.Extra) {
		return false
	}
	for k, v := range a.Extra {
		if bv, ok := b.Extra[k]; !ok || bv != v {
			return false
		}
	}

	return true
}
