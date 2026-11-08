# Prompt and Context Architecture

Clara separates reusable agent mechanisms from coding-product policy. The model
receives stable collaboration instructions, current host policy, labelled context,
and the durable conversation. A longer instruction string alone does not provide
these boundaries.

## Local Codex comparison

The reference is the local Gulugulu Codex source tree. This comparison describes
observable client code, not undisclosed server prompts or guaranteed model behavior.
Model catalog values and enabled extensions can change Codex's final instructions.

| Concern | Reference implementation | Clara implementation |
| --- | --- | --- |
| Base instructions | `crates/prompts/src/model_instructions.rs` resolves model-catalog templates | Adapted from the local Codex base prompt; source revision and license are recorded in NOTICE |
| Host context | `crates/core/src/session/mod.rs`, `build_initial_context_with_world_state` groups developer and contextual-user fragments | A stable initial prefix and anchored incremental updates; developer sections combine into one text block |
| Provenance | `crates/context-fragments/src/fragment.rs` carries roles and classifications | `prompt.Fragment` carries kind, source, scope, lifecycle and role |
| Changing rules | `crates/core/src/context/world_state/agents_md.rs` emits replacement/removal state | Rules are checked every step, emitted initially, and updated only on additions, changes or removals |
| Environment and mode | World-state sections track environment and collaboration mode | Shared host preparation checks environment and mode; only changed fields produce updates |
| Skills | `crates/ext/skills/src/catalog_prompt.rs` renders discovery metadata | Catalog metadata is separate from skill bodies and referenced resources |
| History | `crates/core/src/context_manager/history.rs` normalizes tool pairs, modalities and window usage | Durable messages retain submission metadata; request projection filters unsupported media; compaction validates tool pairs |
| Compaction | Compact prompt, window state and history checkpoints cooperate | Automatic and explicit compaction use the same safe replacement path |

Clara keeps an independent, persisted model-window journal. It retains the initial
prefix in place and appends only changes at transcript boundaries. Fragment kind,
source, scope and lifecycle remain management metadata: the model sees readable
rule headings, policy blocks and environment tags, not management JSON envelopes.
Clara does not advertise unavailable memory, plugin, browser or deferred-tool systems.

## Request composition

1. Stable base instructions describe collaboration, persistence, progress and
   final reporting, default emoji behavior, and how to treat external content.
2. Developer policy describes the actual tools, execution mode, path semantics,
   command defaults, permission boundary and delegation rules.
   Sections retain independent snapshot identities internally but render as one
   text block separated by blank lines. Subsequent diffs emit only changed sections.
3. Initial contextual-user content contains complete applicable AGENTS rules,
   the initial environment and skill catalog. Child sessions identify their task.
4. Durable conversation stays in its original order. Context changes are anchored
   before new user input or after completed tool interactions. An unchanged AGENTS
   block remains once near the front of every complete request; it is never moved
   next to the latest user message. Empty plans, goals and terminal lists are omitted.
5. Later updates replace or withdraw earlier versions explicitly. Environment
   diffs include only changed fields: a new local date, time zone, working directory
   or access boundary. Passing seconds and tool continuations do not produce a
   clock update. There is no per-message sending-time context.

The initial environment contains `cwd`, a shell name such as `bash`, `current_date`,
`timezone`, and `filesystem` with workspace roots and the actual access boundary.
Workspace roots describe the task, not a sandbox: Agent uses `unrestricted` native
paths under current-user OS permissions, while Plan/Review use `read_only` tool
execution. Model identity, image support and token windows remain in model routing,
capability enforcement and budget management, not environment prose. Precise
`timestamp` and `local_datetime` fields are not injected. Date guidance in the
public base template explicitly forbids inferring a precise clock or sending time
from a date. Other hosts can supply this same structure through
`prompt.Environment.Fragments()` without importing Clara application packages.

The client still sends the complete request history with `store=false`; stable
placement allows a provider to reuse its prefix cache. It does not guarantee a
particular hit rate or eliminate required rules from the request.

Fragments are request-only. Their metadata describes provenance and validity,
not extra instruction authority. AGENTS rules remain contextual user instructions;
file contents, attachments and tool results do not become developer policy.
The host enforces Plan/Review permissions independently of model compliance.

Responses uses the top-level `instructions` field for the base prompt. The leading
system message is removed from `input`, while developer policy remains in `input`.
Clara enables this with `provider.WithSystemInstructions(true)`; the provider's
default behavior for other hosts is unchanged. Explicit
`ProviderOptions.Instructions` wins over mapped leading system text. Completions
retains its system message and the same developer/context order. Unknown roles
and non-text developer content fail before dispatch. Local metadata stays off wire.

