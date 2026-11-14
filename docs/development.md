# Developing Clara Code

## Repository layout

Root `main.go` provides the CLI entry point. Startup, terminal UI, configuration, session storage, and other application features live in `internal`. Reusable mechanisms live in `pkg/core`, `pkg/model`, `pkg/tool`, and `pkg/util`. Public packages are independent of Clara's application layer. See [Public Go Packages](core.md) for assembly details and [examples](../examples) for runnable programs.

The CLI and public packages share the `clara-code` Go module. Build from the repository root with Go 1.27. Framework examples continue to use their `CLARA_CORE_*` settings. The [development guide](../AGENTS.md) defines package placement and verification requirements. See [LICENSE](../LICENSE) and [NOTICE](../NOTICE) for the framework's license, attribution, and prompt provenance.

Project graphics include the [mascot icon](images/clara-code-icon.png) and [wordmark](images/clara-code-wordmark.png).

## Build and verify

`make check` runs formatting checks, tests, race tests, vet, lint, and CLI and example builds. `make build` produces `clara-code` at the repository root. To set the reported version, run `go build -ldflags '-X main.version=VERSION' .`.

Terminal regression tests cover inline image markers, atomic editing, asynchronous insertion anchors, deletion, resize, cursor placement, and native scrollback. Clipboard tests use mocked platform adapters and a private named macOS pasteboard. Verify OS IME and clipboard behavior manually in VS Code and Linux desktop sessions.

## Session SQL

Session SQL lives in `internal/session`. After adding a numbered Goose migration or changing a query, run `go generate ./...` and include the generated sqlc code with the SQL changes. The module pins sqlc and golangci-lint for use through `go tool`.

## GitHub releases

Keep the full history on the default branch, `master`. Each push to that branch runs **Publish GitHub Release**. To publish the current tip manually, open **Actions → Publish GitHub Release → Run workflow** and select `master`. Use **Re-run jobs** on the original run to retry the same commit. The tag and binary version use the Forgejo format `r<commit count>.g<12-character commit hash>`.

See the [GitHub release workflow](../.github/workflows/release.yml) and the [Forgejo publishing guide](../.forgejo/README.md) for the two pipelines.
