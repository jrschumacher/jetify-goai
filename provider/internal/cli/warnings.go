package cli

import "go.jetify.com/ai/api"

// CallOptionWarnings returns warnings for unsupported CallOptions settings.
// The supported map declares which settings the provider handles; any set
// option not in the map produces an "unsupported-setting" warning.
func CallOptionWarnings(opts api.CallOptions, supported map[string]bool) []api.CallWarning {
	var warnings []api.CallWarning

	check := func(name string, isSet bool) {
		if isSet && !supported[name] {
			warnings = append(warnings, UnsupportedSetting(name))
		}
	}

	check("max_output_tokens", opts.MaxOutputTokens != 0)
	check("temperature", opts.Temperature != nil)
	check("top_p", opts.TopP != 0)
	check("top_k", opts.TopK != 0)
	check("presence_penalty", opts.PresencePenalty != 0)
	check("frequency_penalty", opts.FrequencyPenalty != 0)
	check("stop_sequences", len(opts.StopSequences) > 0)
	check("seed", opts.Seed != 0)
	check("headers", len(opts.Headers) > 0)
	check("tools", len(opts.Tools) > 0)
	check("tool_choice", opts.ToolChoice != nil)
	check("response_format", opts.ResponseFormat != nil)

	return warnings
}

// UnsupportedSetting creates a warning for an unsupported setting.
func UnsupportedSetting(setting string) api.CallWarning {
	return api.CallWarning{
		Type:    "unsupported-setting",
		Setting: setting,
		Message: "setting is not supported by this provider",
	}
}
