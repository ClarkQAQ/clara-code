# Clara Code Public Go Packages

The embeddable capabilities of the former Clara Core now live in this repository's `pkg` directory and share a Go module with Clara Code. The runtime was ported from Yuzu `f4855d7`, retaining `NewAgent`, `Generate`, `Stream`, `WithTools`, and `PrepareStep`. Terminal, Web, and ACP examples share the same coding agent assembly. The host selects the model, tools, filesystem, executor, and session storage.

## Running a Coding Agent

Run these commands from the repository root:

```sh
export CLARA_CORE_ENDPOINT='https://your-model-gateway.example/v1'
export CLARA_CORE_PROTOCOL='responses'
export CLARA_CORE_MODEL='your-model'
export CLARA_CORE_API_KEY='your-key'

go run ./examples/coding-agent -root /path/to/workspace -store .clara-core/sessions
go run ./examples/web -root /path/to/workspace -store .clara-core/sessions
```

`CLARA_CORE_PROTOCOL` accepts `responses` or `completions`. Examples prefer `CLARA_CORE_*` variables and fall back to the corresponding `CLARY_*` variables when unset.

The terminal example resumes sessions with `-resume SESSION_ID`; the Web example accepts a pasted session ID. When creating the directory specified by `-store`, the host creates missing parents. Point `-store` at the original directory to continue existing sessions.

In these examples, file tools and `exec_command.workdir` use workspace paths. Starting with `-root examples/web` makes `/` refer to `examples/web` and `/main.go` refer to its `main.go`. Shell output can still contain host paths.

The terminal example supports `/plan`, `/agent`, `/cancel`, `/follow TEXT`, `/answer JSON`, `/goal TEXT`, `/pause`, `/resume`, and `/wait`. Ordinary messages submitted during a turn become steering input. Web uses the same session interface and displays model and tool events over SSE. The Web example is intended for local, single-user use and listens on loopback by default. Shell commands run with the host user's permissions. Hosts that need isolation can inject a restricted Runner or an authorization callback.

Use `-demo` to test interaction with a deterministic model. Run `go run ./examples/coding` to verify failure reproduction, search, Hashline editing, and a successful test rerun in a temporary Go project.

## Assembly in a Go Host

```go
import (
    "clara-code/pkg/core"
    "clara-code/pkg/model/provider"
    "clara-code/pkg/tool"
    "clara-code/pkg/tool/fs/local"
)

client, err := provider.New(provider.WithBaseURL(endpoint),
    provider.WithAPI(provider.APIResponses), provider.WithAPIKey(apiKey))
if err != nil { return err }
model, err := client.LanguageModel(ctx, modelName)
if err != nil { return err }
workspace, err := local.New(projectDir)
if err != nil { return err }

files := tool.NewFiles(workspace, nil)
engine := core.NewAgent(model, core.WithTools(files.Read(), files.Grep(), files.Edit()))
result, err := engine.Generate(ctx, core.AgentCall{Prompt: "Find and fix the failing test."})
```

The Go baseline is 1.27. Other Go modules can use a local `replace` directive.

```sh
go mod edit -require=clara-code@v0.0.0
go mod edit -replace=clara-code=/absolute/path/to/clara-code
go mod tidy
```

The module currently uses the short path `clara-code`. Independent projects can reference it through a local `replace` or Go workspace.

