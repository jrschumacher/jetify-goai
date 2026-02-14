package claudecode

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.jetify.com/ai/provider/internal/cli"
)

func TestExtractStderr(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "standard format",
			input:    "cli process error (stderr: some error message): broken pipe",
			expected: "some error message",
		},
		{
			name:     "no stderr marker",
			input:    "some other error",
			expected: "some other error",
		},
		{
			name:     "stderr at end",
			input:    "cli error (stderr: final error)",
			expected: "final error)",
		},
		{
			name:     "stderr with no closing paren",
			input:    "stderr: just a message",
			expected: "just a message",
		},
		{
			name:     "empty stderr",
			input:    "cli process error (stderr: ): broken pipe",
			expected: "",
		},
		{
			name:     "multiline stderr content",
			input:    "cli process error (stderr: line1 line2): broken pipe",
			expected: "line1 line2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractStderr(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestClaudeCodeClassifier_NilError(t *testing.T) {
	c := NewClaudeCodeClassifier()
	info := c.Classify(nil)
	assert.Nil(t, info)
}

func TestClaudeCodeClassifier_AlreadyErrorInfo(t *testing.T) {
	c := NewClaudeCodeClassifier()
	original := &cli.ErrorInfo{
		Category:    cli.CategoryRateLimited,
		UserMessage: "already classified",
	}

	info := c.Classify(original)
	assert.Equal(t, original, info)
}

func TestClaudeCodeClassifier_Overloaded(t *testing.T) {
	c := NewClaudeCodeClassifier()
	err := errors.New("API is overloaded, try again later")

	info := c.Classify(err)
	require.NotNil(t, info)
	assert.Equal(t, cli.CategoryServiceUnavailable, info.Category)
	assert.True(t, info.Retryable)
	assert.Contains(t, info.UserMessage, "overloaded")
}

func TestClaudeCodeClassifier_CreditBalance(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"credit balance", errors.New("credit balance too low")},
		{"credit_balance_too_low", errors.New("error: credit_balance_too_low")},
	}

	c := NewClaudeCodeClassifier()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := c.Classify(tt.err)
			require.NotNil(t, info)
			assert.Equal(t, cli.CategoryQuotaExceeded, info.Category)
			assert.False(t, info.Retryable)
		})
	}
}

func TestClaudeCodeClassifier_FailedToStartClaude(t *testing.T) {
	c := NewClaudeCodeClassifier()
	err := errors.New("failed to start claude CLI process: exec: not found")

	info := c.Classify(err)
	require.NotNil(t, info)
	assert.Equal(t, cli.CategoryProcessFailure, info.Category)
	assert.True(t, info.Retryable)
	assert.Contains(t, info.SuggestedAction, "claude")
}

func TestClaudeCodeClassifier_BrokenPipe(t *testing.T) {
	c := NewClaudeCodeClassifier()
	err := errors.New("write: broken pipe")

	info := c.Classify(err)
	require.NotNil(t, info)
	assert.Equal(t, cli.CategoryProcessFailure, info.Category)
	assert.True(t, info.Retryable)
	assert.Contains(t, info.UserMessage, "exited unexpectedly")
}

func TestClaudeCodeClassifier_ProcessRunningFalse(t *testing.T) {
	c := NewClaudeCodeClassifier()
	err := errors.New("failed to write to CLI stdin (process running=false): write: broken pipe")

	info := c.Classify(err)
	require.NotNil(t, info)
	assert.Equal(t, cli.CategoryProcessFailure, info.Category)
	assert.True(t, info.Retryable)
}

func TestClaudeCodeClassifier_StderrContent(t *testing.T) {
	c := NewClaudeCodeClassifier()
	// Use an error with stderr: that doesn't match earlier branches (broken pipe, process running=false)
	err := errors.New("CLI process error (stderr: model context limit exceeded): write error")

	info := c.Classify(err)
	require.NotNil(t, info)
	assert.Equal(t, cli.CategoryProcessFailure, info.Category)
	assert.True(t, info.Retryable)
	assert.Contains(t, info.UserMessage, "model context limit exceeded")
}

func TestClaudeCodeClassifier_FallsThrough(t *testing.T) {
	c := NewClaudeCodeClassifier()

	// Rate limit should fall through to base classifier
	err := errors.New("rate limit exceeded")
	info := c.Classify(err)
	require.NotNil(t, info)
	assert.Equal(t, cli.CategoryRateLimited, info.Category)

	// Authentication should fall through to base classifier
	err = errors.New("authentication failed")
	info = c.Classify(err)
	require.NotNil(t, info)
	assert.Equal(t, cli.CategoryAuthenticationFailed, info.Category)

	// Unknown error
	err = errors.New("some completely unknown error")
	info = c.Classify(err)
	require.NotNil(t, info)
	assert.Equal(t, cli.CategoryUnknown, info.Category)
}

func TestClassifyError_Convenience(t *testing.T) {
	info := ClassifyError(errors.New("overloaded"))
	require.NotNil(t, info)
	assert.Equal(t, cli.CategoryServiceUnavailable, info.Category)
}

func TestWrapError_Nil(t *testing.T) {
	err := WrapError(nil)
	assert.Nil(t, err)
}

func TestWrapError_Classifies(t *testing.T) {
	err := WrapError(errors.New("broken pipe"))
	require.NotNil(t, err)

	var errInfo *cli.ErrorInfo
	assert.True(t, errors.As(err, &errInfo))
	assert.Equal(t, cli.CategoryProcessFailure, errInfo.Category)
}

func TestErrorInfo_Unwrap(t *testing.T) {
	original := errors.New("the root cause")
	wrapped := WrapError(fmt.Errorf("wrapper: %w", original))

	// Should be able to unwrap through ErrorInfo
	var errInfo *cli.ErrorInfo
	require.True(t, errors.As(wrapped, &errInfo))
	assert.True(t, errors.Is(errInfo, original))
}

func TestClaudeCodeClassifier_FailedToStartNonClaude(t *testing.T) {
	// "failed to start" without "claude" should NOT match the claude-specific branch
	// It should fall through to the base classifier's process failure branch
	c := NewClaudeCodeClassifier()
	err := errors.New("failed to start some other process")

	info := c.Classify(err)
	require.NotNil(t, info)
	// Falls through to base classifier which matches "failed to start" + "process"
	assert.Equal(t, cli.CategoryProcessFailure, info.Category)
}
