package process

import (
	"context"
	"encoding/json"
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

	args := p.buildArgs()
	p.cmd = exec.CommandContext(ctx, "claude", args...)

	// Set working directory for sandboxing
	if p.config.WorkDir != "" {
		p.cmd.Dir = p.config.WorkDir
	}

	// Set up pipes with proper cleanup on error
	var err error
	p.stdin, err = p.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	p.stdout, err = p.cmd.StdoutPipe()
	if err != nil {
		p.stdin.Close()
		p.stdin = nil
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	p.stderr, err = p.cmd.StderrPipe()
	if err != nil {
		p.stdin.Close()
		p.stdin = nil
		p.stdout.Close()
		p.stdout = nil
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start the process
	if err := p.cmd.Start(); err != nil {
		p.stdin.Close()
		p.stdin = nil
		p.stdout.Close()
		p.stdout = nil
		p.stderr.Close()
		p.stderr = nil
		return fmt.Errorf("failed to start claude CLI: %w", err)
	}

	p.running = true
	return nil
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
	defer p.mu.Unlock()

	if !p.running {
		return nil
	}

	var firstErr error

	// Close stdin to signal EOF to the process
	if p.stdin != nil {
		if err := p.stdin.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("failed to close stdin: %w", err)
		}
		p.stdin = nil
	}

	// Signal the process to terminate
	if p.cmd != nil && p.cmd.Process != nil {
		// Send interrupt signal first for graceful shutdown
		_ = p.cmd.Process.Signal(os.Interrupt)

		// Wait for process to exit in a goroutine with timeout
		done := make(chan error, 1)
		go func() {
			done <- p.cmd.Wait()
		}()

		// Wait up to 5 seconds for graceful shutdown, then force kill
		select {
		case <-done:
			// Process exited gracefully
		case <-time.After(5 * time.Second):
			// Force kill if still running
			_ = p.cmd.Process.Kill()
			<-done // Wait for the goroutine to complete
		}
	}

	// Close remaining pipes
	if p.stdout != nil {
		if err := p.stdout.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("failed to close stdout: %w", err)
		}
		p.stdout = nil
	}

	if p.stderr != nil {
		if err := p.stderr.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("failed to close stderr: %w", err)
		}
		p.stderr = nil
	}

	p.running = false
	p.cmd = nil
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
	cmd := p.cmd
	p.mu.Unlock()

	if cmd == nil {
		return nil
	}
	return cmd.Wait()
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
