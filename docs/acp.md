# ACP integration

## Run Clara Code in an editor

In an ACP-compatible editor, configure `clara-code acp` as the stdio agent command. It supports session creation, discovery, loading, and resuming, along with prompting, streaming updates, and cancellation.

`session/list` lists persisted sessions in the Clara data directory, optionally filtered to an exact working directory. Pass the returned `nextCursor` to fetch the next page. `session/load` replays history. `session/resume` restores the original session in its working directory without replaying it.

Clara exposes Agent, Plan, Git review, model selection, and reasoning effort through ACP session configuration options. Clients that advertise form elicitation can answer `request_user_input` over the same connection. Local clients may opt in to the versioned `dev.claracode/extensions` capability for rich transcript pages, input steering, local setup, attachments, and diff pages. See [pkg/appwire](../pkg/appwire) for the Go request and response types.

## Use an external ACP agent

Configure an external ACP role using the [configuration example](../config.example.toml). Clara can call `list_acp_sessions` with that role to discover sessions in the current working directory. Pass a returned `sessionId` as `resume_session_id` to `spawn_agent` or `subagent_task` with the same role. The tool sends the task brief to that session and returns a Clara task ID for follow-ups. Omitting `resume_session_id` starts a new session. Discovery and resuming require the corresponding capabilities from the external agent. See [Delegated tasks](usage.md#delegated-tasks) for task controls.

## Integrate from Go

Go hosts can opt into session discovery and resuming through the `core.ACPServerConfig.List` and `Resume` callbacks. `core.ACPExternal.ListSessions` and `ResumeSession` check the negotiated capabilities before sending requests. See [Public Go Packages](core.md) for general host assembly.
