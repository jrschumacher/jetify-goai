package codec

import "go.jetify.com/ai/api"

// ProviderName is the identifier for this provider in metadata.
const ProviderName = "openai-codex"

// Metadata contains Codex CLI-specific metadata.
type Metadata struct {
	// ThreadID is the Codex thread identifier for conversation continuity.
	ThreadID string `json:"thread_id,omitempty"`

	// Reasoning contains the assistant's reasoning/thinking summary.
	Reasoning string `json:"reasoning,omitempty"`

	// CommandExecutions contains any commands that were executed.
	CommandExecutions []CommandExecution `json:"command_executions,omitempty"`

	// FileChanges contains any file modifications made.
	FileChanges []FileChange `json:"file_changes,omitempty"`
}

// CommandExecution represents a command that was executed.
type CommandExecution struct {
	Command  string `json:"command"`
	ExitCode int    `json:"exit_code"`
	Output   string `json:"output,omitempty"`
}

// FileChange represents a file that was modified.
type FileChange struct {
	FilePath string `json:"file_path"`
	Diff     string `json:"diff,omitempty"`
}

// GetMetadata retrieves Codex-specific metadata from a metadata source.
func GetMetadata(source api.MetadataSource) *Metadata {
	return api.GetMetadata[Metadata](ProviderName, source)
}
