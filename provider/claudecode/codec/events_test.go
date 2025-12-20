package codec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseEvent_SystemInit(t *testing.T) {
	input := `{"type":"system","subtype":"init","cwd":"/tmp/test","session_id":"abc-123","model":"claude-sonnet-4-5-20250929","permissionMode":"default","tools":[],"mcp_servers":[],"apiKeySource":"none","claude_code_version":"2.0.69"}`

	event, err := ParseEvent([]byte(input))
	require.NoError(t, err)

	assert.Equal(t, EventTypeSystem, event.Type)
	assert.Equal(t, "init", event.Subtype)
	assert.Equal(t, "abc-123", event.SessionID)
	assert.Equal(t, "claude-sonnet-4-5-20250929", event.Model)
	assert.Equal(t, "/tmp/test", event.Cwd)
}

func TestParseEvent_Assistant(t *testing.T) {
	input := `{"type":"assistant","message":{"model":"claude-sonnet-4-5-20250929","id":"msg_123","type":"message","role":"assistant","content":[{"type":"text","text":"Hello world"}],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5}},"session_id":"abc-123"}`

	event, err := ParseEvent([]byte(input))
	require.NoError(t, err)

	assert.Equal(t, EventTypeAssistant, event.Type)
	assert.Equal(t, "abc-123", event.SessionID)
	require.NotNil(t, event.Message)
	assert.Equal(t, "msg_123", event.Message.ID)
	assert.Equal(t, "claude-sonnet-4-5-20250929", event.Message.Model)
	assert.Equal(t, "assistant", event.Message.Role)
	require.Len(t, event.Message.Content, 1)
	assert.Equal(t, "text", event.Message.Content[0].Type)
	assert.Equal(t, "Hello world", event.Message.Content[0].Text)
}

func TestParseEvent_StreamEvent_ContentBlockDelta(t *testing.T) {
	input := `{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}},"session_id":"abc-123"}`

	event, err := ParseEvent([]byte(input))
	require.NoError(t, err)

	assert.Equal(t, EventTypeStreamEvent, event.Type)
	assert.Equal(t, "abc-123", event.SessionID)
	require.NotNil(t, event.StreamEvent)
	assert.Equal(t, StreamEventTypeContentBlockDelta, event.StreamEvent.Type)
	assert.Equal(t, 0, event.StreamEvent.Index)
	require.NotNil(t, event.StreamEvent.Delta)
	assert.Equal(t, "text_delta", event.StreamEvent.Delta.Type)
	assert.Equal(t, "Hello", event.StreamEvent.Delta.Text)
}

func TestParseEvent_StreamEvent_MessageStart(t *testing.T) {
	input := `{"type":"stream_event","event":{"type":"message_start","message":{"model":"claude-sonnet-4-5-20250929","id":"msg_123","type":"message","role":"assistant","content":[],"usage":{"input_tokens":3,"output_tokens":1}}},"session_id":"abc-123"}`

	event, err := ParseEvent([]byte(input))
	require.NoError(t, err)

	assert.Equal(t, EventTypeStreamEvent, event.Type)
	require.NotNil(t, event.StreamEvent)
	assert.Equal(t, StreamEventTypeMessageStart, event.StreamEvent.Type)
	require.NotNil(t, event.StreamEvent.Message)
	assert.Equal(t, "msg_123", event.StreamEvent.Message.ID)
}

func TestParseEvent_StreamEvent_MessageDelta(t *testing.T) {
	input := `{"type":"stream_event","event":{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":3,"output_tokens":9}},"session_id":"abc-123"}`

	event, err := ParseEvent([]byte(input))
	require.NoError(t, err)

	assert.Equal(t, EventTypeStreamEvent, event.Type)
	require.NotNil(t, event.StreamEvent)
	assert.Equal(t, StreamEventTypeMessageDelta, event.StreamEvent.Type)
	require.NotNil(t, event.StreamEvent.Delta)
	assert.Equal(t, "end_turn", event.StreamEvent.Delta.StopReason)
	require.NotNil(t, event.StreamEvent.Usage)
	assert.Equal(t, 3, event.StreamEvent.Usage.InputTokens)
	assert.Equal(t, 9, event.StreamEvent.Usage.OutputTokens)
}

