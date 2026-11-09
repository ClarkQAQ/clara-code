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

An adjacent Clara Core checkout is no longer needed. Saved sessions and protocol formats remain compatible; the new application configuration format intentionally replaces the old format; the original examples continue to use their `CLARA_CORE_*` settings. `make check` runs formatting checks, tests, race tests, vet, lint, and CLI and example builds. `make build` produces `clara-code` at the repository root. Inject a version with `go build -ldflags '-X main.version=VERSION' .`. The [Development Guide](AGENTS.md) defines placement rules and verification requirements for new features. The original framework's license, attribution, and prompt provenance are preserved.

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

Use [config.example.toml](config.example.toml) for the complete supported schema. The old top-level `endpoint`, `protocol`, `api_key_*`, and `[models]` format is no longer accepted. There are no profiles. Existing saved sessions are not deleted by this configuration change.

Configuration precedence, from lowest to highest:

1. Built-in defaults.
2. `CLARA_CODE_HOME/config.toml`.
3. `.agents/clara-code.toml` in each directory from the canonical Git root to the selected working directory. Non-Git workspaces load only their current directory's file.
4. `CLARA_CODE_*` environment overrides.
5. Repeated `-c/--config key=value`, applied in order.
6. Dedicated `-m/--model` options.

`--cwd` is resolved before discovery. Resuming a session reloads configuration for its saved working directory; CLI and environment overrides still apply. Project configuration has full authority under YOLO mode, including commands, providers, headers, and authentication. Only use project configuration you intend to run. Project files cannot redefine the home directory or working directory.

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

Ctrl+V reads an image from the system clipboard in the background and falls back to clipboard text. Ordinary terminal paste still uses bracketed paste. macOS uses AppKit through the system JavaScript bridge; Linux uses `wl-paste` on Wayland or `xclip` on X11. Reads have a four-second timeout and bounded output. Missing clipboard helpers are reported without installing anything; `/attach PATH` remains available.

Paste or drag complete local image paths, including quoted or escaped spaces, multiple paths, and `file://` URLs. Paths inside an ordinary sentence remain text. `/attach <path>` explicitly adds an image. Invalid paths retain the pasted draft and report the failure. Attachments appear inline as cyan `[Image #1]` markers at the insertion point. Text can be edited before and after each marker. Left/Right cross the entire marker; Backspace/Delete remove it as one attachment. Numbering follows display order. Ctrl+G opens a filename/dimension selector for managing attachments. Pure-image messages, text with multiple images, steering, and `/follow` are supported. Unaccepted and failed submissions retain text and attachments.

Defaults are four images per submission, a 16 MiB source limit per image, a 64 MP decode limit, a 2048-pixel longest side, and an 8 MiB combined normalized payload. `[images]` can lower or adjust the bounded limits. Images use the existing decoder and normalization path. A model explicitly lacking image support rejects new attachments and blocks drafts containing images. Historical images never block model switching: request copies replace them with an explicit omission notice, including images from file/MCP tools. Original history is unchanged, so switching back restores image input. `image_view` is only offered and executable for image-capable models. The same policy applies to main agents, subagents, Plan, Review, and ACP.

Normalized images are stored by content hash under the application data directory's `attachments/`. Session and transcript files contain references instead of base64. Recovery, forks, and prompt rewind retain images without depending on their original paths. Missing/corrupt blobs produce explicit errors. Requests reconstruct standard model media parts; public packages do not depend on Clara's storage layout. Compaction uses image descriptions and findings, retaining original transcript attachments for replay and branching. Back up `attachments/` together with sessions and turns.

Terminal regression tests cover inline markers, atomic editing, asynchronous insertion anchors, deletion, resize, cursor placement, and native scrollback. Clipboard tests use mocked platform adapters and a private named macOS pasteboard; they do not read or change the user's clipboard. Actual OS IME and clipboard behavior in VS Code and Linux desktop sessions still requires manual acceptance.

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
| Attach or manage images | Ctrl+V, `/attach <path>`, Ctrl+G; Delete removes a selected attachment |
| Select a model and reasoning effort | `/model [name [effort]]` |
| Review current Git changes without writing | `/review` |
| Compact model context manually | `/compact` |
| Run a goal or queue follow-up instructions | `/goal`, `/pause`, `/continue`, `/follow` |
| View connections or manage skills | `/mcp`, `/skills` |
| Show help or exit | `/help`, `/exit`, `/quit` |

Type `/` to open the two-column command menu. Use Up/Down to select, Tab to complete, Enter to confirm, and Esc to close while retaining the draft. Plan mode appears on the right of the status bar.

The main composer, question notes, and free-text answers use the real terminal cursor, allowing Chinese IME candidates and input-mode indicators to follow the insertion point. The cursor keeps the terminal's default shape, is hidden in read-only selectors, and becomes visible again on exit. Positions use terminal display columns for Chinese text, combined emoji, multiline input, and narrow windows. The terminal and system IME control candidate appearance and selection.

Model progress updates and final answers are stored separately. Tool execution appears as an updating record such as `● Bash(command)`, with output beneath `└`. Green indicates success, red indicates failure, and cancellation and background execution have distinct states. Shell results use actual exit codes; subsequent background output updates the original record. Long previews retain the beginning and end. Structured records follow session storage limits and support replay on recovery and resize.

Background terminals belong to their Clara session, including commands started by subagents. `yield_seconds` controls how long an initial execution or subsequent wait blocks before returning. Omitting `timeout_seconds` removes the total runtime limit; explicit timeouts still apply, up to 600 seconds. Esc interrupts only the current agent turn and wait. Detached terminals continue running until explicitly stopped with `/stop`, or cleaned up when their session closes or switches, or the process exits.

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

Agent mode uses YOLO permissions: file tools and Shell operate with the current user's OS permissions, without a project-directory boundary. Reads follow symbolic links; overwrite writes and edits update their targets. Moving or deleting a link operates on the link itself. Creating a file does not overwrite an existing link. Old sessions remain resumable. Their original `/foo` paths referred to project-relative files; recovery displays a migration notice. Read targets again under the new semantics before editing; old snapshots are not reused. Plan mode exposes only read-only and interaction tools. Branching restores model context before the selected prompt; prior file changes and command effects remain in the workspace. Each session has an independent lock, so other processes can open different sessions in the same project. Running goals, subagents, and Shell processes end when the owning session closes.

In an ACP-compatible editor, use `clara-code acp` as the stdio agent command. It supports session creation and loading, prompting, streaming updates, and cancellation, using the same project identification and persistence directories.


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
