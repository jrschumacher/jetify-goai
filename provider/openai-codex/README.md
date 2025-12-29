# OpenAI Codex Provider

This provider enables using OpenAI models through the `codex` CLI without requiring a direct OpenAI API key. Similar to the `claudecode` provider, it leverages CLI authentication (ChatGPT OAuth) to access OpenAI's models.

## Overview

The [Codex CLI](https://developers.openai.com/codex/cli/) is OpenAI's official command-line tool for interacting with their models. It supports OAuth-based authentication through ChatGPT accounts, eliminating the need for API keys.

## Authentication

### ChatGPT OAuth (No API Key Required)

The codex CLI uses device authentication flow tied to ChatGPT accounts:

```bash
codex login
```

This stores OAuth tokens in `~/.codex/auth.json`:

```json
{
  "OPENAI_API_KEY": null,
  "tokens": {
    "id_token": "...",
    "access_token": "...",
    "refresh_token": "..."
  }
}
```

### Login Status

```bash
codex login status
# Output: "Logged in using ChatGPT"
```

## Available Models

With ChatGPT authentication, only certain models are available:

| Model | ChatGPT Auth | API Key |
|-------|--------------|---------|
| `gpt-5.1-codex-max` | ✅ | ✅ |
| `o3` | ❌ | ✅ |
| `o4-mini` | ❌ | ✅ |
| `gpt-4o` | ❌ | ✅ |
| `gpt-4.1` | ❌ | ✅ |

When using an unsupported model with ChatGPT auth, you'll receive:
```
The '<model>' model is not supported when using Codex with a ChatGPT account.
```

## Programmatic Usage

### Non-Interactive Execution

```bash
codex exec --json --skip-git-repo-check "Your prompt here"
```

Key flags:
- `--json`: Output events as JSONL to stdout
- `--skip-git-repo-check`: Allow running outside git repositories
- `--model <MODEL>`: Specify model (default from config)
- `-o, --output-last-message <FILE>`: Write final response to file
- `--output-schema <FILE>`: Enforce JSON schema on output

### Example

```bash
codex exec --json --skip-git-repo-check "What is 2 + 2?"
```

Output:
```jsonl
{"type":"thread.started","thread_id":"019b3cf8-21e8-7430-9ca3-4d38435173e4"}
{"type":"turn.started"}
{"type":"item.completed","item":{"id":"item_0","type":"reasoning","text":"..."}}
{"type":"item.completed","item":{"id":"item_1","type":"agent_message","text":"4"}}
{"type":"turn.completed","usage":{"input_tokens":4972,"cached_input_tokens":0,"output_tokens":9}}
```

## JSONL Event Format

### Event Types

| Event Type | Description |
|------------|-------------|
| `thread.started` | Session initialization, contains `thread_id` |
| `turn.started` | Beginning of a request/response turn |
| `turn.completed` | End of turn, contains `usage` metrics |
| `turn.failed` | Turn failure, contains `error` details |
| `item.started` | Item creation started |
| `item.updated` | Item content updated (streaming) |
| `item.completed` | Item finalized |
| `error` | Unrecoverable error |

### Item Types

Items within `item.*` events have these types:

| Item Type | Description |
|-----------|-------------|
| `agent_message` | Natural language response from the assistant |
| `reasoning` | Summary of the assistant's thinking process |
| `command_execution` | Shell command executed by the assistant |
| `file_change` | File modification made by the assistant |
| `mcp_tool_call` | Model Context Protocol tool invocation |
| `web_search` | Web search operation |
| `todo_list` | Agent's running plan |

### Event Structures

#### thread.started
```json
{
  "type": "thread.started",
  "thread_id": "019b3cf8-21e8-7430-9ca3-4d38435173e4"
}
```

#### item.completed
```json
{
  "type": "item.completed",
  "item": {
    "id": "item_1",
    "type": "agent_message",
    "text": "The response text"
  }
}
```

#### turn.completed
```json
{
  "type": "turn.completed",
  "usage": {
    "input_tokens": 4972,
    "cached_input_tokens": 0,
    "output_tokens": 9
  }
}
```

#### error
```json
{
  "type": "error",
  "message": "Error description"
}
```

## Streaming via App-Server Protocol

The `codex exec --json` mode only provides `item.completed` events (no incremental streaming). For true token-by-token streaming, use the **app-server** JSON-RPC protocol:

```bash
codex app-server
```

### App-Server Protocol

The app-server uses JSON-RPC 2.0 over stdio. Initialize, start a thread, then start a turn:

```json
{"jsonrpc":"2.0","method":"initialize","params":{"clientInfo":{"name":"goai","version":"1.0"}},"id":0}
{"jsonrpc":"2.0","method":"thread/start","params":{},"id":1}
{"jsonrpc":"2.0","method":"turn/start","params":{"threadId":"<from thread/start>","input":[{"type":"text","text":"Hello"}],"approvalPolicy":"never"},"id":2}
```

### Streaming Delta Events

The app-server emits incremental delta events:

| Event Method | Description | Payload |
|--------------|-------------|---------|
| `item/agentMessage/delta` | Agent response text delta | `{itemId, delta}` |
| `item/reasoning/summaryTextDelta` | Reasoning text delta | `{itemId, delta, summaryIndex}` |
| `item/started` | Item creation started | `{item}` |
| `item/completed` | Item finalized | `{item}` |
| `turn/started` | Turn began | `{turn}` |
| `turn/completed` | Turn finished | `{turn, usage}` |
| `thread/started` | Thread initialized | `{thread}` |

### Example Streaming Output

```
>>> initialize
<<< {"id":0,"result":{"userAgent":"codex_cli_rs/0.63.0..."}}

>>> thread/start
<<< {"id":1,"result":{"thread":{"id":"019b..."},"model":"gpt-5.1-codex-max"}}
<<< {"method":"thread/started","params":{...}}

>>> turn/start
<<< {"method":"turn/started","params":{...}}
<<< {"method":"item/started","params":{"item":{"type":"reasoning",...}}}
<<< {"method":"item/reasoning/summaryTextDelta","params":{"delta":"**Thinking"}}
<<< {"method":"item/reasoning/summaryTextDelta","params":{"delta":" about"}}
<<< {"method":"item/completed","params":{"item":{"type":"reasoning",...}}}
<<< {"method":"item/started","params":{"item":{"type":"agentMessage",...}}}
<<< {"method":"item/agentMessage/delta","params":{"delta":"Hello"}}
<<< {"method":"item/agentMessage/delta","params":{"delta":" there"}}
<<< {"method":"item/agentMessage/delta","params":{"delta":" friend"}}
<<< {"method":"item/completed","params":{"item":{"type":"agentMessage","text":"Hello there friend"}}}
<<< {"method":"turn/completed","params":{"usage":{"inputTokens":...,"outputTokens":...}}}
```

### Protocol Schema

Generate JSON schemas for the full protocol:

```bash
codex app-server generate-json-schema --out ./schemas
ls schemas/v2/  # AgentMessageDeltaNotification.json, etc.
```

### Streaming vs Exec Mode

| Feature | `codex exec --json` | `codex app-server` |
|---------|--------------------|--------------------|
| Token streaming | ❌ (completed only) | ✅ (delta events) |
| Protocol | Simple JSONL | JSON-RPC 2.0 |
| Session model | One-shot | Persistent threads |
| Multi-turn | Resume by ID | Same thread |
| Complexity | Simple | More complex |

**Recommendation**: Use `app-server` for streaming, `exec` for simple one-shot queries.

## Comparison with Claude CLI

| Feature | Claude CLI | Codex CLI |
|---------|-----------|-----------|
| Command | `claude` | `codex` |
| Auth command | `claude login` | `codex login` |
| Auth method | Anthropic OAuth | ChatGPT OAuth |
| Non-interactive | `-p` flag | `exec` subcommand |
| Input format | `--input-format stream-json` | Prompt as argument |
| Output format | `--output-format stream-json` | `--json` flag |
| Session model | Stdin-based streaming | One-shot execution |
| Event: init | `{"type":"system","subtype":"init"}` | `{"type":"thread.started"}` |
| Event: content | `{"type":"assistant","message":{}}` | `{"type":"item.completed","item":{}}` |
| Event: result | `{"type":"result"}` | `{"type":"turn.completed"}` |

## Configuration

Config file: `~/.codex/config.toml`

```toml
model = "gpt-5.1-codex-max"

[mcp_servers.playwright]
command = "npx"
args = ["@playwright/mcp@latest"]
```

## Implementation Notes

### Provider Design

This provider follows the same pattern as `claudecode`:

1. **Process Management**: Spawn `codex exec` as subprocess
2. **Message Encoding**: Convert API messages to prompt strings
3. **Event Parsing**: Parse JSONL events from stdout
4. **Response Decoding**: Convert events to `api.Response`

### Key Differences from claudecode

1. **Two Modes**:
   - `codex exec --json` for simple one-shot queries (no streaming)
   - `codex app-server` for streaming via JSON-RPC protocol
2. **Event Structure**: Different event types and item-based content model
3. **Usage Location**: Token usage in `turn.completed` rather than in result event
4. **Session Tracking**: Uses `thread_id` instead of `session_id`
5. **Streaming Protocol**: JSON-RPC 2.0 vs Claude's simple JSONL streaming

### Mapping to API Types

**For `codex exec` (non-streaming):**

| Codex Event | API Response Field |
|-------------|-------------------|
| `item.completed` (agent_message) | `Content` (TextBlock) |
| `item.completed` (reasoning) | `ProviderMetadata` |
| `turn.completed.usage` | `Usage` |
| `thread_id` | `ResponseInfo.ID` |
| `turn.failed.error` | Return as error |

**For `codex app-server` (streaming):**

| App-Server Event | API StreamEvent |
|------------------|-----------------|
| `item/agentMessage/delta` | `TextDeltaEvent{TextDelta: delta}` |
| `item/reasoning/summaryTextDelta` | (provider metadata or skip) |
| `item/started` (agentMessage) | `ResponseMetadataEvent` |
| `item/completed` | (finalize content) |
| `turn/completed` | `FinishEvent{Usage: ...}` |
| `turn/failed` | `ErrorEvent` |

## Implementation Patterns (from anthropic-claudecode)

These patterns were established in the `anthropic-claudecode` provider and apply here:

### Package Architecture

```
provider/openai-codex/
├── constants.go        # ProviderName, model constants
├── llm.go              # LanguageModel implementation
├── llm_test.go         # Unit tests with mock process
├── integration_test.go # Integration tests (build tag)
├── codec/
│   ├── events.go       # JSONL event type definitions
│   ├── encode.go       # api.Message → CLI input format
│   ├── decode.go       # CLI output → api.Response
│   ├── decode_stream.go # Stream event decoding
│   └── metadata.go     # Provider-specific metadata
└── process/
    ├── interface.go    # Process interface for testability
    ├── process.go      # Real CLI process implementation
    └── mock.go         # Mock process for unit tests
```

### Process Interface Pattern

```go
type Process interface {
    Start(ctx context.Context) error
    Stop() error
    Stdin() io.Writer   // For streaming input (if needed)
    Stdout() io.Reader  // JSONL event stream
    Stderr() io.Reader
    Wait() error
    IsRunning() bool
    SessionID() string
    SetSessionID(id string)
}
```

### Streaming Implementation

Return `iter.Seq[api.StreamEvent]` that reads from stdout:

```go
func (d *streamDecoder) decodeEvents() iter.Seq[api.StreamEvent] {
    return func(yield func(api.StreamEvent) bool) {
        scanner := bufio.NewScanner(d.proc.Stdout())

        for scanner.Scan() {
            event, err := codec.ParseEvent(scanner.Bytes())
            if err != nil {
                yield(&api.ErrorEvent{Err: err})
                continue
            }

            streamEvent := decodeStreamEvent(event)
            if streamEvent != nil {
                if !yield(streamEvent) {
                    return
                }
            }
        }

        // End with FinishEvent containing usage stats
        yield(&api.FinishEvent{
            FinishReason: finishReason,
            Usage:        usage,
        })
    }
}
```

### Mock Process for Testing

```go
type MockProcess struct {
    stdout *bytes.Buffer
    stdin  *bytes.Buffer
    // ...
}

func (m *MockProcess) WriteStdout(data []byte) {
    m.stdout.Write(data)
}

// In tests:
mockProc := process.NewMockProcess()
mockProc.WriteStdout([]byte(`{"type":"thread.started","thread_id":"abc"}` + "\n"))
mockProc.WriteStdout([]byte(`{"type":"turn.completed","usage":{...}}` + "\n"))

model := NewLanguageModel("gpt-5.1-codex-max", WithProcess(mockProc))
```

### Integration Tests

Use build tags to separate from unit tests:

```go
//go:build integration

package codex

// Run with: go test ./provider/openai-codex -tags=integration -v
```

### Gotchas from Claude Code Implementation

1. **Response field fallback**: CLI may return text in different fields - implement fallback logic in decoder
2. **Stream event JSON keys**: The inner event data may use a different JSON key than the outer type
3. **Nil event handling**: Some stream events (like `content_block_stop`) should return nil and be skipped
4. **Finish reason mapping**: Map CLI-specific stop reasons to `api.FinishReason` constants
5. **Provider metadata**: Use `api.NewProviderMetadata(map[string]any{ProviderName: &metadata})` pattern

### Event Flow

```
CLI stdout → ParseEvent() → DecodeStreamEvent() → yield to iterator
                                   ↓
                          (nil for internal events)
```

## References

- [Codex CLI Documentation](https://developers.openai.com/codex/cli/)
- [Codex CLI Reference](https://developers.openai.com/codex/cli/reference/)
- [Codex exec Documentation](https://github.com/openai/codex/blob/main/docs/exec.md)
- [Codex CLI Features](https://developers.openai.com/codex/cli/features/)
- [Configuring Codex](https://developers.openai.com/codex/local-config/)
