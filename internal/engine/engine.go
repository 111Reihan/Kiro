// Package engine is the one place that knows what the log events mean. The
// HTTP server, the CLI and the MCP server are thin callers of it.
package engine

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/111Reihan/kiro/internal/receipt"
	"github.com/111Reihan/kiro/internal/store"
	"github.com/111Reihan/kiro/internal/text"
	"github.com/111Reihan/kiro/internal/verify"
)

var (
	ErrInvalid  = errors.New("invalid input")
	ErrNoDoc    = errors.New("no such document or version")
	ErrNoChange = errors.New("text is identical to the latest version")
	ErrNoMatch  = errors.New("quote is not found in that source")
)

const (
	MaxDocBytes   = 2 << 20
	MaxQuoteRunes = 5000
	MaxQuotes     = 200
)

var idRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

type Engine struct {
	St *store.Store

	key ed25519.PrivateKey

	mu sync.Mutex // guards st
	st state

	pmu  sync.Mutex // guards prep
	prep map[string]*verify.Prepared
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
	e := &Engine{St: st, key: key, prep: map[string]*verify.Prepared{}}
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
	n      int
	docs   map[string][]*dv
	order  []string
	cites  []*Cite
	trails map[string]*Trail
	torder []string
	byseq  map[int]*dv
}

func newState() state {
	return state{docs: map[string][]*dv{}, trails: map[string]*Trail{}, byseq: map[int]*dv{}}
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
	case "cite.add":
		var c Cite
		if json.Unmarshal(ev.Data, &c) != nil {
			return
		}
		c.ID, c.TS = "c"+strconv.Itoa(ev.Seq), ev.TS
		s.cites = append(s.cites, &c)
	case "trail.create":
		var t Trail
		if json.Unmarshal(ev.Data, &t) != nil {
			return
		}
		t.ID, t.TS = "t"+strconv.Itoa(ev.Seq), ev.TS
		s.trails[t.ID] = &t
		s.torder = append(s.torder, t.ID)
	case "trail.step":
		var d struct {
			Trail string `json:"trail"`
			Step
		}
		if json.Unmarshal(ev.Data, &d) != nil {
			return
		}
		if t := s.trails[d.Trail]; t != nil {
			d.Step.TS = ev.TS
			t.Steps = append(t.Steps, d.Step)
		}
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

func (e *Engine) KeyID() string {
	return receipt.KeyID(e.key.Public().(ed25519.PublicKey))
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

// prepared loads and folds one version, with a small cache keyed by blob hash.
func (e *Engine) prepared(doc string, v *dv) (*verify.Prepared, error) {
	e.pmu.Lock()
	p := e.prep[v.Blob]
	e.pmu.Unlock()
	if p == nil {
		b, err := e.St.GetBlob(v.Blob)
		if err != nil {
			return nil, err
		}
		p = verify.Prepare("", 0, string(b))
		e.pmu.Lock()
		if len(e.prep) >= 64 {
			e.prep = map[string]*verify.Prepared{}
		}
		e.prep[v.Blob] = p
		e.pmu.Unlock()
	}
	cp := *p
	cp.Doc, cp.Ver = doc, v.Ver
	return &cp, nil
}

// ---- checking ----

// Scope limits which sources a quote is checked against. No docs means all of
// them, latest version. Ver applies only when exactly one doc is named.
type Scope struct {
	Docs []string `json:"docs"`
	Ver  int      `json:"ver"`
}

type target struct {
	doc string
	v   *dv
}

func (e *Engine) targets(sc Scope) ([]target, error) {
	if len(sc.Docs) == 0 {
		e.mu.Lock()
		defer e.mu.Unlock()
		if err := e.sync(); err != nil {
			return nil, err
		}
		var out []target
		for _, id := range e.st.order {
			l := e.st.docs[id]
			out = append(out, target{id, l[len(l)-1]})
		}
		return out, nil
	}
	var out []target
	for _, id := range sc.Docs {
		ver := 0
		if len(sc.Docs) == 1 {
			ver = sc.Ver
		}
		v, _, err := e.pick(id, ver)
		if err != nil {
			return nil, err
		}
		out = append(out, target{id, v})
	}
	return out, nil
}

// Check grades one quote against the scoped sources and returns the best hit.
func (e *Engine) Check(quote string, sc Scope) (verify.Result, error) {
	if utf8.RuneCountInString(quote) > MaxQuoteRunes {
		return verify.Result{}, invalid("quote is over %d characters", MaxQuoteRunes)
	}
	ts, err := e.targets(sc)
	if err != nil {
		return verify.Result{}, err
	}
	if len(ts) == 0 {
		return verify.Result{Verdict: verify.NoSource, Notes: []string{"no source documents are loaded"}}, nil
	}
	var best verify.Result
	for i, t := range ts {
		p, err := e.prepared(t.doc, t.v)
		if err != nil {
			return verify.Result{}, err
		}
		if r := verify.Check(quote, p); i == 0 || verify.Better(r, best) {
			best = r
		}
	}
	if best.Verdict == verify.Unsupported {
		best.Doc, best.Ver = "", 0 // "closest source" would mislead
	}
	return best, nil
}

// Found is one quote found in a pasted text and how it graded.
type Found struct {
	text.Quote
	Result verify.Result `json:"result"`
}

// CheckText finds every double-quoted passage in txt and grades each.
func (e *Engine) CheckText(txt string, sc Scope) ([]Found, error) {
	if len(txt) > MaxDocBytes {
		return nil, invalid("text is over %d bytes", MaxDocBytes)
	}
	qs := text.ExtractQuotes(txt)
	if len(qs) > MaxQuotes {
		return nil, invalid("more than %d quotes; split the text", MaxQuotes)
	}
	out := make([]Found, 0, len(qs))
	for _, q := range qs {
		r, err := e.Check(q.Text, sc)
		if err != nil {
			return nil, err
		}
		out = append(out, Found{q, r})
	}
	return out, nil
}

func (e *Engine) Integrity() error { return e.St.Verify() }
