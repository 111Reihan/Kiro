package ledger

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/111Reihan/kiro/internal/engine"
	"github.com/111Reihan/kiro/internal/spec"
)

const reqs = `# Requirements

### Requirement 1

1. WHEN a user submits a form THEN the system SHALL respond within 2 seconds.
2. The system SHALL NOT store passwords in plain text.
3. The system SHALL support dark mode.
`

const goTest = `package app

import "testing"

// covers: 1.1
func TestLatency(t *testing.T) {
	if 1+1 != 2 {
		t.Fatal("no")
	}
}

// covers: 1.2
func TestHashing(t *testing.T) {
	_ = "bcrypt"
}
`

const goJSON = `{"Action":"pass","Package":"x/app","Test":"TestLatency"}
{"Action":"pass","Package":"x/app","Test":"TestHashing"}
`

type fixture struct {
	t    *testing.T
	root string
	l    *Ledger
}

func newFix(t *testing.T) *fixture {
	root := t.TempDir()
	f := &fixture{t: t, root: root}
	f.write(".kiro/specs/auth/requirements.md", reqs)
	f.write("app_test.go", goTest)
	e, err := engine.New(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	f.l = New(e)
	return f
}

func (f *fixture) write(rel, s string) {
	f.t.Helper()
	p := filepath.Join(f.root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) scan() *spec.Project {
	f.t.Helper()
	p, err := spec.Scan(f.root)
	if err != nil {
		f.t.Fatal(err)
	}
	return p
}

func (f *fixture) status(id string) ReqStatus {
	f.t.Helper()
	rep, err := f.l.Status(f.scan())
	if err != nil {
		f.t.Fatal(err)
	}
	for _, r := range rep.Reqs {
		if r.ID == id {
			return r
		}
	}
	f.t.Fatalf("no requirement %s", id)
	return ReqStatus{}
}

func (f *fixture) run(js string) {
	f.t.Helper()
	rs, err := spec.ParseResults([]byte(js))
	if err != nil {
		f.t.Fatal(err)
	}
	if _, err := f.l.AddRun(rs, "go test"); err != nil {
		f.t.Fatal(err)
	}
}

// covers: specter-core/2.1, specter-core/2.3, specter-core/3.1
func TestLifecycle(t *testing.T) {
	f := newFix(t)
	if s := f.status("auth/1.1").Status; s != Unsealed {
		t.Fatalf("fresh covered requirement: %s", s)
	}
	if s := f.status("auth/1.3").Status; s != Uncovered {
		t.Fatalf("requirement with no test: %s", s)
	}

	if _, err := f.l.Seal(f.scan(), []string{"auth/1.1"}); !errors.Is(err, ErrNotSealable) {
		t.Fatalf("sealing with no run must fail, got %v", err)
	}
	f.run(goJSON)
	res, err := f.l.Seal(f.scan(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Sealed) != 2 || len(res.Skipped) != 1 || res.Skipped[0].Req != "auth/1.3" {
		t.Fatalf("sealed %d skipped %+v", len(res.Sealed), res.Skipped)
	}
	if s := f.status("auth/1.1").Status; s != Verified {
		t.Fatalf("after seal: %s", s)
	}

	// the ghost: 2 seconds becomes 5 seconds
	f.write(".kiro/specs/auth/requirements.md", strings.Replace(reqs, "within 2 seconds", "within 5 seconds", 1))
	st := f.status("auth/1.1")
	if st.Status != Stale || st.Change == nil || st.Change.Severity != "material" {
		t.Fatalf("number change must be stale + material: %+v", st)
	}
	if f.status("auth/1.2").Status != Verified {
		t.Fatal("an untouched requirement must stay verified")
	}

	// punctuation and case only: still verified
	f.write(".kiro/specs/auth/requirements.md", strings.Replace(reqs, "SHALL respond", "shall respond,", 1))
	if s := f.status("auth/1.1"); s.Status != Verified {
		t.Fatalf("cosmetic edit made it %s: %v %+v", s.Status, s.Reasons, s.Change)
	}

	// editing the proving test makes it stale even when the words are the same
	f.write(".kiro/specs/auth/requirements.md", reqs)
	f.write("app_test.go", strings.Replace(goTest, "1+1 != 2", "1+1 != 3", 1))
	if s := f.status("auth/1.1"); s.Status != Stale || !s.Tests[0].Changed {
		t.Fatalf("edited test: %+v", s)
	}
	// and removing the annotation does too
	f.write("app_test.go", strings.Replace(goTest, "// covers: 1.1\n", "", 1))
	if s := f.status("auth/1.1"); s.Status != Stale {
		t.Fatalf("dropped covers: %s", s.Status)
	}
}

// covers: specter-core/2.4
func TestFailingBeatsStale(t *testing.T) {
	f := newFix(t)
	f.run(goJSON)
	if _, err := f.l.Seal(f.scan(), nil); err != nil {
		t.Fatal(err)
	}
	f.write(".kiro/specs/auth/requirements.md", strings.Replace(reqs, "within 2 seconds", "within 9 seconds", 1))
	f.run(`{"Action":"fail","Package":"x/app","Test":"TestLatency"}
{"Action":"pass","Package":"x/app","Test":"TestHashing"}`)
	if s := f.status("auth/1.1").Status; s != Failing {
		t.Fatalf("failing must win over stale, got %s", s)
	}
}

// covers: specter-core/2.5
func TestOrphanAndMove(t *testing.T) {
	f := newFix(t)
	f.run(goJSON)
	if _, err := f.l.Seal(f.scan(), nil); err != nil {
		t.Fatal(err)
	}
	// delete requirement 2 outright, and renumber 1.1 into a new spec folder
	if err := os.Remove(filepath.Join(f.root, ".kiro/specs/auth/requirements.md")); err != nil {
		t.Fatal(err)
	}
	f.write(".kiro/specs/perf/requirements.md", "### Requirement 1\n\n1. WHEN a user submits a form THEN the system SHALL respond within 2 seconds.\n")
	rep, err := f.l.Status(f.scan())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Orphan{}
	for _, o := range rep.Orphans {
		got[o.Req] = o
	}
	if o := got["auth/1.1"]; o.MovedTo != "perf/1.1" {
		t.Fatalf("moved requirement not found: %+v", got)
	}
	if o, ok := got["auth/1.2"]; !ok || o.MovedTo != "" {
		t.Fatalf("deleted requirement must be an orphan with no move: %+v", got)
	}
	if rep.Counts[Orphaned] != 2 {
		t.Fatalf("counts: %v", rep.Counts)
	}
}

// covers: specter-core/3.2, specter-core/3.3
func TestAttestationTamper(t *testing.T) {
	f := newFix(t)
	f.run(goJSON)
	if _, err := f.l.Seal(f.scan(), []string{"auth/1.1"}); err != nil {
		t.Fatal(err)
	}
	s, ok, err := f.l.SealOf("auth/1.1")
	if err != nil || !ok {
		t.Fatalf("seal missing: %v", err)
	}
	if err := VerifySigned(s, f.l.E.PubKey()); err != nil {
		t.Fatal(err)
	}
	bad := s
	bad.Text = strings.Replace(bad.Text, "2 seconds", "20 seconds", 1)
	if err := VerifySigned(bad, ""); !errors.Is(err, ErrBadSig) {
		t.Fatalf("edited text must fail: %v", err)
	}
	bad = s
	bad.Tests = append([]AttTest(nil), s.Tests...)
	bad.Tests[0].Status = "fail"
	if err := VerifySigned(bad, ""); !errors.Is(err, ErrBadSig) {
		t.Fatalf("edited test list must fail: %v", err)
	}
	other, _ := engine.New(filepath.Join(t.TempDir(), "other"))
	if err := VerifySigned(s, other.PubKey()); !errors.Is(err, ErrBadSig) {
		t.Fatalf("a pinned different key must fail: %v", err)
	}
	evs, _ := f.l.Events()
	if err := CheckAnchor(s.Attestation, evs); err != nil {
		t.Fatal(err)
	}
	if err := CheckAnchor(s.Attestation, evs[:1]); err == nil {
		t.Fatal("a shorter log must not satisfy the anchor")
	}
}

// covers: specter-core/3.4
func TestAuditCatchesLogEdit(t *testing.T) {
	f := newFix(t)
	f.run(goJSON)
	if _, err := f.l.Seal(f.scan(), nil); err != nil {
		t.Fatal(err)
	}
	a, err := f.l.Audit()
	if err != nil || !a.ChainOK || len(a.Seals) != 2 {
		t.Fatalf("clean audit: %+v %v", a, err)
	}
	for _, s := range a.Seals {
		if !s.OK {
			t.Fatalf("clean seal flagged: %+v", s)
		}
	}
	// flip a word inside the log, as someone covering their tracks would
	dir := f.l.E.St.Dir()
	lp := filepath.Join(dir, "specter.log")
	b, _ := os.ReadFile(lp)
	i := strings.Index(string(b), "spec.seal")
	if i < 0 {
		t.Fatal("no seal in log")
	}
	b = []byte(strings.Replace(string(b), "TestHashing", "TestHashinX", 1))
	if err := os.WriteFile(lp, b, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := f.l.E.Integrity(); err == nil {
		t.Fatal("edited log must fail integrity")
	}
}

func TestHistory(t *testing.T) {
	f := newFix(t)
	if _, err := f.l.Record(f.scan()); err != nil {
		t.Fatal(err)
	}
	f.write(".kiro/specs/auth/requirements.md", strings.Replace(reqs, "within 2 seconds", "within 5 seconds", 1))
	if _, err := f.l.Record(f.scan()); err != nil {
		t.Fatal(err)
	}
	// a change elsewhere in the file must not add a revision for 1.1
	f.write(".kiro/specs/auth/requirements.md", strings.Replace(strings.Replace(reqs, "within 2 seconds", "within 5 seconds", 1), "dark mode", "light mode", 1))
	if _, err := f.l.Record(f.scan()); err != nil {
		t.Fatal(err)
	}
	h, err := f.l.History(f.scan(), "auth/1.1")
	if err != nil {
		t.Fatal(err)
	}
	if len(h) != 2 || h[1].Severity != "material" {
		t.Fatalf("history: %+v", h)
	}
}

// covers: specter-core/3.1
func TestSealIsIdempotent(t *testing.T) {
	f := newFix(t)
	f.run(goJSON)
	if _, err := f.l.Seal(f.scan(), nil); err != nil {
		t.Fatal(err)
	}
	n, _ := f.l.E.St.Head()
	res, err := f.l.Seal(f.scan(), nil)
	if err != nil || len(res.Sealed) != 0 {
		t.Fatalf("second seal: %+v %v", res, err)
	}
	if m, _ := f.l.E.St.Head(); m != n {
		t.Fatalf("second seal wrote %d events", m-n)
	}
}
