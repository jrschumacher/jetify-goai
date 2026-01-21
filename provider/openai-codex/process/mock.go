package process

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"sync"

	"go.jetify.com/ai/provider/openai-codex/codec/jsonrpc"
)

// MockProcess is a mock implementation of Process for testing.
type MockProcess struct {
	mu sync.Mutex

	stdin  *bytes.Buffer
	stdout *bytes.Buffer
	stderr *bytes.Buffer

	running  bool
	threadID string

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

// ThreadID implements Process.ThreadID.
func (m *MockProcess) ThreadID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.threadID
}

// SetThreadID implements Process.SetThreadID.
func (m *MockProcess) SetThreadID(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.threadID = id
}

// WriteStdout writes data to the mock stdout buffer.
// Used by tests to simulate CLI output.
func (m *MockProcess) WriteStdout(data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stdout.Write(data)
}

// WriteStdoutLine writes a line of data to the mock stdout buffer with a newline.
// Used by tests to simulate JSONL output.
func (m *MockProcess) WriteStdoutLine(data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stdout.Write(data)
	m.stdout.WriteByte('\n')
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
	m.threadID = ""
}

// MockAppServer is a mock implementation of AppServer for testing.
type MockAppServer struct {
	MockProcess

	notifications chan *jsonrpc.Notification
	initialized   bool
	nextThreadID  int

	// OnInitialize is called when Initialize is invoked.
	OnInitialize func(ctx context.Context) error

	// OnStartThread is called when StartThread is invoked.
	OnStartThread func(ctx context.Context) (string, error)

	// OnStartTurn is called when StartTurn is invoked.
	OnStartTurn func(ctx context.Context, threadID string, input []jsonrpc.Input, policy jsonrpc.ApprovalPolicy) error
}

var _ AppServer = &MockAppServer{}

// NewMockAppServer creates a new mock app-server process.
func NewMockAppServer() *MockAppServer {
	return &MockAppServer{
		MockProcess: MockProcess{
			stdin:  new(bytes.Buffer),
			stdout: new(bytes.Buffer),
			stderr: new(bytes.Buffer),
		},
		notifications: make(chan *jsonrpc.Notification, 100),
	}
}

// Initialize implements AppServer.Initialize.
func (m *MockAppServer) Initialize(ctx context.Context) error {
	if m.OnInitialize != nil {
		if err := m.OnInitialize(ctx); err != nil {
			return err
		}
	}
	m.initialized = true
	return nil
}

// StartThread implements AppServer.StartThread.
func (m *MockAppServer) StartThread(ctx context.Context) (string, error) {
	if m.OnStartThread != nil {
		return m.OnStartThread(ctx)
	}
	m.nextThreadID++
	return m.threadID, nil
}

// StartTurn implements AppServer.StartTurn.
func (m *MockAppServer) StartTurn(ctx context.Context, threadID string, input []jsonrpc.Input, policy jsonrpc.ApprovalPolicy) error {
	if m.OnStartTurn != nil {
		return m.OnStartTurn(ctx, threadID, input, policy)
	}
	return nil
}

// Notifications implements AppServer.Notifications.
func (m *MockAppServer) Notifications() <-chan *jsonrpc.Notification {
	return m.notifications
}

// Client implements AppServer.Client.
func (m *MockAppServer) Client() *jsonrpc.Client {
	return nil
}

// SendNotification sends a notification to the mock's notification channel.
// Used by tests to simulate app-server notifications.
func (m *MockAppServer) SendNotification(method string, params any) {
	var rawParams json.RawMessage
	if params != nil {
		data, _ := json.Marshal(params)
		rawParams = data
	}
	m.notifications <- &jsonrpc.Notification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  rawParams,
	}
}

// CloseNotifications closes the notification channel.
// Call this after sending all notifications to signal the end of the stream.
func (m *MockAppServer) CloseNotifications() {
	close(m.notifications)
}
