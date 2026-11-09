package spec

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/111Reihan/kiro/internal/prop"
)

// covers: specter-core/1.1
func TestParseRequirementStyles(t *testing.T) {
	src := "# Reqs\n\n**R1** The app SHALL log in users.\n- **R2**: WHEN idle THEN the app SHALL lock.\nREQ-3: The app SHALL NOT leak keys,\n  even in logs.\n\n```\n**R9** inside a code fence\n```\n"
	rs := ParseRequirements("auth", "auth/requirements.md", src)
	var ids []string
	for _, r := range rs {
		ids = append(ids, r.ID)
	}
	if strings.Join(ids, ",") != "auth/R1,auth/R2,auth/REQ-3" {
		t.Fatalf("ids: %v", ids)
	}
	if !strings.Contains(rs[2].Text, "even in logs") {
		t.Fatalf("continuation line lost: %q", rs[2].Text)
	}
	if rs[1].Kind != "event" {
		t.Fatalf("WHEN..THEN must classify as event, got %q", rs[1].Kind)
	}
}

// covers: specter-core/1.1
func TestParseKiroNumbering(t *testing.T) {
	src := "### Requirement 2\n\n1. The system SHALL a.\n2. The system SHALL b.\n\n### Requirement 3\n\n1. The system SHALL c.\n"
	rs := ParseRequirements("x", "x.md", src)
	var ids []string
	for _, r := range rs {
		ids = append(ids, r.Local)
	}
	if strings.Join(ids, ",") != "2.1,2.2,3.1" {
		t.Fatalf("local ids: %v", ids)
	}
}

func TestLint(t *testing.T) {
	cases := map[string]string{
		"The system should be fast.":                       "weak-verb",
		"The system is fast.":                              "no-shall",
		"The system SHALL be user-friendly and efficient.": "vague",
		"The system SHALL respond within a short time.":    "unquantified",
	}
	for text, want := range cases {
		_, iss := Classify(text)
		found := false
		for _, i := range iss {
			found = found || i.Code == want
		}
		if !found {
			t.Errorf("%q: want %s, got %+v", text, want, iss)
		}
	}
	if _, iss := Classify("WHEN a user submits a form THEN the system SHALL respond within 2 seconds."); len(iss) != 0 {
		t.Errorf("clean requirement flagged: %+v", iss)
	}
}

// covers: specter-core/1.2
func TestParseTestsAcrossLanguages(t *testing.T) {
	cases := []struct{ file, src, name string }{
		{"a_test.go", "// covers: R1\nfunc TestA(t *testing.T) {\n}\n", "TestA"},
		{"a.py", "# covers: R1\ndef test_a():\n    pass\n", "test_a"},
		{"a.test.ts", "// covers: R1\nit('does a thing', () => {})\n", "does a thing"},
		{"a.test.js", "// covers: R1\ntest(\"double quoted\", () => {})\n", "double quoted"},
		{"A.java", "// covers: R1\n@Test\nvoid checksA() {\n}\n", "checksA"},
		{"a.rs", "// covers: R1\n#[test]\nfn rusty() {\n}\n", "rusty"},
	}
	for _, c := range cases {
		ts := ParseTests(c.file, c.src)
		if len(ts) != 1 || ts[0].Name != c.name || len(ts[0].raw) != 1 || ts[0].raw[0] != "R1" {
			t.Errorf("%s: %+v", c.file, ts)
		}
	}
	if ts := ParseTests("b_test.go", "func TestNoNote(t *testing.T) {}\n"); len(ts) != 0 {
		t.Errorf("a test with no annotation must be ignored: %+v", ts)
	}
}

// covers: specter-core/2.3
func TestBodyHashIgnoresComments(t *testing.T) {
	a := ParseTests("a_test.go", "// covers: R1\nfunc TestA(t *testing.T) {\n\tx := 1\n\t_ = x\n}\n")
	b := ParseTests("a_test.go", "// covers: R1\nfunc TestA(t *testing.T) {\n\t// a new comment\n\tx := 1\n\t_ = x\n}\n")
	c := ParseTests("a_test.go", "// covers: R1\nfunc TestA(t *testing.T) {\n\tx := 2\n\t_ = x\n}\n")
	if a[0].Body != b[0].Body {
		t.Error("adding a comment changed the body hash")
	}
	if a[0].Body == c[0].Body {
		t.Error("changing code did not change the body hash")
	}
}

