# Claude Code Provider

A provider that uses the Claude CLI (`claude`) as a backend, allowing users to leverage their existing Claude subscription without requiring a separate API key.

## Overview

This provider spawns and communicates with the Claude CLI in print mode (`-p`), using bidirectional NDJSON streaming for efficient, low-latency interactions.

### Key Benefits

- **No API key required** - Uses existing Claude subscription
- **Persistent process** - Single long-lived process reduces latency
- **Context preservation** - CLI manages session state automatically
- **Streaming support** - Real-time token-by-token output
- **Custom tools** - Supported via JSON schema + system prompt pattern
- **Prompt caching** - CLI handles caching automatically (`cache_read_input_tokens`)

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                    claudecode.LanguageModel                      │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │  Claude CLI Process (long-lived)                          │   │
│  │                                                            │   │
│  │  claude -p --input-format stream-json                     │   │
│  │           --output-format stream-json                     │   │
│  │           --verbose                                        │   │
│  │           --include-partial-messages                       │   │
│  │           --model <model>                                  │   │
│  │           --tools ""                                       │   │
│  └──────────────────────────────────────────────────────────┘   │
│       ▲                                    │                     │
│       │ stdin (NDJSON)                     │ stdout (NDJSON)     │
│       │                                    ▼                     │
│  ┌────────────────┐                 ┌────────────────┐          │
│  │ Encoder        │                 │ Decoder        │          │
│  │ api.Message →  │                 │ NDJSON →       │          │
│  │ CLI format     │                 │ api.Response   │          │
│  └────────────────┘                 └────────────────┘          │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

## CLI Communication Protocol

### Input Format (stdin)

Messages are sent as NDJSON (newline-delimited JSON):

```json
{"type":"user","message":{"role":"user","content":"What is 2+2?"}}
```

### Output Format (stdout)

The CLI emits multiple event types as NDJSON:

| Event Type | Description |
|------------|-------------|
| `system.init` | Session initialization with model info |
| `stream_event` → `message_start` | Message begins |
| `stream_event` → `content_block_start` | Content block begins |
| `stream_event` → `content_block_delta` | Text chunks (streaming) |
| `stream_event` → `content_block_stop` | Content block ends |
| `stream_event` → `message_delta` | Usage stats, stop reason |
| `stream_event` → `message_stop` | Message ends |
| `assistant` | Full message snapshot |
| `result` | Final result with usage and cost |

### Example: Streaming Response

```json
{"type":"system","subtype":"init","session_id":"abc-123","model":"claude-sonnet-4-5-20250929",...}
{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}}
{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}}
{"type":"assistant","message":{"content":[{"type":"text","text":"Hello world"}],"usage":{...}}}
{"type":"result","subtype":"success","result":"Hello world","usage":{...},"total_cost_usd":0.001}
```

## Feature Support

### Supported Features

| Feature | CLI Flag/Method | Notes |
|---------|-----------------|-------|
| Model selection | `--model sonnet\|opus\|haiku` | Or full model ID |
| System prompt | `--system-prompt "..."` | Via flag or first message |
| Temperature | `--settings '{"temperature": 0.5}'` | 0.0 - 1.0 |
| Streaming | `--include-partial-messages` | Token-by-token |
| JSON schema output | `--json-schema '{...}'` | Structured output |
| Multi-turn | Persistent process | Context preserved |
| Custom tools | JSON schema + system prompt | See below |
| Session resume | `--resume <session_id>` | Alternative to persistent process |

### Unsupported/Limited Features

| Feature | Status | Workaround |
|---------|--------|------------|
| MaxOutputTokens | Not exposed | None |
| TopP/TopK | Not exposed | None |
| StopSequences | Not exposed | None |
| Native tool schema | Not supported | Use JSON schema pattern |
| Thinking mode | Not exposed | None |

## Custom Tool Calling

Tools are supported via a JSON schema pattern. The provider encodes tool definitions into the system prompt and uses `--json-schema` to get structured tool call responses.

### Flow

