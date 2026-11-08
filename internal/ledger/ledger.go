// Package ledger ties the scanned project to the hash-chained log. It records
// requirement files as versioned documents, stores test runs, signs
// attestations ("this requirement, in these exact words, is proved by these
// tests") and later says which of them have gone stale.
package ledger

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/111Reihan/kiro/internal/align"
	"github.com/111Reihan/kiro/internal/canon"
	"github.com/111Reihan/kiro/internal/engine"
	"github.com/111Reihan/kiro/internal/spec"
	"github.com/111Reihan/kiro/internal/store"
	"github.com/111Reihan/kiro/internal/verify"
)

// Statuses, worst first. A requirement shows the first one that applies.
const (
	Failing   = "failing"   // a linked test failed in the latest run
	Stale     = "stale"     // wording or proving tests changed since the seal
	Orphaned  = "orphaned"  // sealed once, requirement no longer exists
	Uncovered = "uncovered" // no test says it covers this
	Unsealed  = "unsealed"  // tests exist, nobody has signed off yet
	Verified  = "verified"  // sealed, unchanged, tests passing
)

var Order = []string{Failing, Stale, Orphaned, Uncovered, Unsealed, Verified}

const domain = "specter-attest-v1\n"

var (
	ErrNotSealable = errors.New("not sealable")
	ErrBadSig      = errors.New("signature does not verify")
)

type Ledger struct{ E *engine.Engine }

func New(e *engine.Engine) *Ledger { return &Ledger{E: e} }

// ---- attestations ----

// AttTest is a test as it was when the requirement was sealed.
type AttTest struct {
	Name   string `json:"name"`
	File   string `json:"file"`
	Body   string `json:"body"`
	Status string `json:"status"`
}

type Anchor struct {
	Seq  int    `json:"seq"`  // events in the log when signed
	Hash string `json:"hash"` // hash of the last of them
}

// Attestation is the signed claim. Every field is part of the signature.
type Attestation struct {
	V      int       `json:"v"`
	Req    string    `json:"req"`
	Text   string    `json:"text"`
	Hash   string    `json:"hash"` // spec.WordHash of Text
	File   string    `json:"file"`
	Doc    string    `json:"doc"`
	DocVer int       `json:"doc_ver"`
	Tests  []AttTest `json:"tests"`
	Anchor Anchor    `json:"anchor"`
	At     string    `json:"at"`
	Key    string    `json:"key"` // signer's public key, base64
}

type Signed struct {
	Attestation
	Sig string `json:"sig"`
	Seq int    `json:"seq,omitempty"` // position of the seal event in the log
}

func signBytes(a Attestation) ([]byte, error) {
	b, err := canon.Marshal(a)
	if err != nil {
		return nil, err
	}
	return append([]byte(domain), b...), nil
}

func sign(key ed25519.PrivateKey, a Attestation) (Signed, error) {
	m, err := signBytes(a)
	if err != nil {
		return Signed{}, err
	}
	return Signed{Attestation: a, Sig: base64.StdEncoding.EncodeToString(ed25519.Sign(key, m))}, nil
}

// VerifySigned checks the signature. pin, when not empty, is the public key the
// signer must have used (base64); without it the file only proves it is
// self-consistent.
func VerifySigned(s Signed, pin string) error {
	pub, err := base64.StdEncoding.DecodeString(s.Key)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return fmt.Errorf("%w: bad public key", ErrBadSig)
	}
	if pin != "" && pin != s.Key {
		return fmt.Errorf("%w: signed by key %s, not the pinned key", ErrBadSig, engine.KeyID(pub))
	}
	sig, err := base64.StdEncoding.DecodeString(s.Sig)
	if err != nil {
		return fmt.Errorf("%w: bad signature encoding", ErrBadSig)
	}
	m, err := signBytes(s.Attestation)
	if err != nil {
		return err
	}
	if !ed25519.Verify(pub, m, sig) {
		return ErrBadSig
	}
	if s.Hash != spec.WordHash(s.Text) {
		return fmt.Errorf("%w: text does not match its hash", ErrBadSig)
	}
	return nil
}

