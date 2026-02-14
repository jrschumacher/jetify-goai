package process

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCLIProcess_Defaults(t *testing.T) {
	p := NewCLIProcess()
	require.NotNil(t, p)
	assert.False(t, p.IsRunning())
	assert.Empty(t, p.SessionID())
}

func TestNewCLIProcess_WithOptions(t *testing.T) {
	p := NewCLIProcess(
		WithModel("sonnet"),
		WithSystemPrompt("You are helpful"),
		WithWorkDir("/tmp"),
		WithTemperature(0.5),
		WithJSONSchema(`{"type":"object"}`),
		WithResumeSession("sess-123"),
		WithVerbose(true),
		WithMaxTurns(5),
	)
	require.NotNil(t, p)
	assert.Equal(t, "sonnet", p.config.Model)
	assert.Equal(t, "You are helpful", p.config.SystemPrompt)
	assert.Equal(t, "/tmp", p.config.WorkDir)
	require.NotNil(t, p.config.Temperature)
	assert.Equal(t, 0.5, *p.config.Temperature)
	assert.Equal(t, `{"type":"object"}`, p.config.JSONSchema)
	assert.Equal(t, "sess-123", p.config.ResumeSessionID)
	assert.True(t, p.config.Verbose)
	assert.Equal(t, 5, p.config.MaxTurns)
}

func TestCLIProcess_BuildArgs_Minimal(t *testing.T) {
	p := NewCLIProcess()
	args := p.buildArgs()

	assert.Contains(t, args, "-p")
	assert.Contains(t, args, "--input-format")
	assert.Contains(t, args, "stream-json")
	assert.Contains(t, args, "--output-format")
	assert.Contains(t, args, "--tools")
	assert.Contains(t, args, "--include-partial-messages")
}

func TestCLIProcess_BuildArgs_AllOptions(t *testing.T) {
	p := NewCLIProcess(
		WithModel("opus"),
		WithSystemPrompt("Be concise"),
		WithTemperature(0.7),
		WithJSONSchema(`{"type":"string"}`),
		WithResumeSession("sess-456"),
		WithVerbose(true),
		WithMaxTurns(3),
	)
	args := p.buildArgs()

	assert.Contains(t, args, "--model")
	assert.Contains(t, args, "opus")
	assert.Contains(t, args, "--system-prompt")
	assert.Contains(t, args, "Be concise")
	assert.Contains(t, args, "--settings")
	assert.Contains(t, args, "--json-schema")
	assert.Contains(t, args, `{"type":"string"}`)
	assert.Contains(t, args, "--resume")
	assert.Contains(t, args, "sess-456")
	assert.Contains(t, args, "--verbose")
	assert.Contains(t, args, "--max-turns")
	assert.Contains(t, args, "3")
}

func TestCLIProcess_StopBeforeStart(t *testing.T) {
	p := NewCLIProcess()
	err := p.Stop()
	assert.NoError(t, err)
}

func TestCLIProcess_WaitBeforeStart(t *testing.T) {
	p := NewCLIProcess()
	err := p.Wait()
	assert.NoError(t, err)
}

func TestCLIProcess_IsRunning_BeforeStart(t *testing.T) {
	p := NewCLIProcess()
	assert.False(t, p.IsRunning())
}

func TestCLIProcess_SessionID_GetSet(t *testing.T) {
	p := NewCLIProcess()

	assert.Empty(t, p.SessionID())

	p.SetSessionID("test-session-123")
	assert.Equal(t, "test-session-123", p.SessionID())

	p.SetSessionID("updated-session")
	assert.Equal(t, "updated-session", p.SessionID())
}

func TestCLIProcess_Stdin_BeforeStart(t *testing.T) {
	p := NewCLIProcess()
	assert.Nil(t, p.Stdin())
}

func TestCLIProcess_Stdout_BeforeStart(t *testing.T) {
	p := NewCLIProcess()
	assert.Nil(t, p.Stdout())
}

func TestCLIProcess_Stderr_BeforeStart(t *testing.T) {
	p := NewCLIProcess()
	assert.Nil(t, p.Stderr())
}

func TestCLIProcess_DoubleStart(t *testing.T) {
	p := NewCLIProcess()

	// Simulate running state
	p.mu.Lock()
	p.running = true
	p.mu.Unlock()

	err := p.Start(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "process already running")
}

func TestConfig_ConfigKey(t *testing.T) {
	temp := 0.5
	cfg := &Config{
		Model:           "sonnet",
		SystemPrompt:    "test",
		Temperature:     &temp,
		WorkDir:         "/tmp",
		JSONSchema:      `{"type":"object"}`,
		ResumeSessionID: "sess-1",
	}

	key := cfg.ConfigKey()
	assert.Equal(t, "sonnet", key.Model)
	assert.Equal(t, "test", key.SystemPrompt)
	assert.Equal(t, 0.5, key.Temperature)
	assert.True(t, key.HasTemp)
	assert.Equal(t, "/tmp", key.Extra["workdir"])
	assert.Equal(t, `{"type":"object"}`, key.Extra["jsonschema"])
	assert.Equal(t, "sess-1", key.Extra["resume"])
}

func TestConfig_ConfigKey_NoOptionals(t *testing.T) {
	cfg := &Config{
		Model: "haiku",
	}

	key := cfg.ConfigKey()
	assert.Equal(t, "haiku", key.Model)
	assert.Empty(t, key.SystemPrompt)
	assert.Equal(t, float64(0), key.Temperature)
	assert.False(t, key.HasTemp)
	assert.Empty(t, key.Extra)
}

