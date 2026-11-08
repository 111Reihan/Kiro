package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/111Reihan/kiro/internal/engine"
	"github.com/111Reihan/kiro/internal/receipt"
)

func newServer(t *testing.T, loopback bool) *httptest.Server {
	t.Helper()
	e, err := engine.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(Handler(e, Options{Loopback: loopback}))
	t.Cleanup(ts.Close)
	return ts
}

// raw sends one request and returns status and body.
func raw(t *testing.T, ts *httptest.Server, method, path, ctype string, body io.Reader) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, ts.URL+path, body)
	if err != nil {
		t.Fatal(err)
	}
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, b
}

// call sends JSON, checks the status, and decodes the reply into T.
func call[T any](t *testing.T, ts *httptest.Server, method, path string, in any, want int) T {
	t.Helper()
	var body io.Reader
	if in != nil {
		b, _ := json.Marshal(in)
		body = bytes.NewReader(b)
	}
	code, b := raw(t, ts, method, path, "application/json", body)
	if code != want {
		t.Fatalf("%s %s: status %d, want %d: %s", method, path, code, want, b)
	}
	var out T
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("%s %s: bad JSON %q: %v", method, path, b, err)
	}
	return out
}

type errBody struct {
	Error string `json:"error"`
}

func TestDemoThenCheckReport(t *testing.T) {
	ts := newServer(t, false)
	d := call[struct {
		Loaded []string `json:"loaded"`
		Report string   `json:"report"`
	}](t, ts, "POST", "/api/demo", struct{}{}, 200)
	if len(d.Loaded) != 2 || !strings.Contains(d.Report, "Gettysburg") {
		t.Fatalf("demo: %+v", d)
	}
	call[struct{}](t, ts, "POST", "/api/demo", struct{}{}, 200) // loading twice is fine

	r := call[struct {
		Found  []engine.Found `json:"found"`
		Counts map[string]int `json:"counts"`
	}](t, ts, "POST", "/api/check-text", map[string]any{"text": d.Report}, 200)
	want := map[string]int{"verbatim": 3, "drifted": 2, "unsupported": 1, "material": 1}
	for k, v := range want {
		if r.Counts[k] != v {
			t.Errorf("counts[%s] = %d, want %d (all: %v)", k, r.Counts[k], v, r.Counts)
		}
	}
	if len(r.Found) != 6 {
		t.Errorf("found %d quotes, want 6", len(r.Found))
	}
}

