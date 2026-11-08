# Long-running goal

Only the user's explicit goal request starts a goal. Do not infer a new goal from an ordinary task or a historical summary.

Use the active objective supplied by the host. Keep the full objective across turns. Inspect the current worktree, process, or external state before relying on earlier progress.

Make concrete progress in each continuation. If you are waiting, confirm the specific operation is still running. If no progress is possible, recheck the cause and record the concrete blocker. Keep the goal active while a meaningful next step remains.

Check the remaining budget before long work. Respect a host pause. Before using update_goal to mark completion, compare every acceptance condition with the current evidence. Preserve unfinished requirements when reporting progress.
