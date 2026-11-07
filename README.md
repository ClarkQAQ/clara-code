# Clara Code

Clara Code is a terminal coding assistant built on Clara Core. It uses the terminal's main screen and preserves completed conversations in native scrollback. Resizing rebuilds the terminal buffer from logical messages and reflows the conversation, including the current question, menu, and input draft. The interactive CLI supports macOS and Linux.

## Getting Started

Go 1.27 is required. The CLI and reusable agent, model, and tool packages all live in this repository.

```sh
go run . --demo
go build -o clara-code .
./clara-code --cwd /path/to/project
./clara-code resume --cwd /path/to/project
```

## Project Structure and Library Usage

Root `main.go` provides the CLI entry point. Startup, terminal UI, configuration, session storage, and other application features live in `internal`. Reusable mechanisms live in `pkg/core`, `pkg/model`, `pkg/tool`, and `pkg/util`. Public packages do not depend on Clara's application layer. Examples live in `examples`; see [Public Go Packages](CORE.md) for assembly details.

Other local projects can use:

```sh
go mod edit -require=clara-code@v0.0.0
go mod edit -replace=clara-code=/absolute/path/to/clara-code
go mod tidy
```

```go
import (
    "clara-code/pkg/core"
    "clara-code/pkg/model"
    "clara-code/pkg/tool"
)
```

Migrate old imports using the following mapping. Types and behavior remain unchanged; no compatibility module is provided for the old paths.

| Old path | New path |
| --- | --- |
| `clara-core/agent` (package `agent`) | `clara-code/pkg/core` (package `core`) |
| `clara-core/agent/prompt` | `clara-code/pkg/core/prompt` |
| `clara-core/agent/errs` | `clara-code/pkg/util/errs` |
| `clara-core/agent/id` | `clara-code/pkg/util/id` |
| `clara-core/models/...` | `clara-code/pkg/model/...` |
| `clara-core/tools/...` | `clara-code/pkg/tool/...` |

An adjacent Clara Core checkout is no longer needed. Existing configuration, environment variables, sessions, and protocol formats remain compatible; the original examples continue to use their `CLARA_CORE_*` settings. `make check` runs formatting checks, tests, race tests, vet, lint, and CLI and example builds. `make build` produces `clara-code` at the repository root. Inject a version with `go build -ldflags '-X main.version=VERSION' .`. The [Development Guide](AGENTS.md) defines placement rules and verification requirements for new features. The original framework's license, attribution, and prompt provenance are preserved.

Configure a live model through environment variables:

```sh
export CLARA_CODE_ENDPOINT='https://gateway.example/v1'
export CLARA_CODE_PROTOCOL='responses' # or completions
export CLARA_CODE_MODEL='your-model'
export CLARA_CODE_API_KEY='your-key'
./clara-code
```

## Configuration

On Linux, the data directory defaults to `~/.local/share/clara-code`, or `clara-code` under `XDG_DATA_HOME` when set. On macOS, it is `~/Library/Application Support/clara-code`. `CLARA_CODE_HOME` overrides the directory on either platform.

Create `config.toml` in the data directory. Model environment variables take precedence; unspecified connection settings can fall back to the corresponding `CLARA_CORE_*` variables. You can also set `api_key_file` to a key file accessible only to the current user (mode `0600`). Relative paths resolve from the data directory. A key supplied through the environment takes precedence.

```toml
endpoint = "https://gateway.example/v1"
protocol = "responses"
api_key_env = "MY_GATEWAY_API_KEY"
model = "coding"
context_bytes = 524288

[models.coding]
id = "your-model"
effort = "high"
context_window = 128000
effective_context_window_percent = 95

[models.fast]
id = "another-model"

# MCP stdio example
[mcp_servers.files]
command = "/path/to/mcp-server"
args = ["--stdio"]
required = false

# MCP Streamable HTTP example
[mcp_servers.docs]
url = "http://127.0.0.1:8765/mcp"

# External ACP subagent. The model can use reviewer-external as a task role.
[acp_agents.reviewer-external]
command = "/path/to/another-agent"
args = ["acp"]
```

