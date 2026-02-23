package cli

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.jetify.com/ai/api"
)

func TestCallOptionWarnings_AllUnsupported(t *testing.T) {
	temp := 0.5
	opts := api.CallOptions{
		MaxOutputTokens:  100,
		Temperature:      &temp,
		TopP:             0.9,
		TopK:             40,
		PresencePenalty:   0.1,
		FrequencyPenalty:  0.2,
		StopSequences:    []string{"stop"},
		Seed:             42,
		Headers:          http.Header{"X-Custom": []string{"val"}},
		ResponseFormat:   &api.ResponseFormat{Type: "json"},
	}

	warnings := CallOptionWarnings(opts, nil)

	// All 11 settings should generate warnings (no supported map)
	names := make(map[string]bool)
	for _, w := range warnings {
		assert.Equal(t, "unsupported-setting", w.Type)
		names[w.Setting] = true
	}
	assert.True(t, names["max_output_tokens"])
	assert.True(t, names["temperature"])
	assert.True(t, names["top_p"])
	assert.True(t, names["seed"])
}

func TestCallOptionWarnings_WithSupported(t *testing.T) {
	temp := 0.5
	opts := api.CallOptions{
		Temperature: &temp,
		TopP:        0.9,
	}

	supported := map[string]bool{"temperature": true}
	warnings := CallOptionWarnings(opts, supported)

	// Temperature is supported, so only top_p should warn
	assert.Len(t, warnings, 1)
	assert.Equal(t, "top_p", warnings[0].Setting)
}

func TestCallOptionWarnings_NoneSet(t *testing.T) {
	warnings := CallOptionWarnings(api.CallOptions{}, nil)
	assert.Empty(t, warnings)
}
