package codec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseEvent_ThreadStarted(t *testing.T) {
	data := []byte(`{"type":"thread.started","thread_id":"019b3cf8-21e8-7430-9ca3-4d38435173e4"}`)

	event, err := ParseEvent(data)
	require.NoError(t, err)

	assert.Equal(t, EventTypeThreadStarted, event.Type)
	assert.Equal(t, "019b3cf8-21e8-7430-9ca3-4d38435173e4", event.ThreadID)
}

func TestParseEvent_TurnStarted(t *testing.T) {
	data := []byte(`{"type":"turn.started"}`)

	event, err := ParseEvent(data)
	require.NoError(t, err)

	assert.Equal(t, EventTypeTurnStarted, event.Type)
}

func TestParseEvent_ItemCompleted_AgentMessage(t *testing.T) {
	data := []byte(`{"type":"item.completed","item":{"id":"item_1","type":"agent_message","text":"The answer is 4"}}`)

	event, err := ParseEvent(data)
	require.NoError(t, err)

	assert.Equal(t, EventTypeItemCompleted, event.Type)
	require.NotNil(t, event.Item)
	assert.Equal(t, "item_1", event.Item.ID)
	assert.Equal(t, ItemTypeAgentMessage, event.Item.Type)
	assert.Equal(t, "The answer is 4", event.Item.Text)
}

func TestParseEvent_ItemCompleted_Reasoning(t *testing.T) {
	data := []byte(`{"type":"item.completed","item":{"id":"item_0","type":"reasoning","text":"The user is asking about math."}}`)

	event, err := ParseEvent(data)
	require.NoError(t, err)

	assert.Equal(t, EventTypeItemCompleted, event.Type)
	require.NotNil(t, event.Item)
	assert.Equal(t, "item_0", event.Item.ID)
	assert.Equal(t, ItemTypeReasoning, event.Item.Type)
	assert.Equal(t, "The user is asking about math.", event.Item.Text)
}

func TestParseEvent_ItemCompleted_CommandExecution(t *testing.T) {
	data := []byte(`{"type":"item.completed","item":{"id":"item_2","type":"command_execution","command":"ls -la","exit_code":0,"output":"total 0"}}`)

	event, err := ParseEvent(data)
	require.NoError(t, err)

	assert.Equal(t, EventTypeItemCompleted, event.Type)
	require.NotNil(t, event.Item)
	assert.Equal(t, ItemTypeCommandExecution, event.Item.Type)
	assert.Equal(t, "ls -la", event.Item.Command)
	require.NotNil(t, event.Item.ExitCode)
	assert.Equal(t, 0, *event.Item.ExitCode)
	assert.Equal(t, "total 0", event.Item.Output)
}

func TestParseEvent_ItemCompleted_FileChange(t *testing.T) {
	data := []byte(`{"type":"item.completed","item":{"id":"item_3","type":"file_change","file_path":"test.go","diff":"+ new line"}}`)

	event, err := ParseEvent(data)
	require.NoError(t, err)

	assert.Equal(t, EventTypeItemCompleted, event.Type)
	require.NotNil(t, event.Item)
	assert.Equal(t, ItemTypeFileChange, event.Item.Type)
	assert.Equal(t, "test.go", event.Item.FilePath)
	assert.Equal(t, "+ new line", event.Item.Diff)
}

func TestParseEvent_TurnCompleted(t *testing.T) {
	data := []byte(`{"type":"turn.completed","usage":{"input_tokens":4972,"cached_input_tokens":100,"output_tokens":9}}`)

	event, err := ParseEvent(data)
	require.NoError(t, err)

	assert.Equal(t, EventTypeTurnCompleted, event.Type)
	require.NotNil(t, event.Usage)
	assert.Equal(t, 4972, event.Usage.InputTokens)
	assert.Equal(t, 100, event.Usage.CachedInputTokens)
	assert.Equal(t, 9, event.Usage.OutputTokens)
}

func TestParseEvent_TurnFailed(t *testing.T) {
	data := []byte(`{"type":"turn.failed","error":{"message":"Model not supported with ChatGPT auth"}}`)

	event, err := ParseEvent(data)
	require.NoError(t, err)

	assert.Equal(t, EventTypeTurnFailed, event.Type)
	require.NotNil(t, event.Error)
	assert.Equal(t, "Model not supported with ChatGPT auth", event.Error.Message)
}

func TestParseEvent_Error(t *testing.T) {
	data := []byte(`{"type":"error","message":"Connection failed"}`)

	event, err := ParseEvent(data)
	require.NoError(t, err)

	assert.Equal(t, EventTypeError, event.Type)
	assert.Equal(t, "Connection failed", event.Message)
}

func TestParseEvent_EmptyInput(t *testing.T) {
	_, err := ParseEvent([]byte{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty input")
}

func TestParseEvent_InvalidJSON(t *testing.T) {
	_, err := ParseEvent([]byte(`{invalid json}`))
	require.Error(t, err)
}

func TestParseEvent_ItemUpdated(t *testing.T) {
	data := []byte(`{"type":"item.updated","item":{"id":"item_1","type":"agent_message","text":"partial"}}`)

	event, err := ParseEvent(data)
	require.NoError(t, err)

	assert.Equal(t, EventTypeItemUpdated, event.Type)
	require.NotNil(t, event.Item)
	assert.Equal(t, "partial", event.Item.Text)
}

func TestParseEvent_ItemStarted(t *testing.T) {
	data := []byte(`{"type":"item.started","item":{"id":"item_1","type":"agent_message"}}`)

	event, err := ParseEvent(data)
	require.NoError(t, err)

	assert.Equal(t, EventTypeItemStarted, event.Type)
	require.NotNil(t, event.Item)
	assert.Equal(t, "item_1", event.Item.ID)
}
