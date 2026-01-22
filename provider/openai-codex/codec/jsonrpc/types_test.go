package jsonrpc

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRequest(t *testing.T) {
	req := NewRequest(123, "test/method", map[string]string{"key": "value"})

	assert.Equal(t, "2.0", req.JSONRPC)
	assert.Equal(t, "test/method", req.Method)
	assert.Equal(t, int64(123), req.ID)
	assert.NotNil(t, req.Params)
}

func TestRequest_Marshal(t *testing.T) {
	req := NewRequest(1, "initialize", InitializeParams{
		ClientInfo: ClientInfo{Name: "test", Version: "1.0"},
	})

	data, err := json.Marshal(req)
	require.NoError(t, err)

	var decoded map[string]interface{}
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, "2.0", decoded["jsonrpc"])
	assert.Equal(t, "initialize", decoded["method"])
	assert.Equal(t, float64(1), decoded["id"])
}

func TestError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *Error
		expected string
	}{
		{
			name:     "simple error",
			err:      &Error{Code: -32600, Message: "Invalid request"},
			expected: "jsonrpc error -32600: Invalid request",
		},
		{
			name:     "error with data",
			err:      &Error{Code: -32601, Message: "Method not found", Data: json.RawMessage(`{"detail":"missing"}`)},
			expected: `jsonrpc error -32601: Method not found (data: {"detail":"missing"})`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.err.Error())
		})
	}
}

func TestMessage_IsResponse(t *testing.T) {
	id := int64(1)
	msg := &Message{ID: &id}
	assert.True(t, msg.IsResponse())

	msgNoID := &Message{Method: "test"}
	assert.False(t, msgNoID.IsResponse())
}

func TestMessage_IsNotification(t *testing.T) {
	msg := &Message{Method: "test/notification"}
	assert.True(t, msg.IsNotification())

	id := int64(1)
	msgWithID := &Message{Method: "test", ID: &id}
	assert.False(t, msgWithID.IsNotification())
}

func TestMessage_AsResponse(t *testing.T) {
	id := int64(1)
	msg := &Message{
		JSONRPC: "2.0",
		Result:  json.RawMessage(`{"ok":true}`),
		ID:      &id,
	}

	resp := msg.AsResponse()
	require.NotNil(t, resp)
	assert.Equal(t, "2.0", resp.JSONRPC)
	assert.Equal(t, &id, resp.ID)

	notifMsg := &Message{Method: "test"}
	assert.Nil(t, notifMsg.AsResponse())
}

func TestMessage_AsNotification(t *testing.T) {
	msg := &Message{
		JSONRPC: "2.0",
		Method:  "test/notification",
		Params:  json.RawMessage(`{"data":"value"}`),
	}

	notif := msg.AsNotification()
	require.NotNil(t, notif)
	assert.Equal(t, "2.0", notif.JSONRPC)
	assert.Equal(t, "test/notification", notif.Method)

	id := int64(1)
	respMsg := &Message{ID: &id}
	assert.Nil(t, respMsg.AsNotification())
}

func TestInput_Marshal(t *testing.T) {
	input := Input{
		Type: "text",
		Text: "Hello world",
	}

	data, err := json.Marshal(input)
	require.NoError(t, err)

	var decoded map[string]string
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, "text", decoded["type"])
	assert.Equal(t, "Hello world", decoded["text"])
}

func TestApprovalPolicy_Constants(t *testing.T) {
	assert.Equal(t, ApprovalPolicy("never"), ApprovalNever)
	assert.Equal(t, ApprovalPolicy("always"), ApprovalAlways)
	assert.Equal(t, ApprovalPolicy("once"), ApprovalOnce)
}

func TestThreadStartResult_Unmarshal(t *testing.T) {
	data := `{"thread":{"id":"thread-123"},"model":"gpt-5.2-codex-max"}`

	var result ThreadStartResult
	err := json.Unmarshal([]byte(data), &result)
	require.NoError(t, err)

	assert.Equal(t, "thread-123", result.Thread.ID)
	assert.Equal(t, "gpt-5.2-codex-max", result.Model)
}

func TestTurnStartParams_Marshal(t *testing.T) {
	params := TurnStartParams{
		ThreadID: "thread-123",
		Input: []Input{
			{Type: "text", Text: "Hello"},
		},
		ApprovalPolicy: ApprovalNever,
	}

	data, err := json.Marshal(params)
	require.NoError(t, err)

	var decoded map[string]interface{}
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, "thread-123", decoded["threadId"])
	assert.NotNil(t, decoded["input"])
	assert.Equal(t, "never", decoded["approvalPolicy"])
}

func TestInitializeParams_Marshal(t *testing.T) {
	params := InitializeParams{
		ClientInfo: ClientInfo{
			Name:    "test-client",
			Version: "1.0.0",
		},
	}

	data, err := json.Marshal(params)
	require.NoError(t, err)

	assert.Contains(t, string(data), "test-client")
	assert.Contains(t, string(data), "1.0.0")
}

func TestErrorCodes(t *testing.T) {
	assert.Equal(t, -32700, CodeParseError)
	assert.Equal(t, -32600, CodeInvalidRequest)
	assert.Equal(t, -32601, CodeMethodNotFound)
	assert.Equal(t, -32602, CodeInvalidParams)
	assert.Equal(t, -32603, CodeInternalError)
}
