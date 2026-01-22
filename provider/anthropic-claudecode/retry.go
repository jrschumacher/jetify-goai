package claudecode

import (
	"go.jetify.com/ai/provider/internal/cli"
)

// Type aliases for backward compatibility.
type (
	RetryPolicy       = cli.RetryPolicy
	RetryPolicyOption = cli.RetryPolicyOption
)

// Re-export option functions from the shared package.
var (
	WithMaxAttempts         = cli.WithMaxAttempts
	WithInitialDelay        = cli.WithInitialDelay
	WithMaxDelay            = cli.WithMaxDelay
	WithMultiplier          = cli.WithMultiplier
	WithJitter              = cli.WithJitter
	WithRetryableCategories = cli.WithRetryableCategories
)

// NewRetryPolicy creates a new retry policy with sensible defaults
// and automatically configures it with the Claude Code error classifier.
func NewRetryPolicy(opts ...RetryPolicyOption) *RetryPolicy {
	// Start with the shared retry policy
	allOpts := []cli.RetryPolicyOption{
		cli.WithClassifier(DefaultClassifier),
	}
	allOpts = append(allOpts, opts...)
	return cli.NewRetryPolicy(allOpts...)
}

// DefaultRetryPolicy returns a retry policy with default settings
// configured with the Claude Code error classifier.
var DefaultRetryPolicy = NewRetryPolicy()