`context_bytes` is the serialized byte budget for automatic compaction; set it to `0` to disable automatic compaction. `context_window` is the model's raw token limit. The status bar uses 95% of that limit as the effective window by default; change it with `effective_context_window_percent`. Current context usage comes from the most recent valid request's total tokens. The percentage calculation subtracts a fixed 12,000-token baseline, following the reference Codex implementation. Cumulative usage appears separately in `/status`; unavailable usage is shown as unknown. Put global rules in `AGENTS.md` in the data directory and global skills in `skills/<name>/SKILL.md`. Project rules load from the Git root downward; project skills live in `.agents/skills/` at the applicable directory. Skill files declare `name` and `description` in YAML or TOML front matter.

## Interaction

| Action | Key or command |
| --- | --- |
| Submit input or steer an active turn | Enter |
| Insert a newline | Alt+Enter, or Shift+Enter in terminals that distinguish it |
| Switch between Agent and Plan | Shift+Tab, or `/agent` and `/plan` |
| Interrupt a turn | Esc or `/cancel` |
| View or stop background terminals, including during a turn | `/ps`, `/stop [ID]`; omit the ID to stop all terminals present when called |
| Edit a previous message and create a branch | Press Esc twice with empty input; use Left/Right to select, Up/Down to scroll, Ctrl+T to toggle details, and Enter to confirm |
| Create, resume, or fork a conversation | `/new`, `/resume [ID]`, `/fork` |
| Clear the terminal and start a new conversation | `/clear`; the old session remains resumable |
| View usage, configuration, or the working directory | `/status`, `/pwd` |
| View changes | `/diff`; Tab switches staged, unstaged, and untracked views; n/p changes pages |
| Select a model and reasoning effort | `/model [name [effort]]` |
| Review current Git changes without writing | `/review` |
| Compact model context manually | `/compact` |
| Run a goal or queue follow-up instructions | `/goal`, `/pause`, `/continue`, `/follow` |
| View connection status or select a skill | `/mcp`, `/skills` |
| Show help or exit | `/help`, `/exit`, `/quit` |

Type `/` to open the two-column command menu. Use Up/Down to select, Tab to complete, Enter to confirm, and Esc to close while retaining the draft. Plan mode appears on the right of the status bar.

The main composer, question notes, and free-text answers use the real terminal cursor, allowing Chinese IME candidates and input-mode indicators to follow the insertion point. The cursor keeps the terminal's default shape, is hidden in read-only selectors, and becomes visible again on exit. Positions use terminal display columns for Chinese text, combined emoji, multiline input, and narrow windows. The terminal and system IME control candidate appearance and selection.

Model progress updates and final answers are stored separately. Tool execution appears as an updating record such as `● Bash(command)`, with output beneath `└`. Green indicates success, red indicates failure, and cancellation and background execution have distinct states. Shell results use actual exit codes; subsequent background output updates the original record. Long previews retain the beginning and end. Structured records follow session storage limits and support replay on recovery and resize.

Background terminals belong to their Clara session, including commands started by subagents. `yield_seconds` controls how long an initial execution or subsequent wait blocks before returning. Omitting `timeout_seconds` removes the total runtime limit; explicit timeouts still apply, up to 600 seconds. Esc interrupts only the current agent turn and wait. Detached terminals continue running until explicitly stopped with `/stop`, or cleaned up when their session closes or switches, or the process exits.

While a background command is running, the area above the composer shows `1 background terminal running · /ps to view · /stop to close`, merged into the activity indicator when visible. Status is checked every 500 ms without consuming output, and the UI refreshes only when status or output changes. `/ps` does not take output the model has not yet read. A model poll with empty input produces a separate `Waiting for background terminal` record, retained as `Waited for background terminal` afterward. Sending input produces `Interacted with background terminal`; the original Bash record continues to show output and final status. Canceling an input write only stops waiting; characters already delivered to the terminal cannot be recalled.

Asynchronous command states are saved separately using atomic writes; completed turn files are not rewritten. Natural exits, failures, stops, and timeouts update the original command record. Unread output remains available through `write_stdin`. The completion cache retains at most the latest 32 terminals, with the existing 1 MiB output limit per terminal and head-and-tail previews. Recovery and branching replay records without restarting processes; old records without a final state appear as interrupted. Custom execution backends without Core's `SnapshotProcess` update only when the model explicitly polls; `/ps` reports this limitation.

