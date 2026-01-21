package codex

import (
	"fmt"
	"strings"
	"time"

	"go.jetify.com/ai/provider/openai-codex/codec/jsonrpc"
)

// ErrorCategory classifies the type of error for appropriate handling.
type ErrorCategory string

const (
	// CategoryQuotaExceeded indicates API quota/credits exhausted.
	CategoryQuotaExceeded ErrorCategory = "quota_exceeded"
	// CategoryRateLimited indicates too many requests.
	CategoryRateLimited ErrorCategory = "rate_limited"
	// CategoryServiceUnavailable indicates temporary service outage.
	CategoryServiceUnavailable ErrorCategory = "service_unavailable"
	// CategoryAuthenticationFailed indicates invalid or expired credentials.
	CategoryAuthenticationFailed ErrorCategory = "authentication_failed"
	// CategoryInvalidRequest indicates malformed request.
	CategoryInvalidRequest ErrorCategory = "invalid_request"
	// CategoryModelNotAvailable indicates requested model is unavailable.
	CategoryModelNotAvailable ErrorCategory = "model_not_available"
	// CategoryProcessFailure indicates CLI process failure.
	CategoryProcessFailure ErrorCategory = "process_failure"
	// CategoryUnknown indicates unclassified error.
	CategoryUnknown ErrorCategory = "unknown"
)

// ErrorInfo provides detailed information about an error for handling and user feedback.
type ErrorInfo struct {
	// Category classifies the error type.
	Category ErrorCategory
	// UserMessage is a user-friendly explanation of the error.
	UserMessage string
	// SuggestedAction provides guidance on how to resolve the error.
	SuggestedAction string
	// Retryable indicates if the operation can be retried.
	Retryable bool
	// RetryAfter suggests how long to wait before retrying.
	RetryAfter time.Duration
	// OriginalError is the underlying error for debugging.
	OriginalError error
}

// Error implements the error interface.
func (e *ErrorInfo) Error() string {
	if e.SuggestedAction != "" {
		return fmt.Sprintf("%s. %s", e.UserMessage, e.SuggestedAction)
	}
	return e.UserMessage
}

// Unwrap returns the original error for errors.Is/As compatibility.
func (e *ErrorInfo) Unwrap() error {
	return e.OriginalError
}

// ClassifyError analyzes an error and returns detailed error information.
func ClassifyError(err error) *ErrorInfo {
	if err == nil {
		return nil
	}

	// Check if it's already an ErrorInfo
	if errInfo, ok := err.(*ErrorInfo); ok {
		return errInfo
	}

	// Check for JSON-RPC errors
	if jsonrpcErr, ok := err.(*jsonrpc.Error); ok {
		return classifyJSONRPCError(jsonrpcErr)
	}

	// Check error message content for common patterns
	errMsg := strings.ToLower(err.Error())

	// Quota/credits exhausted
	if strings.Contains(errMsg, "quota") ||
		strings.Contains(errMsg, "insufficient_quota") ||
		strings.Contains(errMsg, "credits") ||
		strings.Contains(errMsg, "billing") {
		return &ErrorInfo{
			Category:        CategoryQuotaExceeded,
			UserMessage:     "OpenAI API quota exceeded",
			SuggestedAction: "Add credits at https://platform.openai.com/account/billing or wait for quota reset",
			Retryable:       false,
			OriginalError:   err,
		}
	}

	// Rate limiting
	if strings.Contains(errMsg, "rate limit") ||
		strings.Contains(errMsg, "too many requests") ||
		strings.Contains(errMsg, "429") {
		return &ErrorInfo{
			Category:        CategoryRateLimited,
			UserMessage:     "OpenAI API rate limit exceeded",
			SuggestedAction: "Wait before retrying or reduce request frequency",
			Retryable:       true,
			RetryAfter:      time.Minute,
			OriginalError:   err,
		}
	}

	// Authentication
	if strings.Contains(errMsg, "authentication") ||
		strings.Contains(errMsg, "unauthorized") ||
		strings.Contains(errMsg, "invalid api key") ||
		strings.Contains(errMsg, "401") {
		return &ErrorInfo{
			Category:        CategoryAuthenticationFailed,
			UserMessage:     "OpenAI authentication failed",
			SuggestedAction: "Run 'codex login' to authenticate",
			Retryable:       false,
			OriginalError:   err,
		}
	}

	// Service unavailable
	if strings.Contains(errMsg, "service unavailable") ||
		strings.Contains(errMsg, "503") ||
		strings.Contains(errMsg, "timeout") ||
		strings.Contains(errMsg, "connection") {
		return &ErrorInfo{
			Category:        CategoryServiceUnavailable,
			UserMessage:     "OpenAI service temporarily unavailable",
			SuggestedAction: "Retry in a few moments",
			Retryable:       true,
			RetryAfter:      30 * time.Second,
			OriginalError:   err,
		}
	}

	// Model not available
	if strings.Contains(errMsg, "model") && strings.Contains(errMsg, "not found") ||
		strings.Contains(errMsg, "model not available") {
		return &ErrorInfo{
			Category:        CategoryModelNotAvailable,
			UserMessage:     "Requested model is not available",
			SuggestedAction: "Check model availability or use a different model",
			Retryable:       false,
			OriginalError:   err,
		}
	}

	// Process failures
	if strings.Contains(errMsg, "failed to start") ||
		strings.Contains(errMsg, "process") ||
		strings.Contains(errMsg, "codex app-server") {
		return &ErrorInfo{
			Category:        CategoryProcessFailure,
			UserMessage:     "Failed to start Codex CLI process",
			SuggestedAction: "Ensure 'codex' CLI is installed and accessible",
			Retryable:       true,
			RetryAfter:      5 * time.Second,
			OriginalError:   err,
		}
	}

	// Unknown error
	return &ErrorInfo{
		Category:        CategoryUnknown,
		UserMessage:     fmt.Sprintf("Unexpected error: %s", err.Error()),
		SuggestedAction: "Check logs for details or contact support",
		Retryable:       false,
		OriginalError:   err,
	}
}

