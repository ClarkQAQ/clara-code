# Configuration

## First launch

When an interactive launch finds neither configuration files nor a complete model configuration from environment variables or flags, Clara Code guides you through setting up a provider and a default model. Enter the provider's base URL and API protocol, the model ID, and optionally the API key and model token limits. The key is masked as you type. Use Shift+Tab to go back or Esc to cancel.

The last step saves a private `config.toml` in the data directory and opens the composer. Clara connects when you send your first message. Demo, resume, and ACP launches skip the guide.

## Configuration files and precedence

The data directory depends on the platform.

| Platform | Default data directory |
| --- | --- |
| Linux | `~/.local/share/clara-code`, or `clara-code` under `XDG_DATA_HOME` when set |
| macOS | `~/Library/Application Support/clara-code` |
| Windows | `%LOCALAPPDATA%\ClaraCode` |

`CLARA_CODE_HOME` overrides this directory on all platforms. See [config.example.toml](../config.example.toml) for the complete schema. Migrate legacy top-level `endpoint`, `protocol`, `api_key_*`, and `[models]` settings to the provider and model tables below. Set `model_provider` and `model` to select them directly. This configuration migration preserves saved sessions.

Clara applies configuration sources in order, from lowest to highest precedence.

1. Built-in defaults.
2. `config.toml` in the data directory.
3. `.agents/clara-code.toml` in each directory from the canonical Git root to the selected working directory. Outside Git, Clara loads only the selected directory's file.
4. `CLARA_CODE_*` environment overrides.
5. Repeated `-c/--config key=value` overrides in argument order.
6. Dedicated `-m/--model` selection.

Clara resolves `--cwd` before discovering files. Resuming a session reloads configuration for its saved working directory and reapplies CLI and environment overrides. Project configuration has full authority over commands, providers, headers, and authentication. Use project configuration you intend to run. Clara resolves the data directory and working directory independently of project files.

## Model providers

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

Providers authenticate with `api_key` in their configuration table. `CLARA_CODE_API_KEY` can override that value. Keep actual keys out of version control. Providers also support literal and environment-backed HTTP headers, request retries, total request deadlines, and stream idle deadlines. Core retries eligible failures up to the start of streaming. The activity line reports timeouts and connection failures, and diagnostics redact authentication.

Model catalogs drive request behavior and the `/model` selectors for provider, model, and reasoning effort. Each model can specify an API `id`, display name, description, priority, visibility, context and output budgets, modalities, reasoning levels, summary, verbosity, and parallel tool calls. Omitted modalities default to text and image. Omitted reasoning metadata retains permissive defaults. Setting an explicitly unsupported reasoning level fails.

Agent and Plan keep independent session settings. If the Plan reasoning level is omitted, it inherits Agent's. Switching models retains context, resets incompatible saved levels, and reports the reset. Selection through `/model` saves to the session. `/status` shows the provider, model capabilities, both levels, and loaded configuration sources.

The Responses provider recognizes configured model IDs, including custom deployment aliases, so it sends explicit Agent and Plan reasoning effort for those IDs as well as models in the built-in reasoning catalog. Set `default_reasoning_summary = "none"` to disable summaries by omitting `reasoning.summary` from requests. The wire values are `auto`, `concise`, and `detailed`. `none` affects only the summary.

## Environment variables

You can configure a live model for a launch through environment variables.

```sh
export CLARA_CODE_ENDPOINT='https://gateway.example/v1'
export CLARA_CODE_PROTOCOL='responses' # or completions
export CLARA_CODE_MODEL='your-model'
export CLARA_CODE_API_KEY='your-key'
./clara-code
```

`CLARA_CODE_MODEL_PROVIDER` and `CLARA_CODE_MODEL` select the provider and model. Clara also reads `CLARA_CODE_MODEL_REASONING_EFFORT` (which takes precedence over `CLARA_CODE_EFFORT`), `CLARA_CODE_PLAN_MODE_REASONING_EFFORT`, and `CLARA_CODE_AUTO_COMPACT_CONTEXT_PERCENT`. `CLARA_CODE_ENDPOINT`, `CLARA_CODE_PROTOCOL`, and `CLARA_CODE_API_KEY` apply only to the provider selected at the environment layer. The `CLARA_CORE_*` fallback is reserved for standalone framework examples.

## Command-line overrides

```sh
clara-code --cwd /path/to/project \
  -c 'model_provider="gulugulu"' \
  -c 'plan_mode_reasoning_effort="max"' \
  -c 'tui.status_line=["model","context","directory"]'
clara-code -c 'model_providers."my.gateway"={base_url="http://localhost:8080/v1",wire_api="completions"}'
```

