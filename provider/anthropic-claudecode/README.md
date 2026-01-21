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
│                    claudecode.LanguageModel                     │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │  Claude CLI Process (long-lived)                         │  │
│  │                                                          │  │
│  │  claude -p --input-format stream-json                    │  │
│  │           --output-format stream-json                    │  │
│  │           --tools ""                                     │  │
│  │           --model <model>                                │  │
│  │           --verbose (if enabled)                         │  │
│  │           --include-partial-messages                     │  │
│  └──────────────────────────────────────────────────────────┘  │
│       ▲                                    │                    │
│       │ stdin (NDJSON)                     │ stdout (NDJSON)    │
│       │                                    ▼                    │
│  ┌────────────────┐                 ┌────────────────┐         │
│  │ codec.Encode   │                 │ codec.Decode   │         │
│  │ api.Message →  │                 │ NDJSON →       │         │
│  │ CLI format     │                 │ api.Response   │         │
│  └────────────────┘                 └────────────────┘         │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

## Quick Start

```go
import (
    "context"
    "go.jetify.com/ai"
    "go.jetify.com/ai/provider/anthropic-claudecode"
)

func main() {
    model := claudecode.NewLanguageModel("claude-sonnet-4-5-20250929")

    response, err := ai.GenerateTextStr(
        context.Background(),
        "Explain quantum computing in simple terms",
        ai.WithModel(model),
    )
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(response)
}
```

## CLI Communication Protocol

The provider communicates with the Claude CLI using NDJSON (newline-delimited JSON):

**Input (stdin):**
```json
{"type":"user","message":{"role":"user","content":"What is 2+2?"}}
```

**Output (stdout):**
```json
{"type":"system","subtype":"init","session_id":"abc-123","model":"claude-sonnet-4-5-20250929"}
{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"4"}}}
{"type":"result","subtype":"success","result":"4","usage":{"input_tokens":10,"output_tokens":5},"total_cost_usd":0.001}
```

The CLI emits various event types including `system.init`, `stream_event`, `assistant`, and `result`. See [pkg.go.dev](https://pkg.go.dev/go.jetify.com/ai/provider/anthropic-claudecode) for complete event type documentation.

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
| Custom tools | JSON schema + system prompt | Encoded in system prompt |
| Session resume | `--resume <session_id>` | Alternative to persistent process |
| Max turns | `--max-turns N` | Limit conversation turns |

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

**Flow:**
1. Provider encodes tools into system prompt
2. Sets `--json-schema` for tool call response format
3. CLI returns structured tool call
4. Provider sends tool result as next message
5. CLI incorporates result in final response

See [examples](../../examples/) for complete tool calling examples.

## Testing

### Unit Tests

Unit tests use mock processes for fast, deterministic testing:

```bash
# Run all unit tests (with mock process)
go test ./provider/anthropic-claudecode/...

# Run with race detector
go test -race ./provider/anthropic-claudecode/...
```

### Integration Tests

Integration tests require the `claude` CLI to be installed and use real API calls:

```bash
# Run integration tests
go test ./provider/anthropic-claudecode -tags=integration -v
```

The integration tests cover:
- Basic text generation and streaming
- System prompts and multi-turn conversations
- Temperature control and special character handling
- JSON output and code generation
- Usage metadata and finish reasons

## API Reference

For complete API documentation including all types, interfaces, and functions, see:
- [pkg.go.dev/go.jetify.com/ai/provider/anthropic-claudecode](https://pkg.go.dev/go.jetify.com/ai/provider/anthropic-claudecode)

Key packages:
- `process` - CLI process management and configuration
- `codec` - NDJSON encoding/decoding between SDK and CLI formats
- `metadata` - Provider-specific metadata extraction

## Requirements

- Claude CLI (`claude`) must be installed and authenticated
- Valid Claude subscription (uses existing subscription, no separate API key needed)

## Future Considerations

- Process pool for concurrent requests
- MCP integration to expose CLI's MCP servers
- Health checks and auto-respawn support
