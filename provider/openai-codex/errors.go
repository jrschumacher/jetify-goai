package codex

import (
	"errors"
	"strings"
	"time"

	"go.jetify.com/ai/provider/internal/cli"
	"go.jetify.com/ai/provider/openai-codex/codec/jsonrpc"
)

// Type aliases for backward compatibility.
// These allow existing code to use the local type names while
// the actual implementation lives in the shared cli package.
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

// CategoryModelNotAvailable indicates requested model is unavailable.
// This is a Codex-specific category not in the shared package.
const CategoryModelNotAvailable ErrorCategory = "model_not_available"

// CodexClassifier extends BaseClassifier with JSON-RPC specific error handling.
type CodexClassifier struct {
	*cli.BaseClassifier
}

// NewCodexClassifier creates a new Codex-specific error classifier.
func NewCodexClassifier() *CodexClassifier {
	return &CodexClassifier{
		BaseClassifier: cli.NewBaseClassifier("OpenAI", "codex login"),
	}
}

// Classify analyzes an error and returns detailed error information.
// It handles JSON-RPC errors specifically, then falls back to base classification.
func (c *CodexClassifier) Classify(err error) *cli.ErrorInfo {
	if err == nil {
		return nil
	}

	// Check if it's already an ErrorInfo (use errors.As for wrapped errors)
	var errInfo *cli.ErrorInfo
	if errors.As(err, &errInfo) {
		return errInfo
	}

	// Check for JSON-RPC errors first (use errors.As for wrapped errors)
	var jsonrpcErr *jsonrpc.Error
	if errors.As(err, &jsonrpcErr) {
		return c.classifyJSONRPCError(jsonrpcErr)
	}

	// Check for model not available (Codex-specific pattern)
	errMsg := strings.ToLower(err.Error())
	if (strings.Contains(errMsg, "model") && strings.Contains(errMsg, "not found")) ||
		strings.Contains(errMsg, "model not available") {
		return &cli.ErrorInfo{
			Category:        CategoryModelNotAvailable,
			UserMessage:     "Requested model is not available",
			SuggestedAction: "Check model availability or use a different model",
			Retryable:       false,
			OriginalError:   err,
		}
	}

	// Check for Codex-specific process failures
	if strings.Contains(errMsg, "codex app-server") {
		return &cli.ErrorInfo{
			Category:        cli.CategoryProcessFailure,
			UserMessage:     "Failed to start Codex CLI process",
			SuggestedAction: "Ensure 'codex' CLI is installed and accessible",
			Retryable:       true,
			RetryAfter:      5 * time.Second,
			OriginalError:   err,
		}
	}

	// Fall back to base classification
	return c.BaseClassifier.Classify(err)
}

// classifyJSONRPCError classifies JSON-RPC specific errors.
func (c *CodexClassifier) classifyJSONRPCError(err *jsonrpc.Error) *cli.ErrorInfo {
	switch err.Code {
	case 429:
		// Rate limit or quota exceeded
		if strings.Contains(strings.ToLower(err.Message), "quota") {
			return &cli.ErrorInfo{
				Category:        cli.CategoryQuotaExceeded,
				UserMessage:     "OpenAI API quota exceeded",
				SuggestedAction: "Add credits at https://platform.openai.com/account/billing",
				Retryable:       false,
				OriginalError:   err,
			}
		}
		return &cli.ErrorInfo{
			Category:        cli.CategoryRateLimited,
			UserMessage:     "OpenAI API rate limit exceeded",
			SuggestedAction: "Wait before retrying or reduce request frequency",
			Retryable:       true,
			RetryAfter:      time.Minute,
			OriginalError:   err,
		}

	case 401:
		return &cli.ErrorInfo{
			Category:        cli.CategoryAuthenticationFailed,
			UserMessage:     "OpenAI authentication failed",
			SuggestedAction: "Run 'codex login' to authenticate",
			Retryable:       false,
			OriginalError:   err,
		}

	case 503:
		return &cli.ErrorInfo{
			Category:        cli.CategoryServiceUnavailable,
			UserMessage:     "OpenAI service temporarily unavailable",
			SuggestedAction: "Retry in a few moments",
			Retryable:       true,
			RetryAfter:      30 * time.Second,
			OriginalError:   err,
		}

	case jsonrpc.CodeInvalidRequest, jsonrpc.CodeInvalidParams:
		// Check for model not found errors
		errMsg := strings.ToLower(err.Message)
		if strings.Contains(errMsg, "model") && (strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "not available")) {
			return &cli.ErrorInfo{
				Category:        CategoryModelNotAvailable,
				UserMessage:     "Requested model is not available",
				SuggestedAction: "Check model availability or use a different model",
				Retryable:       false,
				OriginalError:   err,
			}
		}
		return &cli.ErrorInfo{
			Category:        cli.CategoryInvalidRequest,
			UserMessage:     "Invalid request to OpenAI API",
			SuggestedAction: "Check request parameters",
			Retryable:       false,
			OriginalError:   err,
		}

	case jsonrpc.CodeInternalError:
		return &cli.ErrorInfo{
			Category:        cli.CategoryServiceUnavailable,
			UserMessage:     "OpenAI internal server error",
			SuggestedAction: "Retry in a few moments",
			Retryable:       true,
			RetryAfter:      30 * time.Second,
			OriginalError:   err,
		}

	default:
		return &cli.ErrorInfo{
			Category:        cli.CategoryUnknown,
			UserMessage:     "OpenAI error: " + err.Message,
			SuggestedAction: "Check logs for details",
			Retryable:       false,
			OriginalError:   err,
		}
	}
}

// DefaultClassifier is the default error classifier for the Codex provider.
var DefaultClassifier = NewCodexClassifier()

// ClassifyError analyzes an error using the default classifier.
// This function is kept for backward compatibility.
func ClassifyError(err error) *ErrorInfo {
	return DefaultClassifier.Classify(err)
}

// WrapError wraps an error with classification and user-friendly messaging.
// This function is kept for backward compatibility.
func WrapError(err error) error {
	if err == nil {
		return nil
	}
	return ClassifyError(err)
}
