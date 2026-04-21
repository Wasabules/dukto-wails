// Package audit is a tiny append-only structured logger for security-relevant
// events: session accepts/rejects, policy hits, block-list matches. One line
// per entry, JSON-encoded so the UI can parse without understanding the wire
// format. Rotates at 10 MB by renaming audit.log → audit.log.1 and truncating
// the live file. Best-effort — disk errors never block a transfer.
package audit

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Entry is one audit line. All fields are optional; Time and Kind are the
// only mandatory ones. Reason is a short machine-readable tag
// ("whitelist.deny", "blocklist.hit", "ratelimit.tcp", …).
type Entry struct {
	Time    time.Time `json:"time"`
	Kind    string    `json:"kind"`
	Reason  string    `json:"reason,omitempty"`
	Remote  string    `json:"remote,omitempty"`
	Peer    string    `json:"peer,omitempty"`
	Detail  string    `json:"detail,omitempty"`
}

const (
	// maxBytes is the rotation threshold. Kept small: audit is secondary
	// data and a user who doesn't care shouldn't pay much disk for it.
	maxBytes = 10 * 1024 * 1024
	// maxEntries is the cap for Read's returned slice so huge logs don't
	// blow up a Wails RPC round-trip.
	maxEntries = 2000
)

// Log is a concurrency-safe audit sink. Zero value is unusable; call Open.
type Log struct {
	path string

	mu sync.Mutex
	f  *os.File
}

// Open returns a Log rooted at path. The file is created lazily on first
// Append, so a user who never triggers an audit event leaves no artifact.
func Open(path string) *Log {
	return &Log{path: path}
}

// Append writes one entry. Errors are swallowed intentionally — audit is
// best-effort and must not cascade into transfer failures. Use AppendErr
// when the caller wants to surface the error (mostly tests).
func (l *Log) Append(e Entry) {
	_ = l.AppendErr(e)
}

// AppendErr is Append but returns the underlying IO error for tests.
func (l *Log) AppendErr(e Entry) error {
	if l == nil || l.path == "" {
		return errors.New("audit: log not configured")
	}
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.ensureOpen(); err != nil {
		return err
	}
	if _, err := l.f.Write(data); err != nil {
		return err
	}
	return l.maybeRotate()
}

// Read returns the most recent entries, newest last, up to maxEntries. Lines
// that fail to parse are silently skipped so a corrupted tail doesn't hide
// valid earlier entries.
func (l *Log) Read() ([]Entry, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	f, err := os.Open(l.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	out := make([]Entry, 0, 128)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		var e Entry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			continue
		}
		out = append(out, e)
	}
	if err := sc.Err(); err != nil {
		return out, err
	}
	if len(out) > maxEntries {
		out = out[len(out)-maxEntries:]
	}
	return out, nil
}

// Clear truncates the live log and removes the rotated sibling, if any.
func (l *Log) Clear() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f != nil {
		_ = l.f.Close()
		l.f = nil
	}
	if err := os.Remove(l.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := os.Remove(l.path + ".1"); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// Close releases the file handle. Safe to call more than once.
func (l *Log) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		return nil
	}
	err := l.f.Close()
	l.f = nil
	return err
}

func (l *Log) ensureOpen() error {
	if l.f != nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return fmt.Errorf("audit: mkdir: %w", err)
	}
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("audit: open %q: %w", l.path, err)
	}
	l.f = f
	return nil
}

func (l *Log) maybeRotate() error {
	info, err := l.f.Stat()
	if err != nil || info.Size() < maxBytes {
		return nil
	}
	_ = l.f.Close()
	l.f = nil
	_ = os.Remove(l.path + ".1")
	if err := os.Rename(l.path, l.path+".1"); err != nil {
		return err
	}
	return nil
}
