package session

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"clara-code/internal/filelock"
	"clara-code/internal/session/internal/sessiondb"
	"clara-code/pkg/util/errs"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

//go:generate go tool sqlc generate

//go:embed migrations/*.sql
var migrations embed.FS

const (
	databaseName = "sessions.sqlite3"
	maxMetadata  = 64 << 10
	maxTurn      = 10 << 20
	updatedKey   = "2006-01-02T15:04:05.000000000Z"
)

func openDatabase(ctx context.Context, home string, create bool) (*sql.DB, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, err := filepath.Abs(filepath.Join(home, databaseName))
	if err != nil {
		return nil, err
	}
	if create {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, err
		}
	} else if _, err := os.Lstat(path); err != nil {
		return nil, err
	}

	lock, err := lockMigrations(ctx, path+".lock")
	if err != nil {
		return nil, err
	}
	defer lock.Close()

	info, err := os.Lstat(path)
	switch {
	case err == nil:
		if !info.Mode().IsRegular() {
			return nil, errors.New("session database is not a regular file")
		}
	case create && errors.Is(err, os.ErrNotExist):
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return nil, err
		}
		if err := file.Close(); err != nil {
			return nil, err
		}
	default:
		return nil, err
	}

	params := url.Values{
		"mode":    {"rw"},
		"_pragma": {"busy_timeout(5000)", "foreign_keys(ON)", "synchronous(FULL)"},
		"_txlock": {"immediate"},
	}
	uri := databaseURI(path, params)
	db, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := migrate(ctx, db); err != nil {
		return nil, errors.Join(fmt.Errorf("migrate session database: %w", err), db.Close())
	}
	return db, nil
}

func lockMigrations(ctx context.Context, path string) (*os.File, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	lock, err := filelock.Acquire(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("lock session migrations: %w", err)
	}
	return lock, nil
}

func migrate(ctx context.Context, db *sql.DB) error {
	var journal string
	if err := db.QueryRowContext(ctx, "PRAGMA journal_mode = WAL").Scan(&journal); err != nil {
		return err
	}
	if journal != "wal" {
		return fmt.Errorf("expected WAL journal mode, got %q", journal)
	}
	files, err := fs.Sub(migrations, "migrations")
	if err != nil {
		return err
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, db, files, goose.WithDisableGlobalRegistry(true))
	if err != nil {
		return err
	}
	current, target, err := provider.GetVersions(ctx)
	if err != nil {
		return err
	}
	if current > target {
		return fmt.Errorf("schema version %d exceeds supported version %d", current, target)
	}
	_, err = provider.Up(ctx)
	return err
}

func transact(ctx context.Context, db *sql.DB, write func(*sessiondb.Queries) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := write(sessiondb.New(tx)); err != nil {
		return err
	}
	return tx.Commit()
}

func encodePayload(home string, value any, limit int) ([]byte, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	body, err = mediaJSON(home, body, false)
	if err != nil {
		return nil, err
	}
	if len(body) > limit {
		return nil, errors.New("session data exceeds the size limit")
	}
	return body, nil
}

func decodePayload(home string, body []byte, limit int, dst any) error {
	if len(body) > limit {
		return errors.New("session data exceeds the size limit")
	}
	body, err := mediaJSON(home, body, true)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, dst)
}

func decodeMeta(body []byte) (Meta, error) {
	var meta Meta
	if len(body) > maxMetadata {
		return meta, errors.New("session metadata exceeds the size limit")
	}
	err := json.Unmarshal(body, &meta)
	return meta, err
}

func List(home, project string) ([]Meta, error) {
	ctx := context.Background()
	db, err := openDatabase(ctx, home, false)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer db.Close()
	const maxProjectSessions = 64 << 10
	rows, err := sessiondb.New(db).ListProjectSessions(ctx, sessiondb.ListProjectSessionsParams{
		Project: project, PageLimit: maxProjectSessions + 1,
	})
	if err != nil {
		return nil, err
	}
	if len(rows) > maxProjectSessions {
		return nil, errs.New(errs.CodeLimit, "project exceeds the session listing limit")
	}
	return decodeListedMeta(rows)
}

const SessionPageSize = 100

// ListPage returns one page of metadata plus a lookahead.
func ListPage(ctx context.Context, home, cwd string, after *Meta) ([]Meta, error) {
	db, err := openDatabase(ctx, home, false)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer db.Close()

	key, id := "~", ""
	if after != nil {
		key, id = after.Updated.UTC().Format(updatedKey), after.ID
	}
	queries := sessiondb.New(db)
	var rows [][]byte
	switch {
	case cwd == "":
		rows, err = queries.ListSessionsPage(ctx, sessiondb.ListSessionsPageParams{AfterUpdated: key, AfterID: id, PageLimit: SessionPageSize + 1})
	case filepath.IsAbs(cwd):
		rows, err = queries.ListCwdSessionsPage(ctx, sessiondb.ListCwdSessionsPageParams{Cwd: cwd, AfterUpdated: key, AfterID: id, PageLimit: SessionPageSize + 1})
	}
	if err != nil {
		return nil, err
	}
	return decodeListedMeta(rows)
}

func decodeListedMeta(rows [][]byte) ([]Meta, error) {
	items := make([]Meta, 0, len(rows))
	for _, row := range rows {
		meta, err := decodeMeta(row)
		if err != nil {
			return nil, err
		}
		items = append(items, meta)
	}
	return items, nil
}