// CheckAnchor confirms the log (given as events) really had this head when the
// attestation was signed.
func CheckAnchor(a Attestation, evs []store.Event) error {
	if a.Anchor.Seq < 1 || a.Anchor.Seq > len(evs) {
		return fmt.Errorf("log has %d events; the attestation is anchored at %d", len(evs), a.Anchor.Seq)
	}
	if evs[a.Anchor.Seq-1].Hash != a.Anchor.Hash {
		return errors.New("log event at the anchor has a different hash: this is not the same history")
	}
	return nil
}

// ---- recording ----

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// DocID names the log document that holds a requirements file.
func DocID(file string) string {
	h := spec.WordHash(file)
	s := strings.Trim(slugRe.ReplaceAllString(strings.ToLower(file), "-"), "-")
	if len(s) > 50 {
		s = s[:50]
	}
	return "req-" + strings.Trim(s, "-") + "-" + h[:6]
}

// Record stores each requirements file as a new document version when its text
// changed. It returns the latest version of each, by file path.
func (l *Ledger) Record(p *spec.Project) (map[string]engine.DocVersion, error) {
	out := map[string]engine.DocVersion{}
	for _, f := range p.Files {
		if strings.TrimSpace(f.Text) == "" {
			continue
		}
		id := DocID(f.Path)
		v, err := l.E.PutDoc(id, f.Path, f.Text)
		if errors.Is(err, engine.ErrNoChange) {
			v, err = l.E.Doc(id, 0)
			v.Text = ""
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.Path, err)
		}
		out[f.Path] = v
	}
	return out, nil
}

// RunInfo describes a stored test run.
type RunInfo struct {
	Seq  int    `json:"seq"`
	TS   string `json:"ts"`
	Cmd  string `json:"cmd"`
	Pass int    `json:"pass"`
	Fail int    `json:"fail"`
	Skip int    `json:"skip"`
	Blob string `json:"blob"`
}

// AddRun stores a test run. Results are kept as a blob so the log can prove
// them later.
func (l *Ledger) AddRun(rs []spec.Result, cmd string) (RunInfo, error) {
	if len(rs) == 0 {
		return RunInfo{}, errors.New("no test results to store")
	}
	b, err := canon.Marshal(rs)
	if err != nil {
		return RunInfo{}, err
	}
	blob, err := l.E.St.PutBlob(b)
	if err != nil {
		return RunInfo{}, err
	}
	ri := RunInfo{Cmd: cmd, Blob: blob}
	for _, r := range rs {
		switch r.Status {
		case spec.Pass:
			ri.Pass++
		case spec.Fail:
			ri.Fail++
		default:
			ri.Skip++
		}
	}
	ev, err := l.E.St.Append("spec.run", ri)
	if err != nil {
		return RunInfo{}, err
	}
	ri.Seq, ri.TS = ev.Seq, ev.TS
	return ri, nil
}

// ---- reading the log ----

type view struct {
	evs     []store.Event
	seals   map[string]Signed // latest seal per requirement
	all     []Signed          // every seal, in order
	run     *RunInfo
	results []spec.Result
}

func (l *Ledger) view() (*view, error) {
	if err := l.E.St.Sync(); err != nil {
		return nil, err
	}
	v := &view{evs: l.E.St.Events(), seals: map[string]Signed{}}
	for _, ev := range v.evs {
		switch ev.Type {
		case "spec.seal":
			var s Signed
			if json.Unmarshal(ev.Data, &s) != nil {
				continue
			}
			s.Seq = ev.Seq
			v.seals[s.Req] = s
			v.all = append(v.all, s)
		case "spec.run":
			var ri RunInfo
			if json.Unmarshal(ev.Data, &ri) != nil {
				continue
			}
			ri.Seq, ri.TS = ev.Seq, ev.TS
			v.run = &ri
		}
	}
	if v.run != nil {
		if b, err := l.E.St.GetBlob(v.run.Blob); err == nil {
			_ = json.Unmarshal(b, &v.results)
		}
	}
	return v, nil
}

// ---- status ----

type TestLink struct {
	Name    string `json:"name"`
	File    string `json:"file"`
	Line    int    `json:"line"`
	Result  string `json:"result"`  // pass, fail, skip, none
	Changed bool   `json:"changed"` // code differs from what was sealed
}

type TaskLink struct {
	Text string `json:"text"`
	Done bool   `json:"done"`
	File string `json:"file"`
	Line int    `json:"line"`
}

