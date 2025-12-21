package process

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
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

	// Close stdin to signal EOF to the process
	if p.stdin != nil {
		p.stdin.Close()
	}

	// Wait for the process to exit
	if p.cmd != nil && p.cmd.Process != nil {
		// Send interrupt signal first
		p.cmd.Process.Signal(os.Interrupt)
	}

	p.running = false
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
	return p.cmd.Wait()
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
