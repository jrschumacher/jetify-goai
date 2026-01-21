package process

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
)

// CLIProcess is the real implementation of Process that spawns the Codex CLI.
// Unlike Claude Code, Codex uses one-shot execution with `codex exec`.
type CLIProcess struct {
	mu sync.Mutex

	config *Config
	cmd    *exec.Cmd

	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser

	running  bool
	threadID string
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
	p.cmd = exec.CommandContext(ctx, "codex", args...)

	// Set working directory
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
		return fmt.Errorf("failed to start Codex CLI: %w", err)
	}

	p.running = true
	return nil
}

// buildArgs constructs the CLI arguments from config.
func (p *CLIProcess) buildArgs() []string {
	args := []string{"exec"}

	if p.config.JSONOutput {
		args = append(args, "--json")
	}

	if p.config.SkipGitRepoCheck {
		args = append(args, "--skip-git-repo-check")
	}

	if p.config.Model != "" {
		args = append(args, "--model", p.config.Model)
	}

	if p.config.FullAutoMode {
		args = append(args, "--full-auto")
	}

	// Add the prompt as the final argument
	if p.config.Prompt != "" {
		args = append(args, p.config.Prompt)
	}

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

	// Send interrupt signal
	if p.cmd != nil && p.cmd.Process != nil {
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

// ThreadID implements Process.ThreadID.
func (p *CLIProcess) ThreadID() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.threadID
}

// SetThreadID implements Process.SetThreadID.
func (p *CLIProcess) SetThreadID(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.threadID = id
}
