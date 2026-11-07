# File search

Use glob to find filenames and grep to find literal text. Use the current tool schemas, not argument names from another agent. Paths are logical workspace paths. A search root limits traversal. Patterns without / match filenames at any depth; relative patterns with / start at the search root; a leading / starts at the workspace root. A whole ** component matches zero or more directories.

Examples:
- glob: {"path":"/","pattern":"**/*.go"}
- grep: {"path":"/internal","query":"cursorPosition","include":["*.go"]}

Read search warnings. An incomplete search does not prove that a file or text is absent. If arguments fail validation, correct the named fields using the schema. Do not repeat the same invalid request. Distinguish a tool failure from an empty result and from a command that ran but exited unsuccessfully.
