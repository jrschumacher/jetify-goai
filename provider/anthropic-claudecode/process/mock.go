package process

import (
	"bytes"
	"context"
	"io"
	"sync"
)

// MockProcess is a mock implementation of Process for testing.
type MockProcess struct {
	mu sync.Mutex

	stdin  *bytes.Buffer
	stdout *bytes.Buffer
	stderr *bytes.Buffer

	running   bool
	sessionID string

	// OnStart is called when Start is invoked.
	OnStart func(ctx context.Context) error

	// OnStop is called when Stop is invoked.
	OnStop func() error

	// OnWait is called when Wait is invoked.
	OnWait func() error
}

var _ Process = &MockProcess{}

// NewMockProcess creates a new mock process.
func NewMockProcess() *MockProcess {
	return &MockProcess{
		stdin:  new(bytes.Buffer),
		stdout: new(bytes.Buffer),
		stderr: new(bytes.Buffer),
	}
}

// Start implements Process.Start.
func (m *MockProcess) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.OnStart != nil {
		if err := m.OnStart(ctx); err != nil {
			return err
		}
	}

	m.running = true
	return nil
}

// Stop implements Process.Stop.
func (m *MockProcess) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.OnStop != nil {
		if err := m.OnStop(); err != nil {
			return err
		}
	}

	m.running = false
	return nil
}

// Stdin implements Process.Stdin.
func (m *MockProcess) Stdin() io.Writer {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stdin
}

// Stdout implements Process.Stdout.
func (m *MockProcess) Stdout() io.Reader {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stdout
}

// Stderr implements Process.Stderr.
func (m *MockProcess) Stderr() io.Reader {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stderr
}

// Wait implements Process.Wait.
func (m *MockProcess) Wait() error {
	if m.OnWait != nil {
		return m.OnWait()
	}
	return nil
}

// IsRunning implements Process.IsRunning.
func (m *MockProcess) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// SessionID implements Process.SessionID.
func (m *MockProcess) SessionID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessionID
}

// SetSessionID implements Process.SetSessionID.
func (m *MockProcess) SetSessionID(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessionID = id
}

// WriteStdout writes data to the mock stdout buffer.
// Used by tests to simulate CLI output.
func (m *MockProcess) WriteStdout(data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stdout.Write(data)
}

// WriteStderr writes data to the mock stderr buffer.
// Used by tests to simulate CLI errors.
func (m *MockProcess) WriteStderr(data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stderr.Write(data)
}

// ReadStdin reads data from the mock stdin buffer.
// Used by tests to verify what was sent to the CLI.
func (m *MockProcess) ReadStdin() []byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stdin.Bytes()
}

// Reset clears all buffers and resets state.
func (m *MockProcess) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stdin.Reset()
	m.stdout.Reset()
	m.stderr.Reset()
	m.running = false
	m.sessionID = ""
}
