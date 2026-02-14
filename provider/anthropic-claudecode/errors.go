package claudecode

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"go.jetify.com/ai/provider/internal/cli"
)

// Type aliases for backward compatibility.
type (
	ErrorCategory = cli.ErrorCategory
	ErrorInfo     = cli.ErrorInfo
)

// Error category constants (re-exported from shared package).
const (
	CategoryQuotaExceeded        = cli.CategoryQuotaExceeded
	CategoryRateLimited          = cli.CategoryRateLimited
	CategoryServiceUnavailable   = cli.CategoryServiceUnavailable
	CategoryAuthenticationFailed = cli.CategoryAuthenticationFailed
	CategoryInvalidRequest       = cli.CategoryInvalidRequest
	CategoryModelNotAvailable    = cli.CategoryModelNotAvailable
	CategoryProcessFailure       = cli.CategoryProcessFailure
	CategoryContextCanceled      = cli.CategoryContextCanceled
	CategoryUnknown              = cli.CategoryUnknown
)

// ClaudeCodeClassifier extends BaseClassifier with Claude Code specific error handling.
type ClaudeCodeClassifier struct {
	*cli.BaseClassifier
}

// NewClaudeCodeClassifier creates a new Claude Code specific error classifier.
func NewClaudeCodeClassifier() *ClaudeCodeClassifier {
	return &ClaudeCodeClassifier{
		BaseClassifier: cli.NewBaseClassifier("Claude Code", "claude login"),
	}
}

// Classify analyzes an error and returns detailed error information.
// It handles Claude Code specific errors, then falls back to base classification.
func (c *ClaudeCodeClassifier) Classify(err error) *cli.ErrorInfo {
	if err == nil {
		return nil
	}

	// Check if it's already an ErrorInfo (use errors.As for wrapped errors)
	var errInfo *cli.ErrorInfo
	if errors.As(err, &errInfo) {
		return errInfo
	}

	errMsg := strings.ToLower(err.Error())

	// Claude Code specific: overloaded errors
	if strings.Contains(errMsg, "overloaded") {
		return &cli.ErrorInfo{
			Category:        cli.CategoryServiceUnavailable,
			UserMessage:     "Anthropic API is currently overloaded",
			SuggestedAction: "Wait a moment and retry",
			Retryable:       true,
			RetryAfter:      30 * time.Second,
			OriginalError:   err,
		}
	}

	// Claude Code specific: credit balance errors
	if strings.Contains(errMsg, "credit balance") ||
		strings.Contains(errMsg, "credit_balance_too_low") {
		return &cli.ErrorInfo{
			Category:        cli.CategoryQuotaExceeded,
			UserMessage:     "Anthropic credit balance too low",
			SuggestedAction: "Add credits at https://console.anthropic.com/settings/billing",
			Retryable:       false,
			OriginalError:   err,
		}
	}

	// Claude Code specific: actual process start failures
	// Only match when the error is explicitly about starting the CLI,
	// not about writing to or reading from a running process.
	if strings.Contains(errMsg, "failed to start") && strings.Contains(errMsg, "claude") {
		return &cli.ErrorInfo{
			Category:        cli.CategoryProcessFailure,
			UserMessage:     "Failed to start Claude CLI process",
			SuggestedAction: "Ensure 'claude' CLI is installed and in your PATH",
			Retryable:       true,
			RetryAfter:      5 * time.Second,
			OriginalError:   err,
		}
	}

	// Dead process / broken pipe: the process exited but caller tried to use it.
	if strings.Contains(errMsg, "broken pipe") ||
		strings.Contains(errMsg, "process running=false") {
		return &cli.ErrorInfo{
			Category:        cli.CategoryProcessFailure,
			UserMessage:     "Claude CLI process exited unexpectedly",
			SuggestedAction: "The process may have crashed; retry will start a new one",
			Retryable:       true,
			RetryAfter:      time.Second,
			OriginalError:   err,
		}
	}

	// Stderr content surfaced from the CLI
	if strings.Contains(errMsg, "stderr:") {
		// Extract the stderr portion for the user message
		stderrContent := extractStderr(errMsg)
		return &cli.ErrorInfo{
			Category:        cli.CategoryProcessFailure,
			UserMessage:     fmt.Sprintf("Claude CLI error: %s", stderrContent),
			SuggestedAction: "Check the error details and retry",
			Retryable:       true,
			RetryAfter:      2 * time.Second,
			OriginalError:   err,
		}
	}

	// Fall back to base classification
	return c.BaseClassifier.Classify(err)
}

// extractStderr pulls the stderr content from an error message like
// "CLI process error (stderr: some message): broken pipe"
func extractStderr(errMsg string) string {
	start := strings.Index(errMsg, "stderr:")
	if start == -1 {
		return errMsg
	}
	start += len("stderr:")
	rest := errMsg[start:]
	rest = strings.TrimSpace(rest)
	// Trim trailing "): ..." if present
	if end := strings.Index(rest, "):"); end != -1 {
		rest = rest[:end]
	}
	return strings.TrimSpace(rest)
}

// DefaultClassifier is the default error classifier for the Claude Code provider.
var DefaultClassifier = NewClaudeCodeClassifier()

// ClassifyError analyzes an error using the default classifier.
func ClassifyError(err error) *ErrorInfo {
	return DefaultClassifier.Classify(err)
}

// WrapError wraps an error with classification and user-friendly messaging.
func WrapError(err error) error {
	if err == nil {
		return nil
	}
	return ClassifyError(err)
}