- Use `Generate` or `Stream` for calls without a session. `PrepareStep` can select the model, context, and tools at each step. Tools can implement `model.AgentTool` or use `model.NewAgentTool` or `tool.Func`.
- For session-based calls, use `core.NewSession`, `Session.Start`, `Run.Wait`, `Run.Cancel`, `Run.Steer`, and `Run.FollowUp`. `core.OpenSession` restores a session from its Store. Subscribe from a sequence number with `Session.NextEvent`; an expired replay cursor produces a `GapError` carrying the earliest available sequence.
- Sessions use memory by default. For file storage, the host supplies `core.Blobs`. Examples write checkpoints through a root-confined local filesystem. Version-conflict detection covers sessions sharing the same Store. Cross-process access requires a host-provided Store with mutual exclusion.
- `tool.NewLocal` runs Windows commands in PowerShell 7, Windows PowerShell 5.1, or cmd.exe, in that order. `tool.LocalShellName()` reports the selected executable name. Local commands use the host user’s permissions and support pipe or pseudoconsole sessions.
- `tool.NewFiles` accepts a readable backend. Storage can use memory, virtual, mount, restricted, or SFTP implementations. The host registers Shell separately. SSH uses `tool.NewSSH` with an explicit `HostKeyCallback`. With `Spec.Workdir` or `Spec.Env`, the remote shell must support POSIX shell quoting. Relative working directories are resolved from the remote session directory. Environment names use ASCII shell identifiers, and an invalid directory stops execution before the script starts. SFTP uses `pkg/tool/fs/sftp`.
- `tool.PlanOnly` enforces authorization at execution, while `tool.PlanTools` filters tool definitions. Execution checklists use a separate `PlanStore`. Hosts integrate `Goal` through `Start`, `NextTurn`, and `Charge`. A `TaskManager` factory creates child sessions, with each scope restricted by its parent's allowed tool list.
- `pkg/core/prompt` supplies embedded fragments, dynamic context, and optional compaction. Terminal and Web examples enable compaction with `-context-bytes`. Custom hosts can use `Compactor` in `PrepareStep`. Plan, Goal, and subtask state in these examples belong to the current host process. Persisted sessions retain the generic transcript and events.

For layered prompts, use `prompt.Instructions` for stable base text and `prompt.Policy` for capability sections. Load a `prompt.Window` with `prompt.LoadWindow(step.ContextState)`, call `Window.Prepare` with the durable transcript and current `Fragment` values, and return the marshalled candidate through `PrepareStepResult.ContextState`. Unchanged rules remain in the initial prefix; changes are anchored at safe transcript boundaries. The `prompt.Prepare` helper supplies this flow. The stateless `Assemble` and string-based entry points remain available for existing hosts.

`core.State.ContextState` is optional opaque JSON, bounded to 8 MiB and stored separately from `Messages`. The session checkpoints it before model dispatch and exposes a copy through `Session.ContextState()`. Persistence failure prevents dispatch and retains the previous checkpoint. JSON null resets host context. `model.NewDeveloperMessage` is supported by both protocols; management metadata stays off wire. Keep `PrepareStepResult.Messages` request-only and use `History` only for durable transcript replacement. Transcript replacement resets the old context window before installing its replacement.

`provider.WithSystemInstructions(true)` moves leading Responses system text to the top-level `instructions` field and removes the duplicate input messages. An explicit `ProviderOptions.Instructions` takes precedence. Clara enables the option; other hosts retain the default mapping and Completions is unchanged.

A task factory can declare `TaskSetup.DynamicToolNames` for tools added later by `PrepareStep`, such as discovered skills. Names are intersected with the parent scope's allowlist and execution authorization still applies. Undeclared or parent-forbidden tools remain unavailable; declaring a name does not itself add a tool definition.

`prompt.Compactor` supports `BudgetTokens`, `BudgetBytes`, optional early `TriggerTokens`, request-only `Context`, modality `Project`, and explicit `Force`. Token limits exclude the caller's output reservation. `TriggerTokens` optionally starts automatic compaction earlier; zero retains the `BudgetTokens` trigger. The replacement must fit `BudgetTokens` even when it remains above the early trigger. Generic hosts can still use `BudgetBytes`; Clara application configuration uses token-window percentages only. `prompt.Measure` returns conservative byte/token estimates, separately from actual usage. Compaction preserves the latest turn and media, rejects incomplete summaries and unpaired tools, and returns a replacement only after validation. See [Prompt and Context Architecture](PROMPTS.md) for composition, compatibility and the local Codex comparison.

