package cli

import (
	"os"
	"strings"
)

// CleanEnv returns a copy of the current environment with variables stripped
// whose key matches any of the given prefixes or exact names.
// This prevents child CLI processes from inheriting parent session markers
// (e.g. nested-session detection variables).
func CleanEnv(stripPrefixes []string, stripExact []string) []string {
	exactSet := make(map[string]bool, len(stripExact))
	for _, e := range stripExact {
		exactSet[e] = true
	}

	var env []string
	for _, e := range os.Environ() {
		key := e[:strings.Index(e, "=")]
		if exactSet[key] {
			continue
		}
		skip := false
		for _, prefix := range stripPrefixes {
			if strings.HasPrefix(key, prefix) {
				skip = true
				break
			}
		}
		if !skip {
			env = append(env, e)
		}
	}
	return env
}
