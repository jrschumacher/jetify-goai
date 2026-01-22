package claudecode

import (
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
	CategoryProcessFailure       = cli.CategoryProcessFailure
	CategoryUnknown              = cli.CategoryUnknown
)

// ClaudeCodeClassifier extends BaseClassifier with Claude Code specific error handling.
type ClaudeCodeClassifier struct {
	*cli.BaseClassifier
}

// NewClaudeCodeClassifier creates a new Claude Code specific error classifier.
func NewClaudeCodeClassifier() *ClaudeCodeClassifier {
	return &ClaudeCodeClassifier{
		BaseClassifier: cli.NewBaseClassifier("Anthropic", "claude login"),
	}
}

// Classify analyzes an error and returns detailed error information.
// It handles Claude Code specific errors, then falls back to base classification.
func (c *ClaudeCodeClassifier) Classify(err error) *cli.ErrorInfo {
	if err == nil {
		return nil
	}

	// Check if it's already an ErrorInfo
	if errInfo, ok := err.(*cli.ErrorInfo); ok {
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

	// Claude Code specific: Claude CLI process failures
	if strings.Contains(errMsg, "claude") && strings.Contains(errMsg, "cli") {
		return &cli.ErrorInfo{
			Category:        cli.CategoryProcessFailure,
			UserMessage:     "Failed to start Claude CLI process",
			SuggestedAction: "Ensure 'claude' CLI is installed and accessible",
			Retryable:       true,
			RetryAfter:      5 * time.Second,
			OriginalError:   err,
		}
	}

	// Fall back to base classification
	return c.BaseClassifier.Classify(err)
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
