package claudecode

import (
	"go.jetify.com/ai/api"
	"go.jetify.com/ai/provider/internal/cli"
)

// claudeCodeSupported declares which CallOptions settings this provider handles.
var claudeCodeSupported = map[string]bool{
	"temperature": true,
}

func callOptionWarnings(opts api.CallOptions) []api.CallWarning {
	return cli.CallOptionWarnings(opts, claudeCodeSupported)
}
