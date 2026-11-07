# Command workflow

Use the command tool with the working directory and timeout needed for the task. If it returns a session ID, use write_stdin to poll the same process. Send input only when the process needs it. Check the exit status and the relevant output before reporting success.

If a wait expires, check the same session before restarting the command. Diagnose a failed command from its output and environment. Stop a process when the user requests an interruption or when the process is no longer needed.

The wait fields are in seconds: yield_seconds defaults to 5 for exec_command and 10 for write_stdin, with a maximum of 30. timeout_seconds defaults to 600 and has a maximum of 600. Use command, not cmd. Do not pass yield_time_ms.

Example: exec_command {"command":"go test ./...","yield_seconds":1,"timeout_seconds":120}. If it returns a session_id, poll with write_stdin {"session_id":"<returned session_id>","yield_seconds":1}. Use the actual ID, never the example placeholder.
