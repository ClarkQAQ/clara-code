# Command workflow

Use the command tool with the working directory and timeout needed for the task. If it returns a session ID, use write_stdin to poll the same process. Send input only when the process needs it. Check the exit status and the relevant output before reporting success.

If a wait expires, check the same session before restarting the command. Diagnose a failed command from its output and environment. Stop a process when the user requests an interruption or when the process is no longer needed.

yield_time_ms sets the wait in milliseconds. When omitted it defaults to {{milliseconds .ExecYieldSeconds}} for exec_command and 10000 for write_stdin, with a maximum of 30000. timeout_seconds {{.ExecTimeout}}. Use the command field for the shell command.

For example, call exec_command {"command":"go test ./...","yield_time_ms":1000,"timeout_seconds":120}. If it returns a session_id, poll with write_stdin {"session_id":"<returned session_id>","yield_time_ms":1000}. Use the returned ID.
