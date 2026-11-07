package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type result struct {
	code     int
	out, err string
}

func do(t *testing.T, stdin string, args ...string) result {
	t.Helper()
	var out, errb bytes.Buffer
	code := run(args, strings.NewReader(stdin), &out, &errb)
	return result{code, out.String(), errb.String()}
}

const source = "Four score and seven years ago our fathers brought forth on this continent, a new nation, conceived in Liberty, and dedicated to the proposition that all men are created equal."

func TestCommandLineFlow(t *testing.T) {
	dir := t.TempDir()
	data := filepath.Join(dir, "data")
	src := filepath.Join(dir, "g.txt")
	os.WriteFile(src, []byte(source), 0o644)

	if r := do(t, "", "add", "-data", data, "-title", "Gettysburg", "g", src); r.code != 0 || !strings.Contains(r.out, "version 1") {
		t.Fatalf("add: %+v", r)
	}
	if r := do(t, "", "add", "-data", data, "g", src); r.code != 0 || !strings.Contains(r.out, "unchanged") {
		t.Fatalf("add again: %+v", r)
	}

	good := `He said "Four score and seven years ago our fathers brought forth on this continent" first.`
	if r := do(t, good, "check", "-data", data, "-"); r.code != 0 || !strings.Contains(r.out, "verbatim") {
		t.Fatalf("check good: %+v", r)
	}
	bad := good + ` Then "a nation that never forgets will always remember its own founders."`
	r := do(t, bad, "check", "-data", data, "-")
	if r.code != 1 || !strings.Contains(r.out, "unsupported") || !strings.Contains(r.out, "1 of 2 quotes are not verbatim") {
		t.Fatalf("check bad: %+v", r)
	}
	if r := do(t, bad, "check", "-data", data, "-json", "-"); r.code != 1 || !strings.Contains(r.out, `"not_verbatim": 1`) {
		t.Fatalf("check -json: %+v", r)
	}
	if r := do(t, "no quotes here", "check", "-data", data, "-"); r.code != 0 {
		t.Fatalf("check without quotes: %+v", r)
	}

	// receipt, then verify it offline: no -data needed
	r = do(t, "", "receipt", "-data", data, "-doc", "g", "Four score and seven years ago our fathers brought forth")
	if r.code != 0 || !strings.Contains(r.err, "receipt id") {
		t.Fatalf("receipt: %+v", r)
	}
	rf := filepath.Join(dir, "r.json")
	os.WriteFile(rf, []byte(r.out), 0o644)
	if v := do(t, "", "verify-receipt", "-source", src, rf); v.code != 0 || !strings.Contains(v.out, "NOT checked") {
		t.Fatalf("verify-receipt: %+v", v)
	}
	key := do(t, "", "key", "-data", data)
	pub := strings.TrimSpace(key.out[strings.LastIndex(key.out, " ")+1:])
	if v := do(t, "", "verify-receipt", "-pin", pub, rf); v.code != 0 || !strings.Contains(v.out, "pinned key") {
		t.Fatalf("pinned: %+v", v)
	}
	if v := do(t, "", "verify-receipt", "-pin", "AAAA", rf); v.code != 1 {
		t.Fatalf("wrong pin must fail: %+v", v)
	}
	other := filepath.Join(dir, "other.txt")
	os.WriteFile(other, []byte(source+" x"), 0o644)
	if v := do(t, "", "verify-receipt", "-source", other, rf); v.code != 1 {
		t.Fatalf("wrong source must fail: %+v", v)
	}
	tampered := strings.Replace(r.out, "Four score", "Five score", 1)
	if v := do(t, tampered, "verify-receipt", "-"); v.code != 1 || !strings.Contains(v.out, "FAIL") {
		t.Fatalf("tampered receipt must fail: %+v", v)
	}
	if rr := do(t, "", "receipt", "-data", data, "-doc", "g", "this quote is not in that source"); rr.code != 1 {
		t.Fatalf("receipt for missing quote: %+v", rr)
	}

	if a := do(t, "", "audit", "-data", data); a.code != 0 || !strings.Contains(a.out, "OK") {
		t.Fatalf("audit: %+v", a)
	}
	// edit one byte of the log: audit must notice
	lp := filepath.Join(data, "revenant.log")
	b, _ := os.ReadFile(lp)
	os.WriteFile(lp, bytes.Replace(b, []byte("Gettysburg"), []byte("Gettysburh"), 1), 0o644)
	if a := do(t, "", "audit", "-data", data); a.code != 1 || !strings.Contains(a.out, "FAIL") {
		t.Fatalf("audit of edited log: %+v", a)
	}
}

func TestUsageErrors(t *testing.T) {
	for _, args := range [][]string{{}, {"nope"}, {"add", "only-one-arg"}, {"check"}, {"receipt", "q"}} {
		if r := do(t, "", args...); r.code != 2 {
			t.Errorf("%v: exit %d, want 2 (%+v)", args, r.code, r)
		}
	}
	if r := do(t, "", "version"); r.code != 0 || !strings.Contains(r.out, version) {
		t.Errorf("version: %+v", r)
	}
	if r := do(t, "", "help"); r.code != 0 || !strings.Contains(r.out, "usage:") {
		t.Errorf("help: %+v", r)
	}
}