// The ghost story: cite a passage, change the source, see the cite drift,
// see the receipt and the trail step still hold their old version.
func TestCiteEditReceiptTrail(t *testing.T) {
	ts := newServer(t, false)
	call[struct{}](t, ts, "POST", "/api/demo", struct{}{}, 200)
	quote := "Four score and seven years ago our fathers brought forth on this continent"

	c := call[engine.CiteView](t, ts, "POST", "/api/cites",
		map[string]any{"label": "essay, para 1", "doc": "gettysburg", "quote": quote, "mode": "follow"}, 201)
	if c.Status != "intact" || c.Ver != 1 {
		t.Fatalf("new cite: %+v", c.Loc)
	}

	rc := call[struct {
		ID      string           `json:"id"`
		Receipt receipt.Envelope `json:"receipt"`
	}](t, ts, "POST", "/api/receipts", map[string]any{"quote": quote, "doc": "gettysburg"}, 201)
	if len(rc.ID) != 64 {
		t.Fatalf("receipt id %q", rc.ID)
	}
	tr := call[engine.Trail](t, ts, "POST", "/api/trails", map[string]any{"title": "Opening line"}, 201)
	call[engine.Step](t, ts, "POST", "/api/trails/"+tr.ID+"/steps",
		map[string]any{"claim": "Lincoln dated the nation from 1776", "quote": quote, "doc": "gettysburg"}, 201)

	// edit the source: "fathers" becomes "forefathers"
	v1 := call[engine.DocVersion](t, ts, "GET", "/api/docs/gettysburg", nil, 200)
	edited := strings.Replace(v1.Text, "our fathers", "our forefathers", 1)
	v2 := call[engine.DocVersion](t, ts, "POST", "/api/docs", map[string]any{"id": "gettysburg", "text": edited}, 201)
	if v2.Ver != 2 {
		t.Fatalf("version %d, want 2", v2.Ver)
	}
	call[errBody](t, ts, "POST", "/api/docs", map[string]any{"id": "gettysburg", "text": edited}, 409)

	cs := call[[]engine.CiteView](t, ts, "GET", "/api/cites?doc=gettysburg", nil, 200)
	if len(cs) != 1 || cs[0].Status != "drifted" || len(cs[0].Ops) == 0 {
		t.Fatalf("cite after edit: %+v", cs)
	}
	hs := call[[]engine.CiteView](t, ts, "GET", "/api/haunts?doc=gettysburg&start=0&end=30", nil, 200)
	if len(hs) != 1 {
		t.Fatalf("haunts: %d, want 1", len(hs))
	}
	hs = call[[]engine.CiteView](t, ts, "GET", "/api/haunts?doc=gettysburg&start=400&end=410", nil, 200)
	if len(hs) != 0 {
		t.Fatalf("haunts far away: %d, want 0", len(hs))
	}

	steps := call[[]engine.StepReplay](t, ts, "GET", "/api/trails/"+tr.ID+"/replay", nil, 200)
	if len(steps) != 1 || !steps[0].SigOK || !steps[0].Changed {
		t.Fatalf("replay: %+v", steps)
	}

	// the receipt still verifies, offline and with its own version of the source
	vr := call[struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}](t, ts, "POST", "/api/receipts/verify", map[string]any{"receipt": rc.Receipt, "source": v1.Text, "pin": rc.Receipt.Pub}, 200)
	if !vr.OK {
		t.Fatalf("verify: %s", vr.Error)
	}
	vr = call[struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}](t, ts, "POST", "/api/receipts/verify", map[string]any{"receipt": rc.Receipt, "source": edited}, 200)
	if vr.OK {
		t.Fatal("receipt must not verify against the edited source")
	}
	rc.Receipt.Body.Quote += " and then some"
	vr = call[struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}](t, ts, "POST", "/api/receipts/verify", map[string]any{"receipt": rc.Receipt}, 200)
	if vr.OK {
		t.Fatal("tampered receipt verified")
	}
	vr = call[struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}](t, ts, "POST", "/api/receipts/verify", map[string]any{"receipt": rc.Receipt, "pin": "AAAA"}, 200)
	if vr.OK {
		t.Fatal("pin mismatch accepted")
	}

	evs := call[[]struct {
		Seq        int    `json:"seq"`
		Hash, Prev string `json:"hash"`
	}](t, ts, "GET", "/api/events", nil, 200)
	for i := 1; i < len(evs); i++ {
		if evs[i].Prev != evs[i-1].Hash || evs[i].Seq != evs[i-1].Seq+1 {
			t.Fatalf("events do not form a chain at %d: %+v", i, evs[i-1:i+1])
		}
	}

	au := call[struct {
		OK     bool `json:"ok"`
		Events int  `json:"events"`
	}](t, ts, "GET", "/api/audit", nil, 200)
	if !au.OK || au.Events < 6 {
		t.Fatalf("audit: %+v", au)
	}
	st := call[struct {
		Key  string `json:"key"`
		Docs int    `json:"docs"`
	}](t, ts, "GET", "/api/status", nil, 200)
	if len(st.Key) != 16 || st.Docs != 2 {
		t.Fatalf("status: %+v", st)
	}
}

func TestNoSourceBeforeAnyDocument(t *testing.T) {
	ts := newServer(t, false)
	r := call[struct {
		Verdict string `json:"verdict"`
	}](t, ts, "POST", "/api/check", map[string]any{"quote": "nothing is loaded to check this against"}, 200)
	if r.Verdict != "no-source" {
		t.Fatalf("verdict %q", r.Verdict)
	}
}

