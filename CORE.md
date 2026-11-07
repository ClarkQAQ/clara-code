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

The Go baseline is 1.27. Other Go modules can use a local `replace` directive. When migrating an existing project, update its imports to the new module paths.

```sh
go mod edit -require=clara-code@v0.0.0
go mod edit -replace=clara-code=/absolute/path/to/clara-code
go mod tidy
```

The module currently uses the short path `clara-code`. Independent projects can reference it through a local `replace` or Go workspace.

- Use `Generate` or `Stream` for calls without a session. `PrepareStep` can select the model, context, and tools at each step. Tools can implement `model.AgentTool` or use `model.NewAgentTool` or `tool.Func`.
- For session-based calls, use `core.NewSession`, `Session.Start`, `Run.Wait`, `Run.Cancel`, `Run.Steer`, and `Run.FollowUp`. `core.OpenSession` restores a session from its Store. Subscribe from a sequence number with `Session.NextEvent`; an expired replay cursor produces a `GapError` carrying the earliest available sequence.
- Sessions use memory by default. For file storage, the host supplies `core.Blobs`. Examples write checkpoints through a root-confined local filesystem. Version-conflict detection covers sessions sharing the same Store. Cross-process access requires a host-provided Store with mutual exclusion.
- `tool.NewFiles` accepts a readable backend. Storage can use memory, virtual, mount, restricted, or SFTP implementations. The host registers Shell separately. SSH uses `tool.NewSSH` with an explicit `HostKeyCallback`; SFTP uses `pkg/tool/fs/sftp`.
- `tool.PlanOnly` enforces authorization at execution, while `tool.PlanTools` filters tool definitions. Execution checklists use a separate `PlanStore`. Hosts integrate `Goal` through `Start`, `NextTurn`, and `Charge`. A `TaskManager` factory creates child sessions, with each scope restricted by its parent's allowed tool list.
- `pkg/core/prompt` supplies embedded fragments, dynamic context, and optional compaction. Terminal and Web examples enable compaction with `-context-bytes`. Custom hosts can use `Compactor` in `PrepareStep`. Plan, Goal, and subtask state in these examples belong to the current host process. Persisted sessions retain the generic transcript and events.

Model tools require strict JSON objects as arguments. Structured streaming previews are emitted only after the accumulated content passes JSON and schema validation; the final result is validated again after a normal termination event. Hosts can explicitly configure `RepairToolCall` or `RepairText`. Session files and event JSON formats remain compatible.

ACP entry points live in `clara-code/pkg/core`; the task adapter is `tool.NewACPTaskBackend`. Import MCP connection management from `clara-code/pkg/tool/mcp` and Relay from `clara-code/pkg/model/relay`.

## Background Commands

`Commands.Terminals` returns background snapshots, `Refresh(ctx)` updates them, and `Stop(ctx, id)` stops a terminal. An empty ID stops all terminals present when called. `OnChange` notifies the host when status or retained output changes; elapsed time alone does not trigger a notification. The existing `Runner` and `Process` interfaces remain compatible. Local and SSH processes implement the optional `SnapshotProcess` interface, which reads state without advancing the `write_stdin` output cursor. For custom backends without this interface, automatic monitoring does not consume output; explicit polling updates the state.

Natural exits, nonzero exits, timeouts, and explicit stops retain distinct final states. Finished processes release their resources while caching unread output for a subsequent `write_stdin` call. The cache holds at most the latest 32 completed terminals, and each terminal retains at most 1 MiB of output. Canceling `write_stdin` only cancels the wait and leaves the process running. Concurrent explicit stopping and polling report the stopped state. Waiting for an input write can be canceled, but bytes already delivered to stdin cannot be recalled. `Commands.Close` cleans up all live terminals. Persisted sessions do not restore processes.

Core retains its default total runtime limit of 600 seconds. Hosts can explicitly set `Commands.NoDefaultTimeout=true` so commands that omit `timeout_seconds` have no total deadline. `Spec.NoTimeout` expresses the same intent to custom Runners. Explicit timeouts still apply; `yield_seconds` controls only the duration of one wait. Hosts should set `Commands.Context` to the owning session's lifetime and arrange their own refresh loop and notification display.

## Native Host Paths

`host.New(cwd)` in `pkg/tool/fs/host` provides an explicitly selected open filesystem backend that uses the current user's OS permissions. Relative paths resolve from cwd; absolute paths and `..` can reach outside the project, and `~/` expands to the home directory. Reads follow symbolic links. Overwrite writes preserve links and update their targets; deletion and renaming operate on links themselves. Creating a file does not overwrite an existing file or link. The root confinement of `local.New(root)` in `pkg/tool/fs/local` remains unchanged.

`Files.InstructionsSource` customizes rule lookup for a target file. `fs.InstructionsFrom` limits the ancestor at which rule loading begins. Default host behavior remains compatible. Hashline uses the filesystem's path resolver, supporting both native and confined logical paths.

## File Search Contract

`glob` and `grep.include` share glob semantics. `*.go` matches filenames at any depth; `**` as a whole path component matches zero or more directories. Relative patterns containing `/` resolve against the search `path`; a leading `/` resolves against the logical workspace root. Patterns support `*`, `?`, and character classes, but not brace expansion. Invalid patterns return argument errors without suffix-matching fallbacks. `grep.query` is a whitespace-preserving literal string, searched case-insensitively by default.

Search reads only regular text files and skips symbolic links, sockets, FIFOs, devices, and binary content. `GlobDetailed` and `GrepDetailed` return entries with `SearchDiagnostics`, including truncation, skipped counts, and at most 100 diagnostic records. The original `Glob` and `Grep` interfaces remain as compatibility wrappers returning entries only. Glob defaults to at most 500 results, with a traversal budget four times that limit. Grep defaults to at most 200 matching lines, 20,000 traversal entries, and 4 MiB per file. Model tools explicitly report incomplete searches when a limit is reached or some files cannot be read.

Core does not depend on Git or apply ignore rules by default. Hosts can set `Files.SearchSource` before using the tools to supply a filtered filesystem for each search. The model tool's `include_ignored` value is passed to that policy; without a policy, the search scope is unchanged. Direct reads and file mutations do not use the search policy. Argument validation remains strict and reports the tool name, error reason, allowed fields, and required fields. The model or host must explicitly correct failed calls.

The `stat` tool's `modified` value uses RFC3339Nano with `Z` or an explicit offset, preserving the file timestamp's instant and fractional-second precision. Structured timestamps for events, files, and background terminals continue to use `time.Time` RFC3339 JSON encoding. Hosts handle UI localization and provide the current clock to the model. The default compaction prompt preserves the meaning of historical dates and time zones and prevents old clock snapshots from becoming the summary's current time. PDF creation dates retain their dedicated format with an explicit UTC marker ([ISO 32000-1 section 7.9.4](https://opensource.adobe.com/dc-acrobat-sdk-docs/standards/pdfstandards/pdf/PDF32000_2008.pdf)).

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
