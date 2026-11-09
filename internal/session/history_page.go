package session

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"clara-code/internal/session/internal/sessiondb"
	"clara-code/pkg/util/errs"
)

// HistoryPage reads the visible transcript without taking a session writer lock
// or restoring the model checkpoint and its media.
func HistoryPage(ctx context.Context, home, id string, before int) (Meta, []Turn, int, error) {
	var meta Meta
	if !ValidID(id) || before < -1 {
		return meta, nil, 0, errs.ErrNotFound
	}
	db, err := openDatabase(ctx, home, false)
	if err != nil {
		return meta, nil, 0, err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return meta, nil, 0, err
	}
	defer tx.Rollback()
	q := sessiondb.New(tx)
	var metadata []byte
	if err := tx.QueryRowContext(ctx, "SELECT metadata FROM sessions WHERE id = ?", id).Scan(&metadata); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return meta, nil, 0, errs.ErrNotFound
		}
		return meta, nil, 0, err
	}
	meta, err = decodeMeta(metadata)
	if err != nil {
		return meta, nil, 0, err
	}
	var count int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM session_turns WHERE session_id = ?", id).Scan(&count); err != nil {
		return meta, nil, 0, err
	}
	if before == -1 || before > count {
		before = count
	}
	const pageTurns = 20
	const pageBytes = 2 << 20
	rows, err := tx.QueryContext(ctx, `SELECT turn_id FROM session_turns
		WHERE session_id = ? ORDER BY position DESC LIMIT ? OFFSET ?`, id, pageTurns, count-before)
	if err != nil {
		return meta, nil, 0, err
	}
	var ids []string
	for rows.Next() {
		var runID string
		if err := rows.Scan(&runID); err != nil {
			rows.Close()
			return meta, nil, 0, err
		}
		ids = append(ids, runID)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return meta, nil, 0, err
	}
	var turns []Turn
	total := 0
	for _, runID := range ids {
		body, err := q.GetTranscript(ctx, runID)
		if err != nil {
			return meta, nil, 0, err
		}
		if len(turns) > 0 && total+len(body) > pageBytes {
			break
		}
		var turn Turn
		if err := json.Unmarshal(body, &turn); err != nil {
			return meta, nil, 0, err
		}
		turns = append(turns, turn)
		total += len(body)
	}
	return meta, turns, before - len(turns), nil
}