Window-based hosts use `Compactor.Assemble` to budget the complete rendered request and `Compactor.Transcript` with `Window.Prefix` to restore historical context for summarization. The latter excludes updates belonging to the retained recent turn. Internal message metadata is removed from the serialized summary transcript. After successful compaction, install a fresh window for the replacement history; ordinary recovery can reuse a window whose transcript fingerprint still matches.

`model.Usage.InputTokens` excludes `CacheReadTokens`; add them for the full input denominator. Optional `CacheReadReported` distinguishes an explicit zero cache hit from omitted usage. Positive cache counts in older records remain usable evidence of reporting. Both provider protocols preserve this distinction. Step-completion events are saved with their transcript checkpoint so host usage views can update before the next model request. Clara's cache rollups are application state, not Core policy.

Model tools require strict JSON objects as arguments. Structured streaming previews are emitted only after the accumulated content passes JSON and schema validation; the final result is validated again after a normal termination event. Hosts can explicitly configure `RepairToolCall` or `RepairText`. Session files and event JSON formats remain compatible.

ACP entry points live in `clara-code/pkg/core`; the task adapter is `tool.NewACPTaskBackend`. Import MCP connection management from `clara-code/pkg/tool/mcp` and Relay from `clara-code/pkg/model/relay`.

`mcp.ConnectWithProgress(ctx, configs, report)` reports each server synchronously before attempting its connection. `mcp.StartupProgress` includes the next server name, the number of prior attempts, and the total number of servers. Use `Manager.Statuses()` for connection results and tool counts after startup.

Set `mcp.ServerConfig.ProtocolVersion` to a version supported by the Go MCP SDK to request that version during the handshake. For example, `ProtocolVersion: "2025-03-26"` starts with `initialize`. The server can negotiate a different supported version. An empty value uses the SDK's default negotiation.

## Migrating from Clara Core

The `clara-code` module includes the former Clara Core packages. Update imports using the following mapping. Types and behavior remain compatible, including protocol formats. The new packages replace the old import paths directly.

| Old path | New path |
| --- | --- |
| `clara-core/agent` (package `agent`) | `clara-code/pkg/core` (package `core`) |
| `clara-core/agent/prompt` | `clara-code/pkg/core/prompt` |
| `clara-core/agent/errs` | `clara-code/pkg/util/errs` |
| `clara-core/agent/id` | `clara-code/pkg/util/id` |
| `clara-core/models/...` | `clara-code/pkg/model/...` |
| `clara-core/tools/...` | `clara-code/pkg/tool/...` |

For the standalone CLI, see the [application configuration](docs/configuration.md).

## Background Commands

`Commands.Terminals` returns background snapshots, `Refresh(ctx)` updates them, and `Stop(ctx, id)` stops a terminal. An empty ID stops all terminals present when called. `OnChange` notifies the host when status or retained output changes; elapsed time alone does not trigger a notification. The existing `Runner` and `Process` interfaces remain compatible. Local and SSH processes implement the optional `SnapshotProcess` interface, which reads state without advancing the `write_stdin` output cursor. For custom backends without this interface, automatic monitoring does not consume output; explicit polling updates the state.

Natural exits, nonzero exits, timeouts, and explicit stops retain distinct final states. Finished processes release their resources while caching unread output for a subsequent `write_stdin` call. The cache holds at most the latest 32 completed terminals, and each terminal retains at most 1 MiB of output. Canceling `write_stdin` only cancels the wait and leaves the process running. Concurrent explicit stopping and polling report the stopped state. Waiting for an input write can be canceled, but bytes already delivered to stdin cannot be recalled. `Commands.Close` cleans up all live terminals. Persisted sessions do not restore processes.

Core retains its default total runtime limit of 600 seconds. Hosts can explicitly set `Commands.NoDefaultTimeout=true` so commands that omit `timeout_seconds` have no total deadline. `Spec.NoTimeout` expresses the same intent to custom Runners. Explicit timeouts still apply. The `yield_time_ms` argument to `exec_command` and `write_stdin` controls the duration of one wait in milliseconds. It defaults to 5000 for execution and 10000 for polling, with a maximum of 30000. To migrate existing calls, replace `yield_seconds` with `yield_time_ms` and multiply the value by 1000. Hosts should set `Commands.Context` to the owning session's lifetime and arrange their own refresh loop and notification display.