Clara splits overrides at the first `=` and parses values as TOML, falling back to strings for invalid TOML values. Overrides support dotted and quoted keys, arrays, and inline tables. Tables merge by key, while arrays replace the entire value. Unknown settings and unsupported values produce errors with the configuration source and path. Errors redact secret values.

## Terminal UI

`[tui]` controls color, Markdown, animation, hints, preview size, and ordered status fields. `[tui.keymap.<context>]` remaps existing actions for `composer`, `picker`, `question`, and `rewind`. The [example configuration](../config.example.toml) lists every action. Conflicts within one context cause startup to fail. Printable text, IME, and bracketed paste keep their normal input path. Hints use the configured keys.

`tui.status_line` selects and orders `state`, `model`, `context`, `directory`, `git_branch`, `session_cache_efficiency`, and `round_cache_efficiency`. An empty array hides the left status fields. Existing configurations keep their default field list.

```toml
[tui]
status_line = ["state", "model", "context", "session_cache_efficiency", "round_cache_efficiency"]
```

Cache efficiency divides cached input tokens by all input tokens. Session efficiency sums this conversation's model requests. Round efficiency sums every model step in the current agent run, including tool continuations and steering. It shows the most recent run while idle. Each ratio is weighted by input tokens. Counters survive recovery, model changes, and compaction. New and forked sessions start fresh. Missing cache reports, incomplete legacy accounting, or an empty input denominator display `unknown`. An explicitly reported zero displays `0.0%`. `/status` also shows both metrics.

The context status field uses total tokens from the most recent valid request. The percentage calculation subtracts a fixed 12,000-token baseline, following the reference Codex implementation. Cumulative usage appears separately in `/status`, and unavailable usage appears as `unknown`.

## Tool deadlines

Set tool call deadlines globally or per tool. Command yield and runtime defaults are separate, with explicit model arguments taking precedence.

## MCP servers

MCP servers support stdio and HTTP, headers, environment variables, and separate connection and tool call deadlines. Use `enabled` to control which servers connect. External ACP agents have their own entries under `[acp_agents]`.

Set `protocol_version` under a server's `[mcp_servers.<name>]` table to request a specific handshake version. For example, `auv 0.0.43` connects with `protocol_version = "2025-03-26"`, which starts with `initialize`. Omit the field or set it to an empty string for automatic negotiation. Override an existing `auv` entry for one launch with `clara-code -c 'mcp_servers.auv.protocol_version="2025-03-26"'`.

See [MCP connections](usage.md#mcp-connections) for startup behavior and connection status.

## Context window and compaction

`context_window` is the model's raw token limit. The effective window defaults to 95% of that limit and can be set with `effective_context_window_percent`. Automatic compaction defaults to 90% of the raw window, matching the local Codex reference. Set `auto_compact_context_percent` (1–100) at the root for the application default or under an individual model to override it. `CLARA_CODE_AUTO_COMPACT_CONTEXT_PERCENT` overrides the root setting. Clara caps resolved percentages at 90% of the raw window.

Clara caps the trigger at the effective window minus the output reservation and checks that the resulting summary fits the same input budget. Recent usage and conservative estimates calibrate triggering. A model without a declared `context_window` uses a bounded 256K-token heuristic for compaction while the UI labels its actual context limit unknown. Set `context_window` to use a declared limit. Replace the removed `context_bytes` setting and `CLARA_CODE_CONTEXT_BYTES` environment variable with token-window settings.

Compaction preserves the latest turn and its images. `/compact` requires an older complete turn. Failed or truncated summaries preserve the original working history. See [Prompt and Context Architecture](prompts.md) for details and the Codex comparison.

## Rules and skills

Put global rules in `AGENTS.md` in the data directory and global skills in `skills/<name>/SKILL.md`. Project rules load from the Git root downward. Project skills live in `.agents/skills/` at the applicable directory. Skill files declare `name` and `description` in YAML or TOML front matter.

Clara checks rules on each model step, includes them in full in the initial prefix, and updates that context only when rules change or are removed. Skill bodies load only when requested.

## Prompt composition

Clara uses a Codex-derived base prompt with native-path and actual-tool adaptations. See [NOTICE](../NOTICE) for attribution. Responses sends the base prompt through top-level `instructions`, while Completions uses its system message. Developer policy and initial rules and environment stay at the front. Later requests retain that prefix and append updates when context changes, including changes to the environment date, time zone, paths, or permissions. Clara presents this context with readable AGENTS headings and `<environment_context>` in place of management JSON wrappers.

An independent, optional model-window checkpoint preserves prefix placement across recovery. Branching and successful compaction rebuild a fresh prefix. Context management stays separate from the raw conversation and terminal replay. Provider caching depends on the endpoint.
