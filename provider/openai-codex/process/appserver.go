package process

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"go.jetify.com/ai/provider/internal/cli"
	"go.jetify.com/ai/provider/openai-codex/codec/jsonrpc"
)

// AppServerProcess manages a persistent codex app-server subprocess.
type AppServerProcess struct {
	mu sync.Mutex

	config *Config
	cmd    *exec.Cmd

	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser

	client      *jsonrpc.Client
	running     bool
	initialized bool
	reaped      chan struct{} // closed by the reaper goroutine after cmd.Wait() returns
	waitErr     error        // result of cmd.Wait(), valid after reaped is closed
	threadID    string
}

var _ AppServer = &AppServerProcess{}

// NewAppServerProcess creates a new app-server process with the given configuration.
func NewAppServerProcess(opts ...Option) *AppServerProcess {
	return &AppServerProcess{
		config: NewConfig(opts...),
	}
}

// Start launches the codex app-server process.
func (p *AppServerProcess) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running {
		return fmt.Errorf("process already running")
	}

	// Build the command
	p.cmd = exec.CommandContext(ctx, "codex", "app-server")

	// Clean the environment so the spawned CLI doesn't detect a parent
	// session and refuse to start (nested session guard).
	p.cmd.Env = cli.CleanEnv([]string{"CODEX_"}, []string{"CODEX"})

	// Set working directory if configured
	if p.config.WorkDir != "" {
		p.cmd.Dir = p.config.WorkDir
	}

	// Set up pipes
	var err error
	p.stdin, err = p.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	p.stdout, err = p.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	p.stderr, err = p.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start the process
	if err := p.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start codex app-server: %w", err)
	}

	// Create JSON-RPC client
	p.client = jsonrpc.NewClient(p.stdout, p.stdin)

	p.running = true
	p.reaped = make(chan struct{})

	// Monitor process exit so IsRunning() reflects reality.
	// This is the ONLY goroutine that calls cmd.Wait(). All other code
	// that needs to wait for exit blocks on the reaped channel instead.
	go func() {
		waitErr := p.cmd.Wait()
		p.mu.Lock()
		p.running = false
		p.waitErr = waitErr
		p.mu.Unlock()
		// Close the channel AFTER updating state so readers see consistent state.
		close(p.reaped)
	}()

	return nil
}

// Initialize sends the initialize RPC to the server.
func (p *AppServerProcess) Initialize(ctx context.Context) error {
	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		return fmt.Errorf("process not running")
	}
	if p.initialized {
		p.mu.Unlock()
		return nil // Already initialized
	}
	client := p.client
	p.mu.Unlock()

	params := jsonrpc.InitializeParams{
		ClientInfo: jsonrpc.ClientInfo{
			Name:    "goai",
			Version: "1.0",
		},
	}

	result, err := client.Call(ctx, "initialize", params)
	if err != nil {
		return fmt.Errorf("initialize RPC failed: %w", err)
	}

	var initResult jsonrpc.InitializeResult
	if err := json.Unmarshal(result, &initResult); err != nil {
		return fmt.Errorf("failed to parse initialize result: %w", err)
	}

	p.mu.Lock()
	p.initialized = true
	p.mu.Unlock()

	return nil
}

// StartThread creates a new conversation thread.
func (p *AppServerProcess) StartThread(ctx context.Context) (string, error) {
	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		return "", fmt.Errorf("process not running")
	}
	client := p.client
	cfg := *p.config
	p.mu.Unlock()

	params := jsonrpc.ThreadStartParams{
		Model:                 cfg.Model,
		BaseInstructions:      cfg.SystemPrompt,
		DeveloperInstructions: "",
		Cwd:                   cfg.WorkDir,
	}

	if mode, ok := sandboxModeFromString(cfg.SandboxMode); ok {
		params.Sandbox = mode
	}

	// Best-effort config overrides (matches ~/.codex/config.toml layout).
	if len(cfg.MCPServers) > 0 {
		servers := make(map[string]any, len(cfg.MCPServers))
		for name, server := range cfg.MCPServers {
			serverCfg := map[string]any{
				"command": server.Command,
				"args":    server.Args,
			}
			if len(server.Env) > 0 {
				serverCfg["env"] = server.Env
			}
			servers[name] = serverCfg
		}
		params.Config = map[string]any{
			"mcp_servers": servers,
		}
	}

	result, err := client.Call(ctx, "thread/start", params)
	if err != nil {
		return "", fmt.Errorf("thread/start RPC failed: %w", err)
	}

	var threadResult jsonrpc.ThreadStartResult
	if err := json.Unmarshal(result, &threadResult); err != nil {
		return "", fmt.Errorf("failed to parse thread/start result: %w", err)
	}

	p.mu.Lock()
	p.threadID = threadResult.Thread.ID
	p.mu.Unlock()

	return threadResult.Thread.ID, nil
}