## Native Host Paths

`host.New(cwd)` in `pkg/tool/fs/host` provides an explicitly selected open filesystem backend that uses the current user's OS permissions. Relative paths resolve from cwd; absolute paths and `..` can reach outside the project, and `~/` expands to the home directory. Reads follow symbolic links. Overwrite writes preserve links and update their targets; deletion and renaming operate on links themselves. Creating a file does not overwrite an existing file or link. The root confinement of `local.New(root)` in `pkg/tool/fs/local` remains unchanged.

`Files.InstructionsSource` customizes rule lookup for a target file. `fs.InstructionsFrom` limits the ancestor at which rule loading begins. Default host behavior remains compatible. Hashline uses the filesystem's path resolver, supporting both native and confined logical paths.

Use Read and Edit from the same `Files` instance so they share snapshots. Read with `{"path":"/main.go","anchored":true,"offset":2,"limit":1}`, then copy the returned first line into Edit's `input`. If the header is `[/main.go#ABCD]`, delete line 2 with `{"input":"[/main.go#ABCD]\nCUT 2.=2"}` or replace it with `{"input":"[/main.go#ABCD]\nPUT 2.=2:\n+// replacement"}`. Ranges include both endpoints. A CUT takes no colon or body; a literal PUT takes one `+` row per new line. Read again with `anchored: true` before the next edit to the same file or when a snapshot is unavailable, including after reopening a saved conversation.

## File Search Contract

`glob` and `grep.include` share glob semantics. `*.go` matches filenames at any depth; `**` as a whole path component matches zero or more directories. Relative patterns containing `/` resolve against the search `path`; a leading `/` resolves against the logical workspace root. Patterns support `*`, `?`, and character classes, but not brace expansion. Invalid patterns return argument errors without suffix-matching fallbacks. `grep.query` is a whitespace-preserving literal string, searched case-insensitively by default.

Search reads only regular text files and skips symbolic links, sockets, FIFOs, devices, and binary content. `GlobDetailed` and `GrepDetailed` return entries with `SearchDiagnostics`, including truncation, skipped counts, and at most 100 diagnostic records. The original `Glob` and `Grep` interfaces remain as compatibility wrappers returning entries only. Glob defaults to at most 500 results, with a traversal budget four times that limit. Grep defaults to at most 200 matching lines, 20,000 traversal entries, and 4 MiB per file. Model tools explicitly report incomplete searches when a limit is reached or some files cannot be read.

Core does not depend on Git or apply ignore rules by default. Hosts can set `Files.SearchSource` before using the tools to supply a filtered filesystem for each search. The model tool's `include_ignored` value is passed to that policy; without a policy, the search scope is unchanged. Direct reads and file mutations do not use the search policy. Argument validation remains strict and reports the tool name, error reason, allowed fields, and required fields. The model or host must explicitly correct failed calls.

The `stat` tool's `modified` value uses RFC3339Nano with `Z` or an explicit offset, preserving the file timestamp's instant and fractional-second precision. Structured timestamps for events, files, and background terminals continue to use `time.Time` RFC3339 JSON encoding. Hosts handle UI localization and decide which date/time context to supply to the model. The public base prompt uses the latest supplied date and time zone without inventing an exact current time or message sending times. The default compaction prompt preserves the meaning of historical dates and time zones and prevents old clock snapshots from becoming the summary's current time. PDF creation dates retain their dedicated format with an explicit UTC marker ([ISO 32000-1 section 7.9.4](https://opensource.adobe.com/dc-acrobat-sdk-docs/standards/pdfstandards/pdf/PDF32000_2008.pdf)).

## Examples and Checks

