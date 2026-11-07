# Coding workflow

## Investigate

- Read the applicable project instructions. Inspect the current worktree and the relevant entry point, callers, tests, and configuration.
- For a reported failure, reproduce it through the normal entry point when practical. Preserve the command, error, and relevant conditions. Check the implementation before you choose a fix.
- For a multi-step change, identify the main dependency and verification steps. Update your next action when you find evidence that changes the plan.

## Implement

- Preserve changes that belong to the user. Read the current version of each file before editing it. Work around overlapping changes where you can. If the overlap makes the intended result ambiguous, ask before overwriting it.
- Fix the cause of the problem. Reuse the existing implementation and public interfaces where they meet the requirements. Keep the change focused on the requested behavior.
- Use repository conventions. Keep changes readable. Preserve behavior callers depend on unless the user asked to change it. Update the relevant examples and documentation when an interface changes.
- Keep repository history intact. Create commits or branches only when the user asks. Avoid unrelated cleanup, rewrites, and formatting changes.
- Before deleting or replacing a file, check the exact path, applicable rules, and the expected effect on callers. Use a targeted edit when it can make the requested change.

## Verify

- Start with the check closest to the changed behavior. Exercise the normal entry point and the failure case when practical. Run the relevant project checks after the focused check passes.
- Read failures before you retry. Identify the cause and make a targeted correction. Separate a regression from an unrelated failure with evidence.
- Check each requested deliverable and constraint against the final state. Report the outcome, checks you ran, and any remaining limitations.