// covers: specter-core/1.3
func TestResults(t *testing.T) {
	goOut := `{"Action":"run","Package":"p/a","Test":"TestX"}
{"Action":"pass","Package":"p/a","Test":"TestX"}
{"Action":"fail","Package":"p/b","Test":"TestX"}
{"Action":"skip","Package":"p/a","Test":"TestY"}
{"Action":"pass","Package":"p/a"}
`
	rs, err := ParseResults([]byte(goOut))
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 3 {
		t.Fatalf("package-level lines must be ignored: %+v", rs)
	}
	// same name in two packages: the folder decides, and a failure wins when it cannot
	if s := Status(Test{Name: "TestX", File: "a/x_test.go"}, rs); s != Pass {
		t.Errorf("folder match: %s", s)
	}
	if s := Status(Test{Name: "TestX", File: "z/x_test.go"}, rs); s != Fail {
		t.Errorf("ambiguous name must fail: %s", s)
	}
	if s := Status(Test{Name: "TestZ"}, rs); s != "none" {
		t.Errorf("unknown test: %s", s)
	}
	junit := `<testsuites><testsuite name="s"><testcase classname="c" name="ok"/><testcase classname="c" name="bad"><failure message="x"/></testcase></testsuite></testsuites>`
	rs, err = ParseResults([]byte(junit))
	if err != nil || len(rs) != 2 || rs[1].Status != Fail {
		t.Fatalf("junit: %+v %v", rs, err)
	}
	if _, err := ParseResults([]byte("hello")); err == nil {
		t.Error("garbage must be rejected")
	}
}

// covers: specter-core/1.2
func TestScanResolution(t *testing.T) {
	root := t.TempDir()
	w := func(rel, s string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	w("specs/a/requirements.md", "**R1** The a SHALL x.\n")
	w("specs/b/requirements.md", "**R1** The b SHALL y.\n**R2** The b SHALL z.\n")
	w("specs/b/tasks.md", "- [x] do it\n  _Requirements: R2, R9_\n")
	w("node_modules/q/requirements.md", "**R1** ignored.\n")
	w("t_test.go", "// covers: R1\nfunc TestAmb(t *testing.T) {}\n// covers: b/R1, R2\nfunc TestOK(t *testing.T) {}\n")
	p, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Reqs) != 3 {
		t.Fatalf("skipped dir was read: %d reqs", len(p.Reqs))
	}
	var amb, ok Test
	for _, x := range p.Tests {
		if x.Name == "TestAmb" {
			amb = x
		} else {
			ok = x
		}
	}
	if len(amb.Covers) != 0 {
		t.Errorf("ambiguous R1 must not resolve: %+v", amb.Covers)
	}
	if strings.Join(ok.Covers, ",") != "b/R1,b/R2" {
		t.Errorf("covers: %v", ok.Covers)
	}
	if len(p.Tasks) != 1 || strings.Join(p.Tasks[0].Reqs, ",") != "b/R2" || !p.Tasks[0].Done {
		t.Errorf("tasks: %+v", p.Tasks)
	}
	warn := strings.Join(p.Warnings, "\n")
	if !strings.Contains(warn, "ambiguous") || !strings.Contains(warn, "R9") {
		t.Errorf("warnings missing: %q", warn)
	}
}

// Case, spacing and trailing punctuation never change a requirement's hash.
func TestWordHashStable(t *testing.T) {
	prop.Check(t, 300, func(r *rand.Rand) error {
		ws := prop.Words(r, 3+r.Intn(12))
		a := strings.Join(ws, " ")
		var b []string
		for _, w := range ws {
			if r.Intn(2) == 0 {
				w = strings.ToUpper(w)
			}
			b = append(b, w)
		}
		bs := strings.Join(b, strings.Repeat(" ", 1+r.Intn(3))) + " "
		if WordHash(a) != WordHash(bs) {
			return errf("hash differs for %q vs %q", a, bs)
		}
		return nil
	})
}

func errf(f string, a ...any) error { return &perr{strings.TrimSpace(fmt.Sprintf(f, a...))} }

type perr struct{ s string }

func (e *perr) Error() string { return e.s }

// covers: specter-core/1.2
func TestFixturesInRawStringsAreNotTests(t *testing.T) {
	src := "package x\n\nconst fixture = `\n// covers: R1\nfunc TestInside(t *testing.T) {}\n`\n\n// covers: R2\nfunc TestReal(t *testing.T) {}\n"
	ts := ParseTests("x_test.go", src)
	if len(ts) != 1 || ts[0].Name != "TestReal" {
		t.Fatalf("%+v", ts)
	}
}
