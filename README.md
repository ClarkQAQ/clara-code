# Clara Code

Clara Code is a terminal coding assistant built on Clara Core. It uses the terminal's main screen and preserves completed conversations in native scrollback. Resizing rebuilds the terminal buffer from logical messages and reflows the conversation, including the current question, menu, and input draft. The interactive CLI supports macOS, Linux, and Windows 11 (x64 and ARM64).

## Getting Started

For prebuilt packages, see [installation on Linux, macOS, and Windows](.forgejo/README.md#installation).
Building from source requires Go 1.27. The CLI and reusable agent, model, and tool packages all live in this repository.

```sh
go run . --demo
go build -o clara-code .
./clara-code --cwd /path/to/project
./clara-code resume --cwd /path/to/project
```

On Windows, open Windows Terminal and use PowerShell or Command Prompt:

```powershell
go build -o clara-code.exe .
.\clara-code.exe --demo
.\clara-code.exe --cwd 'C:\Projects\Example'
```

On Windows, the local command tool selects PowerShell 7 from PATH, Windows PowerShell 5.1, or Command Prompt in that order. Clara reports the selected shell to the model. The shell that launched Clara can differ from the shell Clara uses to run tool commands. When using the Command Prompt fallback, use a drive-backed working directory for command execution. For stdio ACP and MCP, configure the server executable and arguments directly. A batch shim can be invoked through an explicitly configured `cmd.exe`.

Clara's terminal title shows the project name, with a spinner during requests and an `Action Required` prefix while it waits for an answer. In VS Code, use `${sequence}` for `terminal.integrated.tabs.title` if your tab only shows the process name.

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

An adjacent Clara Core checkout is no longer needed. Protocol formats remain compatible. The application uses the new configuration format. The original examples continue to use their `CLARA_CORE_*` settings. `make check` runs formatting checks, tests, race tests, vet, lint, and CLI and example builds. `make build` produces `clara-code` at the repository root. Inject a version with `go build -ldflags '-X main.version=VERSION' .`. The [Development Guide](AGENTS.md) defines placement rules and verification requirements for new features. The original framework's license, attribution, and prompt provenance are preserved.

Session SQL lives in `internal/session`. After adding a numbered Goose migration or changing a query, run `go generate ./...` and include the generated sqlc code with the SQL changes. The module pins sqlc and golangci-lint for use through `go tool`.

Configure a live model through environment variables:

```sh
export CLARA_CODE_ENDPOINT='https://gateway.example/v1'
export CLARA_CODE_PROTOCOL='responses' # or completions
export CLARA_CODE_MODEL='your-model'
export CLARA_CODE_API_KEY='your-key'
./clara-code
```

## Configuration

On Linux, the data directory defaults to `~/.local/share/clara-code`, or `clara-code` under `XDG_DATA_HOME` when set. On macOS, it is `~/Library/Application Support/clara-code`. On Windows, it is `%LOCALAPPDATA%\ClaraCode`. `CLARA_CODE_HOME` overrides the directory on all platforms.

On a regular interactive launch with no configuration files and no complete model configuration from environment variables or flags, Clara Code guides you through setting up a provider and a default model. Enter the provider's base URL and API protocol, the model ID, and optionally the API key and model token limits. The key is masked as you type. Use Shift+Tab to go back or Esc to cancel. The last step saves a private `config.toml` in the data directory and opens the normal composer. No connection is made until you send a message. Demo, resume, and ACP launches skip the guide.

Use [config.example.toml](config.example.toml) for the complete supported schema. The old top-level `endpoint`, `protocol`, `api_key_*`, and `[models]` format is no longer accepted. There are no profiles. Existing saved sessions are not deleted by this configuration change.

Configuration precedence, from lowest to highest:

1. Built-in defaults.
2. `CLARA_CODE_HOME/config.toml`.
3. `.agents/clara-code.toml` in each directory from the canonical Git root to the selected working directory. Non-Git workspaces load only their current directory's file.
4. `CLARA_CODE_*` environment overrides.
5. Repeated `-c/--config key=value`, applied in order.
6. Dedicated `-m/--model` options.

`--cwd` is resolved before discovery. Resuming a session reloads configuration for its saved working directory; CLI and environment overrides still apply. Project configuration has full authority, including commands, providers, headers, and authentication. Only use project configuration you intend to run. Project files cannot redefine the home directory or working directory.

```toml
model = "deepseek-flash"
model_provider = "gulugulu"
model_reasoning_effort = "high"
plan_mode_reasoning_effort = "high"

[model_providers.gulugulu]
name = "Gulugulu"
base_url = "https://gateway.example/v1"
wire_api = "responses" # or completions
api_key = "REPLACE_ME"
request_max_retries = 3
stream_idle_timeout_ms = 120000

[model_providers.gulugulu.models.deepseek-flash]
context_window = 1048576
input_modalities = ["text", "image"]
default_reasoning_level = "high"
supported_reasoning_levels = ["low", "high", "max"]
```

Environment overrides: `CLARA_CODE_MODEL_PROVIDER`, `CLARA_CODE_MODEL`, `CLARA_CODE_MODEL_REASONING_EFFORT` (preferred over `CLARA_CODE_EFFORT`), `CLARA_CODE_PLAN_MODE_REASONING_EFFORT`, and `CLARA_CODE_AUTO_COMPACT_CONTEXT_PERCENT`. `CLARA_CODE_ENDPOINT`, `CLARA_CODE_PROTOCOL`, and `CLARA_CODE_API_KEY` override the provider selected at the environment layer. They are not reused as credentials for other providers. `CLARA_CORE_*` fallback is reserved for standalone framework examples, not application configuration.

```sh
clara-code --cwd /path/to/project \
  -c 'model_provider="gulugulu"' \
  -c 'plan_mode_reasoning_effort="max"' \
  -c 'tui.status_line=["model","context","directory"]'
clara-code -c 'model_providers."my.gateway"={base_url="http://localhost:8080/v1",wire_api="completions"}'
```

Overrides split at the first `=`. Values are parsed as TOML; invalid TOML values become strings. Dotted and quoted keys, arrays, and inline tables are supported. Tables merge by key; arrays replace the entire value. Unknown settings and unsupported values fail with their source and configuration path, without echoing secret values.

Providers authenticate with `api_key` directly in their configuration table. `CLARA_CODE_API_KEY` remains an optional environment override. Keep actual keys out of version control. Providers also support literal and environment-backed HTTP headers, request retries, total request deadlines, and stream idle deadlines. Only requests that fail before streaming starts are eligible for Core's retry policy. Streams are never automatically replayed after output begins. The activity line reports timeouts and connection failures. Authentication is redacted from diagnostics.

Model catalogs drive `/model` provider/model/effort selectors and request behavior. Each model can specify an API `id`, display name, description, priority, visibility, context/output budgets, modalities, reasoning levels, summary, verbosity, and parallel tool calls. Omitted modalities default to text/image; omitted reasoning metadata retains permissive defaults. Explicit unsupported reasoning levels fail. Agent and Plan keep independent session settings; an omitted Plan level inherits Agent. Switching models retains context, resets incompatible saved levels, and reports the reset. UI selection is saved in the session, not written to configuration. `/status` shows the provider, model capabilities, both levels, and loaded configuration sources.

`[tui]` controls color, Markdown, animation, hints, preview size, and ordered status fields. `[tui.keymap.<context>]` remaps existing actions for `composer`, `picker`, `question`, and `rewind`; the example lists every action. Same-context conflicts fail startup. Printable text, IME, and bracketed paste keep their normal input path. Hints use configured keys. Tool call deadlines can be set globally or per tool; command yield/runtime defaults are separate, and explicit model arguments take precedence. MCP servers can be disabled and have separate connection and call deadlines; stdio/HTTP, headers, environment, and external ACP agents remain supported.

`tui.status_line` selects and orders `state`, `model`, `context`, `directory`, `git_branch`, `session_cache_efficiency`, and `round_cache_efficiency`. An empty array hides the left status fields. For example:

```toml
[tui]
status_line = ["state", "model", "context", "session_cache_efficiency", "round_cache_efficiency"]
```

Cache efficiency is cached input tokens divided by all input tokens; output/reasoning tokens are excluded. Session efficiency sums this conversation's model requests. Round efficiency sums every model step in the current agent run, including tool continuations and steering, and shows the most recent run while idle. Ratios are weighted by tokens, not averages of request percentages. Counters survive recovery, model changes and compaction; a new or forked session starts fresh. Missing cache reports, incomplete legacy accounting, and an empty input denominator display `unknown`; an explicitly reported zero displays `0.0%`. `/status` also shows both metrics. Existing configurations keep their default field list.

Configured model IDs are declared to the Responses provider, including custom deployment aliases, so explicit Agent/Plan reasoning effort is transmitted even when the model name is absent from the built-in reasoning-model catalog.

`default_reasoning_summary = "none"` disables summaries by omitting `reasoning.summary` from requests. The wire values are `auto`, `concise`, and `detailed`; `none` is a local configuration value and does not change the reasoning effort.

`context_window` is the model's raw token limit. Its effective window defaults to 95%; change it with `effective_context_window_percent`. Automatic compaction defaults to 90% of the raw window, matching the local Codex reference; it does not multiply the effective-window ratio again. Set root `auto_compact_context_percent` (1–100) for the application default or the same field under an individual model to override it. Resolved values above 90% are clamped to 90%, following Codex's raw-window ceiling. The trigger is capped by the effective window minus output reservation; the resulting summary must fit that hard input budget. Recent usage and conservative estimates calibrate triggering. Models without a declared window use a bounded 256K-token heuristic for compaction only; the UI still labels their actual context limit unknown. Configure `context_window` to replace that heuristic. Application `context_bytes` and `CLARA_CODE_CONTEXT_BYTES` have been removed; delete old keys and use token-window settings instead. `CLARA_CODE_AUTO_COMPACT_CONTEXT_PERCENT` overrides the root setting. Compaction preserves the latest turn and its images; `/compact` requires an older complete turn. Failed or truncated summaries preserve the original working history. See [Prompt and Context Architecture](PROMPTS.md) for details and the Codex comparison.

Current context usage in the status bar comes from the most recent valid request's total tokens, independently of budget estimates. The percentage calculation subtracts a fixed 12,000-token baseline, following the reference Codex implementation. Cumulative usage appears separately in `/status`; unavailable usage is shown as unknown. Put global rules in `AGENTS.md` in the data directory and global skills in `skills/<name>/SKILL.md`. Project rules load from the Git root downward; project skills live in `.agents/skills/` at the applicable directory. Skill files declare `name` and `description` in YAML or TOML front matter. Rules are checked each model step, provided in full in the initial prefix, and updated only when changed or removed. Skill bodies load only when requested.

Clara uses a Codex-derived base prompt (see NOTICE), with native-path and actual-tool adaptations. Responses sends this base through top-level `instructions`; Completions keeps its system message. Developer policy and initial rules/environment stay at the front. Later requests retain that prefix and append only context changes, including environment updates only when its date, time zone, paths or permissions change. Readable AGENTS headings and `<environment_context>` replace management JSON wrappers. An independent, optional model-window checkpoint preserves prefix placement across recovery; branching and successful compaction rebuild a fresh prefix. Context management is excluded from raw conversation and terminal replay. Provider caching depends on the endpoint, so stable prefixes do not guarantee a particular cache hit rate.

## Images

Ctrl+V reads an image from the system clipboard in the background and falls back to clipboard text. Ordinary terminal paste still uses bracketed paste. macOS uses AppKit through the system JavaScript bridge; Linux uses `wl-paste` on Wayland or `xclip` on X11. Windows reads Unicode text and PNG or bitmap images from the system clipboard. If Windows Terminal handles Ctrl+V itself, change its paste binding to let Clara receive that shortcut. Reads have a four-second timeout and bounded output. Missing clipboard helpers are reported without installing anything; `/attach PATH` remains available.

Paste or drag complete local image paths, including quoted or escaped spaces, multiple paths, and `file://` URLs. Paths inside an ordinary sentence remain text. `/attach <path>` explicitly adds an image. Invalid paths retain the pasted draft and report the failure. Attachments appear inline as cyan `[Image #1]` markers at the insertion point. Text can be edited before and after each marker. Left/Right cross the entire marker; Backspace/Delete remove it as one attachment. Numbering follows display order. Ctrl+G opens a filename/dimension selector for managing attachments. Pure-image messages, text with multiple images, steering, and `/follow` are supported. Unaccepted and failed submissions retain text and attachments.

By default, each submission can include any number of images without an aggregate byte limit. Set `[images].max_attachments` or `max_total_bytes` to a positive value to cap the image count or combined normalized size. Omit either setting or use `0` or `-1` to disable that cap. Clara shrinks decoded images to fit both `max_pixels` and `max_dimension`, which default to 64 million pixels and a 2048-pixel longest side. The default source size limit is 16 MiB per image, and the decoder accepts up to 64 million source pixels. A model explicitly lacking image support rejects new attachments and blocks drafts containing images. Historical images never block model switching: request copies replace them with an explicit omission notice, including images from file/MCP tools. Original history is unchanged, so switching back restores image input. `image_view` is only offered and executable for image-capable models. The same policy applies to main agents, subagents, Plan, Review, and ACP.

Normalized images are stored by content hash under the application data directory's `attachments/`. SQLite checkpoints and transcripts store attachment references. Recovery, forks, and prompt rewind retain images without depending on their original paths. Missing/corrupt blobs produce explicit errors. Requests reconstruct standard model media parts; public packages do not depend on Clara's storage layout. Compaction uses image descriptions and findings, retaining original transcript attachments for replay and branching. For a file-copy backup, exit all Clara processes first, then copy `sessions.sqlite3` and `attachments/` together.

Terminal regression tests cover inline markers, atomic editing, asynchronous insertion anchors, deletion, resize, cursor placement, and native scrollback. Clipboard tests use mocked platform adapters and a private named macOS pasteboard; they do not read or change the user's clipboard. Actual OS IME and clipboard behavior in VS Code and Linux desktop sessions still requires manual acceptance.

## Interaction

| Action | Key or command |
| --- | --- |
| Submit input or steer an active turn | Enter |
| Insert a newline | Shift+Enter in terminals that support extended keys, or Alt+Enter / Ctrl+J |
| Switch between Agent and Plan | Shift+Tab, or `/agent` and `/plan` |
| Interrupt a turn | Esc or `/cancel` |
| View or stop background terminals, including during a turn | `/ps`, `/stop [ID]`; omit the ID to stop all terminals present when called |
| Edit a previous message and create a branch | Press Esc twice with empty input; use Left/Right to select, Up/Down to scroll, Ctrl+T to toggle details, and Enter to confirm |
| Create, resume, or fork a conversation | `/new`, `/resume [ID]`, `/fork` |
| Clear the terminal and start a new conversation | `/clear`; the old session remains resumable |
| View usage, configuration, or the working directory | `/status`, `/pwd` |
| View changes | `/diff`; Tab switches staged, unstaged, and untracked views; n/p changes pages |
| Attach or manage images | Ctrl+V, `/attach <path>`, Ctrl+G; Delete removes a selected attachment |
| Select a model and reasoning effort | `/model [name [effort]]` |
| Review current Git changes without writing | `/review` |
| Compact model context manually | `/compact` |
| Run a goal or queue follow-up instructions | `/goal`, `/pause`, `/continue`, `/follow` |
| View connections or manage skills | `/mcp`, `/skills` |
| Show help or exit | `/help`, `/exit`, `/quit` |

Clara requests the Kitty keyboard protocol in compatible terminals. In tmux,
Shift+Enter requires extended keys (`set -s extended-keys on`) and a terminal that
can report them.

Type `/` to open the two-column command menu. Use Up/Down to select, Tab to complete, Enter to confirm, and Esc to close while retaining the draft. Plan mode appears on the right of the status bar.

When opening a conversation, the terminal shows the MCP server currently connecting and the number of attempts completed. If any optional servers fail, the session opens and the terminal prints one warning with the affected servers and their errors. Use `/mcp` to see all enabled servers, their connection results, and the tool counts for successful connections. A failure for a server configured with `required = true` prevents the conversation from opening.

Set `protocol_version` under a server's `[mcp_servers.<name>]` table to request a specific handshake version. For example, `auv 0.0.43` connects with `protocol_version = "2025-03-26"`, which starts with `initialize`. Omit the field or set it to an empty string to use automatic negotiation. To override an existing `auv` entry for one launch, run `clara-code -c 'mcp_servers.auv.protocol_version="2025-03-26"'`.

In Plan mode, a completed proposal appears in a **Proposed Plan** card. Use Up/Down and Enter, or press 1–3, to keep the current conversation and start implementing in Agent mode, start a fresh Agent conversation with the approved plan, or stay in Plan mode to refine it. Esc dismisses the choices and stays in Plan mode. Starting fresh keeps the original planning conversation available through `/resume`.

The main composer, question notes, and free-text answers use the real terminal cursor, allowing Chinese IME candidates and input-mode indicators to follow the insertion point. The cursor keeps the terminal's default shape, is hidden in read-only selectors, and becomes visible again on exit. Positions use terminal display columns for Chinese text, combined emoji, multiline input, and narrow windows. The terminal and system IME control candidate appearance and selection.

Model progress updates and final answers are stored separately. Tool execution appears as an updating record such as `● Bash(command)`, with output beneath `└`. Green indicates success, red indicates failure, and cancellation and background execution have distinct states. Shell results use actual exit codes; subsequent background output updates the original record. Long previews retain the beginning and end. Structured records follow session storage limits and support replay on recovery and resize.

Background terminals belong to their Clara session, including commands started by subagents. `yield_time_ms` controls how many milliseconds an initial execution or subsequent wait blocks before returning. The TOML setting `[tools.exec_command].yield_seconds` still uses seconds. Omitting `timeout_seconds` removes the total runtime limit; explicit timeouts still apply, up to 600 seconds. Esc interrupts only the current agent turn and wait. Detached terminals continue running until explicitly stopped with `/stop`, or cleaned up when their session closes or switches, or the process exits.

While a background command is running, the area above the composer shows `1 background terminal running · /ps to view · /stop to close`, merged into the activity indicator when visible. Status is checked every 500 ms without consuming output, and the UI refreshes only when status or output changes. `/ps` does not take output the model has not yet read. A model poll with empty input produces a separate `Waiting for background terminal` record, retained as `Waited for background terminal` afterward. Sending input produces `Interacted with background terminal`; the original Bash record continues to show output and final status. Canceling an input write only stops waiting; characters already delivered to the terminal cannot be recalled.

Asynchronous command states are saved separately using atomic writes; completed turn files are not rewritten. Natural exits, failures, stops, and timeouts update the original command record. Unread output remains available through `write_stdin`. The completion cache retains at most the latest 32 terminals, with the existing 1 MiB output limit per terminal and head-and-tail previews. Recovery and branching replay records without restarting processes; old records without a final state appear as interrupted. Custom execution backends without Core's `SnapshotProcess` update only when the model explicitly polls; `/ps` reports this limitation.

The activity line above the composer shows waiting, execution, compaction, and reconnection states, including elapsed time and the Esc interruption hint. It is temporarily hidden for interactive questions. Public reasoning summaries may supply an activity title; raw reasoning is not displayed as a progress update. Core retries eligible connection-establishment errors up to three times, displaying the attempt, delay, and redacted reason. Interruptions after streaming begins preserve received text and report the error without automatically replaying executed tools. Final failures preserve the current draft; an empty composer is refilled with that turn's prompt for editing and retrying.

Interactive questions contain numbered choices and descriptions. Use Up/Down or number keys to select, Tab to edit notes, Left/Right to switch questions, Enter to submit, and Esc to interrupt. Type `$` at a word boundary to open the inline skill menu, type to filter names or descriptions, use Up/Down to select, and Enter or Tab to insert `$skill-name` without submitting. Inserted skill references are highlighted and edited as atomic tokens: Left/Right jumps over them, and Backspace/Delete removes the whole reference, like an image attachment. Restored drafts recover known skill references without changing their submitted text. Esc closes the menu; shell expansions such as `$HOME` and `${HOME}` stay ordinary text. Text before and after the mention and image attachments are preserved. The model loads selected skills and supporting text files with `read_skill` and `read_skill_resource`.

`/skills` opens the **Skills** management menu: **List skills** opens the same `$` menu, and **Enable/Disable Skills** opens a searchable checkbox list. Up/Down selects, Space or Enter toggles, and Esc goes back. Changes save automatically in `skills-state.json` under the Clara data directory, without rewriting `config.toml` or skill files. Preferences use canonical `SKILL.md` paths, so disabling a project skill does not disable unrelated skills with the same name. The nearest definition still wins: disabling it does not expose a shadowed global definition. Disabled skills are removed from completion, model catalogs and catalog-reading tools on the next request, for main agents, children and ACP. Re-enabling restores them. This is a discovery preference, not a filesystem access restriction. Atomic saves and a process lock protect concurrent changes.

File tools and Shell use real host paths. Relative paths start at the current working directory, `..` can access adjacent directories, absolute paths keep their system meaning, `/` is the system root, and `~/` expands to the current user's home. File tools return real absolute paths, including Hashline headers. Shell `workdir` follows the same rules and supports directories outside the project.

Full date-times in the UI use local `time.DateTime`, formatted as `YYYY-MM-DD HH:mm:ss`; dates use `YYYY-MM-DD`. `/status` shows the current local time and time zone. The time zone follows the process's system settings and `TZ`. When a region name is unavailable, the UI shows the UTC offset instead of guessing from abbreviations such as CST. Main agents, subagents, Plan, Review, and ACP receive the local date and time zone in a compact environment block. The host checks these each step but emits an update only when a value changes; passing seconds do not add messages. No precise clock or per-message sending timestamp is injected. Environment context stays in the separate model-window checkpoint, outside the raw conversation and terminal replay. Compaction preserves the original meaning of historical dates and time zones.

Session, event, and background-terminal metadata use RFC3339 JSON timestamps with `Z` or an explicit offset, retaining fractional-second precision without rewriting old records. `stat.modified` uses RFC3339Nano. ACP diagnostic logs also include time zones and go only to standard error. Session and terminal IDs remain compatible; their embedded time fragments should not be used to infer the current date. The latest environment date and time zone are the model's reference for relative dates; they do not provide a precise current clock. Elapsed times and timeouts continue to use durations. Dedicated fields in formats such as PDF follow their own date specifications.

Clara reads local Git repositories with go-git, including for `/diff`, `/review`, the branch indicator, and search filters. These features work without the Git executable or cgo. Staged and unstaged patches show a notice in place of text larger than 1 MiB per file.

By default, `glob` and `grep` use Git metadata to search tracked files and unignored untracked files, including tracked files that match ignore rules. Set `include_ignored=true` to include ignored files. Recursive searches skip Git metadata and submodules unless explicitly selected as the search root. Searches across projects use the target repository's rules; direct `read` calls are unaffected. Non-Git projects search the full directory tree. Git rules refresh on every search. Git enumeration failures are reported explicitly; narrow the search path when enumeration exceeds 20,000 candidate files or 8 MiB of candidate path names.

Glob patterns support `*.go`, `**/*.go`, `?`, and character classes; `**` can match zero directory levels. `grep` searches literal text with `query` and filters files with `include`, for example `{"path":"internal","query":"cursorPosition","include":["*.go"]}`. When results report an incomplete search, inspect the diagnostics and narrow the scope rather than concluding that the project contains no matching code. `fs_instructions` queries project rules; the host injects global rules separately.

Agent mode lets file tools and Shell operate with the current user's OS permissions, without a project-directory boundary. Reads follow symbolic links; overwrite writes and edits update their targets. Moving or deleting a link operates on the link itself. Creating a file does not overwrite an existing link. Plan mode exposes read-only and interaction tools plus Shell for diagnostics, tests, and builds that preserve tracked files and task-relevant state. Shell runs with the current user's OS permissions. Plan instructions restrict command use, and the Shell process has no additional read-only sandbox. Branching restores model context before the selected prompt; prior file changes and command effects remain in the workspace. Each session has an independent lock, so other processes can open different sessions in the same project. Running goals, subagents, and Shell processes end when the owning session closes.

Clara stores sessions and turns in `sessions.sqlite3` in the data directory. A blank conversation stays in memory until its first message starts a turn. The application applies bundled schema migrations when it opens the database. Legacy JSON files under `sessions/` and `turns/` stay on disk. Session discovery and resume use the SQLite database exclusively.

In an ACP-compatible editor, use `clara-code acp` as the stdio agent command. It supports session creation, discovery, loading, and resuming, along with prompting, streaming updates, and cancellation. `session/list` lists persisted sessions in the Clara data directory, optionally filtered to an exact working directory. Pass the returned `nextCursor` to fetch the next page. `session/load` replays history, while `session/resume` restores the original session in its working directory without replaying it.

When an external ACP role is configured, Clara can call `list_acp_sessions` with that role to discover sessions in the current working directory. Pass a returned `sessionId` as `resume_session_id` to `spawn_agent` or `subagent_task` with the same role. The tool sends the task brief to that session and returns a Clara task ID for follow-ups. Omitting `resume_session_id` starts a new session. Discovery and resuming require the corresponding capabilities from the external agent.

Go hosts can opt into these capabilities through the `core.ACPServerConfig.List` and `Resume` callbacks. `core.ACPExternal.ListSessions` and `ResumeSession` check the negotiated capabilities before sending requests.

Clara also exposes Agent, Plan, Git review, model selection, and reasoning effort through ACP session configuration options. Clients that advertise form elicitation can answer `request_user_input` over the same connection. Local clients may opt in to the versioned `dev.claracode/extensions` capability for rich transcript pages, input steering, local setup, attachments, and diff pages. The Go request and response types are in `pkg/appwire`.


### Delegated tasks

Clara delegates only when authorized by the user or applicable instructions.
Task names use lowercase ASCII letters, digits and underscores, up to 100
characters: `repo_survey` is valid, `repo-survey` is not. Every task needs a role
and a complete brief; children start without parent conversation unless a history
fork is explicitly requested. Built-in `general` can implement and run commands;
`explorer` and `reviewer` inspect files read-only without Shell.

`spawn_agent` returns a task ID for subsequent controls. `subagent_task` waits for
one result without closing the child. Waiting uses `timeout_ms` in milliseconds;
a timeout leaves the child running, so inspect `list_agents` and wait again.
`send_message` steers running work, `followup_task` can continue finished work,
`interrupt_agent` retains its session, and `close_agent` ends it and its descendants.
Read and verify results before closing. Filesystem changes remain shared.


### Reasoning selection and input history

`/model` selects a provider/model, then shows separate Agent and Plan reasoning
pickers with numbered choices, explanations, `(default)` and `(current)` markers.
Plan can inherit the staged Agent level. Both values are saved to the session only
after the final confirmation; Esc goes back without partially changing settings.
The choices do not rewrite the global configuration or clear model context.

Submitted prompts and slash commands are saved in `history.jsonl` in the Clara
data directory and loaded on startup for Up/Down recall across conversations.
Menu commands record their canonical form (`/status`, not `/sta`); consecutive
identical inputs are collapsed. Like Codex's local command recall, `/clear` itself
is omitted. Each JSONL record contains `session_id`, an RFC3339 `ts`, `text`, kind,
and optional attachment references. Files use private permissions, process locking
and atomic replacement; retention keeps at most 1000 entries and 4 MiB. Malformed
records are reported while valid records remain usable. History write failures do
not prevent command dispatch or discard the draft.

Input recall is separate from terminal scrollback, saved model turns and the
Esc-Esc conversation preview. Commands do not become model user messages or
rewind checkpoints. Recalled images and available skill mentions are restored in
the composer. A recalled command stays in history navigation until edited rather
than opening a new slash-command menu over the Up/Down keys.
