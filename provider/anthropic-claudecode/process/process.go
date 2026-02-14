package process

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"sync"
)

// CLIProcess is the real implementation of Process that spawns the Claude CLI.
type CLIProcess struct {
	mu sync.Mutex

	config *Config
	cmd    *exec.Cmd
	logger *slog.Logger

	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser

	running   bool
	waited    bool // true after cmd.Wait() has been called
	sessionID string
}

var _ Process = &CLIProcess{}

// NewCLIProcess creates a new CLI process with the given configuration.
func NewCLIProcess(opts ...Option) *CLIProcess {
	cfg := NewConfig(opts...)
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &CLIProcess{
		config: cfg,
		logger: logger,
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
	p.logger.Info("starting claude CLI", "args", args, "workdir", p.config.WorkDir)
	p.cmd = exec.CommandContext(ctx, "claude", args...)

	// Set working directory for sandboxing
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
		p.logger.Error("failed to start claude CLI", "error", err)
		return fmt.Errorf("failed to start claude CLI: %w", err)
	}

	p.logger.Info("claude CLI started", "pid", p.cmd.Process.Pid)
	p.running = true
	p.waited = false

	// Monitor process exit so IsRunning() reflects reality.
	// Without this goroutine the running flag stays true after the CLI
	// exits (e.g., print-mode exits after producing a result) and
	// ensureProcess incorrectly reuses the dead process.
	go func() {
		err := p.cmd.Wait()
		p.mu.Lock()
		pid := 0
		if p.cmd != nil && p.cmd.Process != nil {
			pid = p.cmd.Process.Pid
		}
		p.running = false
		p.waited = true
		p.mu.Unlock()
		if err != nil {
			p.logger.Warn("claude CLI exited with error", "pid", pid, "error", err)
		} else {
			p.logger.Info("claude CLI exited", "pid", pid)
		}
	}()

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

	if !p.running {
		// Process already stopped (either via Stop or the reaper goroutine).
		// If the reaper already called Wait, nothing to do.
		// If we stopped but haven't waited, reap now to avoid zombies.
		if !p.waited && p.cmd != nil {
			p.waited = true
			cmd := p.cmd
			p.mu.Unlock()
			p.logger.Debug("reaping already-stopped process")
			_ = cmd.Wait()
			return nil
		}
		p.mu.Unlock()
		return nil
	}

	p.logger.Info("stopping claude CLI")
	p.running = false
	stdin := p.stdin
	cmd := p.cmd
	waited := p.waited
	p.mu.Unlock()

	// Close stdin to signal EOF to the process
	if stdin != nil {
		stdin.Close()
	}

	if cmd != nil && cmd.Process != nil {
		// Send interrupt signal
		_ = cmd.Process.Signal(os.Interrupt)

		// Reap the process if the reaper goroutine hasn't already.
		// This prevents zombie processes.
		if !waited {
			_ = cmd.Wait()
			p.mu.Lock()
			p.waited = true
			p.mu.Unlock()
		}
	}

	return nil
}

// Stdin implements Process.Stdin.
func (p *CLIProcess) Stdin() io.Writer {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stdin
}

// Stdout implements Process.Stdout.
func (p *CLIProcess) Stdout() io.Reader {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stdout
}

// Stderr implements Process.Stderr.
func (p *CLIProcess) Stderr() io.Reader {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stderr
}

// Wait implements Process.Wait.
func (p *CLIProcess) Wait() error {
	if p.cmd == nil {
		return nil
	}
	// The reaper goroutine already calls cmd.Wait(). Calling it again
	// is safe (returns the same result) but we track it to avoid races.
	err := p.cmd.Wait()
	p.mu.Lock()
	p.waited = true
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