```
1. Provider encodes tools into system prompt
2. Sets --json-schema for tool call response format
3. CLI returns structured tool call
4. Provider sends tool result as next message
5. CLI incorporates result in final response
```

### Example

**Request with tools:**
```bash
claude -p \
  --system-prompt 'You have tools: get_weather(location: string). Respond with tool calls as JSON.' \
  --json-schema '{"type":"object","properties":{"tool_call":{"type":"object","properties":{"name":{"type":"string"},"args":{"type":"object"}}}}}' \
  --model sonnet \
  --tools "" \
  "What's the weather in NYC?"
```

**Response:**
```json
{
  "structured_output": {
    "tool_call": {
      "name": "get_weather",
      "args": {"location": "NYC"}
    }
  },
  "session_id": "abc-123"
}
```

**Send tool result (persistent process):**
```json
{"type":"user","message":{"role":"user","content":"Tool result for get_weather: 72°F, sunny"}}
```

**Or via --resume:**
```bash
claude -p --resume "abc-123" "Tool result for get_weather: 72°F, sunny"
```

## Implementation Plan

### Package Structure

```
provider/claudecode/
├── README.md           # This file
├── constants.go        # Provider name, model constants
├── llm.go              # LanguageModel implementation
├── llm_test.go         # Tests
├── process.go          # CLI process management
├── process_test.go     # Process tests
└── codec/
    ├── encode.go       # api.Message → CLI input format
    ├── decode.go       # CLI output → api.Response
    ├── events.go       # NDJSON event type definitions
    └── tools.go        # Tool encoding (JSON schema pattern)
```

### Core Types

```go
// LanguageModel wraps a persistent Claude CLI process
type LanguageModel struct {
    modelID  string
    process  *Process
    settings Settings
}

// Settings configurable via --settings flag
type Settings struct {
    Temperature *float64 `json:"temperature,omitempty"`
}

// Process manages the long-lived CLI subprocess
type Process struct {
    cmd      *exec.Cmd
    stdin    io.WriteCloser
    stdout   *bufio.Scanner
    tmpDir   string       // Sandbox directory (cleaned up on Close)
    mu       sync.Mutex
}

// CLIEvent represents a parsed NDJSON event from stdout
type CLIEvent struct {
    Type    string          `json:"type"`
    Subtype string          `json:"subtype,omitempty"`
    Event   json.RawMessage `json:"event,omitempty"`
    Message json.RawMessage `json:"message,omitempty"`
    Result  string          `json:"result,omitempty"`
    // ... other fields
}
```

### LanguageModel Interface

```go
func (m *LanguageModel) ProviderName() string { return "claudecode" }
func (m *LanguageModel) ModelID() string { return m.modelID }
func (m *LanguageModel) SupportedUrls() []api.SupportedURL { ... }
func (m *LanguageModel) Generate(ctx context.Context, prompt []api.Message, opts api.CallOptions) (*api.Response, error)
func (m *LanguageModel) Stream(ctx context.Context, prompt []api.Message, opts api.CallOptions) (*api.StreamResponse, error)
```

### Process Lifecycle

1. **Sandbox creation**: Create temp directory for process cwd
2. **Lazy initialization**: Process spawned on first Generate/Stream call
3. **Keep-alive**: Process reused for subsequent calls
4. **Health checks**: Verify process is responsive
5. **Graceful shutdown**: Clean termination on Close()
6. **Cleanup**: Remove temp directory on Close()
7. **Error recovery**: Respawn on unexpected termination

### Sandboxing

The CLI process is spawned in an **isolated temp directory** to prevent it from accessing or modifying files in the calling application's directory.

```go
// Create isolated working directory
tmpDir, err := os.MkdirTemp("", "claudecode-*")
if err != nil {
    return nil, fmt.Errorf("failed to create sandbox: %w", err)
}

cmd := exec.Command("claude", "-p", ...)
cmd.Dir = tmpDir  // Sandbox the process
```

This is important because:
- Claude Code has access to the cwd by default
- Even with `--tools ""`, this prevents any accidental file access
- Provides clean isolation between provider instances
- Temp dir is cleaned up on `Close()`

