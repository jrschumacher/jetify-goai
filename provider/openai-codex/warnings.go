package codex

import (
	"go.jetify.com/ai/api"
	"go.jetify.com/ai/provider/internal/cli"
)

func callOptionWarnings(opts api.CallOptions) []api.CallWarning {
	return cli.CallOptionWarnings(opts, nil) // No per-call settings supported
}