Templates receive structured path and command parameters; their text is not
repaired with substring replacements. `prompt.System`, `prompt.Default` and
`prompt.Context.Message` remain available for existing hosts. New hosts should use
`Policy`, `Fragment`, `LoadWindow` and `Window.Prepare`, or the layered `Prepare`
helper. `Assemble` remains a stateless compatibility helper.

## Context checkpoints

`core.State.ContextState` is optional host-owned JSON, separate from `Messages`.
`PrepareStepFunctionOptions.ContextState` supplies it to the host, and a non-nil
`PrepareStepResult.ContextState` checkpoints its replacement before model dispatch.
JSON null resets it. `AgentStreamCall.OnContextState` observes the checkpoint;
`Session.ContextState()` returns a copy. Failed persistence prevents dispatch and
retains the prior window. JSON is bounded by `core.MaxContextStateBytes` (8 MiB).

A window records a version, transcript fingerprint, fragment snapshot and insertion
anchors. Its journal is bounded to 8192 entries and the Core JSON limit. Invalid
versions or anchors fail explicitly. Transcript replacement or branching starts a
fresh prefix; valid restored sessions retain their old prefix and append current
changes. When restoring older Clara windows, the host removes obsolete environment
fields (`project_root`, `model`, `image_input`, `effective_context_window`,
`local_datetime`, `timestamp`) from a copied snapshot and journal. Empty entries
are dropped; mixed entries retain their other changes and original anchors. No
withdrawal messages are emitted for removed fields. The cleaned window is saved
with the normal atomic checkpoint, without rewriting raw conversations or replay.
Old sessions without the optional field rebuild a prefix. Live terminal,
goal and task state is rechecked on recovery; historical process IDs grant no live
capability. None of this journal is included in terminal replay or editable prompts.

## Window budgets and compaction

Models with a declared window use their effective window, less configured maximum
output tokens. Without an output limit, Clara reserves 4096 tokens or one quarter
of a smaller window. Explicit `context_bytes` adds a second ceiling; zero disables
that byte ceiling. An undeclared window retains the default 512 KiB byte fallback.

`prompt.Measure` reports estimates separately from provider usage. It includes
policy, current context, history and tool schemas, using a conservative serialized
text heuristic of roughly one token per two bytes. Each image has an 8192-token
allowance independent of base64 size. These are heuristics, not exact tokenizer or
provider image accounting. Text-only models budget their omission projection.
Recent provider totals calibrate growth within a run; restored usage calibrates
the previous completed transcript when a new run begins. UI token totals continue
to represent measured usage, not these estimates.

`Compactor` accepts token/byte ceilings, an exact request `Assemble` callback, optional modality
projection, and an explicit force flag. Summarization replaces complete old turns
and retains the latest user turn, steering, tool interactions and original media.
Only the summarized prefix projects attachments to descriptions. Parallel calls
must all have matching results before that prefix can be removed. An unresolved
call or orphan result prevents unsafe replacement.

`Compactor.Transcript` restores context anchored within the summarized prefix as
historical task data. This lets the summary resolve older dates and scoped rules
without injecting today's environment into earlier turns. Management metadata is
removed from the serialized summary transcript; current policy is installed again
after compaction and retains its own authority.

Summaries are user-level handoffs, not policy. They retain objectives, constraints,
verified progress, failures, partial effects and remaining work. Empty, interrupted,
truncated or over-budget summaries do not replace history. Explicit compaction
requires an older complete turn; a single recent turn is preserved intact.

New optional `Message.Metadata` fields identify user submissions, steering,
follow-ups and handoffs. Legacy user messages remain conservative submission
boundaries; legacy handoff prefixes are recognized. No existing session is rewritten
on load. Original replay records, attachments and pre-submission checkpoints stay
available for resume and branching after compaction. A successful compaction
installs a fresh initial context alongside the replacement history; failure keeps
the old window. Its budget includes the base instructions, journal, tools and media.

## Verification

Tests use deterministic models and local Responses/Completions endpoints. They
cover role conversion, request ordering, rule replacement and removal, lazy skill
loading, child/Plan/Review/ACP dates and permissions, real tool schema errors and corrected calls,
media projections, compaction rollback, parallel tool pairs, and old-session forks.
The existing PTY/tmux suite verifies that these changes preserve inline rendering.
`make check` also runs race tests, vet, lint and builds without external test suites.
