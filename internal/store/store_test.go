package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func seed(t *testing.T, n int) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		if _, err := s.Append("note", map[string]any{"i": i, "text": "line <&> " + strings.Repeat("x", i)}); err != nil {
			t.Fatal(err)
		}
	}
	return s, filepath.Join(dir, "revenant.log")
}

func lines(t *testing.T, p string) []string {
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimRight(string(b), "\n"), "\n")
}

func write(t *testing.T, p string, ls []string) {
	if err := os.WriteFile(p, []byte(strings.Join(ls, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestChainHolds(t *testing.T) {
	s, p := seed(t, 4)
	if _, err := ReadLog(p); err != nil {
		t.Fatal(err)
	}
	if err := s.Verify(); err != nil {
		t.Fatal(err)
	}
}

func TestChainDetectsEditDeleteSwap(t *testing.T) {
	_, p := seed(t, 4)
	orig := lines(t, p)
	cases := map[string][]string{
		"edit":            {orig[0], strings.Replace(orig[1], `"i":1`, `"i":9`, 1), orig[2], orig[3]},
		"delete":          {orig[0], orig[2], orig[3]},
		"swap":            {orig[0], orig[2], orig[1], orig[3]},
		"truncate-middle": {orig[0], orig[1][:len(orig[1])-5], orig[2], orig[3]},
	}
	for name, ls := range cases {
		write(t, p, ls)
		if _, err := ReadLog(p); err == nil {
			t.Errorf("%s: tampering went unnoticed", name)
		}
	}
}

func TestBlobTamperDetected(t *testing.T) {
	s, _ := seed(t, 0)
	h, _ := s.PutBlob([]byte("original text"))
	if _, err := s.Append("doc.version", map[string]string{"blob": h}); err != nil {
		t.Fatal(err)
	}
	bp := filepath.Join(s.dir, "blobs", h)
	os.Chmod(bp, 0o644)
	os.WriteFile(bp, []byte("forged text"), 0o644)
	if _, err := s.GetBlob(h); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("got %v", err)
	}
	if err := s.Verify(); err == nil {
		t.Fatal("Verify missed a forged blob")
	}
}

func TestTornTailIsDropped(t *testing.T) {
	s, p := seed(t, 2)
	f, _ := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0o644)
	f.WriteString(`{"seq":3,"ts":"2026-`) // crash mid-write, no newline
	f.Close()
	s2, err := Open(s.dir)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(s2.Events()); n != 2 {
		t.Fatalf("expected 2 events, got %d", n)
	}
	if _, err := s2.Append("note", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadLog(p); err != nil {
		t.Fatalf("log not clean after repair: %v", err)
	}
}

func TestTwoWritersOneLog(t *testing.T) {
	a, p := seed(t, 0)
	b, err := Open(a.dir)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		w := []*Store{a, b}[i%2]
		if _, err := w.Append("note", i); err != nil {
			t.Fatal(err)
		}
	}
	a.Sync()
	b.Sync()
	if len(a.Events()) != 6 || len(b.Events()) != 6 {
		t.Fatalf("a=%d b=%d", len(a.Events()), len(b.Events()))
	}
	if _, err := ReadLog(p); err != nil {
		t.Fatal(err)
	}
}

func TestRedaction(t *testing.T) {
	s, _ := seed(t, 0)
	h, _ := s.PutBlob([]byte("remove me"))
	s.Append("doc.version", map[string]string{"blob": h})
	if err := s.Redact(h, "requested"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetBlob(h); !errors.Is(err, ErrRedacted) {
		t.Fatalf("got %v", err)
	}
	if err := s.Verify(); err != nil {
		t.Fatalf("redaction should leave the log valid: %v", err)
	}
}

func TestBlobNamesAreValidated(t *testing.T) {
	s, _ := seed(t, 0)
	for _, bad := range []string{"", "../x", strings.Repeat("g", 64), strings.Repeat("a", 63)} {
		if _, err := s.GetBlob(bad); !errors.Is(err, ErrNotFound) {
			t.Errorf("%q: got %v", bad, err)
		}
	}
}
