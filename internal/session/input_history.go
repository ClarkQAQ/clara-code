package session

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"time"

	"golang.org/x/sys/unix"
)

var errInputHistoryMalformed = errors.New("history.jsonl contains malformed records; valid entries were retained")

const maxInputHistoryBytes = 4 << 20
const maxInputHistoryEntries = 1000

// InputHistoryEntry is local composer history, independent of model/session turns.
type InputHistoryEntry struct {
	SessionID   string       `json:"session_id"`
	Timestamp   time.Time    `json:"ts,omitzero"`
	Text        string       `json:"text"`
	Kind        string       `json:"kind,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"`
}

func ReadInputHistory(home string) ([]InputHistoryEntry, error) {
	file, err := os.OpenFile(filepath.Join(home, "history.jsonl"), os.O_RDONLY|unix.O_NONBLOCK, 0)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("history.jsonl must be a regular file")
	}
	offset := max(int64(0), info.Size()-maxInputHistoryBytes)
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}
	body, err := io.ReadAll(io.LimitReader(file, maxInputHistoryBytes))
	if err != nil {
		return nil, err
	}
	if offset > 0 {
		if at := bytes.IndexByte(body, '\n'); at >= 0 {
			body = body[at+1:]
		} else {
			body = nil
		}
	}
	var entries []InputHistoryEntry
	invalid := false
	for _, line := range bytes.Split(body, []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var entry InputHistoryEntry
		if len(line) > 2<<20 || json.Unmarshal(line, &entry) != nil || len(entry.Text) > 256<<10 || len(entry.Attachments) > 64 {
			invalid = true
			continue
		}
		if entry.Text != "" || len(entry.Attachments) > 0 {
			entries = append(entries, entry)
		}
	}
	if len(entries) > maxInputHistoryEntries {
		entries = entries[len(entries)-maxInputHistoryEntries:]
	}
	if invalid {
		return entries, errInputHistoryMalformed
	}
	return entries, nil
}

func AppendInputHistory(ctx context.Context, home string, entry InputHistoryEntry) error {
	if len(entry.Text) > 256<<10 || len(entry.Attachments) > 64 {
		return errors.New("input history entry exceeds its limit")
	}
	if entry.Text == "" && len(entry.Attachments) == 0 {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(home, 0700); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(home, "history.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
	entries, readErr := ReadInputHistory(home)
	// Recover malformed lines, but never replace a history file that cannot be read.
	if readErr != nil && !errors.Is(readErr, errInputHistoryMalformed) {
		return readErr
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}
	entries = append(entries, entry)
	if len(entries) > maxInputHistoryEntries {
		entries = entries[len(entries)-maxInputHistoryEntries:]
	}
	var lines [][]byte
	size := 0
	for i := len(entries) - 1; i >= 0; i-- {
		line, err := json.Marshal(entries[i])
		if err != nil {
			return err
		}
		if len(line) > 2<<20 {
			return errors.New("input history entry exceeds 2 MiB")
		}
		if size+len(line)+1 > maxInputHistoryBytes {
			break
		}
		lines = append(lines, line)
		size += len(line) + 1
	}
	slices.Reverse(lines)
	file, err := os.CreateTemp(home, ".history-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	for _, line := range lines {
		if _, err := file.Write(append(line, '\n')); err != nil {
			file.Close()
			return err
		}
	}
	if err := errors.Join(file.Sync(), file.Close()); err != nil {
		return err
	}
	return os.Rename(file.Name(), filepath.Join(home, "history.jsonl"))
}
