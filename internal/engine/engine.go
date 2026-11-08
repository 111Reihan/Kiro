// Package engine keeps versioned documents in the hash-chained log and owns the
// signing key. Everything else (specs, attestations) is built on top of it.
package engine

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/111Reihan/kiro/internal/store"
)

var (
	ErrInvalid  = errors.New("invalid input")
	ErrNoDoc    = errors.New("no such document or version")
	ErrNoChange = errors.New("text is identical to the latest version")
)

const MaxDocBytes = 2 << 20

var idRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

type Engine struct {
	St *store.Store

	key ed25519.PrivateKey

	mu sync.Mutex // guards st
	st state
}

// New opens (or creates) a data directory.
func New(dir string) (*Engine, error) {
	st, err := store.Open(dir)
	if err != nil {
		return nil, err
	}
	key, err := loadKey(filepath.Join(dir, "key.ed25519"))
	if err != nil {
		return nil, err
	}
	e := &Engine{St: st, key: key}
	e.st = newState()
	return e, nil
}

// loadKey reads the signing seed, creating it on first use. The file is the
// only secret in the data directory; keep it out of version control.
func loadKey(path string) (ed25519.PrivateKey, error) {
	if b, err := os.ReadFile(path); err == nil {
		seed, err := hex.DecodeString(strings.TrimSpace(string(b)))
		if err != nil || len(seed) != ed25519.SeedSize {
			return nil, errors.New("key.ed25519 is not a 32-byte hex seed")
		}
		return ed25519.NewKeyFromSeed(seed), nil
	}
	seed := make([]byte, ed25519.SeedSize)
	if _, err := rand.Read(seed); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) { // another process won the race
		return loadKey(path)
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if _, err := f.WriteString(hex.EncodeToString(seed) + "\n"); err != nil {
		return nil, err
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

// ---- state: derived from the log, never written back ----

type dv struct {
	Ver         int
	Title, Blob string
	TS, Hash    string
	Seq, Runes  int
}

type verData struct {
	Doc   string `json:"doc"`
	Title string `json:"title"`
	Blob  string `json:"blob"`
	Runes int    `json:"runes"`
}

type state struct {
	n     int
	docs  map[string][]*dv
	order []string
	byseq map[int]*dv
}

func newState() state {
	return state{docs: map[string][]*dv{}, byseq: map[int]*dv{}}
}

func (s *state) apply(ev store.Event) {
	switch ev.Type {
	case "doc.version":
		var d verData
		if json.Unmarshal(ev.Data, &d) != nil {
			return
		}
		l := s.docs[d.Doc]
		if l == nil {
			s.order = append(s.order, d.Doc)
		}
		// the version number is the position in the log, so two racing writers
		// can never produce the same number twice
		v := &dv{Ver: len(l) + 1, Title: d.Title, Blob: d.Blob, TS: ev.TS, Hash: ev.Hash, Seq: ev.Seq, Runes: d.Runes}
		s.docs[d.Doc] = append(l, v)
		s.byseq[ev.Seq] = v
	}
}

// sync folds new log events into state. e.mu held.
func (e *Engine) sync() error {
	if err := e.St.Sync(); err != nil {
		return err
	}
	for _, ev := range e.St.EventsFrom(e.st.n) {
		e.st.apply(ev)
		e.st.n++
	}
	return nil
}

// ---- documents ----

type DocInfo struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Versions int    `json:"versions"`
	Runes    int    `json:"runes"`
	TS       string `json:"ts"`
}

type DocVersion struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Ver   int    `json:"ver"`
	Text  string `json:"text,omitempty"`
	Blob  string `json:"blob"`
	Seq   int    `json:"seq"`
	Hash  string `json:"hash"`
	TS    string `json:"ts"`
}

// KeyID is a short fingerprint of a public key. It names a key; it is not an identity.
func KeyID(pub ed25519.PublicKey) string {
	s := sha256.Sum256(pub)
	return hex.EncodeToString(s[:8])
}

// Key returns the signing key, for the ledger to sign attestations with.
func (e *Engine) Key() ed25519.PrivateKey { return e.key }