func TestParseEvent_StreamEvent_MessageStop(t *testing.T) {
	input := `{"type":"stream_event","event":{"type":"message_stop"},"session_id":"abc-123"}`

	event, err := ParseEvent([]byte(input))
	require.NoError(t, err)

	assert.Equal(t, EventTypeStreamEvent, event.Type)
	require.NotNil(t, event.StreamEvent)
	assert.Equal(t, StreamEventTypeMessageStop, event.StreamEvent.Type)
}

func TestParseEvent_StreamEvent_ContentBlockStart(t *testing.T) {
	input := `{"type":"stream_event","event":{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}},"session_id":"abc-123"}`

	event, err := ParseEvent([]byte(input))
	require.NoError(t, err)

	assert.Equal(t, EventTypeStreamEvent, event.Type)
	require.NotNil(t, event.StreamEvent)
	assert.Equal(t, StreamEventTypeContentBlockStart, event.StreamEvent.Type)
	assert.Equal(t, 0, event.StreamEvent.Index)
	require.NotNil(t, event.StreamEvent.ContentBlock)
	assert.Equal(t, "text", event.StreamEvent.ContentBlock.Type)
}

func TestParseEvent_StreamEvent_ContentBlockStop(t *testing.T) {
	input := `{"type":"stream_event","event":{"type":"content_block_stop","index":0},"session_id":"abc-123"}`

	event, err := ParseEvent([]byte(input))
	require.NoError(t, err)

	assert.Equal(t, EventTypeStreamEvent, event.Type)
	require.NotNil(t, event.StreamEvent)
	assert.Equal(t, StreamEventTypeContentBlockStop, event.StreamEvent.Type)
	assert.Equal(t, 0, event.StreamEvent.Index)
}

func TestParseEvent_Result_Success(t *testing.T) {
	input := `{"type":"result","subtype":"success","is_error":false,"duration_ms":2779,"duration_api_ms":2769,"num_turns":1,"result":"Hi there!","session_id":"abc-123","total_cost_usd":0.096,"usage":{"input_tokens":3,"output_tokens":9,"cache_creation_input_tokens":25615,"cache_read_input_tokens":0}}`

	event, err := ParseEvent([]byte(input))
	require.NoError(t, err)

	assert.Equal(t, EventTypeResult, event.Type)
	assert.Equal(t, "success", event.Subtype)
	assert.False(t, event.IsError)
	assert.Equal(t, "Hi there!", event.Result)
	assert.Equal(t, "abc-123", event.SessionID)
	assert.InDelta(t, 0.096, event.TotalCostUSD, 0.001)
	assert.Equal(t, 2779, event.DurationMS)
	assert.Equal(t, 1, event.NumTurns)
	require.NotNil(t, event.Usage)
	assert.Equal(t, 3, event.Usage.InputTokens)
	assert.Equal(t, 9, event.Usage.OutputTokens)
	assert.Equal(t, 25615, event.Usage.CacheCreationInputTokens)
}

func TestParseEvent_Result_WithStructuredOutput(t *testing.T) {
	input := `{"type":"result","subtype":"success","is_error":false,"result":"","session_id":"abc-123","structured_output":{"answer":"42"}}`

	event, err := ParseEvent([]byte(input))
	require.NoError(t, err)

	assert.Equal(t, EventTypeResult, event.Type)
	require.NotNil(t, event.StructuredOutput)
	// StructuredOutput is json.RawMessage, verify it can be parsed
	assert.Contains(t, string(event.StructuredOutput), "42")
}

func TestParseEvent_Result_Error(t *testing.T) {
	input := `{"type":"result","subtype":"error","is_error":true,"result":"Something went wrong","session_id":"abc-123"}`

	event, err := ParseEvent([]byte(input))
	require.NoError(t, err)

	assert.Equal(t, EventTypeResult, event.Type)
	assert.Equal(t, "error", event.Subtype)
	assert.True(t, event.IsError)
	assert.Equal(t, "Something went wrong", event.Result)
}

func TestParseEvent_InvalidJSON(t *testing.T) {
	input := `{not valid json}`

	_, err := ParseEvent([]byte(input))
	require.Error(t, err)
}

func TestParseEvent_EmptyInput(t *testing.T) {
	_, err := ParseEvent([]byte(""))
	require.Error(t, err)
}

func TestParseEvent_UnknownType(t *testing.T) {
	input := `{"type":"unknown_type","session_id":"abc-123"}`

	event, err := ParseEvent([]byte(input))
	require.NoError(t, err)
	assert.Equal(t, EventType("unknown_type"), event.Type)
}
