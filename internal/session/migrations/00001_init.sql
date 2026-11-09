-- +goose Up
CREATE TABLE sessions (
    id TEXT NOT NULL PRIMARY KEY,
    project TEXT NOT NULL,
    cwd TEXT NOT NULL,
    updated_key TEXT NOT NULL,
    metadata BLOB NOT NULL,
    checkpoint BLOB NOT NULL
);

CREATE INDEX sessions_updated ON sessions (updated_key DESC, id);
CREATE INDEX sessions_project_updated ON sessions (project, updated_key DESC, id);
CREATE INDEX sessions_cwd_updated ON sessions (cwd, updated_key DESC, id);

CREATE TABLE turns (
    id TEXT NOT NULL PRIMARY KEY,
    before_messages BLOB NOT NULL,
    transcript BLOB
);

CREATE TABLE session_turns (
    session_id TEXT NOT NULL REFERENCES sessions (id),
    position INTEGER NOT NULL CHECK (position >= 0),
    turn_id TEXT NOT NULL REFERENCES turns (id),
    PRIMARY KEY (session_id, position),
    UNIQUE (session_id, turn_id)
);

-- +goose Down
DROP TABLE session_turns;
DROP TABLE turns;
DROP TABLE sessions;