func TestNewConfig_Defaults(t *testing.T) {
	cfg := NewConfig()
	assert.True(t, cfg.Verbose, "default verbose should be true")
	assert.Empty(t, cfg.Model)
	assert.Nil(t, cfg.Temperature)
}

func TestNewConfig_WithOptions(t *testing.T) {
	cfg := NewConfig(
		WithModel("opus"),
		WithVerbose(false),
	)
	assert.Equal(t, "opus", cfg.Model)
	assert.False(t, cfg.Verbose)
}

func TestMockProcess_Lifecycle(t *testing.T) {
	m := NewMockProcess()

	assert.False(t, m.IsRunning())
	assert.Empty(t, m.SessionID())

	// Start
	err := m.Start(context.Background())
	require.NoError(t, err)
	assert.True(t, m.IsRunning())

	// Set session ID
	m.SetSessionID("mock-sess")
	assert.Equal(t, "mock-sess", m.SessionID())

	// Stop
	err = m.Stop()
	require.NoError(t, err)
	assert.False(t, m.IsRunning())
}

func TestMockProcess_OnStart_Error(t *testing.T) {
	m := NewMockProcess()
	m.OnStart = func(ctx context.Context) error {
		return assert.AnError
	}

	err := m.Start(context.Background())
	assert.Error(t, err)
	assert.False(t, m.IsRunning())
}

func TestMockProcess_OnStop_Error(t *testing.T) {
	m := NewMockProcess()
	_ = m.Start(context.Background())

	m.OnStop = func() error {
		return assert.AnError
	}

	err := m.Stop()
	assert.Error(t, err)
	// running state unchanged on error
	assert.True(t, m.IsRunning())
}

func TestMockProcess_ReadWrite(t *testing.T) {
	m := NewMockProcess()

	// Write to stdout
	m.WriteStdout([]byte("hello from stdout"))
	buf := make([]byte, 100)
	n, err := m.Stdout().(interface{ Read([]byte) (int, error) }).Read(buf)
	require.NoError(t, err)
	assert.Equal(t, "hello from stdout", string(buf[:n]))

	// Write to stderr
	m.WriteStderr([]byte("error msg"))
	n, err = m.Stderr().(interface{ Read([]byte) (int, error) }).Read(buf)
	require.NoError(t, err)
	assert.Equal(t, "error msg", string(buf[:n]))

	// Write to stdin
	_, err = m.Stdin().(interface{ Write([]byte) (int, error) }).Write([]byte("input"))
	require.NoError(t, err)
	assert.Equal(t, "input", string(m.ReadStdin()))
}

func TestMockProcess_Reset(t *testing.T) {
	m := NewMockProcess()
	_ = m.Start(context.Background())
	m.SetSessionID("sess")
	m.WriteStdout([]byte("data"))

	m.Reset()

	assert.False(t, m.IsRunning())
	assert.Empty(t, m.SessionID())
	assert.Empty(t, m.ReadStdin())
}

func TestNewCLIProcess_WithAllowedTools(t *testing.T) {
	p := NewCLIProcess(
		WithAllowedTools([]string{"Read", "Bash"}),
	)
	require.NotNil(t, p)
	assert.Equal(t, []string{"Read", "Bash"}, p.config.AllowedTools)
}

func TestCLIProcess_ToolsArg_Default(t *testing.T) {
	p := NewCLIProcess()
	assert.Equal(t, "", p.toolsArg())
}

func TestCLIProcess_ToolsArg_WithTools(t *testing.T) {
	p := NewCLIProcess(WithAllowedTools([]string{"Read", "Bash", "Write"}))
	assert.Equal(t, "Read,Bash,Write", p.toolsArg())
}

func TestCLIProcess_BuildArgs_WithAllowedTools(t *testing.T) {
	p := NewCLIProcess(WithAllowedTools([]string{"Read", "Bash"}))
	args := p.buildArgs()

	// Find the --tools flag and its value
	for i, arg := range args {
		if arg == "--tools" && i+1 < len(args) {
			assert.Equal(t, "Read,Bash", args[i+1])
			return
		}
	}
	t.Fatal("--tools flag not found in args")
}

func TestCLIProcess_BuildArgs_NoAllowedTools(t *testing.T) {
	p := NewCLIProcess()
	args := p.buildArgs()

	// Should have empty string for --tools (all disabled)
	for i, arg := range args {
		if arg == "--tools" && i+1 < len(args) {
			assert.Equal(t, "", args[i+1])
			return
		}
	}
	t.Fatal("--tools flag not found in args")
}

func TestConfig_ConfigKey_WithAllowedTools(t *testing.T) {
	cfg := &Config{
		Model:        "sonnet",
		AllowedTools: []string{"Bash", "Read"},
	}
	key := cfg.ConfigKey()
	// Tools should be sorted in the key
	assert.Equal(t, "Bash,Read", key.Extra["tools"])
}

func TestConfig_ConfigKey_AllowedTools_SortOrder(t *testing.T) {
	cfg1 := &Config{AllowedTools: []string{"Write", "Bash", "Read"}}
	cfg2 := &Config{AllowedTools: []string{"Read", "Write", "Bash"}}
	// Both should produce the same key regardless of input order
	assert.Equal(t, cfg1.ConfigKey().Extra["tools"], cfg2.ConfigKey().Extra["tools"])
	assert.Equal(t, "Bash,Read,Write", cfg1.ConfigKey().Extra["tools"])
}
