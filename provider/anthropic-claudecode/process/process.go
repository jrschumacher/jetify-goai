package process

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

// CLIProcess is the real implementation of Process that spawns the Claude CLI.
type CLIProcess struct {
	mu sync.Mutex

	config *Config
	cmd    *exec.Cmd

	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser

	running   bool
	sessionID string

	// Sandbox and lifecycle management
	tempDir  string        // Path to auto-created temp directory (empty if not created)
	waitDone chan struct{} // Closed when cmd.Wait() completes (for detecting unexpected exit)
	waitErr  error         // Captured error from cmd.Wait()
}

var _ Process = &CLIProcess{}

// NewCLIProcess creates a new CLI process with the given configuration.
func NewCLIProcess(opts ...Option) *CLIProcess {
	return &CLIProcess{
		config: NewConfig(opts...),
	}
}

// Start implements Process.Start.
func (p *CLIProcess) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running {
		return fmt.Errorf("process already running")
	}

	// Clean up any previous state (allows restart after unexpected exit)
	p.cleanupStateLocked()
	p.waitErr = nil

	// Auto-create sandbox directory if WorkDir not specified
	workDir := p.config.WorkDir
	if workDir == "" {
		tempDir, err := os.MkdirTemp("", "claudecode-*")
		if err != nil {
			return fmt.Errorf("failed to create sandbox directory: %w", err)
		}
		p.tempDir = tempDir
		workDir = tempDir
	}

	args := p.buildArgs()
	p.cmd = exec.CommandContext(ctx, "claude", args...)
	p.cmd.Dir = workDir

	// Set up pipes with proper cleanup on error
	var err error
	p.stdin, err = p.cmd.StdinPipe()
	if err != nil {
		p.cleanupOnStartError()
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	p.stdout, err = p.cmd.StdoutPipe()
	if err != nil {
		p.cleanupOnStartError()
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	p.stderr, err = p.cmd.StderrPipe()
	if err != nil {
		p.cleanupOnStartError()
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start the process
	if err := p.cmd.Start(); err != nil {
		p.cleanupOnStartError()
		return fmt.Errorf("failed to start claude CLI: %w", err)
	}

	// Start monitoring goroutine to detect unexpected exit
	p.waitDone = make(chan struct{})
	go func() {
		err := p.cmd.Wait()
		p.mu.Lock()
		p.waitErr = err
		// Only update state if Stop() hasn't already done so
		if p.running && p.waitDone != nil {
			p.running = false
		}
		p.mu.Unlock()
		close(p.waitDone)
	}()

	p.running = true
	return nil
}

// cleanupStateLocked cleans up any previous process state.
// Must be called with mu held.
func (p *CLIProcess) cleanupStateLocked() {
	// Close any leftover pipes from previous run (ignore errors during cleanup)
	if p.stdin != nil {
		_ = p.stdin.Close()
		p.stdin = nil
	}
	if p.stdout != nil {
		_ = p.stdout.Close()
		p.stdout = nil
	}
	if p.stderr != nil {
		_ = p.stderr.Close()
		p.stderr = nil
	}
	p.cmd = nil
	p.waitDone = nil
	p.waitErr = nil
}

// cleanupOnStartError cleans up resources after a Start() error.
// Must be called with mu held.
func (p *CLIProcess) cleanupOnStartError() {
	// Ignore errors during cleanup - best effort
	if p.stdin != nil {
		_ = p.stdin.Close()
		p.stdin = nil
	}
	if p.stdout != nil {
		_ = p.stdout.Close()
		p.stdout = nil
	}
	if p.stderr != nil {
		_ = p.stderr.Close()
		p.stderr = nil
	}
	// Clean up temp directory on error
	if p.tempDir != "" {
		_ = os.RemoveAll(p.tempDir)
		p.tempDir = ""
	}
	p.cmd = nil
	p.waitErr = nil
}

// buildArgs constructs the CLI arguments from config.
func (p *CLIProcess) buildArgs() []string {
	args := []string{
		"-p", // Print mode
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--tools", "", // Disable built-in tools
	}

	if p.config.Model != "" {
		args = append(args, "--model", p.config.Model)
	}

	if p.config.SystemPrompt != "" {
		args = append(args, "--system-prompt", p.config.SystemPrompt)
	}

	if p.config.Temperature != nil {
		settings := map[string]any{
			"temperature": *p.config.Temperature,
		}
		settingsJSON, _ := json.Marshal(settings)
		args = append(args, "--settings", string(settingsJSON))
	}

	if p.config.JSONSchema != "" {
		args = append(args, "--json-schema", p.config.JSONSchema)
	}

	if p.config.ResumeSessionID != "" {
		args = append(args, "--resume", p.config.ResumeSessionID)
	}

	if p.config.Verbose {
		args = append(args, "--verbose")
	}

	if p.config.MaxTurns > 0 {
		args = append(args, "--max-turns", fmt.Sprintf("%d", p.config.MaxTurns))
	}

	// Include partial messages for streaming
	args = append(args, "--include-partial-messages")

	return args
}

// Stop implements Process.Stop.
func (p *CLIProcess) Stop() error {
	p.mu.Lock()

	if !p.running {
		// Even if not running, clean up temp directory if it exists
		tempDir := p.tempDir
		p.tempDir = ""
		p.mu.Unlock()
		if tempDir != "" {
			_ = os.RemoveAll(tempDir)
		}
		return nil
	}

	var firstErr error

	// Close stdin to signal EOF to the process
	if p.stdin != nil {
		if err := p.stdin.Close(); err != nil && firstErr == nil &&
			!errors.Is(err, os.ErrClosed) && !errors.Is(err, io.ErrClosedPipe) {
			firstErr = fmt.Errorf("failed to close stdin: %w", err)
		}
		p.stdin = nil
	}

	// Signal the process to terminate and wait for monitoring goroutine
	waitDone := p.waitDone
	if p.cmd != nil && p.cmd.Process != nil {
		// Send interrupt signal first for graceful shutdown
		_ = p.cmd.Process.Signal(os.Interrupt)
	}

	// Mark as not running before releasing lock (prevents monitoring goroutine from setting it)
	p.running = false
	p.mu.Unlock()

	// Wait for process to exit via monitoring goroutine (with timeout)
	if waitDone != nil {
		select {
		case <-waitDone:
			// Process exited
		case <-time.After(5 * time.Second):
			// Force kill if still running
			p.mu.Lock()
			if p.cmd != nil && p.cmd.Process != nil {
				_ = p.cmd.Process.Kill()
			}
			p.mu.Unlock()
			<-waitDone // Wait for monitoring goroutine to complete
		}
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Close remaining pipes
	if p.stdout != nil {
		if err := p.stdout.Close(); err != nil && firstErr == nil &&
			!errors.Is(err, os.ErrClosed) && !errors.Is(err, io.ErrClosedPipe) {
			firstErr = fmt.Errorf("failed to close stdout: %w", err)
		}
		p.stdout = nil
	}

	if p.stderr != nil {
		if err := p.stderr.Close(); err != nil && firstErr == nil &&
			!errors.Is(err, os.ErrClosed) && !errors.Is(err, io.ErrClosedPipe) {
			firstErr = fmt.Errorf("failed to close stderr: %w", err)
		}
		p.stderr = nil
	}

	// Clean up temp directory
	if p.tempDir != "" {
		if err := os.RemoveAll(p.tempDir); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("failed to remove sandbox directory: %w", err)
		}
		p.tempDir = ""
	}

	p.cmd = nil
	p.waitDone = nil
	p.waitErr = nil
	return firstErr
}

// Stdin implements Process.Stdin.
// Returns nil if the process is not running.
func (p *CLIProcess) Stdin() io.Writer {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.running {
		return nil
	}
	return p.stdin
}

// Stdout implements Process.Stdout.
// Returns nil if the process is not running.
func (p *CLIProcess) Stdout() io.Reader {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.running {
		return nil
	}
	return p.stdout
}

// Stderr implements Process.Stderr.
// Returns nil if the process is not running.
func (p *CLIProcess) Stderr() io.Reader {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.running {
		return nil
	}
	return p.stderr
}

// Wait implements Process.Wait.
// Blocks until the process exits. Returns nil if the process was never started
// or has already been stopped.
func (p *CLIProcess) Wait() error {
	p.mu.Lock()
	waitDone := p.waitDone
	p.mu.Unlock()

	if waitDone == nil {
		return nil
	}
	<-waitDone

	p.mu.Lock()
	err := p.waitErr
	p.mu.Unlock()
	return err
}

// IsRunning implements Process.IsRunning.
func (p *CLIProcess) IsRunning() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.running
}

// SessionID implements Process.SessionID.
func (p *CLIProcess) SessionID() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.sessionID
}

// SetSessionID implements Process.SetSessionID.
func (p *CLIProcess) SetSessionID(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sessionID = id
}