func TestErrorMapping(t *testing.T) {
	ts := newServer(t, false)
	call[struct{}](t, ts, "POST", "/api/demo", struct{}{}, 200)
	cases := []struct {
		method, path string
		body         any
		want         int
	}{
		{"GET", "/api/docs/nope", nil, 404},
		{"GET", "/api/docs/gettysburg?ver=9", nil, 404},
		{"GET", "/api/docs/gettysburg?ver=x", nil, 400},
		{"POST", "/api/docs", map[string]any{"id": "Bad Id", "text": "hello there"}, 400},
		{"POST", "/api/docs", map[string]any{"id": "ok", "text": "   "}, 400},
		{"POST", "/api/docs", map[string]any{"id": "ok", "text": "hi", "extra": 1}, 400},
		{"POST", "/api/cites", map[string]any{"label": "x", "doc": "gettysburg", "quote": "this sentence is not in the speech at all"}, 422},
		{"POST", "/api/cites", map[string]any{"label": "x", "doc": "gettysburg", "quote": "Four score and seven years ago", "mode": "wild"}, 400},
		{"POST", "/api/receipts", map[string]any{"quote": "this sentence is not in the speech at all", "doc": "gettysburg"}, 422},
		{"GET", "/api/haunts?doc=gettysburg&start=5&end=5", nil, 400},
		{"GET", "/api/trails/t999", nil, 404},
		{"GET", "/api/receipts/" + strings.Repeat("a", 64), nil, 404},
		{"GET", "/api/nope", nil, 404},
	}
	for _, c := range cases {
		e := call[errBody](t, ts, c.method, c.path, c.body, c.want)
		if e.Error == "" {
			t.Errorf("%s %s: empty error message", c.method, c.path)
		}
	}
}

func TestPostNeedsJSON(t *testing.T) {
	ts := newServer(t, false)
	code, _ := raw(t, ts, "POST", "/api/check", "text/plain", strings.NewReader(`{"quote":"x"}`))
	if code != 415 {
		t.Fatalf("text/plain POST: %d, want 415", code)
	}
	code, _ = raw(t, ts, "POST", "/api/check", "application/x-www-form-urlencoded", strings.NewReader("quote=x"))
	if code != 415 {
		t.Fatalf("form POST: %d, want 415", code)
	}
}

func TestBodyLimit(t *testing.T) {
	ts := newServer(t, false)
	big := `{"id":"big","text":"` + strings.Repeat("a", maxBody+10) + `"}`
	code, _ := raw(t, ts, "POST", "/api/docs", "application/json", strings.NewReader(big))
	if code != 413 {
		t.Fatalf("oversized body: %d, want 413", code)
	}
}

func TestLoopbackGuard(t *testing.T) {
	ts := newServer(t, true)
	req, _ := http.NewRequest("GET", ts.URL+"/api/status", nil)
	res, err := http.DefaultClient.Do(req) // Host is 127.0.0.1:port
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("loopback host refused: %v %v", err, res)
	}
	res.Body.Close()
	req.Host = "evil.example"
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 403 {
		t.Fatalf("foreign host: %d, want 403", res.StatusCode)
	}
}

func TestSecurityHeaders(t *testing.T) {
	ts := newServer(t, false)
	res, err := http.Get(ts.URL + "/api/status")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	for _, h := range []string{"Content-Security-Policy", "X-Content-Type-Options", "Referrer-Policy"} {
		if res.Header.Get(h) == "" {
			t.Errorf("missing header %s", h)
		}
	}
	if res.Header.Get("Cache-Control") != "no-store" {
		t.Error("API responses must not be cached")
	}
}

func TestLoopbackHost(t *testing.T) {
	for h, want := range map[string]bool{
		"localhost": true, "localhost:8080": true, "127.0.0.1:1": true, "[::1]:9": true,
		"evil.com": false, "127.0.0.1.evil.com": false, "localhost.evil.com:80": false, "": false,
	} {
		if loopbackHost(h) != want {
			t.Errorf("loopbackHost(%q) = %v, want %v", h, !want, want)
		}
	}
}