### Codec: Encoding

```go
// EncodeMessage converts api.Message to CLI input format
func EncodeMessage(msg api.Message) ([]byte, error) {
    // {"type":"user","message":{"role":"user","content":"..."}}
}

// EncodeSystemPrompt extracts system prompt from messages
func EncodeSystemPrompt(messages []api.Message) string

// EncodeTools converts tool definitions to system prompt + JSON schema
func EncodeTools(tools []api.ToolDefinition) (systemPromptAddition string, jsonSchema string, error)
```

### Codec: Decoding

```go
// DecodeEvent parses a single NDJSON line
func DecodeEvent(line []byte) (*CLIEvent, error)

// DecodeResponse converts final result event to api.Response
func DecodeResponse(event *CLIEvent) (*api.Response, error)

// DecodeStreamEvent converts stream_event to api.StreamEvent
func DecodeStreamEvent(event *CLIEvent) (api.StreamEvent, error)
```

## CLI Command Reference

### Basic Generation (non-streaming)

```bash
claude -p \
  --model sonnet \
  --tools "" \
  --output-format json \
  "Your prompt here"
```

### Streaming Generation

```bash
claude -p \
  --model sonnet \
  --tools "" \
  --output-format stream-json \
  --verbose \
  --include-partial-messages \
  "Your prompt here"
```

### Persistent Process (bidirectional)

```bash
claude -p \
  --model sonnet \
  --tools "" \
  --input-format stream-json \
  --output-format stream-json \
  --verbose \
  --include-partial-messages
```

Then send NDJSON messages to stdin:
```json
{"type":"user","message":{"role":"user","content":"Hello"}}
```

### With Settings

```bash
claude -p \
  --settings '{"temperature": 0.7}' \
  --model sonnet \
  --tools "" \
  "Your prompt"
```

### With System Prompt

```bash
claude -p \
  --system-prompt "You are a helpful assistant." \
  --model sonnet \
  --tools "" \
  "Your prompt"
```

### With JSON Schema Output

```bash
claude -p \
  --json-schema '{"type":"object","properties":{"answer":{"type":"string"}},"required":["answer"]}' \
  --model sonnet \
  --tools "" \
  "What is 2+2? Respond in JSON."
```

Response includes:
```json
{
  "structured_output": {"answer": "4"},
  "result": "..."
}
```

## Response Mapping

### Usage Mapping

| CLI Field | SDK Field |
|-----------|-----------|
| `usage.input_tokens` | `Usage.InputTokens` |
| `usage.output_tokens` | `Usage.OutputTokens` |
| `usage.cache_read_input_tokens` | `Usage.CachedInputTokens` |
| `total_cost_usd` | `ProviderMetadata.claudecode.cost_usd` |

### Finish Reason Mapping

| CLI `stop_reason` | SDK `FinishReason` |
|-------------------|-------------------|
| `end_turn` | `FinishReasonStop` |
| `max_tokens` | `FinishReasonLength` |
| `tool_use` | `FinishReasonToolCalls` |

### Stream Event Mapping

| CLI Event | SDK Event |
|-----------|-----------|
| `content_block_delta` → `text_delta` | `TextDeltaEvent` |
| `message_delta` | `FinishEvent` |
| `message_stop` | (end of stream) |

## Error Handling

### Process Errors

- **Process not found**: Return `ErrCLINotFound` with install instructions
- **Process crash**: Attempt respawn, return error if fails
- **Timeout**: Context cancellation propagated to process

### CLI Errors

- **result.is_error=true**: Map to appropriate SDK error type
- **Permission denied**: Return with details from `permission_denials`
- **Invalid input**: Parse error message and return `ErrInvalidPrompt`

## Testing Strategy

### Unit Tests

- Codec encode/decode with fixture data
- Event parsing for all event types
- Tool encoding to JSON schema

### Integration Tests

- Requires `claude` CLI installed
- Skip with `SKIP_CLAUDE_CLI_TESTS=1`
- Test basic generation, streaming, multi-turn