type SealInfo struct {
	Seq    int    `json:"seq"`
	TS     string `json:"ts"`
	Text   string `json:"text"`
	DocVer int    `json:"doc_ver"`
	Tests  int    `json:"tests"`
}

// Change is the ghost diff: what the words were when sealed against now.
type Change struct {
	Severity string     `json:"severity"` // cosmetic, minor, material
	Ops      []align.Op `json:"ops"`      // q = sealed word, s = current word
}

type ReqStatus struct {
	spec.Req
	Status  string     `json:"status"`
	Reasons []string   `json:"reasons,omitempty"`
	Tests   []TestLink `json:"tests"`
	Tasks   []TaskLink `json:"tasks"`
	Sealed  *SealInfo  `json:"sealed,omitempty"`
	Change  *Change    `json:"change,omitempty"`
}

type Orphan struct {
	Req     string `json:"req"`
	Text    string `json:"text"`
	Seq     int    `json:"seq"`
	MovedTo string `json:"moved_to,omitempty"` // the requirement that now has these words
	Match   int    `json:"match,omitempty"`    // percent of words shared with it
}

type Report struct {
	Root     string         `json:"root"`
	Reqs     []ReqStatus    `json:"reqs"`
	Orphans  []Orphan       `json:"orphans"`
	Counts   map[string]int `json:"counts"`
	Warnings []string       `json:"warnings"`
	Run      *RunInfo       `json:"run,omitempty"`
	Events   int            `json:"events"`
	Head     string         `json:"head"`
	Key      string         `json:"key"`
	Tests    int            `json:"tests"`
	Untied   []string       `json:"untied_tasks,omitempty"`
}

// Status compares the project on disk with what the log has sealed.
func (l *Ledger) Status(p *spec.Project) (*Report, error) {
	v, err := l.view()
	if err != nil {
		return nil, err
	}
	return l.status(p, v), nil
}

