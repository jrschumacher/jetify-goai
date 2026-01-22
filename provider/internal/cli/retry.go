package cli

import (
	"context"
	"fmt"
	"math/rand"
	"time"
)

// RetryPolicy defines how operations should be retried on failure.
type RetryPolicy struct {
	// MaxAttempts is the maximum number of attempts (including the initial attempt).
	// Default is 3.
	MaxAttempts int

	// InitialDelay is the delay before the first retry.
	// Default is 1 second.
	InitialDelay time.Duration

	// MaxDelay is the maximum delay between retries.
	// Default is 60 seconds.
	MaxDelay time.Duration

	// Multiplier is applied to the delay after each retry.
	// Default is 2.0 (exponential backoff).
	Multiplier float64

	// Jitter adds randomness to delays to prevent thundering herd.
	// Default is true.
	Jitter bool

	// RetryableCategories defines which error categories should be retried.
	// If nil, uses default retryable categories.
	RetryableCategories map[ErrorCategory]bool

	// Classifier is used to classify errors for retry decisions.
	// If nil, errors are not classified before retry decisions.
	Classifier ErrorClassifier
}

// RetryPolicyOption configures a RetryPolicy.
type RetryPolicyOption func(*RetryPolicy)

// WithMaxAttempts sets the maximum number of attempts.
func WithMaxAttempts(attempts int) RetryPolicyOption {
	return func(p *RetryPolicy) {
		if attempts > 0 {
			p.MaxAttempts = attempts
		}
	}
}

// WithInitialDelay sets the initial delay before the first retry.
func WithInitialDelay(delay time.Duration) RetryPolicyOption {
	return func(p *RetryPolicy) {
		if delay > 0 {
			p.InitialDelay = delay
		}
	}
}

// WithMaxDelay sets the maximum delay between retries.
func WithMaxDelay(delay time.Duration) RetryPolicyOption {
	return func(p *RetryPolicy) {
		if delay > 0 {
			p.MaxDelay = delay
		}
	}
}

// WithMultiplier sets the backoff multiplier.
func WithMultiplier(multiplier float64) RetryPolicyOption {
	return func(p *RetryPolicy) {
		if multiplier > 1.0 {
			p.Multiplier = multiplier
		}
	}
}

// WithJitter enables or disables jitter.
func WithJitter(enabled bool) RetryPolicyOption {
	return func(p *RetryPolicy) {
		p.Jitter = enabled
	}
}

// WithRetryableCategories sets custom retryable error categories.
func WithRetryableCategories(categories map[ErrorCategory]bool) RetryPolicyOption {
	return func(p *RetryPolicy) {
		p.RetryableCategories = categories
	}
}

// WithClassifier sets the error classifier for the retry policy.
func WithClassifier(classifier ErrorClassifier) RetryPolicyOption {
	return func(p *RetryPolicy) {
		p.Classifier = classifier
	}
}

// DefaultRetryableCategories returns the default set of retryable error categories.
func DefaultRetryableCategories() map[ErrorCategory]bool {
	return map[ErrorCategory]bool{
		CategoryRateLimited:        true,
		CategoryServiceUnavailable: true,
		CategoryProcessFailure:     true,
	}
}

// NewRetryPolicy creates a new retry policy with sensible defaults.
func NewRetryPolicy(opts ...RetryPolicyOption) *RetryPolicy {
	p := &RetryPolicy{
		MaxAttempts:         3,
		InitialDelay:        time.Second,
		MaxDelay:            60 * time.Second,
		Multiplier:          2.0,
		Jitter:              true,
		RetryableCategories: DefaultRetryableCategories(),
	}

	for _, opt := range opts {
		opt(p)
	}

	return p
}

// Execute runs a function with retry logic according to the policy.
// The classifier parameter is used to classify errors if not set on the policy.
func (p *RetryPolicy) Execute(ctx context.Context, fn func() error) error {
	var lastErr error
	delay := p.InitialDelay

	for attempt := 1; attempt <= p.MaxAttempts; attempt++ {
		// Execute the function
		err := fn()
		if err == nil {
			return nil // Success!
		}

		lastErr = err

		// Check if we should retry
		if !p.shouldRetry(err) {
			return p.wrapError(err) // Non-retryable error
		}

		// Check if we've exhausted attempts
		if attempt >= p.MaxAttempts {
			break
		}

		// Calculate delay with jitter if enabled
		actualDelay := delay
		if p.Jitter {
			actualDelay = p.addJitter(delay)
		}

		// Wait before retrying, respecting context cancellation
		select {
		case <-time.After(actualDelay):
			// Continue to next attempt
		case <-ctx.Done():
			return ctx.Err()
		}

		// Increase delay for next iteration (exponential backoff)
		delay = min(time.Duration(float64(delay)*p.Multiplier), p.MaxDelay)
	}

	// All attempts exhausted
	return &ErrorInfo{
		Category:        CategoryUnknown,
		UserMessage:     fmt.Sprintf("operation failed after %d attempts", p.MaxAttempts),
		SuggestedAction: "The issue may be temporary; try again later",
		Retryable:       false,
		OriginalError:   lastErr,
	}
}

// shouldRetry determines if an error should be retried.
func (p *RetryPolicy) shouldRetry(err error) bool {
	// Classify the error
	errInfo := p.classifyError(err)
	if errInfo == nil {
		return false
	}

	// Check if explicitly retryable
	if errInfo.Retryable {
		// Check if category is in retryable list
		if retryable, ok := p.RetryableCategories[errInfo.Category]; ok {
			return retryable
		}
		// If not in list but error says retryable, retry
		return true
	}

	return false
}

// classifyError classifies an error using the policy's classifier.
func (p *RetryPolicy) classifyError(err error) *ErrorInfo {
	// If error is already classified, return it
	if errInfo, ok := err.(*ErrorInfo); ok {
		return errInfo
	}

	// Use the policy's classifier if available
	if p.Classifier != nil {
		return p.Classifier.Classify(err)
	}

	// Default classification for unknown errors
	return &ErrorInfo{
		Category:      CategoryUnknown,
		UserMessage:   err.Error(),
		Retryable:     false,
		OriginalError: err,
	}
}

// wrapError wraps an error with classification.
func (p *RetryPolicy) wrapError(err error) error {
	if p.Classifier != nil {
		return WrapError(p.Classifier, err)
	}
	return err
}

// addJitter adds randomness to the delay to prevent thundering herd.
// Returns a delay between 50% and 100% of the input delay.
func (p *RetryPolicy) addJitter(delay time.Duration) time.Duration {
	jitter := time.Duration(rand.Int63n(int64(delay) / 2))
	return delay/2 + jitter
}