### Mock Process

- Create mock process for deterministic testing
- Replay recorded NDJSON sessions

## TDD Implementation Plan

### Phase 1: Codec Layer (Pure Functions, No I/O)

Test-first development of encoding/decoding logic using fixture data.

#### 1.1 Event Types (`codec/events.go` + `codec/events_test.go`)

**Write tests first** for parsing all CLI event types:

```go
func TestParseEvent_SystemInit(t *testing.T)
func TestParseEvent_Assistant(t *testing.T)
func TestParseEvent_StreamEvent_ContentBlockDelta(t *testing.T)
func TestParseEvent_StreamEvent_MessageDelta(t *testing.T)
func TestParseEvent_Result(t *testing.T)
func TestParseEvent_InvalidJSON(t *testing.T)
```

**Fixtures**: Create `testdata/events/` with captured NDJSON from real CLI runs.

#### 1.2 Response Decoding (`codec/decode.go` + `codec/decode_test.go`)

**Write tests first** for converting events to SDK types:

```go
func TestDecodeResponse_TextContent(t *testing.T)
func TestDecodeResponse_StructuredOutput(t *testing.T)
func TestDecodeResponse_Usage(t *testing.T)
func TestDecodeResponse_FinishReason(t *testing.T)
func TestDecodeResponse_Error(t *testing.T)
```

#### 1.3 Stream Event Decoding (`codec/decode_stream.go` + `codec/decode_stream_test.go`)

**Write tests first** for streaming events:

```go
func TestDecodeStreamEvent_TextDelta(t *testing.T)
func TestDecodeStreamEvent_MessageStart(t *testing.T)
func TestDecodeStreamEvent_MessageStop(t *testing.T)
```

#### 1.4 Message Encoding (`codec/encode.go` + `codec/encode_test.go`)

**Write tests first** for SDK → CLI format:

```go
func TestEncodeUserMessage(t *testing.T)
func TestEncodeSystemMessage(t *testing.T)
func TestEncodeAssistantMessage(t *testing.T)
func TestEncodeMultiTurnConversation(t *testing.T)
```

#### 1.5 Tool Encoding (`codec/tools.go` + `codec/tools_test.go`)

**Write tests first** for tool → JSON schema conversion:

```go
func TestEncodeTools_SingleFunction(t *testing.T)
func TestEncodeTools_MultipleFunctions(t *testing.T)
func TestEncodeToolResult(t *testing.T)
func TestEncodeTools_GeneratesValidJSONSchema(t *testing.T)
```

---

### Phase 2: Process Management (With Mocking)

#### 2.1 Process Interface (`process.go`)

Define interface for testability:

```go
type CLIProcess interface {
    Send(msg []byte) error
    Receive() ([]byte, error)
    Close() error
}
```

#### 2.2 Mock Process (`process_mock_test.go`)

Create mock for deterministic testing:

```go
type MockProcess struct {
    responses []string  // Pre-recorded NDJSON lines
    index     int
}

func (m *MockProcess) Send(msg []byte) error { return nil }
func (m *MockProcess) Receive() ([]byte, error) {
    // Return next pre-recorded response
}
```

#### 2.3 Process Tests (`process_test.go`)

**Write tests first** using mock:

```go
func TestProcess_SendReceive(t *testing.T)
func TestProcess_MultipleMessages(t *testing.T)
func TestProcess_ContextCancellation(t *testing.T)
func TestProcess_ErrorHandling(t *testing.T)
```

#### 2.4 Real Process (`process_real.go`)

Implement actual CLI spawning:

```go
func TestRealProcess_Spawn(t *testing.T)           // Skip if no CLI
func TestRealProcess_SandboxCreation(t *testing.T) // Verify temp dir
func TestRealProcess_Cleanup(t *testing.T)         // Verify temp dir removed
```

---

### Phase 3: LanguageModel Integration

#### 3.1 Generate Tests (`llm_test.go`)

**Write tests first** with mock process:

```go
func TestGenerate_SimplePrompt(t *testing.T)
func TestGenerate_WithSystemMessage(t *testing.T)
func TestGenerate_WithTemperature(t *testing.T)
func TestGenerate_WithJSONSchema(t *testing.T)
func TestGenerate_MultiTurn(t *testing.T)
func TestGenerate_Error(t *testing.T)
```

#### 3.2 Stream Tests (`llm_test.go`)

**Write tests first** with mock process:

```go
func TestStream_TextDeltas(t *testing.T)
func TestStream_Cancellation(t *testing.T)
func TestStream_Error(t *testing.T)
```

#### 3.3 Tool Calling Tests (`llm_test.go`)

**Write tests first** for custom tool flow:

```go
func TestGenerate_WithTools_ReturnsToolCall(t *testing.T)
func TestGenerate_WithToolResult_ReturnsResponse(t *testing.T)
```

---

### Phase 4: Integration Tests (Optional, Real CLI)

Skip unless `CLAUDECODE_INTEGRATION_TESTS=1`:

```go
func TestIntegration_RealCLI_SimpleGenerate(t *testing.T)
func TestIntegration_RealCLI_Streaming(t *testing.T)
func TestIntegration_RealCLI_MultiTurn(t *testing.T)
```

---

### File Structure After Implementation

```
provider/claudecode/
├── README.md
├── constants.go            # ProviderName, model constants
├── llm.go                  # LanguageModel implementation
├── llm_test.go             # LanguageModel tests (with mock)
├── process.go              # CLIProcess interface + real implementation
├── process_test.go         # Process tests
├── codec/
│   ├── events.go           # CLI event type definitions
│   ├── events_test.go
│   ├── decode.go           # CLI → SDK response
│   ├── decode_test.go
│   ├── decode_stream.go    # CLI → SDK stream events
│   ├── decode_stream_test.go
│   ├── encode.go           # SDK messages → CLI format
│   ├── encode_test.go
│   ├── tools.go            # Tool → JSON schema encoding
│   ├── tools_test.go
│   └── testdata/
│       ├── events/         # Captured NDJSON fixtures
│       │   ├── simple_response.ndjson
│       │   ├── streaming_response.ndjson
│       │   ├── tool_call_response.ndjson
│       │   └── error_response.ndjson
│       └── messages/       # SDK message fixtures
│           └── ...
└── integration_test.go     # Real CLI tests (build tag: integration)
```

---

### Execution Order

| Step | Files | Tests First | Depends On |
|------|-------|-------------|------------|
| 1 | `codec/events.go` | `codec/events_test.go` | - |
| 2 | `codec/decode.go` | `codec/decode_test.go` | Step 1 |
| 3 | `codec/decode_stream.go` | `codec/decode_stream_test.go` | Step 1 |
| 4 | `codec/encode.go` | `codec/encode_test.go` | - |
| 5 | `codec/tools.go` | `codec/tools_test.go` | Step 4 |
| 6 | `process.go` (interface) | - | - |
| 7 | `process.go` (mock) | `process_test.go` | Step 6 |
| 8 | `process.go` (real) | `process_test.go` | Step 7 |
| 9 | `llm.go` | `llm_test.go` | Steps 2-5, 7 |
| 10 | `constants.go` | - | - |
| 11 | `integration_test.go` | - | All above |

---

### Testing Commands

```bash
# Run codec tests only (fast, no I/O)
go test ./provider/claudecode/codec/...

# Run all unit tests (with mock process)
go test ./provider/claudecode/...

# Run integration tests (requires CLI)
CLAUDECODE_INTEGRATION_TESTS=1 go test ./provider/claudecode/... -v

# Run with race detector
go test -race ./provider/claudecode/...
```

## Future Considerations

1. **Process pool**: Multiple processes for concurrent requests
2. **Connection reuse**: Keep-alive semantics for high throughput
3. **MCP integration**: Expose CLI's MCP servers to SDK
4. **Cost tracking**: Surface `total_cost_usd` in provider metadata
5. **Model auto-detection**: Query available models from CLI