func (l *Ledger) status(p *spec.Project, v *view) *Report {
	n, head := l.E.St.Head()
	rep := &Report{Root: p.Root, Warnings: append([]string{}, p.Warnings...), Run: v.run, Events: n, Head: head,
		Key: l.E.KeyID(), Counts: map[string]int{}, Tests: len(p.Tests), Reqs: []ReqStatus{}, Orphans: []Orphan{}}
	present := map[string]bool{}
	seenReq := map[string]bool{}
	for _, r := range p.Reqs {
		if seenReq[r.ID] {
			continue // a duplicate id keeps its first appearance
		}
		seenReq[r.ID] = true
		present[r.ID] = true
		rs := ReqStatus{Req: r, Tests: []TestLink{}, Tasks: []TaskLink{}}
		seal, sealed := v.seals[r.ID]
		cur := map[string]spec.Test{}
		failing := false
		for _, t := range p.TestsFor(r.ID) {
			res := spec.Status(t, v.results)
			cur[t.File+"\x00"+t.Name] = t
			if res == spec.Fail {
				failing = true
			}
			tl := TestLink{Name: t.Name, File: t.File, Line: t.Line, Result: res}
			if sealed {
				for _, st := range seal.Tests {
					if st.File == t.File && st.Name == t.Name && st.Body != t.Body {
						tl.Changed = true
					}
				}
			}
			rs.Tests = append(rs.Tests, tl)
		}
		for _, t := range p.TasksFor(r.ID) {
			rs.Tasks = append(rs.Tasks, TaskLink{Text: t.Text, Done: t.Done, File: t.File, Line: t.Line})
		}
		stale := false
		if sealed {
			rs.Sealed = &SealInfo{Seq: seal.Seq, TS: seal.At, Text: seal.Text, DocVer: seal.DocVer, Tests: len(seal.Tests)}
			if sev, ops, changed := verify.Compare(seal.Text, r.Text); changed {
				stale = true
				rs.Change = &Change{Severity: string(sev), Ops: ops}
				rs.Reasons = append(rs.Reasons, fmt.Sprintf("wording changed since the seal (%s)", sev))
			}
			for _, st := range seal.Tests {
				t, ok := cur[st.File+"\x00"+st.Name]
				switch {
				case !ok:
					stale = true
					rs.Reasons = append(rs.Reasons, fmt.Sprintf("%s no longer says it covers this", st.Name))
				case t.Body != st.Body:
					stale = true
					rs.Reasons = append(rs.Reasons, fmt.Sprintf("%s was edited since the seal", st.Name))
				}
			}
		}
		switch {
		case failing:
			rs.Status = Failing
			rs.Reasons = append([]string{"a covering test failed in the latest run"}, rs.Reasons...)
		case stale:
			rs.Status = Stale
		case len(rs.Tests) == 0:
			rs.Status = Uncovered
			rs.Reasons = append(rs.Reasons, "no test says it covers this")
		case !sealed:
			rs.Status = Unsealed
			rs.Reasons = append(rs.Reasons, "tests exist but nobody has sealed it")
		default:
			rs.Status = Verified
		}
		rep.Counts[rs.Status]++
		rep.Reqs = append(rep.Reqs, rs)
	}
	ids := make([]string, 0, len(v.seals))
	for id := range v.seals {
		if !present[id] {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		s := v.seals[id]
		o := Orphan{Req: id, Text: s.Text, Seq: s.Seq}
		if len(ids)*len(p.Reqs) <= 200_000 {
			for _, r := range p.Reqs {
				if pct := similarity(s.Text, r.Text); pct >= 60 && pct > o.Match {
					o.MovedTo, o.Match = r.ID, pct
				}
			}
		}
		rep.Orphans = append(rep.Orphans, o)
		rep.Counts[Orphaned]++
	}
	for _, t := range p.Tasks {
		if len(t.Reqs) == 0 {
			rep.Untied = append(rep.Untied, fmt.Sprintf("%s:%d %s", t.File, t.Line, t.Text))
		}
	}
	return rep
}

func similarity(a, b string) int {
	_, ops, _ := verify.Compare(a, b)
	eq := 0
	for _, o := range ops {
		if o.Kind == "eq" {
			eq++
		}
	}
	if len(ops) == 0 {
		return 0
	}
	return eq * 100 / len(ops)
}

// ---- sealing ----

type Skip struct {
	Req string `json:"req"`
	Why string `json:"why"`
}

type SealResult struct {
	Sealed  []Signed `json:"sealed"`
	Skipped []Skip   `json:"skipped"`
}

// Seal signs requirements whose tests exist and passed in the latest run.
// With no ids it seals every unsealed or stale one it can and lists the rest
// as skipped; with ids it fails on the first one that cannot be sealed.
func (l *Ledger) Seal(p *spec.Project, ids []string) (*SealResult, error) {
	docs, err := l.Record(p)
	if err != nil {
		return nil, err
	}
	v, err := l.view()
	if err != nil {
		return nil, err
	}
	rep := l.status(p, v)
	byID := map[string]ReqStatus{}
	for _, r := range rep.Reqs {
		byID[r.ID] = r
	}
	explicit := len(ids) > 0
	if !explicit {
		for _, r := range rep.Reqs {
			ids = append(ids, r.ID)
		}
	}
	res := &SealResult{Sealed: []Signed{}, Skipped: []Skip{}}
	for _, id := range ids {
		r, ok := byID[id]
		why := ""
		switch {
		case !ok:
			why = "no such requirement"
		case r.Status == Verified:
			why = "already verified"
		case len(r.Tests) == 0:
			why = "no test covers it"
		case v.run == nil:
			why = "no test run stored yet"
		default:
			for _, t := range r.Tests {
				if t.Result != spec.Pass {
					why = fmt.Sprintf("%s is %s in the latest run", t.Name, t.Result)
					break
				}
			}
		}
		if why != "" {
			if explicit && why != "already verified" {
				return res, fmt.Errorf("%w: %s: %s", ErrNotSealable, id, why)
			}
			res.Skipped = append(res.Skipped, Skip{id, why})
			continue
		}
		dv := docs[r.File]
		a := Attestation{V: 1, Req: id, Text: r.Text, Hash: r.Hash, File: r.File, Doc: dv.ID, DocVer: dv.Ver,
			Tests: []AttTest{}, At: time.Now().UTC().Format(time.RFC3339),
			Key: l.E.PubKey()}
		for _, t := range p.TestsFor(id) {
			a.Tests = append(a.Tests, AttTest{Name: t.Name, File: t.File, Body: t.Body, Status: spec.Status(t, v.results)})
		}
		a.Anchor.Seq, a.Anchor.Hash = l.E.St.Head()
		s, err := sign(l.E.Key(), a)
		if err != nil {
			return res, err
		}
		ev, err := l.E.St.Append("spec.seal", s)
		if err != nil {
			return res, err
		}
		s.Seq = ev.Seq
		res.Sealed = append(res.Sealed, s)
	}
	return res, nil
}

// ---- history ----

type Rev struct {
	Ver      int        `json:"ver"`
	TS       string     `json:"ts"`
	Present  bool       `json:"present"`
	Text     string     `json:"text"`
	Severity string     `json:"severity,omitempty"` // against the previous revision
	Ops      []align.Op `json:"ops,omitempty"`
}

// History lists how one requirement's wording changed across recorded versions
// of its file. Only revisions where it changed are returned.
func (l *Ledger) History(p *spec.Project, id string) ([]Rev, error) {
	var f *spec.File
	for _, r := range p.Reqs {
		if r.ID == id {
			for i := range p.Files {
				if p.Files[i].Path == r.File {
					f = &p.Files[i]
				}
			}
			break
		}
	}
	if f == nil {
		return nil, fmt.Errorf("%w: no requirement %q", engine.ErrNoDoc, id)
	}
	vers, err := l.E.Versions(DocID(f.Path))
	if errors.Is(err, engine.ErrNoDoc) {
		return []Rev{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []Rev{}
	prev := ""
	for _, dv := range vers {
		d, err := l.E.Doc(DocID(f.Path), dv.Ver)
		if err != nil {
			return nil, err
		}
		cur, found := "", false
		for _, r := range spec.ParseRequirements(f.Spec, f.Path, d.Text) {
			if r.ID == id {
				cur, found = r.Text, true
				break
			}
		}
		if len(out) > 0 && out[len(out)-1].Present == found && (!found || spec.WordHash(cur) == spec.WordHash(prev)) {
			continue
		}
		rv := Rev{Ver: dv.Ver, TS: dv.TS, Present: found, Text: cur}
		if found && len(out) > 0 && out[len(out)-1].Present {
			sev, ops, _ := verify.Compare(prev, cur)
			rv.Severity, rv.Ops = string(sev), ops
		}
		out = append(out, rv)
		prev = cur
	}
	return out, nil
}

// ---- audit ----

type AuditSeal struct {
	Req string `json:"req"`
	Seq int    `json:"seq"`
	OK  bool   `json:"ok"`
	Err string `json:"err,omitempty"`
}

type Audit struct {
	Events   int         `json:"events"`
	ChainOK  bool        `json:"chain_ok"`
	ChainErr string      `json:"chain_err,omitempty"`
	Seals    []AuditSeal `json:"seals"`
	Head     string      `json:"head"`
	Key      string      `json:"key"`
	KeyID    string      `json:"key_id"`
}

// Audit re-reads the log from disk, checks the hash chain and the blobs, then
// checks every seal's signature (against this ledger's key) and anchor.
func (l *Ledger) Audit() (*Audit, error) {
	v, err := l.view()
	if err != nil {
		return nil, err
	}
	n, head := l.E.St.Head()
	a := &Audit{Events: n, Head: head, Key: l.E.PubKey(), KeyID: l.E.KeyID(), ChainOK: true, Seals: []AuditSeal{}}
	if err := l.E.Integrity(); err != nil {
		a.ChainOK, a.ChainErr = false, err.Error()
	}
	for _, s := range v.all {
		as := AuditSeal{Req: s.Req, Seq: s.Seq, OK: true}
		if err := VerifySigned(s, l.E.PubKey()); err != nil {
			as.OK, as.Err = false, err.Error()
		} else if err := CheckAnchor(s.Attestation, v.evs); err != nil {
			as.OK, as.Err = false, err.Error()
		}
		a.Seals = append(a.Seals, as)
	}
	return a, nil
}

// Events returns the log for display, newest last.
func (l *Ledger) Events() ([]store.Event, error) {
	if err := l.E.St.Sync(); err != nil {
		return nil, err
	}
	return l.E.St.Events(), nil
}

// Seal returns the latest seal for a requirement.
func (l *Ledger) SealOf(id string) (Signed, bool, error) {
	v, err := l.view()
	if err != nil {
		return Signed{}, false, err
	}
	s, ok := v.seals[id]
	return s, ok, nil
}
