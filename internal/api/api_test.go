package api

import (
	"encoding/json"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/111Reihan/kiro/internal/engine"
	"github.com/111Reihan/kiro/internal/ledger"
	"github.com/111Reihan/kiro/internal/spec"
)

const reqs = "### Requirement 1\n\n1. WHEN a user submits a form THEN the system SHALL respond within 2 seconds.\n"
const tst = "package a\n\n// covers: 1.1\nfunc TestLat(t *testing.T) {\n\t_ = 1\n}\n"

func setup(t *testing.T, o Options) (http.Handler, *ledger.Ledger) {
	root := t.TempDir()
	for rel, s := range map[string]string{".kiro/specs/perf/requirements.md": reqs, "a_test.go": tst} {
		p := filepath.Join(root, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	e, err := engine.New(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	l := ledger.New(e)
	rs, _ := spec.ParseResults([]byte(`{"Action":"pass","Package":"p/a","Test":"TestLat"}`))
	if _, err := l.AddRun(rs, "go test"); err != nil {
		t.Fatal(err)
	}
	o.Root = root
	return Handler(l, o), l
}

func do(h http.Handler, method, path, body string, hdr ...string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if method == "POST" {
		r.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		r.Header.Set(hdr[i], hdr[i+1])
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// covers: specter-core/4.2
func TestSealThenPreviewWritesNothing(t *testing.T) {
	h, l := setup(t, Options{})
	if w := do(h, "POST", "/api/seal", `{}`); w.Code != 200 {
		t.Fatalf("seal: %d %s", w.Code, w.Body)
	}
	n, _ := l.E.St.Head()
	w := do(h, "POST", "/api/preview", `{"id":"perf/1.1","text":"WHEN a user submits a form THEN the system SHALL respond within 5 seconds."}`)
	if w.Code != 200 {
		t.Fatalf("preview: %d %s", w.Code, w.Body)
	}
	var out previewOut
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Before != ledger.Verified || out.After != ledger.Stale || out.Change == nil || out.Change.Severity != "material" {
		t.Fatalf("preview: %+v", out)
	}
	if m, _ := l.E.St.Head(); m != n {
		t.Fatal("preview wrote to the log")
	}
	// the real project is untouched
	var rep ledger.Report
	json.Unmarshal(do(h, "GET", "/api/project", "").Body.Bytes(), &rep)
	if rep.Counts[ledger.Verified] != 1 {
		t.Fatalf("counts after preview: %v", rep.Counts)
	}
}

// covers: specter-core/4.2
func TestReadOnlyRefusesWrites(t *testing.T) {
	h, _ := setup(t, Options{ReadOnly: true})
	for _, p := range []string{"/api/seal", "/api/rescan"} {
		if w := do(h, "POST", p, `{}`); w.Code != 403 {
			t.Errorf("%s: %d", p, w.Code)
		}
	}
	if w := do(h, "POST", "/api/preview", `{"id":"perf/1.1","text":"The system SHALL x."}`); w.Code != 200 {
		t.Errorf("preview must work read-only: %d", w.Code)
	}
}

// covers: specter-core/4.3
func TestGuards(t *testing.T) {
	h, _ := setup(t, Options{Loopback: true})
	if w := do(h, "GET", "/api/info", ""); w.Code != 403 { // httptest host is example.com
		t.Errorf("foreign host: %d", w.Code)
	}
	r := httptest.NewRequest("POST", "http://localhost:8080/api/seal", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 415 {
		t.Errorf("form post without JSON type: %d", w.Code)
	}
	h, _ = setup(t, Options{})
	big := `{"id":"x","text":"` + strings.Repeat("a", 2<<20) + `"}`
	if w := do(h, "POST", "/api/preview", big); w.Code != 413 {
		t.Errorf("oversize: %d", w.Code)
	}
	if w := do(h, "POST", "/api/preview", `{"id":"x","text":"y","extra":1}`); w.Code != 400 {
		t.Errorf("unknown field: %d", w.Code)
	}
	if w := do(h, "POST", "/api/preview", `{"id":"nope","text":"The system SHALL x."}`); w.Code != 404 {
		t.Errorf("unknown requirement: %d", w.Code)
	}
	if got := do(h, "GET", "/api/info", "").Header().Get("Content-Security-Policy"); !strings.Contains(got, "script-src 'self'") {
		t.Errorf("CSP: %q", got)
	}
}

// covers: specter-core/3.3
func TestVerifyEndpointCatchesEdit(t *testing.T) {
	h, _ := setup(t, Options{})
	do(h, "POST", "/api/seal", `{}`)
	w := do(h, "GET", "/api/attestation?id=perf/1.1", "")
	if w.Code != 200 {
		t.Fatalf("attestation: %d", w.Code)
	}
	var ok map[string]any
	json.Unmarshal(do(h, "POST", "/api/attestation/verify", w.Body.String()).Body.Bytes(), &ok)
	if ok["ok"] != true {
		t.Fatalf("clean attestation rejected: %v", ok)
	}
	bad := strings.Replace(w.Body.String(), "within 2 seconds", "within 20 seconds", 1)
	var no map[string]any
	json.Unmarshal(do(h, "POST", "/api/attestation/verify", bad).Body.Bytes(), &no)
	if no["ok"] != false {
		t.Fatalf("edited attestation accepted: %v", no)
	}
	if w := do(h, "GET", "/api/attestation?id=nope", ""); w.Code != 404 {
		t.Errorf("missing seal: %d", w.Code)
	}
}

// The server may read the tree and the log. It must never be able to start a process.
// covers: specter-core/4.1
func TestServerNeverExecs(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool { return !strings.HasSuffix(fi.Name(), "_test.go") }, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range pkgs {
		for name, f := range p.Files {
			for _, im := range f.Imports {
				if path := strings.Trim(im.Path.Value, `"`); path == "os/exec" || path == "syscall" || path == "plugin" {
					t.Errorf("%s imports %s", name, path)
				}
			}
		}
	}
}
