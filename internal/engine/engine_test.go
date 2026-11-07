package engine

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/111Reihan/kiro/internal/prop"
	"github.com/111Reihan/kiro/internal/receipt"
	"github.com/111Reihan/kiro/internal/verify"
)

const v1 = "Four score and seven years ago our fathers brought forth on this continent, a new nation, " +
	"conceived in Liberty, and dedicated to the proposition that all men are created equal. " +
	"Now we are engaged in a great civil war, testing whether that nation, or any nation so conceived " +
	"and so dedicated, can long endure."

const passage = "dedicated to the proposition that all men are created equal"

func newEng(t *testing.T) *Engine {
	t.Helper()
	e, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func mustPut(t *testing.T, e *Engine, id, txt string) DocVersion {
	t.Helper()
	d, err := e.PutDoc(id, id, txt)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestVersions(t *testing.T) {
	e := newEng(t)
	d := mustPut(t, e, "gb", v1)
	if d.Ver != 1 {
		t.Fatalf("ver %d", d.Ver)
	}
	if _, err := e.PutDoc("gb", "gb", v1); !errors.Is(err, ErrNoChange) {
		t.Fatalf("got %v", err)
	}
	if d2 := mustPut(t, e, "gb", "Intro. "+v1); d2.Ver != 2 {
		t.Fatalf("ver %d", d2.Ver)
	}
	old, _ := e.Doc("gb", 1)
	if old.Text != v1 {
		t.Fatal("version 1 changed after version 2 was added")
	}
	if _, err := e.Doc("gb", 3); !errors.Is(err, ErrNoDoc) {
		t.Fatalf("got %v", err)
	}
	for _, bad := range []string{"", "Has Space", "../x", strings.Repeat("a", 70)} {
		if _, err := e.PutDoc(bad, "t", "some text"); !errors.Is(err, ErrInvalid) {
			t.Errorf("id %q accepted", bad)
		}
	}
}

func TestCheckTextGradesEachQuote(t *testing.T) {
	e := newEng(t)
	mustPut(t, e, "gb", v1)
	report := `The address says "our fathers brought forth on this continent, a new nation" and also ` +
		`"testing whether that nation, or any nation so conceived and so dedicated, can not long endure" ` +
		`while another passage reads "the committee shall review every budget line each quarter".`
	got, err := e.CheckText(report, Scope{})
	if err != nil || len(got) != 3 {
		t.Fatalf("%v %+v", err, got)
	}
	want := []verify.Verdict{verify.Verbatim, verify.Drifted, verify.Unsupported}
	for i, f := range got {
		if f.Result.Verdict != want[i] {
			t.Errorf("quote %d: %s, want %s", i, f.Result.Verdict, want[i])
		}
	}
	if got[1].Result.Severity != verify.Material {
		t.Errorf("negation flip should be material, got %q", got[1].Result.Severity)
	}
}

func TestNoSource(t *testing.T) {
	e := newEng(t)
	r, err := e.Check("any quote here that is long enough", Scope{})
	if err != nil || r.Verdict != verify.NoSource {
		t.Fatalf("%v %+v", err, r)
	}
}

func TestFollowCiteLifecycle(t *testing.T) {
	e := newEng(t)
	mustPut(t, e, "gb", v1)
	c, err := e.AddCite("essay.md", "gb", 0, passage, "follow")
	if err != nil || c.Status != "intact" {
		t.Fatalf("%v %+v", err, c)
	}
	// 2: text added in front, the passage moves
	mustPut(t, e, "gb", "A short foreword. "+v1)
	cs, _ := e.Cites("gb")
	if cs[0].Status != "moved" || cs[0].Now != passage {
		t.Fatalf("moved: %+v", cs[0])
	}
	// 3: a word changes, the ghost diff appears
	mustPut(t, e, "gb", strings.Replace("A short foreword. "+v1, "that all men", "that most men", 1))
	cs, _ = e.Cites("gb")
	if cs[0].Status != "drifted" || len(cs[0].Ops) == 0 {
		t.Fatalf("drifted: %+v", cs[0])
	}
	var swapped bool
	for _, o := range cs[0].Ops {
		swapped = swapped || o.Kind == "sub" && o.Q == "all" && o.S == "most"
	}
	if !swapped {
		t.Fatalf("ops miss all->most: %+v", cs[0].Ops)
	}
	// 4: the sentence is gone
	mustPut(t, e, "gb", "Nothing of the old words is left in this text at all, not one of them.")
	cs, _ = e.Cites("gb")
	if cs[0].Status != "orphaned" {
		t.Fatalf("orphaned: %+v", cs[0])
	}
}

func TestPinnedCiteStaysPut(t *testing.T) {
	e := newEng(t)
	mustPut(t, e, "gb", v1)
	e.AddCite("essay.md", "gb", 0, passage, "pinned")
	mustPut(t, e, "gb", "Completely rewritten text with no trace of the earlier wording whatsoever.")
	cs, _ := e.Cites("gb")
	if cs[0].Status != "intact" || cs[0].AtVer != 1 {
		t.Fatalf("%+v", cs[0])
	}
}

func TestRepeatedPassagePicksByContext(t *testing.T) {
	e := newEng(t)
	a := "First the rule: all men are created equal and free. Later, the preamble: all men are created equal in rights."
	mustPut(t, e, "r", a)
	c, err := e.AddCite("x", "r", 0, "Later, the preamble: all men are created equal", "follow")
	if err != nil {
		t.Fatal(err)
	}
	_ = c
	e.AddCite("y", "r", 0, "First the rule: all men are created equal", "follow")
	mustPut(t, e, "r", "Added opening line. "+a)
	cs, _ := e.Cites("r")
	for _, c := range cs {
		if c.Status != "moved" || c.Now != c.Exact {
			t.Errorf("%s: %+v", c.Label, c)
		}
	}
}

func TestHaunts(t *testing.T) {
	e := newEng(t)
	mustPut(t, e, "gb", v1)
	e.AddCite("essay.md", "gb", 0, passage, "pinned")
	e.AddCite("talk.md", "gb", 0, "can long endure", "follow")
	i := strings.Index(v1, passage)
	hit, err := e.Haunts("gb", 1, i+5, i+20)
	if err != nil || len(hit) != 1 || hit[0].Label != "essay.md" {
		t.Fatalf("%v %+v", err, hit)
	}
	if hit, _ := e.Haunts("gb", 1, 0, 4); len(hit) != 0 {
		t.Fatalf("nothing quotes the first words: %+v", hit)
	}
	// after an edit in front, a pinned cite still haunts the same words in v2
	mustPut(t, e, "gb", "Foreword. "+v1)
	j := strings.Index("Foreword. "+v1, passage)
	hit, _ = e.Haunts("gb", 2, j+5, j+20)
	if len(hit) != 1 || hit[0].Label != "essay.md" {
		t.Fatalf("%+v", hit)
	}
	if _, err := e.Haunts("gb", 1, 5, 5); !errors.Is(err, ErrInvalid) {
		t.Fatalf("empty range: %v", err)
	}
}

func TestCiteRejectsUnsupportedQuote(t *testing.T) {
	e := newEng(t)
	mustPut(t, e, "gb", v1)
	if _, err := e.AddCite("x", "gb", 0, "the committee shall review every budget line", "pinned"); !errors.Is(err, ErrNoMatch) {
		t.Fatalf("got %v", err)
	}
}

func TestReceiptEndToEnd(t *testing.T) {
	e := newEng(t)
	mustPut(t, e, "gb", v1)
	env, id, err := e.Issue("our fathers brought forth on this continent", "gb", 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := receipt.Verify(env); err != nil {
		t.Fatal(err)
	}
	if err := receipt.CheckSource(env, []byte(v1)); err != nil {
		t.Fatal(err)
	}
	if err := receipt.CheckSource(env, []byte(v1+" ")); err == nil {
		t.Fatal("a different source must not match")
	}
	back, err := e.GetReceipt(id)
	if err != nil || back.Sig != env.Sig {
		t.Fatalf("%v", err)
	}
	if _, _, err := e.Issue("the committee shall review every budget line", "gb", 0); !errors.Is(err, ErrNoMatch) {
		t.Fatalf("got %v", err)
	}
}

func TestTrailReplayShowsChange(t *testing.T) {
	e := newEng(t)
	mustPut(t, e, "gb", v1)
	tr, err := e.NewTrail("Equality claim")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.AddStep(tr.ID, "The speech grounds equality in its founding", passage, "gb", 0); err != nil {
		t.Fatal(err)
	}
	rp, err := e.Replay(tr.ID)
	if err != nil || len(rp) != 1 || !rp[0].SigOK || rp[0].Changed {
		t.Fatalf("%v %+v", err, rp)
	}
	mustPut(t, e, "gb", strings.Replace(v1, "that all men", "that most men", 1))
	rp, _ = e.Replay(tr.ID)
	if !rp[0].Changed || rp[0].Then.Verdict != verify.Verbatim || rp[0].Now.Verdict != verify.Drifted {
		t.Fatalf("%+v", rp[0])
	}
}

func TestIntegrity(t *testing.T) {
	dir := t.TempDir()
	e, _ := New(dir)
	d := mustPut(t, e, "gb", v1)
	if err := e.Integrity(); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "blobs", d.Blob)
	os.Chmod(p, 0o644)
	os.WriteFile(p, []byte("forged"), 0o644)
	if err := e.Integrity(); err == nil {
		t.Fatal("forged source blob not detected")
	}
	if _, err := e.Doc("gb", 1); err == nil {
		t.Fatal("forged blob must not be served")
	}
}

func TestKeyPersists(t *testing.T) {
	dir := t.TempDir()
	a, _ := New(dir)
	b, _ := New(dir)
	if a.KeyID() != b.KeyID() {
		t.Fatal("signing key changed between opens")
	}
}

// Property: however the surrounding text is edited, a cite whose words are
// still there resolves to exactly those words.
func TestPropCiteSurvivesUnrelatedEdits(t *testing.T) {
	prop.Check(t, 60, func(r *rand.Rand) error {
		e := newEng(t)
		words := prop.Words(r, 60)
		n := 5 + r.Intn(6)
		i := 10 + r.Intn(30)
		q := strings.Join(words[i:i+n], " ")
		base := strings.Join(words, " ")
		if _, err := e.PutDoc("s", "s", base); err != nil {
			return err
		}
		c, err := e.AddCite("n", "s", 0, q, "follow")
		if err != nil {
			return err
		}
		edited := strings.Join(prop.Words(r, 1+r.Intn(8)), " ") + " " + base + " " + strings.Join(prop.Words(r, 1+r.Intn(8)), " ")
		if _, err := e.PutDoc("s", "s", edited); err != nil {
			return err
		}
		cs, _ := e.Cites("s")
		got := cs[0]
		if got.Status != "intact" && got.Status != "moved" || got.Now != c.Exact {
			return fmt.Errorf("status %s, now %q, want %q", got.Status, got.Now, c.Exact)
		}
		return nil
	})
}