func (e *Engine) KeyID() string {
	return KeyID(e.key.Public().(ed25519.PublicKey))
}

// PubKey is the signing public key, base64. Hand it out so others can pin it
// when they verify receipts.
func (e *Engine) PubKey() string {
	return base64.StdEncoding.EncodeToString(e.key.Public().(ed25519.PublicKey))
}

// PutDoc adds a document, or a new version of one. Old versions are never changed.
func (e *Engine) PutDoc(id, title, txt string) (DocVersion, error) {
	switch {
	case !idRe.MatchString(id):
		return DocVersion{}, invalid("id must match %s", idRe)
	case !utf8.ValidString(txt):
		return DocVersion{}, invalid("text is not valid UTF-8")
	case strings.TrimSpace(txt) == "":
		return DocVersion{}, invalid("text is empty")
	case len(txt) > MaxDocBytes:
		return DocVersion{}, invalid("text is over %d bytes", MaxDocBytes)
	case utf8.RuneCountInString(title) > 200:
		return DocVersion{}, invalid("title is over 200 characters")
	}
	if title == "" {
		title = id
	}
	blob, err := e.St.PutBlob([]byte(txt))
	if err != nil {
		return DocVersion{}, err
	}
	e.mu.Lock()
	if err := e.sync(); err != nil {
		e.mu.Unlock()
		return DocVersion{}, err
	}
	if l := e.st.docs[id]; len(l) > 0 && l[len(l)-1].Blob == blob {
		e.mu.Unlock()
		return DocVersion{}, ErrNoChange
	}
	e.mu.Unlock()
	ev, err := e.St.Append("doc.version", verData{Doc: id, Title: title, Blob: blob, Runes: utf8.RuneCountInString(txt)})
	if err != nil {
		return DocVersion{}, err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.sync(); err != nil {
		return DocVersion{}, err
	}
	return toVersion(id, e.st.byseq[ev.Seq], ""), nil
}

func toVersion(id string, v *dv, txt string) DocVersion {
	return DocVersion{ID: id, Title: v.Title, Ver: v.Ver, Text: txt, Blob: v.Blob, Seq: v.Seq, Hash: v.Hash, TS: v.TS}
}

func (e *Engine) Docs() ([]DocInfo, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.sync(); err != nil {
		return nil, err
	}
	out := make([]DocInfo, 0, len(e.st.order))
	for _, id := range e.st.order {
		l := e.st.docs[id]
		last := l[len(l)-1]
		out = append(out, DocInfo{ID: id, Title: last.Title, Versions: len(l), Runes: last.Runes, TS: l[0].TS})
	}
	return out, nil
}

// pick returns one version of a document; ver 0 means the latest.
func (e *Engine) pick(id string, ver int) (*dv, []*dv, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.sync(); err != nil {
		return nil, nil, err
	}
	l := e.st.docs[id]
	if len(l) == 0 {
		return nil, nil, fmt.Errorf("%w: %q", ErrNoDoc, id)
	}
	if ver == 0 {
		ver = len(l)
	}
	if ver < 1 || ver > len(l) {
		return nil, nil, fmt.Errorf("%w: %q has no version %d", ErrNoDoc, id, ver)
	}
	return l[ver-1], append([]*dv(nil), l...), nil
}

// Doc returns one version with its text. ver 0 means the latest.
func (e *Engine) Doc(id string, ver int) (DocVersion, error) {
	v, _, err := e.pick(id, ver)
	if err != nil {
		return DocVersion{}, err
	}
	b, err := e.St.GetBlob(v.Blob)
	if err != nil {
		return DocVersion{}, err
	}
	return toVersion(id, v, string(b)), nil
}

func (e *Engine) Versions(id string) ([]DocVersion, error) {
	_, l, err := e.pick(id, 0)
	if err != nil {
		return nil, err
	}
	out := make([]DocVersion, len(l))
	for i, v := range l {
		out[i] = toVersion(id, v, "")
	}
	return out, nil
}

func (e *Engine) Integrity() error { return e.St.Verify() }
