-- name: GetSessionProject :one
SELECT project FROM sessions WHERE id = ?;

-- name: GetSession :one
SELECT metadata, checkpoint FROM sessions WHERE id = ?;

-- name: InsertSession :execrows
INSERT INTO sessions (id, project, cwd, updated_key, metadata, checkpoint)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO NOTHING;

-- name: UpdateSession :execrows
UPDATE sessions
SET project = ?, cwd = ?, updated_key = ?, metadata = ?, checkpoint = ?
WHERE id = ?;

-- name: ListProjectSessions :many
SELECT metadata FROM sessions WHERE project = ?
ORDER BY updated_key DESC, id
LIMIT sqlc.arg(page_limit);

-- name: ListSessionsPage :many
SELECT metadata FROM sessions
WHERE cwd GLOB '/*'
  AND updated_key <= sqlc.arg(after_updated)
  AND (updated_key < sqlc.arg(after_updated) OR id > sqlc.arg(after_id))
ORDER BY updated_key DESC, id
LIMIT sqlc.arg(page_limit);

-- name: ListCwdSessionsPage :many
SELECT metadata FROM sessions
WHERE cwd = sqlc.arg(cwd)
  AND updated_key <= sqlc.arg(after_updated)
  AND (updated_key < sqlc.arg(after_updated) OR id > sqlc.arg(after_id))
ORDER BY updated_key DESC, id
LIMIT sqlc.arg(page_limit);

-- name: InsertTurn :exec
INSERT INTO turns (id, before_messages) VALUES (?, ?);

-- name: UpdateTranscript :execrows
UPDATE turns SET transcript = ? WHERE id = ?;

-- name: GetTranscript :one
SELECT transcript FROM turns WHERE id = ?;

-- name: GetBeforeMessages :one
SELECT before_messages FROM turns WHERE id = ?;

-- name: AppendSessionTurn :exec
INSERT INTO session_turns (session_id, position, turn_id) VALUES (?, ?, ?);

-- name: ListSessionTurns :many
SELECT turn_id FROM session_turns WHERE session_id = ? ORDER BY position;
