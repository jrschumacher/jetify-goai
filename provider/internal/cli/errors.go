// Package cli provides shared utilities for CLI-based language model providers.
package cli

import (
	"fmt"
	"strings"
	"time"
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
	// CategoryContextCanceled indicates the operation was canceled.
	CategoryContextCanceled ErrorCategory = "context_canceled"
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

// ErrorClassifier converts raw errors to ErrorInfo.
// Providers implement this interface to add protocol-specific error classification.
type ErrorClassifier interface {
	// Classify analyzes an error and returns detailed error information.
	// Returns nil if the error is nil.
	Classify(err error) *ErrorInfo
}

// BaseClassifier provides common error pattern matching that works across providers.
// Providers can embed this and extend it with protocol-specific classification.
type BaseClassifier struct {
	// ProviderName is used in error messages (e.g., "OpenAI", "Anthropic").
	ProviderName string
	// LoginCommand is the command to authenticate (e.g., "codex login", "claude login").
	LoginCommand string
}

// NewBaseClassifier creates a new base classifier with the given provider details.
func NewBaseClassifier(providerName, loginCommand string) *BaseClassifier {
	return &BaseClassifier{
		ProviderName: providerName,
		LoginCommand: loginCommand,
	}
}

// Classify analyzes an error by message content and returns detailed error information.
// This provides generic error classification based on common error message patterns.
func (c *BaseClassifier) Classify(err error) *ErrorInfo {
	if err == nil {
		return nil
	}

	// Check if it's already an ErrorInfo
	if errInfo, ok := err.(*ErrorInfo); ok {
		return errInfo
	}

	errMsg := strings.ToLower(err.Error())

	// Quota/credits exhausted
	if strings.Contains(errMsg, "quota") ||
		strings.Contains(errMsg, "insufficient_quota") ||
		strings.Contains(errMsg, "credits") ||
		strings.Contains(errMsg, "billing") {
		return &ErrorInfo{
			Category:        CategoryQuotaExceeded,
			UserMessage:     fmt.Sprintf("%s API quota exceeded", c.ProviderName),
			SuggestedAction: "Check your account billing or wait for quota reset",
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
			UserMessage:     fmt.Sprintf("%s API rate limit exceeded", c.ProviderName),
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
		action := "Check your API key or credentials"
		if c.LoginCommand != "" {
			action = fmt.Sprintf("Run '%s' to authenticate", c.LoginCommand)
		}
		return &ErrorInfo{
			Category:        CategoryAuthenticationFailed,
			UserMessage:     fmt.Sprintf("%s authentication failed", c.ProviderName),
			SuggestedAction: action,
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
			UserMessage:     fmt.Sprintf("%s service temporarily unavailable", c.ProviderName),
			SuggestedAction: "Retry in a few moments",
			Retryable:       true,
			RetryAfter:      30 * time.Second,
			OriginalError:   err,
		}
	}

	// Model not available
	if (strings.Contains(errMsg, "model") && strings.Contains(errMsg, "not found")) ||
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
		strings.Contains(errMsg, "cli") {
		cliName := strings.ToLower(c.ProviderName)
		return &ErrorInfo{
			Category:        CategoryProcessFailure,
			UserMessage:     fmt.Sprintf("Failed to start %s CLI process", c.ProviderName),
			SuggestedAction: fmt.Sprintf("Ensure '%s' CLI is installed and accessible", cliName),
			Retryable:       true,
			RetryAfter:      5 * time.Second,
			OriginalError:   err,
		}
	}

	// Context canceled
	if strings.Contains(errMsg, "context canceled") ||
		strings.Contains(errMsg, "context deadline exceeded") {
		return &ErrorInfo{
			Category:        CategoryContextCanceled,
			UserMessage:     "Operation was canceled",
			SuggestedAction: "Retry the operation if needed",
			Retryable:       true,
			RetryAfter:      0,
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

// WrapError wraps an error with classification using the provided classifier.
// If the classifier is nil, returns the error as-is.
func WrapError(classifier ErrorClassifier, err error) error {
	if err == nil {
		return nil
	}
	if classifier == nil {
		return err
	}
	return classifier.Classify(err)
}
