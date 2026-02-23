package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCleanEnv_StripsExact(t *testing.T) {
	t.Setenv("CLAUDECODE", "1")
	t.Setenv("CLAUDECODE_OTHER", "keep") // not exact match

	env := CleanEnv(nil, []string{"CLAUDECODE"})

	assertNotContainsKey(t, env, "CLAUDECODE")
	assertContainsKey(t, env, "CLAUDECODE_OTHER")
}

func TestCleanEnv_StripsPrefixes(t *testing.T) {
	t.Setenv("CLAUDE_CODE_SESSION", "abc")
	t.Setenv("CLAUDE_CODE_TIMEOUT", "30")
	t.Setenv("CLAUDE_OTHER", "keep")

	env := CleanEnv([]string{"CLAUDE_CODE_"}, nil)

	assertNotContainsKey(t, env, "CLAUDE_CODE_SESSION")
	assertNotContainsKey(t, env, "CLAUDE_CODE_TIMEOUT")
	assertContainsKey(t, env, "CLAUDE_OTHER")
}

func TestCleanEnv_StripsBothPrefixesAndExact(t *testing.T) {
	t.Setenv("CLAUDECODE", "1")
	t.Setenv("CLAUDE_CODE_FOO", "bar")
	t.Setenv("PATH", "/usr/bin")

	env := CleanEnv([]string{"CLAUDE_CODE_"}, []string{"CLAUDECODE"})

	assertNotContainsKey(t, env, "CLAUDECODE")
	assertNotContainsKey(t, env, "CLAUDE_CODE_FOO")
	assertContainsKey(t, env, "PATH")
}

func TestCleanEnv_PreservesOtherVars(t *testing.T) {
	t.Setenv("MY_APP_VAR", "hello")

	env := CleanEnv([]string{"CODEX_"}, []string{"CODEX"})

	assertContainsKey(t, env, "MY_APP_VAR")
	assertContainsKey(t, env, "PATH") // PATH should always be present
}

func assertContainsKey(t *testing.T, env []string, key string) {
	t.Helper()
	prefix := key + "="
	for _, e := range env {
		if len(e) >= len(prefix) && e[:len(prefix)] == prefix {
			return
		}
	}
	assert.Failf(t, "missing env var", "expected env to contain key %q", key)
}

func assertNotContainsKey(t *testing.T, env []string, key string) {
	t.Helper()
	prefix := key + "="
	for _, e := range env {
		if len(e) >= len(prefix) && e[:len(prefix)] == prefix {
			assert.Failf(t, "unexpected env var", "expected env to NOT contain key %q, but found %q", key, e)
			return
		}
	}
}
