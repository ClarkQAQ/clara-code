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

1. The first `developer` message combines the stable base instructions with the
   initial host policy for tools, execution mode, path semantics, permissions and
   delegation. The base is rendered from its template. Policy sections retain
   independent snapshot identities and render as one text block separated by
   blank lines. Subsequent diffs emit only changed sections.
2. Initial contextual-user content contains complete applicable AGENTS rules,
   the initial environment and skill catalog. Child sessions identify their task.
3. Durable conversation stays in its original order. Context changes are anchored
   before new user input or after completed tool interactions. An unchanged AGENTS
   block remains once near the front of every complete request; it is never moved
   next to the latest user message. Empty plans, goals and terminal lists are omitted.
4. Later updates replace or withdraw earlier versions explicitly. Environment
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

Responses sends the first `developer` message in `input`, where the base and
initial policy can be reused by prefix caching. Completions uses the same order
in `messages`. The provider exposes `ProviderOptions.Instructions` and the
opt-in `provider.WithSystemInstructions(true)` mapping for callers that use the
Responses `instructions` field. Unknown roles and non-text developer content
fail before dispatch. Local metadata stays off wire.

Templates receive structured path and command parameters; their text is not
repaired with substring replacements. `prompt.System`, `prompt.Default` and
`prompt.Context.Message` remain available for existing hosts. New hosts can use
`Policy`, `Fragment`, `LoadWindow` and `Window.PrepareWithBase`, or the layered
`Prepare` helper. `Assemble` remains a stateless compatibility helper.

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
fresh prefix. Restored sessions retain the saved fragment anchors, and request
rendering attaches the base to the first developer message. When restoring older
Clara windows, the host removes obsolete environment
fields (`project_root`, `model`, `image_input`, `effective_context_window`,
`local_datetime`, `timestamp`) from a copied snapshot and journal. Empty entries
are dropped; mixed entries retain their other changes and original anchors. No
withdrawal messages are emitted for removed fields. The cleaned window is saved
with the normal atomic checkpoint, without rewriting raw conversations or replay.
Old sessions without the optional field rebuild a prefix. Live terminal,
goal and task state is rechecked on recovery; historical process IDs grant no live
capability. None of this journal is included in terminal replay or editable prompts.

## Window budgets and compaction

Clara checks the established history before adding a new submission and before a
model continuation. The trigger is the lower of the effective window and
`auto_compact_context_percent` of the raw window. The default and maximum
percentage is 90. A model without a declared window uses a 256K-token heuristic.
A final answer ends the turn even when the threshold was reached.

An upstream token total covers the response that reported it. Clara adds the cost
of local messages delivered after that response, including tool results and steering.
The estimate counts model-visible UTF-8 text at about four bytes per token and a
standard image at about 1,844 tokens. Text-only models use their text projection.
If no usage sample matches the history, Clara estimates the established window.
The status line shows the latest model usage or, just after compaction, an estimate
for the replacement window. Cumulative usage and cache statistics record provider
measurements.

Clara sends the history to the same model with its original message roles, adds a
final user instruction asking for a handoff, and omits tools. It inherits the model's
configured output limit. An unset limit lets the provider choose its default.
A provider-reported input overflow drops the oldest eligible history item from a
temporary copy and retries. Tool calls and results are removed together. A cancelled
request, a truncated answer or an empty summary leaves the original history intact.

A successful replacement ends the summarized history with a user-level handoff.
Up to 20,000 estimated tokens of recent user text are retained in chronological
order, subject to the host's window budget. Attachments belonging to the active
submission or its steering remain available. New submissions awaiting dispatch
follow the handoff. Earlier handoffs inform the next summary. Pending tool calls
delay automatic compaction until their results arrive.

The window and replacement history are checkpointed together. Restored sessions
can read older handoffs and rebuild missing estimates. Original terminal replay,
attachments and fork points remain available after compaction.

## Verification

Tests use deterministic models and local Responses/Completions endpoints. They
cover role conversion, request ordering, rule replacement and removal, lazy skill
loading, child/Plan/Review/ACP dates and permissions, real tool schema errors and corrected calls,
media projections, compaction rollback, parallel tool pairs, and old-session forks.
The existing PTY/tmux suite verifies that these changes preserve inline rendering.
`make check` also runs race tests, vet, lint and builds without external test suites.


## Explicit skill mentions and availability

The composer inserts `$skill-name` as a user mention, preserving surrounding text
and attachments. The public skills template tells the model to load that catalog
entry with `read_skill` before following its workflow. Selection does not preload
all skill bodies or promote their instructions to developer policy.

Clara's management UI stores disabled canonical skill-file paths in its data
directory. Discovery resolves project precedence before filtering disabled entries;
it never falls back to a shadowed definition because the winning one is disabled.
Each preparation refreshes availability for the catalog and `read_skill` /
`read_skill_resource` tools. A change produces the existing catalog replacement or
removal delta. Already-read conversation text remains historical content; toggles
are not a new filesystem permission boundary.
