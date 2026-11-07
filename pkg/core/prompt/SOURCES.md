# Prompt sources

| Fragment | Source | Adaptation |
|---|---|---|
| Base, Coding, and Skills | Codex `75bf850427`, `crates/models-manager/prompt.md` | Keep steering, authorization, progress updates, discovery, scoped edits, skills, and verification. Use the host's available capabilities. |
| Plan Mode | Codex `crates/collaboration-mode-templates/templates/plan.md` | Keep exploration, intent, design, and handoff as separate phases. Use host mode selection. Load the structured question fragment only when the tool is registered. |
| Checklist | Codex plan template and Clara `3eaf583`, `internal/agent/prompts/goal.md` | Keep progress tracking independent of Plan Mode. |
| Goal | Codex `crates/ext/goal/templates/goals/continuation.md` and Clara `internal/agent/prompts/goal.md` | Keep the objective across turns. Report actual progress and check each acceptance condition. |
| Delegation | Codex collaboration tooling and Clara `internal/agent/prompts/main.md` | Give tasks scoped briefs. Use the host's task results, follow-up and lifecycle controls. |
| Reviewer | Codex `crates/prompts/templates/review/rubric.md` | Keep actionable defects, evidence, severity, and workspace-rule attribution. Leave transport formatting to the host. |
| Compaction | Codex `crates/prompts/templates/compact/prompt.md` and Clara `internal/agent/prompts/compress.md` | Keep the verified state and next steps. Compact only complete transcript prefixes. |
| Workspace, Hashline, and Commands | Codex `crates/models-manager/prompt.md` and Clara `internal/agent/prompts/main.md` | Use scoped instructions, anchored edits, and process-session polling that match Clara Core's tools. |
| Dynamic context | Clara `internal/agent/prompt.go` | Render current workspace rules, plan, goal, and child status after the history. Refresh rules from the workspace at each step. |

Model-facing text uses short active sentences, consistent terms and direct instructions. The writing method follows the [ASD-STE100 skill](https://github.com/danyuchn/asd-ste100-skill/blob/master/SKILL.md). Host code can replace fragments with `prompt.Config.Overrides`.
