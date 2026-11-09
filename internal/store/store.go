// Package store is an append-only event log plus a content-addressed blob
// directory.
//
// The log is one JSON object per line. Every event carries the hash of the
// event before it, so editing, deleting or reordering any line breaks the chain
// from that point on. Blobs are named by their SHA-256 and are read back
// through the same hash, so a swapped file is detected too.
//
// Several processes (the web server, the MCP server, the CLI) may share one
// data directory; appends are serialised through a lock file.
package store

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/111Reihan/kiro/internal/canon"
)

var (
	ErrNotFound = errors.New("blob not found")
	ErrRedacted = errors.New("blob was redacted")
	ErrCorrupt  = errors.New("blob does not match its hash")
)

type Event struct {
	Seq  int             `json:"seq"`
	TS   string          `json:"ts"`
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
	Prev string          `json:"prev"`
	Hash string          `json:"hash"`
}

const genesis = "0000000000000000000000000000000000000000000000000000000000000000"

func hashOf(prev string, seq int, ts, typ string, data []byte) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\n%d\n%s\n%s\n", prev, seq, ts, typ)
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

type Store struct {
	dir  string
	mu   sync.Mutex
	evs  []Event
	size int64 // bytes of the log already read
	last string
}

func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dir, "blobs"), 0o755); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, last: genesis}
	if err := s.repairTail(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s, s.refresh()
}

// Dir is the data directory.
func (s *Store) Dir() string { return s.dir }

func (s *Store) logPath() string { return filepath.Join(s.dir, "specter.log") }

// repairTail drops a half-written last line (a crash mid-append). Only the
// fragment after the final newline is removed; a complete line is never touched.
func (s *Store) repairTail() error {
	b, err := os.ReadFile(s.logPath())
	if errors.Is(err, os.ErrNotExist) || len(b) == 0 {
		return nil
	}
	if err != nil {
		return err
	}
	if b[len(b)-1] != '\n' {
		cut := bytes.LastIndexByte(b, '\n') + 1
		return os.Truncate(s.logPath(), int64(cut))
	}
	return nil
}

// refresh reads lines other processes appended since we last looked. s.mu held.
func (s *Store) refresh() error {
	f, err := os.Open(s.logPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	if st.Size() == s.size {
		return nil
	}
	buf := make([]byte, st.Size()-s.size)
	if _, err := f.ReadAt(buf, s.size); err != nil {
		return err
	}
	end := bytes.LastIndexByte(buf, '\n') + 1 // ignore a line still being written
	for _, line := range bytes.Split(buf[:end], []byte{'\n'}) {
		if len(line) == 0 {
			continue
		}
		var ev Event
		if err := json.Unmarshal(line, &ev); err != nil {
			return fmt.Errorf("log line after seq %d: %w", len(s.evs), err)
		}
		if err := link(s.last, len(s.evs)+1, ev); err != nil {
			return err
		}
		s.evs = append(s.evs, ev)
		s.last = ev.Hash
	}
	s.size += int64(end)
	return nil
}

func link(prev string, seq int, ev Event) error {
	switch {
	case ev.Seq != seq:
		return fmt.Errorf("seq %d: expected %d", ev.Seq, seq)
	case ev.Prev != prev:
		return fmt.Errorf("seq %d: prev hash does not match the event before it", ev.Seq)
	case ev.Hash != hashOf(prev, ev.Seq, ev.TS, ev.Type, ev.Data):
		return fmt.Errorf("seq %d: event hash does not match its content", ev.Seq)
	}
	return nil
}

// Sync picks up events appended by other processes.
func (s *Store) Sync() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.refresh()
}

// Head returns how many events are in the log and the hash of the last one.
func (s *Store) Head() (int, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.evs), s.last
}

// Events returns a copy of every event read so far.
func (s *Store) Events() []Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Event(nil), s.evs...)
}

// EventsFrom returns events after the first n (a cheap way to follow the log).
func (s *Store) EventsFrom(n int) []Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n >= len(s.evs) {
		return nil
	}
	return append([]Event(nil), s.evs[n:]...)
}