The activity line above the composer shows waiting, execution, compaction, and reconnection states, including elapsed time and the Esc interruption hint. It is temporarily hidden for interactive questions. Public reasoning summaries may supply an activity title; raw reasoning is not displayed as a progress update. Core retries eligible connection-establishment errors up to three times, displaying the attempt, delay, and redacted reason. Interruptions after streaming begins preserve received text and report the error without automatically replaying executed tools. Final failures preserve the current draft; an empty composer is refilled with that turn's prompt for editing and retrying.

Interactive questions contain numbered choices and descriptions. Use Up/Down or number keys to select, Tab to edit notes, Left/Right to switch questions, Enter to submit, and Esc to interrupt. Selecting a skill inserts `$skill-name` into the composer. The model can load skills and their supporting text files with `read_skill` and `read_skill_resource`.

File tools and Shell use real host paths. Relative paths start at the current working directory, `..` can access adjacent directories, absolute paths keep their system meaning, `/` is the system root, and `~/` expands to the current user's home. File tools return real absolute paths, including Hashline headers. Shell `workdir` follows the same rules and supports directories outside the project.

Full date-times in the UI use local `time.DateTime`, formatted as `YYYY-MM-DD HH:mm:ss`; dates use `YYYY-MM-DD`. `/status` shows the current local time and time zone. The time zone follows the process's system settings and `TZ`. When a region name is unavailable, the UI shows the UTC offset instead of guessing from abbreviations such as CST. Main agents, subagents, Plan, Review, and ACP receive a fresh date, time, RFC3339 timestamp, and offset at each model step, including after date changes, recovery, and compaction. These clock snapshots belong only to the current request and are not saved in conversation context. Compaction preserves the original meaning of historical dates and time zones.

Session, event, and background-terminal metadata use RFC3339 JSON timestamps with `Z` or an explicit offset, retaining fractional-second precision without rewriting old records. `stat.modified` uses RFC3339Nano. ACP diagnostic logs also include time zones and go only to standard error. Session and terminal IDs remain compatible; their embedded time fragments should not be used to infer the current date. The current step's clock snapshot is the model's time reference. Elapsed times and timeouts continue to use durations. Dedicated fields in formats such as PDF follow their own date specifications.

By default, `glob` and `grep` use Git to search tracked files and unignored untracked files, including tracked files that match ignore rules. Set `include_ignored=true` to include ignored files. Recursive searches skip Git metadata and submodules unless explicitly selected as the search root. Searches across projects use the target repository's rules; direct `read` calls are unaffected. Non-Git projects search the full directory tree. Git rules refresh on every search. Git enumeration failures are reported explicitly; narrow the search path when enumeration exceeds 20,000 candidate files or 8 MiB of output.

Glob patterns support `*.go`, `**/*.go`, `?`, and character classes; `**` can match zero directory levels. `grep` searches literal text with `query` and filters files with `include`, for example `{"path":"internal","query":"cursorPosition","include":["*.go"]}`. When results report an incomplete search, inspect the diagnostics and narrow the scope rather than concluding that the project contains no matching code. `fs_instructions` queries project rules; the host injects global rules separately.

Agent mode uses YOLO permissions: file tools and Shell operate with the current user's OS permissions, without a project-directory boundary. Reads follow symbolic links; overwrite writes and edits update their targets. Moving or deleting a link operates on the link itself. Creating a file does not overwrite an existing link. Old sessions remain resumable. Their original `/foo` paths referred to project-relative files; recovery displays a migration notice. Read targets again under the new semantics before editing; old snapshots are not reused. Plan mode exposes only read-only and interaction tools. Branching restores model context before the selected prompt; prior file changes and command effects remain in the workspace. Each session has an independent lock, so other processes can open different sessions in the same project. Running goals, subagents, and Shell processes end when the owning session closes.

In an ACP-compatible editor, use `clara-code acp` as the stdio agent command. It supports session creation and loading, prompting, streaming updates, and cancellation, using the same project identification and persistence directories.
