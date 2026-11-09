# Anchored file edits

Before editing existing text, call read with `anchored: true`. Use `offset` (first line, 1-based) and `limit` (line count) to show the whole target range. Copy the first line of the read, including both brackets and the tag, into edit's `input` string. Use the visible line numbers without the `:` from the read. All operations in a patch refer to the original snapshot's line numbers.

For example, call read with `{"path":"{{.ExamplePath}}","anchored":true}`. If the first line it returns is `[{{.ExampleHeader}}#ABCD]`:

- Replace line 1: `{"input":"[{{.ExampleHeader}}#ABCD]\nPUT 1.=1:\n+package main"}`
- Delete line 2: `{"input":"[{{.ExampleHeader}}#ABCD]\nCUT 2.=2"}`

The range separator is `.=`, and both ends are included. Repeat the line number for a single line. A literal PUT ends with `:` and takes one `+` row per new line, with the new text immediately after the `+`. A lone `+` means a blank line. A CUT has no trailing colon or body.

Use the header and line numbers from your actual read. If a snapshot is unknown or stale, or the target lines were not visible, call read again with `anchored: true` and the needed range, then rebuild the edit from that result. Read again after a successful edit before making another edit to the same file: the edit result's header does not show any anchored lines.

Read the applicable instructions for every target directory. When editing several files, use a separate header for each file. If the tool reports partial writes, inspect the paths that changed and the ones that remain before retrying. Use write only to create a new file.