// classifyJSONRPCError classifies JSON-RPC specific errors.
func classifyJSONRPCError(err *jsonrpc.Error) *ErrorInfo {
	switch err.Code {
	case 429:
		// Rate limit or quota exceeded
		if strings.Contains(strings.ToLower(err.Message), "quota") {
			return &ErrorInfo{
				Category:        CategoryQuotaExceeded,
				UserMessage:     "OpenAI API quota exceeded",
				SuggestedAction: "Add credits at https://platform.openai.com/account/billing",
				Retryable:       false,
				OriginalError:   err,
			}
		}
		return &ErrorInfo{
			Category:        CategoryRateLimited,
			UserMessage:     "OpenAI API rate limit exceeded",
			SuggestedAction: "Wait before retrying or reduce request frequency",
			Retryable:       true,
			RetryAfter:      time.Minute,
			OriginalError:   err,
		}

	case 401:
		return &ErrorInfo{
			Category:        CategoryAuthenticationFailed,
			UserMessage:     "OpenAI authentication failed",
			SuggestedAction: "Run 'codex login' to authenticate",
			Retryable:       false,
			OriginalError:   err,
		}

	case 503:
		return &ErrorInfo{
			Category:        CategoryServiceUnavailable,
			UserMessage:     "OpenAI service temporarily unavailable",
			SuggestedAction: "Retry in a few moments",
			Retryable:       true,
			RetryAfter:      30 * time.Second,
			OriginalError:   err,
		}

	case jsonrpc.CodeInvalidRequest, jsonrpc.CodeInvalidParams:
		return &ErrorInfo{
			Category:        CategoryInvalidRequest,
			UserMessage:     "Invalid request to OpenAI API",
			SuggestedAction: "Check request parameters",
			Retryable:       false,
			OriginalError:   err,
		}

	case jsonrpc.CodeInternalError:
		return &ErrorInfo{
			Category:        CategoryServiceUnavailable,
			UserMessage:     "OpenAI internal server error",
			SuggestedAction: "Retry in a few moments",
			Retryable:       true,
			RetryAfter:      30 * time.Second,
			OriginalError:   err,
		}

	default:
		return &ErrorInfo{
			Category:        CategoryUnknown,
			UserMessage:     fmt.Sprintf("OpenAI error: %s", err.Message),
			SuggestedAction: "Check logs for details",
			Retryable:       false,
			OriginalError:   err,
		}
	}
}

// WrapError wraps an error with classification and user-friendly messaging.
func WrapError(err error) error {
	if err == nil {
		return nil
	}
	return ClassifyError(err)
}
