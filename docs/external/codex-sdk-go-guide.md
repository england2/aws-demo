# Codex SDK Go Guide

Source: [`github.com/pmenglund/codex-sdk-go`](https://github.com/pmenglund/codex-sdk-go) on pkg.go.dev  
Version checked: `v0.0.0-...-89b0f6c`  
Published: 2026-05-09  
License: Apache-2.0

## Package Status

- Module: `github.com/pmenglund/codex-sdk-go`
- Package name: `codex`
- Imports: 10
- Imported by: 2
- Valid `go.mod`: yes
- Redistributable license: yes
- Tagged version: no
- Stable version: no, pre-`v1`
- Repository: <https://github.com/pmenglund/codex-sdk-go>

## Overview

`codex-sdk-go` embeds the Codex app-server in Go workflows.

The SDK speaks JSON-RPC to the `codex app-server` process. By default it spawns the Codex CLI and communicates over stdio. It exposes a high-level facade for threads and turns, with lower-level access available through `(*Codex).Client()`.

## Requirements

- Go 1.25+
- `codex` available on `PATH`

## Install

```sh
go get github.com/pmenglund/codex-sdk-go
```

## Quickstart

```go
package main

import (
    "context"
    "fmt"
    "log/slog"
    "os"

    "github.com/pmenglund/codex-sdk-go"
)

func main() {
    ctx := context.Background()
    logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
    prompt := "Diagnose the test failure and propose a fix"

    client, err := codex.New(ctx, codex.Options{Logger: logger})
    if err != nil {
        panic(err)
    }
    defer client.Close()

    thread, err := client.StartThread(ctx, codex.ThreadStartOptions{})
    if err != nil {
        panic(err)
    }

    result, err := thread.Run(ctx, prompt, nil)
    if err != nil {
        panic(err)
    }

    fmt.Println(result.FinalResponse)
}
```

`New` uses its `context.Context` for initialization requests (`initialize` / `initialized`). After `New` returns successfully, the spawned app-server lifetime is managed by `Close`, so canceling the constructor context later does not terminate the process.

## Streaming

Use `RunStreamed` to receive notifications as the turn progresses.

```go
prompt := "Inspect the repo"
stream, err := thread.RunStreamed(ctx, []codex.Input{codex.TextInput(prompt)}, nil)
if err != nil {
    panic(err)
}

defer stream.Close()

for {
    note, err := stream.Next(ctx)
    if err != nil {
        break
    }
    fmt.Printf("%s\n", note.Method)
    if note.Method == "turn/completed" {
        break
    }
}
```

`RunStreamed` returns thread-scoped events plus notifications that omit `threadId`, such as account/session updates, so global events are not silently dropped.

## Approvals

Configure approval handling by supplying a handler when constructing the client.

```go
logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
client, err := codex.New(ctx, codex.Options{
    Logger:          logger,
    ApprovalHandler: codex.AutoApproveHandler{Logger: logger},
})
```

For custom approval logic, implement `rpc.ServerRequestHandler` from the `rpc` package.

## Structured Output

Provide a JSON Schema to constrain the final assistant message.

```go
prompt := "Summarize repo status"
schema := codex.MustJSON(map[string]any{
    "type": "object",
    "properties": map[string]any{
        "summary": map[string]any{"type": "string"},
        "status": map[string]any{"type": "string", "enum": []string{"ok", "action_required"}},
    },
    "required": []string{"summary", "status"},
    "additionalProperties": false,
})

_, err := thread.RunInputs(ctx, []codex.Input{codex.TextInput(prompt)}, &codex.TurnOptions{
    OutputSchema: schema,
})
```

## JSON-Typed Options

Fields such as `ApprovalPolicy`, `SandboxPolicy`, `Effort`, `Summary`, and `OutputSchema` accept any JSON-marshalable value. If you already have raw JSON, pass a `json.RawMessage` or `codex.MustJSON(...)` to avoid double encoding.

For common values, prefer typed constants from this package:

- `codex.ApprovalPolicyNever`
- `codex.ApprovalPolicyOnFailure`
- `codex.ApprovalPolicyOnRequest`
- `codex.ApprovalPolicyUntrusted`
- `codex.SandboxModeReadOnly`
- `codex.SandboxModeWorkspaceWrite`
- `codex.SandboxModeDangerFullAccess`
- `codex.ReasoningEffortNone`
- `codex.ReasoningEffortMinimal`
- `codex.ReasoningEffortLow`
- `codex.ReasoningEffortMedium`
- `codex.ReasoningEffortHigh`
- `codex.ReasoningEffortXHigh`

## Low-Level RPC

Use the RPC client directly for full control.

```go
rpcClient := client.Client()
models, err := rpcClient.ModelList(ctx, protocol.ModelListParams{})
```

## Code Generation

Regenerate protocol types and RPC stubs:

```sh
go generate ./...
```

Generate from a specific Codex tag, branch, or commit without changing the checkout at `$CODEX_REPO_ROOT`:

```sh
CODEX_REPO_ROOT=../codex CODEX_REPO_REF=<tag> go generate ./...
```

This runs:

- `cargo run -p codex-app-server-protocol --bin export`
- `go-jsonschema`, via `internal/codegen`

The generator needs a checkout of `openai/codex` to export schemas. It resolves that checkout in this order:

1. `$CODEX_REPO_ROOT`, if set
2. `../codex`, by default

If `$CODEX_REPO_REF` is set, generation runs from a temporary detached Git worktree at that ref. Fetch the desired tag or ref in the Codex checkout before running generation.

Generated files include a header line with the exact Codex commit hash used. Generated files are checked in under `protocol` and `rpc`.

## API Reference

### Constants

```go
const (
    // InputTypeText represents a plain text input.
    InputTypeText = "text"

    // InputTypeImage represents a remote image input.
    InputTypeImage = "image"

    // InputTypeLocalImage represents a local image input.
    InputTypeLocalImage = "localImage"

    // InputTypeSkill represents a skill invocation input.
    InputTypeSkill = "skill"
)
```

### Variables

This section is empty.

### Functions

This section is empty.

### `type ApprovalPolicy`

```go
type ApprovalPolicy = string
```

`ApprovalPolicy` is a typed alias for common approval policy values.

```go
const (
    ApprovalPolicyNever     ApprovalPolicy = "never"
    ApprovalPolicyOnFailure ApprovalPolicy = "on-failure"
    ApprovalPolicyOnRequest ApprovalPolicy = "on-request"
    ApprovalPolicyUntrusted ApprovalPolicy = "untrusted"
)
```

### `type AutoApproveHandler`

```go
type AutoApproveHandler struct {
    Logger *slog.Logger
}
```

`AutoApproveHandler` accepts every approval request it can. `Logger` controls approval logging. When nil, logs are discarded.

Methods:

```go
func (h AutoApproveHandler) AccountChatgptAuthTokensRefresh(ctx context.Context, params protocol.ChatgptAuthTokensRefreshParams) (*protocol.ChatgptAuthTokensRefreshResponse, error)
```

Returns an error for auth refresh requests.

```go
func (h AutoApproveHandler) ApplyPatchApproval(ctx context.Context, params protocol.ApplyPatchApprovalParams) (*protocol.ApplyPatchApprovalResponse, error)
```

Approves legacy patch requests.

```go
func (h AutoApproveHandler) ExecCommandApproval(ctx context.Context, params protocol.ExecCommandApprovalParams) (*protocol.ExecCommandApprovalResponse, error)
```

Approves legacy command requests.

```go
func (h AutoApproveHandler) ItemCommandExecutionRequestApproval(ctx context.Context, params protocol.CommandExecutionRequestApprovalParams) (*protocol.CommandExecutionRequestApprovalResponse, error)
```

Approves command execution requests.

```go
func (h AutoApproveHandler) ItemFileChangeRequestApproval(ctx context.Context, params protocol.FileChangeRequestApprovalParams) (*protocol.FileChangeRequestApprovalResponse, error)
```

Approves file change requests.

```go
func (h AutoApproveHandler) ItemPermissionsRequestApproval(ctx context.Context, params protocol.PermissionsRequestApprovalParams) (*protocol.PermissionsRequestApprovalResponse, error)
```

Approves permission escalation requests.

```go
func (h AutoApproveHandler) ItemToolCall(ctx context.Context, params protocol.DynamicToolCallParams) (*protocol.DynamicToolCallResponse, error)
```

Returns an error for dynamic tool calls.

```go
func (h AutoApproveHandler) ItemToolRequestUserInput(ctx context.Context, params protocol.ToolRequestUserInputParams) (*protocol.ToolRequestUserInputResponse, error)
```

Returns an error for tool user input prompts.

```go
func (h AutoApproveHandler) McpServerElicitationRequest(ctx context.Context, params protocol.McpServerElicitationRequestParams) (*protocol.McpServerElicitationRequestResponse, error)
```

Returns an error for MCP elicitation prompts.

### `type Codex`

```go
type Codex struct {
    // contains filtered or unexported fields
}
```

`Codex` is the main entrypoint for the Go SDK.

```go
func New(ctx context.Context, opts Options) (*Codex, error)
```

Creates a new Codex client and performs the initialize handshake.

```go
func (c *Codex) Client() *rpc.Client
```

Exposes the underlying RPC client for low-level access.

```go
func (c *Codex) Close() error
```

Closes the underlying transport.

```go
func (c *Codex) ResumeThread(ctx context.Context, options ThreadResumeOptions) (*Thread, error)
```

Resumes an existing thread.

```go
func (c *Codex) StartThread(ctx context.Context, options ThreadStartOptions) (*Thread, error)
```

Starts a new thread using the app-server.

### `type Input`

```go
type Input struct {
    // Type must be one of the InputType* constants.
    Type         string                 `json:"type"`
    Text         string                 `json:"text,omitempty"`
    TextElements []protocol.TextElement `json:"textElements,omitempty"`
    URL          string                 `json:"url,omitempty"`
    Path         string                 `json:"path,omitempty"`
    Name         string                 `json:"name,omitempty"`
}
```

`Input` represents a structured user input message.

```go
func ImageInput(url string) Input
```

Creates a remote image input entry.

```go
func LocalImageInput(path string) Input
```

Creates a local image input entry.

```go
func SkillInput(name, path string) Input
```

Creates a skill input entry.

```go
func TextInput(text string) Input
```

Creates a text input entry.

### `type Options`

```go
type Options struct {
    // Transport overrides the default stdio spawn.
    Transport rpc.Transport

    // Spawn controls how the default stdio process is launched.
    Spawn SpawnOptions

    // Logger receives SDK logs. If nil, logging is disabled.
    Logger *slog.Logger

    // ClientInfo identifies this SDK to the app-server.
    ClientInfo protocol.ClientInfo

    // ApprovalHandler handles server approval requests.
    ApprovalHandler rpc.ServerRequestHandler
}
```

`Options` configures the Codex client.

### `type RawJSON`

```go
type RawJSON = json.RawMessage
```

`RawJSON` represents a pre-serialized JSON value.

```go
func JSON(value any) (RawJSON, error)
func MustJSON(value any) RawJSON
```

`JSON` marshals a value into `RawJSON`. `MustJSON` does the same and panics on error.

### `type ReasoningEffort`

```go
type ReasoningEffort = protocol.ReasoningEffort
```

`ReasoningEffort` is a typed alias for standard effort values.

```go
const (
    ReasoningEffortNone    ReasoningEffort = protocol.ReasoningEffortNone
    ReasoningEffortMinimal ReasoningEffort = protocol.ReasoningEffortMinimal
    ReasoningEffortLow     ReasoningEffort = protocol.ReasoningEffortLow
    ReasoningEffortMedium  ReasoningEffort = protocol.ReasoningEffortMedium
    ReasoningEffortHigh    ReasoningEffort = protocol.ReasoningEffortHigh
    ReasoningEffortXHigh   ReasoningEffort = protocol.ReasoningEffortXhigh
)
```

### `type SandboxMode`

```go
type SandboxMode = protocol.SandboxMode
```

`SandboxMode` is a typed alias for simple sandbox mode values.

```go
const (
    SandboxModeReadOnly         SandboxMode = protocol.SandboxModeReadOnly
    SandboxModeWorkspaceWrite   SandboxMode = protocol.SandboxModeWorkspaceWrite
    SandboxModeDangerFullAccess SandboxMode = protocol.SandboxModeDangerFullAccess
)
```

### `type SpawnOptions`

```go
type SpawnOptions struct {
    // CodexPath is the path to the codex binary (defaults to "codex").
    CodexPath string

    // ConfigOverrides are passed as --config key=value flags.
    ConfigOverrides []string

    // ExtraArgs are appended to the command line.
    ExtraArgs []string

    // Stderr captures stderr from the codex process (defaults to os.Stderr).
    Stderr io.Writer
}
```

`SpawnOptions` configures the spawned `codex app-server` process.

### `type Thread`

```go
type Thread struct {
    // contains filtered or unexported fields
}
```

`Thread` represents an active conversation thread.

```go
func (t *Thread) ID() string
```

Returns the thread ID.

```go
func (t *Thread) Run(ctx context.Context, prompt string, opts *TurnOptions) (*TurnResult, error)
```

Sends a text prompt and waits for the turn to finish.

```go
func (t *Thread) RunInputs(ctx context.Context, inputs []Input, opts *TurnOptions) (*TurnResult, error)
```

Sends structured inputs and waits for the turn to finish.

```go
func (t *Thread) RunStreamed(ctx context.Context, inputs []Input, opts *TurnOptions) (*TurnStream, error)
```

Sends structured inputs and returns a streaming iterator. The iterator includes thread-scoped events and any notifications that omit `threadId`, such as account/session updates.

### `type ThreadResumeHistoryElem`

```go
type ThreadResumeHistoryElem = json.RawMessage
```

`ThreadResumeHistoryElem` keeps the old unstable history field compilable for callers, but the current app-server protocol no longer accepts history-based thread resume.

### `type ThreadResumeOptions`

```go
type ThreadResumeOptions struct {
    // ThreadID resumes a persisted thread by id.
    ThreadID string

    // History is retained for source compatibility, but the current app-server
    // protocol no longer supports history-based resume. Passing History returns an
    // error from toParams.
    History []ThreadResumeHistoryElem

    // Path is retained for source compatibility, but the current app-server
    // protocol no longer supports path-based resume. Passing Path returns an error
    // from toParams.
    Path string

    Model         string
    ModelProvider string
    Cwd           string

    // ApprovalPolicy is marshaled as JSON and sent as "approvalPolicy".
    // Prefer ApprovalPolicy* constants for simple policies.
    ApprovalPolicy any

    // Sandbox is marshaled as JSON and sent as "sandbox".
    // Prefer SandboxMode* constants for simple policies.
    Sandbox any

    Config                map[string]any
    BaseInstructions      string
    DeveloperInstructions string
}
```

`ThreadResumeOptions` configures a `thread/resume` request.

### `type ThreadStartOptions`

```go
type ThreadStartOptions struct {
    Model string
    Cwd   string

    // ApprovalPolicy is marshaled as JSON and sent as "approvalPolicy".
    // Prefer ApprovalPolicy* constants for simple policies.
    ApprovalPolicy any

    // SandboxPolicy is marshaled as JSON and sent as "sandbox".
    // Prefer SandboxMode* constants for simple policies.
    SandboxPolicy any

    Config                map[string]any
    BaseInstructions      string
    DeveloperInstructions string

    // ExperimentalRawEvents is retained for source compatibility, but the current
    // app-server protocol no longer supports this option. Setting it returns an
    // error from toParams.
    ExperimentalRawEvents bool
}
```

`ThreadStartOptions` configures a `thread/start` request.

### `type TurnOptions`

```go
type TurnOptions struct {
    Cwd string

    // ApprovalPolicy is marshaled as JSON and sent as "approvalPolicy".
    // Prefer ApprovalPolicy* constants for simple policies.
    ApprovalPolicy any

    // SandboxPolicy is marshaled as JSON and sent as "sandboxPolicy".
    // Prefer SandboxMode* constants for simple policies.
    SandboxPolicy any

    Model string

    // Effort is marshaled as JSON and sent as "effort".
    // Prefer ReasoningEffort* constants for standard values.
    Effort any

    // Summary is marshaled as JSON and sent as "summary".
    Summary any

    // OutputSchema is marshaled as JSON and sent as "outputSchema".
    OutputSchema any

    // CollaborationMode is retained for source compatibility, but the current
    // app-server protocol no longer supports this option. Setting it returns an
    // error from buildTurnParams.
    CollaborationMode any
}
```

`TurnOptions` configures a `turn/start` request.

### `type TurnResult`

```go
type TurnResult struct {
    TurnID        string
    Notifications []rpc.Notification

    // Items holds the raw JSON payloads for completed items.
    Items         []json.RawMessage
    FinalResponse string
}
```

`TurnResult` aggregates notifications for a completed turn.

### `type TurnStream`

```go
type TurnStream struct {
    // contains filtered or unexported fields
}
```

`TurnStream` iterates notifications for a running turn. Notifications that omit `threadId` are still emitted to avoid dropping global events sent during the turn.

```go
func (s *TurnStream) Close()
```

Stops the iterator.

```go
func (s *TurnStream) Next(ctx context.Context) (rpc.Notification, error)
```

Returns the next notification for this turn. Notifications without `threadId` are treated as belonging to the active stream.

## Source Files

- `approvals.go`
- `codex.go`
- `doc.go`
- `gen.go`
- `input.go`
- `json.go`
- `logging.go`
- `option_values.go`
- `options.go`
- `thread.go`
- `thread_options.go`
- `turn.go`

## Directories

Examples:

- `examples/approvals`
- `examples/low_level_rpc`
- `examples/quickstart`
- `examples/streaming`
- `examples/structured_output`

Internal directories:

- `internal/codegen`
- `internal/protocol`
- `internal/rpc`: package `rpc` provides a minimal JSON-RPC client tailored to the Codex app-server.
- `internal/testutil`