| Command | Purpose |
| --- | --- |
| `go run ./examples/basic` | Synchronous calls without a session |
| `go run ./examples/streaming` | Incremental model callbacks |
| `go run ./examples/custom-tool` | Custom tools and arguments |
| `go run ./examples/custom-model` | Model selection at each step |
| `go run ./examples/custom-fs` | An in-memory filesystem and a separately registered read tool |
| `go run ./examples/subagent` | Delegation and result delivery on the same runtime |
| `go run ./examples/relay` | HTTP, SSE, and WebSocket Relay; accepts `CLARA_CORE_UPSTREAM_URL` |
| `go run ./examples/acp` | ACP stdio, session recovery, and MCP injection |

```sh
make check
```

## Host media and capability policy

`model.FilePart` and media tool results carry standard bytes and MIME types. Core does not know about Clara attachment directories. Hosts may normalize new input with the optional `ACPBinding.PrepareInput` hook before an ACP prompt is accepted. Image-only prompts are valid. Hosts should reject unsupported new images and project historical media into text-only request copies without changing persisted history.

Model capabilities, provider catalogs, layered TOML configuration, clipboard adapters, and content-addressed storage are Clara application policy in `internal`. Public providers continue to accept ordinary `model.Call` options for output limits, reasoning, summaries, verbosity, and parallel calls. `tool.TaskSetup.MaxRetries` optionally supplies a child agent's retry budget; nil retains Core defaults. Neither hook requires application imports from public packages.


## Reusable environment context

`prompt.Environment` contains optional `Cwd`, `Shell`, `CurrentDate`, `Timezone`,
`WorkspaceRoots`, and `FileSystemAccess` fields. Hosts discover their own values;
the package does not inspect the OS, read Clara configuration, or choose access
policy. `FileSystemAccess` describes the enforced boundary (Clara uses
`unrestricted` or `read_only`), while roots describe the workspace rather than
limiting native paths. This is model context, not an authorization mechanism.

```go
environment := prompt.Environment{
    Cwd: "/work/repo", Shell: "bash",
    CurrentDate: "2026-11-09", Timezone: "Asia/Shanghai (UTC+08:00)",
    WorkspaceRoots: []string{"/work/repo"}, FileSystemAccess: "read_only",
}
fragments := append(prompt.Policy(config), environment.Fragments()...)
messages, next, err := window.Prepare(history, fragments)
```

Fields have deterministic order, empty values are omitted, and text values are
escaped, including roots inside the filesystem block. `Window` compares fields
independently and emits only actual changes. Keep the same environment date
throughout a local day; do not add precision to trigger a new delta every step.
Other existing Fragment/Window and string-based prompt APIs remain available.


## Delegated task contracts

`tool.SpawnAgent` starts a child asynchronously; `tool.RunTask` starts one and
waits for its result. Both expose the same schema: `task_name` matches
`^[a-z0-9_]{1,100}$` (for example `repo_survey`, never `repo-survey`), `role`
is a required host-supported role, and `message` is a non-blank complete brief
of at most 262144 UTF-8 bytes. `TaskScope.Spawn` enforces these constraints for
direct callers too. Invalid calls identify the rejected field and start no child;
names are never silently normalized. Duplicate names within a scope require
reusing the existing child or choosing another name.

Model and reasoning overrides are optional; omit them for host defaults.
`fork_history` defaults to false. A history fork copies conversation, not tools,
permissions or live processes. Hosts define their own roles and authorization.
Clara offers `general` for implementation and `explorer`/`reviewer` for read-only
file inspection without Shell. Configured external ACP roles do not support
model, reasoning or history overrides.

Use the returned task ID as `target` for control tools. `wait_agent.timeout_ms`
is milliseconds, capped at 600000; omitted/non-positive uses 10 minutes.
A wait timeout does not cancel the child. `list_agents` locates existing tasks;
retry waiting instead of spawning duplicate work. `send_message` steers a running
turn; `followup_task` also restarts finished children. `interrupt_agent` retains
the session for follow-up; `close_agent` cancels and releases it and descendants.
Collect and verify results before closing. `subagent_task` does not close children.
