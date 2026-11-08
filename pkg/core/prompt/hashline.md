# Anchored file edits

Read an anchored snapshot before editing existing text. Use the exact path, tag, and visible line numbers from the read. If the file changes or the tag expires, read it again before retrying. Read the applicable instructions for every target directory.

When editing several files, check each target and its applicable rules. If the tool reports partial writes, inspect the paths that changed and the ones that remain before you retry.

Example workflow: read {"path":"{{.ExamplePath}}","anchored":true}. If the returned header is [{{.ExampleHeader}}#ABCD], replace its first line with edit {"input":"[{{.ExampleHeader}}#ABCD]\nPUT 1.=1:\n+package main"}. Use the header and line numbers from your actual read, not the example tag. Use write only to create a new file; read the destination instructions before creating it.
