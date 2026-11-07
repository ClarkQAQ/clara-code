# Clara Code Development Guide

## Project

Clara Code combines a terminal coding agent and reusable Go agent packages in one
Go 1.27 module, `clara-code`. Build and run from the repository root. The adjacent
historical `clara-core` checkout is not a dependency.

## Architecture

- Root `main.go` owns signals, build-version injection, and process exit.
  `internal/cli` owns argument parsing, startup, diagnostics, and CLI integration tests.
- `internal/coding` owns Clara's agent assembly, native-path and Git policies,
  instruction and skill discovery, interactive questions, and tool presentation.
  Terminal UI and application-specific persistence belong to their respective
  `internal` packages; application configuration belongs to `internal/setting`.
- `pkg/core` owns the agent loop, extension points, sessions, events, and ACP
  client/server adapters. Keep filesystem implementations, concrete tools, and
  model providers out of this package. `pkg/core/prompt` provides optional prompt
  composition and embedded templates.
- `pkg/model` owns model contracts, providers, and relay protocols.
  `pkg/tool` owns reusable tools, execution backends, filesystem and Hashline
  capabilities, MCP adapters, plans, goals, and task orchestration.
- `pkg/util/errs`, `pkg/util/id`, and `pkg/util/localtime` are independent foundational packages.
  Do not turn `util` into a general-purpose collection of unrelated helpers.
- Public packages must not import application packages under root `internal`.
  Model contracts and utilities do not depend on Core or application policy.
  Tools may depend on Core, models, and utilities. Package-local implementation
  directories such as `pkg/model/relay/internal` remain private to their subtree.
- Put reusable mechanisms in `pkg`; put Clara-specific policy and interaction in
  `internal`. Examples belong in `examples`, including example-only shared helpers.

## Implementation

- Preserve synchronous and streaming loop extension points. Use contexts for
  cancellation, checkpoint results before continuing, and surface errors.
- Bound external input, output, traversal, process caches, and replay queues.
  Keep tool definitions deterministic and enforce authorization at execution.
- Use the filesystem interfaces for mutations. Require a current anchored read
  before editing existing text, validate multi-file changes before applying them,
  and report partial writes.
- Keep current workspace and clock state in dynamic model context, outside the
  stable system prefix and persisted conversation. Embed static prompt resources.
- Preserve configuration, environment variables, protocol wire formats, session
  identifiers, and saved sessions unless a migration is explicitly requested.
  UI date-times use local `time.DateTime`; structured timestamps use RFC3339 with
  explicit offsets or Z and retain precision. Preserve monotonic duration timing.
- The terminal uses the main screen and native scrollback. Preserve the real
  input cursor, IME positioning, resize reflow, and transient-region boundaries.
- Write clean, minimal code with explicit names. Keep unrelated changes out of
  the task and preserve existing working-tree edits. Do not create Git commits
  without an explicit request.
- When changing public APIs or tool contracts, update their documentation,
  examples, and relevant regression tests together. Consumers use `pkg` imports;
  there is no compatibility module for old `clara-core` import paths.

## Verification

Run `make check`: formatting, tests, race tests, vet, lint, CLI build, and example
builds. Use `make fmt` only when intentionally formatting changes.

For protocol changes, extend the repository's protocol regression tests using
deterministic models and local endpoints. Keep verification self-contained;
do not add external test-suite downloads or runners.

For terminal changes, run the existing PTY/tmux tests. For platform-sensitive
changes, build macOS and Linux on amd64 and arm64. For public-package changes,
verify imports from an independent temporary Go module using a local replace.

CLI smoke checks: `go run . --help`, `go run . --version`, and
`go run . --demo`. Release versions can be supplied with `-ldflags '-X main.version=VERSION'`.
