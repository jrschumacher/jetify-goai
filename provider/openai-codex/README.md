# OpenAI Codex Provider

This provider enables using OpenAI models through the `codex` CLI without requiring a direct OpenAI API key. It leverages CLI authentication (ChatGPT OAuth) to access OpenAI's models with true token-by-token streaming support.

## Overview

The [Codex CLI](https://developers.openai.com/codex/cli/) is OpenAI's official command-line tool for interacting with their models. Key features:

- **No API key required** - Uses ChatGPT OAuth authentication
- **True streaming** - Token-by-token streaming via app-server mode
- **Persistent sessions** - Multi-turn conversations with automatic context management
- **Model access** - ChatGPT subscription provides access to codex-optimized models

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                   openaicodex.LanguageModel                      │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │  Codex CLI App-Server (persistent process)                │   │
│  │                                                            │   │
│  │  codex app-server                                         │   │
│  │  (JSON-RPC 2.0 over stdio)                                │   │
│  └──────────────────────────────────────────────────────────┘   │
│       ▲                                    │                     │
│       │ JSON-RPC requests                  │ Streaming deltas    │
│       │                                    ▼                     │
│  ┌────────────────┐                 ┌────────────────┐          │
│  │ Encoder        │                 │ Decoder        │          │
│  │ api.Message →  │                 │ JSON-RPC →     │          │
│  │ turn/start     │                 │ api.Response   │          │
│  └────────────────┘                 └────────────────┘          │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

## Authentication

### ChatGPT OAuth (No API Key Required)

```bash
codex login
```

This stores OAuth tokens in `~/.codex/auth.json`. Verify login status:

```bash
codex login status
# Output: "Logged in using ChatGPT"
```

## Available Models

With ChatGPT authentication, codex-optimized models are available:

| Model | ChatGPT Auth | API Key |
|-------|--------------|---------|
| `gpt-5.2-codex-max` | ✅ | ✅ |
| `gpt-5.2-codex` | ✅ | ✅ |
| `gpt-5.1-codex-max` | ✅ | ✅ |
| `o3` | ❌ | ✅ |
| `o4-mini` | ❌ | ✅ |
| `gpt-4o` | ❌ | ✅ |
| `gpt-4.1` | ❌ | ✅ |

*Last updated: January 2026. Model availability changes frequently.*

## Quick Start

### Basic Usage

```go
import (
    "context"
    "github.com/jetify-com/goai/api"
    codex "github.com/jetify-com/goai/provider/openai-codex"
)

// Create model (uses ChatGPT OAuth from ~/.codex/auth.json)
model := codex.NewLanguageModel("gpt-5.2-codex-max")

// Generate response
resp, err := model.Generate(ctx, []api.Message{
    {Role: api.RoleUser, Content: "What is 2 + 2?"},
}, api.CallOptions{})

// Stream response (true token-by-token streaming)
streamResp, err := model.Stream(ctx, []api.Message{
    {Role: api.RoleUser, Content: "Explain recursion"},
}, api.CallOptions{})

for event := range streamResp.Stream {
    if delta, ok := event.(*api.TextDeltaEvent); ok {
        fmt.Print(delta.TextDelta)
    }
}
```

### Example Output

```json
{"method":"item/agentMessage/delta","params":{"delta":"4"}}
{"method":"turn/completed","params":{"usage":{"inputTokens":4972,"outputTokens":9}}}
```

## Streaming Modes

The provider uses the **app-server** mode for true streaming:

### App-Server Protocol (Default)

```bash
codex app-server
```

- **Protocol**: JSON-RPC 2.0 over stdio
- **Streaming**: Token-by-token delta events (`item/agentMessage/delta`)
- **Sessions**: Persistent threads for multi-turn conversations
- **Performance**: Low latency, efficient for interactive use

### Key Differences from Claude CLI

| Feature | Claude CLI | Codex CLI |
|---------|-----------|-----------|
| Command | `claude` | `codex` |
| Auth | Anthropic OAuth | ChatGPT OAuth |
| Streaming mode | `-p --output-format stream-json` | `app-server` (JSON-RPC) |
| Session model | Stdin streaming | Persistent threads |
| Event format | JSONL | JSON-RPC notifications |

## Implementation Details

The provider follows the same architecture pattern as `anthropic-claudecode`:

- **Process Management**: Persistent `codex app-server` subprocess with automatic restart
- **JSON-RPC Communication**: Request/response with notification stream handling
- **Response Decoding**: Converts JSON-RPC notifications to `api.StreamEvent`
- **Codec Layer**: Separate encoding (`api.Message` → JSON-RPC) and decoding (JSON-RPC → `api.Response`)

For implementation details, see the [package documentation](https://pkg.go.dev/github.com/jetify-com/goai/provider/openai-codex).

## Package Structure

```
provider/openai-codex/
├── llm.go              # LanguageModel implementation
├── codec/
│   ├── events.go       # Event type definitions
│   ├── encode.go       # api.Message → JSON-RPC
│   ├── decode.go       # JSON-RPC → api.Response
│   ├── decode_appserver.go # App-server notification decoding
│   └── metadata.go     # Provider-specific metadata
└── process/
    ├── interface.go    # AppServer interface
    ├── appserver.go    # Implementation
    └── mock.go         # MockAppServer for testing
```

## Testing

### Unit Tests

Run unit tests with mock process (no CLI required):

```bash
go test ./provider/openai-codex/...
```

### Integration Tests

Integration tests require the `codex` CLI installed and configured:

```bash
# Skip integration tests (default)
go test ./provider/openai-codex/...

# Run integration tests
go test ./provider/openai-codex -tags=integration -v
```

**Requirements for integration tests**:
- `codex` CLI installed ([installation guide](https://developers.openai.com/codex/cli/))
- Authenticated via `codex login`
- ChatGPT subscription with access to codex models

## Error Handling

The provider includes robust error handling with user-friendly messages and automatic classification.

### Error Classification

Errors are automatically classified into categories with actionable guidance:

```go
import codex "github.com/jetify-com/goai/provider/openai-codex"

model := codex.NewLanguageModel("gpt-5.2-codex-max")
resp, err := model.Generate(ctx, prompt, api.CallOptions{})

if err != nil {
    // Errors are automatically classified
    errInfo, ok := err.(*codex.ErrorInfo)
    if ok {
        fmt.Printf("Category: %s\n", errInfo.Category)
        fmt.Printf("Message: %s\n", errInfo.UserMessage)
        fmt.Printf("Action: %s\n", errInfo.SuggestedAction)
        fmt.Printf("Retryable: %v\n", errInfo.Retryable)
    }
}
```

### Error Categories

| Category | Retryable | Common Causes | Suggested Action |
|----------|-----------|---------------|------------------|
| `quota_exceeded` | No | API credits exhausted | Add credits at platform.openai.com/account/billing |
| `rate_limited` | Yes | Too many requests | Wait before retrying or reduce frequency |
| `authentication_failed` | No | Invalid credentials | Run 'codex login' to authenticate |
| `service_unavailable` | Yes | Temporary outage | Retry in a few moments |
| `model_not_available` | No | Invalid model ID | Check model availability |
| `process_failure` | Yes | CLI not found | Ensure 'codex' CLI is installed |

### Retry Logic

Automatic retry with exponential backoff for transient failures:

```go
retryPolicy := codex.NewRetryPolicy(
    codex.WithMaxAttempts(3),
    codex.WithInitialDelay(time.Second),
    codex.WithMaxDelay(60 * time.Second),
    codex.WithMultiplier(2.0),
    codex.WithJitter(true),
)

model := codex.NewLanguageModel("gpt-5.2-codex-max",
    codex.WithRetryPolicy(retryPolicy),
)

// Automatic retries on rate limits, service unavailability, etc.
resp, err := model.Generate(ctx, prompt, api.CallOptions{})
```

**Retry Behavior**:
- Retries only on transient failures (rate limits, service issues)
- Exponential backoff: 1s → 2s → 4s (with jitter)
- Respects context cancellation
- Non-retryable errors fail immediately

## Cost Monitoring

Track token usage and enforce budget limits to prevent unexpected costs.

### Basic Usage

```go
// Create cost monitor with 100K token budget
monitor := codex.NewCostMonitor(100_000,
    codex.WithWarningThreshold(0.8), // Alert at 80%
    codex.WithAlertCallback(func(alert codex.CostAlert) {
        log.Printf("ALERT: Used %d/%d tokens (%.1f%%)",
            alert.Used, alert.Budget, alert.Percent*100)
    }),
)

model := codex.NewLanguageModel("gpt-5.2-codex-max",
    codex.WithCostMonitor(monitor),
)

// Usage is tracked automatically
resp, err := model.Generate(ctx, prompt, api.CallOptions{})
if err != nil {
    // Budget exceeded errors are caught here
    return err
}

// Check current usage
stats := monitor.Usage()
fmt.Printf("Used: %d/%d tokens (%.1f%% remaining)\n",
    stats.Used, stats.Budget, float64(stats.Remaining)/float64(stats.Budget)*100)
```

### Alert Types

**Warning Alert** (default: 80% of budget):
```go
monitor := codex.NewCostMonitor(100_000,
    codex.WithAlertCallback(func(alert codex.CostAlert) {
        if alert.AlertType == codex.AlertTypeWarning {
            // Notify user approaching limit
            log.Printf("WARNING: %d%% of budget used", int(alert.Percent*100))
        }
    }),
)
```

**Critical Alert** (budget exceeded):
```go
monitor := codex.NewCostMonitor(100_000,
    codex.WithAlertCallback(func(alert codex.CostAlert) {
        if alert.AlertType == codex.AlertTypeCritical {
            // Budget exceeded - requests will fail
            log.Printf("CRITICAL: Budget exceeded!")
            // Trigger notifications, email, etc.
        }
    }),
)
```

### Usage Statistics

```go
stats := monitor.Usage()

fmt.Printf("Total: %d tokens\n", stats.Used)
fmt.Printf("Input: %d tokens\n", stats.InputTokens)
fmt.Printf("Output: %d tokens\n", stats.OutputTokens)
fmt.Printf("Remaining: %d tokens\n", stats.Remaining)
fmt.Printf("Percent: %.1f%%\n", stats.Percent*100)
fmt.Printf("Duration: %s\n", stats.Duration)

// Check if you can afford an estimated request
if monitor.CanAfford(5000) {
    // Safe to make request
    resp, err := model.Generate(ctx, prompt, api.CallOptions{})
}

// Reset usage (e.g., monthly reset)
monitor.Reset()
```

### Per-User Budgets

```go
type UserSession struct {
    userID  string
    monitor *codex.CostMonitor
    model   *codex.LanguageModel
}

func NewUserSession(userID string, budget int) *UserSession {
    monitor := codex.NewCostMonitor(budget)
    model := codex.NewLanguageModel("gpt-5.2-codex-max",
        codex.WithCostMonitor(monitor),
    )

    return &UserSession{
        userID:  userID,
        monitor: monitor,
        model:   model,
    }
}

// Each user has their own budget and tracking
session := NewUserSession("user-123", 50_000)
resp, err := session.model.Generate(ctx, prompt, api.CallOptions{})
```

## Configuration

### CLI Config File

Config file: `~/.codex/config.toml`

```toml
model = "gpt-5.2-codex-max"

[mcp_servers.playwright]
command = "npx"
args = ["@playwright/mcp@latest"]
```

### Programmatic Configuration

```go
import (
    codex "github.com/jetify-com/goai/provider/openai-codex"
    "github.com/jetify-com/goai/provider/openai-codex/process"
)

// Create model with advanced options
model := codex.NewLanguageModel("gpt-5.2-codex-max")

// Configure sandbox mode (restricts file system access)
process.WithSandboxMode("workspace-write")

// Enable network access and web search
process.WithNetworkAccess(true)
process.WithWebSearch(true)

// Configure MCP servers for tool extensions
process.WithMCPServer("playwright", "npx", "@playwright/mcp@latest")
process.WithMCPServerEnv("custom-tool", "python",
    map[string]string{"API_KEY": "secret"},
    "-m", "custom_tool")

// Set working directory for process isolation
process.WithWorkDir("/path/to/workspace")
```

### Advanced Options

| Option | Type | Description | Default |
|--------|------|-------------|---------|
| `SandboxMode` | string | File system access: "workspace-write", "read-only", "no-access" | "" |
| `SkipGitRepoCheck` | bool | Allow non-git directories | false |
| `NetworkAccess` | bool | Enable network connectivity | false |
| `WebSearch` | bool | Enable web search capability | false |
| `MCPServers` | map | MCP server configurations for tool extensions | nil |
| `WorkDir` | string | Working directory for process isolation | "" |

**Sandboxing Note**: Unlike the `claude` CLI which has filesystem access to the CWD by default, the `codex app-server` process can be sandboxed by setting `WorkDir` to an isolated directory. For security-sensitive applications, consider:

```go
// Create isolated temp directory for process
tmpDir, err := os.MkdirTemp("", "codex-sandbox-*")
if err != nil {
    return err
}
defer os.RemoveAll(tmpDir) // Cleanup

model := codex.NewLanguageModel("gpt-5.2-codex-max",
    codex.WithAppServer(
        process.NewAppServerProcess(
            process.WithWorkDir(tmpDir),
            process.WithSandboxMode("workspace-write"),
        ),
    ),
)
```

**Note**: Some options may require specific codex CLI versions or additional configuration. Refer to the [Codex CLI documentation](https://developers.openai.com/codex/cli/) for compatibility details.

## References

- [Codex CLI Documentation](https://developers.openai.com/codex/cli/)
- [Codex CLI Reference](https://developers.openai.com/codex/cli/reference/)
- [App-Server Protocol](https://github.com/openai/codex/blob/main/docs/app-server.md)
- [Package Documentation](https://pkg.go.dev/github.com/jetify-com/goai/provider/openai-codex)
