package codex

import "go.jetify.com/ai/api"

func callOptionWarnings(opts api.CallOptions) []api.CallWarning {
	var warnings []api.CallWarning

	// Codex app-server is primarily an agentic interface and does not currently support
	// most per-call tuning options from api.CallOptions.

	if opts.MaxOutputTokens != 0 {
		warnings = append(warnings, unsupportedSetting("max_output_tokens"))
	}
	if opts.Temperature != nil {
		warnings = append(warnings, unsupportedSetting("temperature"))
	}
	if opts.TopP != 0 {
		warnings = append(warnings, unsupportedSetting("top_p"))
	}
	if opts.TopK != 0 {
		warnings = append(warnings, unsupportedSetting("top_k"))
	}
	if opts.PresencePenalty != 0 {
		warnings = append(warnings, unsupportedSetting("presence_penalty"))
	}
	if opts.FrequencyPenalty != 0 {
		warnings = append(warnings, unsupportedSetting("frequency_penalty"))
	}
	if len(opts.StopSequences) > 0 {
		warnings = append(warnings, unsupportedSetting("stop_sequences"))
	}
	if opts.Seed != 0 {
		warnings = append(warnings, unsupportedSetting("seed"))
	}
	if len(opts.Headers) > 0 {
		warnings = append(warnings, unsupportedSetting("headers"))
	}
	if len(opts.Tools) > 0 {
		warnings = append(warnings, unsupportedSetting("tools"))
	}
	if opts.ToolChoice != nil {
		warnings = append(warnings, unsupportedSetting("tool_choice"))
	}
	if opts.ResponseFormat != nil {
		warnings = append(warnings, unsupportedSetting("response_format"))
	}

	return warnings
}

func unsupportedSetting(setting string) api.CallWarning {
	return api.CallWarning{
		Type:    "unsupported-setting",
		Setting: setting,
		Message: "setting is not supported by this provider",
	}
}