// StartTurn begins a new turn in the conversation.
// Returns immediately - responses come via Notifications().
func (p *AppServerProcess) StartTurn(ctx context.Context, threadID string, input []jsonrpc.Input, policy jsonrpc.ApprovalPolicy) error {
	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		return fmt.Errorf("process not running")
	}
	client := p.client
	cfg := *p.config
	p.mu.Unlock()

	params := jsonrpc.TurnStartParams{
		ThreadID:       threadID,
		Input:          input,
		ApprovalPolicy: policy,
		Model:          cfg.Model,
		Cwd:            cfg.WorkDir,
		SandboxPolicy:  sandboxPolicyFromConfig(cfg),
	}

	_, err := client.Call(ctx, "turn/start", params)
	if err != nil {
		return fmt.Errorf("turn/start RPC failed: %w", err)
	}

	return nil
}

// Notifications returns the channel for receiving streaming notifications.
func (p *AppServerProcess) Notifications() <-chan *jsonrpc.Notification {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.client == nil {
		// Return a closed channel if no client
		ch := make(chan *jsonrpc.Notification)
		close(ch)
		return ch
	}

	return p.client.Notifications()
}

// Stop gracefully terminates the process.
func (p *AppServerProcess) Stop() error {
	p.mu.Lock()

	if !p.running {
		reaped := p.reaped
		p.mu.Unlock()
		// If there's a reaper channel, wait for it to finish to avoid zombies.
		if reaped != nil {
			<-reaped
		}
		return nil
	}

	p.running = false
	p.initialized = false
	stdin := p.stdin
	cmd := p.cmd
	reaped := p.reaped
	p.mu.Unlock()

	// Close the JSON-RPC client
	p.mu.Lock()
	if p.client != nil {
		p.client.Close()
	}
	p.mu.Unlock()

	// Close stdin to signal EOF
	if stdin != nil {
		stdin.Close()
	}

	// Send interrupt signal
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Signal(os.Interrupt)
	}

	// Wait for the reaper goroutine to finish cmd.Wait().
	if reaped != nil {
		select {
		case <-reaped:
			// Process fully reaped
		case <-time.After(10 * time.Second):
			// Safety timeout — kill forcefully if interrupt didn't work
			if cmd != nil && cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			<-reaped
		}
	}

	return nil
}

// Stdin returns a writer for sending input to the process.
// Note: For app-server mode, use the JSON-RPC client methods instead.
func (p *AppServerProcess) Stdin() io.Writer {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stdin
}

// Stdout returns a reader for the process output.
// Note: For app-server mode, use Notifications() instead.
func (p *AppServerProcess) Stdout() io.Reader {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stdout
}

// Stderr returns a reader for error output.
func (p *AppServerProcess) Stderr() io.Reader {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stderr
}

// Wait blocks until the process exits.
func (p *AppServerProcess) Wait() error {
	p.mu.Lock()
	reaped := p.reaped
	p.mu.Unlock()
	if reaped == nil {
		return nil
	}
	<-reaped
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.waitErr
}

// IsRunning returns true if the process is currently running.
func (p *AppServerProcess) IsRunning() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.running
}

// ThreadID returns the current thread ID.
func (p *AppServerProcess) ThreadID() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.threadID
}

// SetThreadID sets the thread ID.
func (p *AppServerProcess) SetThreadID(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.threadID = id
}

// Client returns the JSON-RPC client for direct access if needed.
func (p *AppServerProcess) Client() *jsonrpc.Client {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.client
}

func sandboxModeFromString(mode string) (jsonrpc.SandboxMode, bool) {
	if mode == "" {
		return "", false
	}

	normalized := strings.TrimSpace(mode)
	normalized = strings.ToLower(normalized)
	normalized = strings.ReplaceAll(normalized, "_", "-")

	switch normalized {
	case "read-only", "readonly", "readonly-mode":
		return jsonrpc.SandboxReadOnly, true
	case "workspace-write", "workspacewrite", "workspace":
		return jsonrpc.SandboxWorkspaceWrite, true
	case "danger-full-access", "dangerfullaccess", "danger":
		return jsonrpc.SandboxDangerFullAccess, true
	case "no-access":
		// Legacy value - treat as read-only.
		return jsonrpc.SandboxReadOnly, true
	default:
		return "", false
	}
}

func sandboxPolicyFromConfig(cfg Config) *jsonrpc.SandboxPolicy {
	mode, ok := sandboxModeFromString(cfg.SandboxMode)
	if !ok {
		return nil
	}

	switch mode {
	case jsonrpc.SandboxReadOnly:
		return &jsonrpc.SandboxPolicy{Type: string(jsonrpc.SandboxReadOnly)}
	case jsonrpc.SandboxDangerFullAccess:
		return &jsonrpc.SandboxPolicy{Type: string(jsonrpc.SandboxDangerFullAccess)}
	case jsonrpc.SandboxWorkspaceWrite:
		policy := &jsonrpc.SandboxPolicy{Type: string(jsonrpc.SandboxWorkspaceWrite)}
		if cfg.NetworkAccess {
			policy.NetworkAccess = true
		}
		return policy
	default:
		return nil
	}
}