// Append adds one event. data is stored in canonical JSON.
func (s *Store) Append(typ string, data any) (Event, error) {
	d, err := canon.Marshal(data)
	if err != nil {
		return Event{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Event{}, err
	}
	defer unlock()
	if err := s.refresh(); err != nil { // someone else may have appended
		return Event{}, err
	}
	ev := Event{Seq: len(s.evs) + 1, TS: time.Now().UTC().Format(time.RFC3339), Type: typ, Data: d, Prev: s.last}
	ev.Hash = hashOf(ev.Prev, ev.Seq, ev.TS, ev.Type, ev.Data)
	var line bytes.Buffer
	e := json.NewEncoder(&line)
	e.SetEscapeHTML(false)
	if err := e.Encode(ev); err != nil {
		return Event{}, err
	}
	f, err := os.OpenFile(s.logPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return Event{}, err
	}
	defer f.Close()
	if _, err := f.Write(line.Bytes()); err != nil {
		return Event{}, err
	}
	if err := f.Sync(); err != nil {
		return Event{}, err
	}
	s.evs = append(s.evs, ev)
	s.last = ev.Hash
	s.size += int64(line.Len())
	return ev, nil
}

// lock takes a lock file with O_EXCL; works on every OS without syscall tricks.
// shortcut: a crashed holder leaves the file behind; it is treated as stale
// after 30s. Upgrade to flock/LockFileEx if writers ever run on one hot path.
func (s *Store) lock() (func(), error) {
	p := filepath.Join(s.dir, "specter.lock")
	deadline := time.Now().Add(10 * time.Second)
	for {
		f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			f.Close()
			return func() { os.Remove(p) }, nil
		}
		if st, e := os.Stat(p); e == nil && time.Since(st.ModTime()) > 30*time.Second {
			os.Remove(p)
			continue
		}
		if time.Now().After(deadline) {
			return nil, errors.New("store is locked by another process")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// PutBlob stores b under its SHA-256 and returns the hash. Existing blobs are kept.
func (s *Store) PutBlob(b []byte) (string, error) {
	sum := sha256.Sum256(b)
	h := hex.EncodeToString(sum[:])
	p := filepath.Join(s.dir, "blobs", h)
	if _, err := os.Stat(p); err == nil {
		return h, nil
	}
	tmp, err := os.CreateTemp(filepath.Join(s.dir, "blobs"), ".tmp-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Chmod(tmp.Name(), 0o444); err != nil {
		return "", err
	}
	return h, os.Rename(tmp.Name(), p)
}

// GetBlob reads a blob and checks it still hashes to its name.
func (s *Store) GetBlob(h string) ([]byte, error) {
	if len(h) != 64 || !isHex(h) {
		return nil, ErrNotFound
	}
	b, err := os.ReadFile(filepath.Join(s.dir, "blobs", h))
	if errors.Is(err, os.ErrNotExist) {
		if s.Redacted(h) {
			return nil, ErrRedacted
		}
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if sum := sha256.Sum256(b); hex.EncodeToString(sum[:]) != h {
		return nil, ErrCorrupt
	}
	return b, nil
}

func isHex(s string) bool {
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// Redact removes a blob's bytes and records that in the log. The chain keeps
// the hash, so the history shows something was removed and when.
func (s *Store) Redact(h, reason string) error {
	if _, err := s.Append("blob.redact", map[string]string{"blob": h, "reason": reason}); err != nil {
		return err
	}
	err := os.Remove(filepath.Join(s.dir, "blobs", h))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// Redacted reports whether the log records a redaction of h.
func (s *Store) Redacted(h string) bool {
	for _, ev := range s.Events() {
		if ev.Type != "blob.redact" {
			continue
		}
		var d struct{ Blob string }
		if json.Unmarshal(ev.Data, &d) == nil && d.Blob == h {
			return true
		}
	}
	return false
}

// Verify re-reads the whole log from disk, checks every link, and checks every
// blob an event points at (redacted blobs may be absent).
func (s *Store) Verify() error {
	evs, err := ReadLog(s.logPath())
	if err != nil {
		return err
	}
	redacted := map[string]bool{}
	for _, ev := range evs {
		var d struct{ Blob string }
		_ = json.Unmarshal(ev.Data, &d)
		if ev.Type == "blob.redact" {
			redacted[d.Blob] = true
		}
	}
	for _, ev := range evs {
		var d struct{ Blob string }
		if json.Unmarshal(ev.Data, &d) != nil || d.Blob == "" || ev.Type == "blob.redact" {
			continue
		}
		if _, err := s.GetBlob(d.Blob); err != nil && !(errors.Is(err, ErrRedacted) || redacted[d.Blob] && errors.Is(err, ErrNotFound)) {
			return fmt.Errorf("seq %d: blob %.12s: %w", ev.Seq, d.Blob, err)
		}
	}
	return nil
}

// ReadLog parses a log file and checks its chain. It needs no Store, so a
// receipt can be checked against a log copied from elsewhere.
func ReadLog(path string) ([]Event, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var evs []Event
	prev := genesis
	for i, line := range bytes.Split(b, []byte{'\n'}) {
		if len(line) == 0 {
			continue
		}
		var ev Event
		if err := json.Unmarshal(line, &ev); err != nil {
			return nil, fmt.Errorf("line %d: %w", i+1, err)
		}
		if err := link(prev, len(evs)+1, ev); err != nil {
			return nil, err
		}
		evs = append(evs, ev)
		prev = ev.Hash
	}
	return evs, nil
}
