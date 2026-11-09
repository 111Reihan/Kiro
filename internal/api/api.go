// Package api serves the ledger over HTTP. It reads the project tree and the
// log; it never runs a command, so a public instance cannot be turned into a
// shell. Writes (rescan, seal) are off in read-only mode.
package api

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net"
	"net/http"
	"strings"

	"github.com/111Reihan/kiro/internal/engine"
	"github.com/111Reihan/kiro/internal/ledger"
	"github.com/111Reihan/kiro/internal/spec"
	"github.com/111Reihan/kiro/internal/store"
)

const maxBody = 1 << 20

// Options configures the handler.
type Options struct {
	Root     string // project directory to scan
	ReadOnly bool   // refuse rescan and seal
	Loopback bool   // only answer Host: localhost, 127.0.0.1, [::1] (blocks DNS rebinding)
	Static   fs.FS  // built UI; nil serves the API only
}

type server struct {
	l *ledger.Ledger
	o Options
}

// Handler returns the whole site: /api/* plus the static UI.
func Handler(l *ledger.Ledger, o Options) http.Handler {
	s := &server{l: l, o: o}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/info", s.info)
	mux.HandleFunc("GET /api/project", s.project)
	mux.HandleFunc("GET /api/history", s.history)
	mux.HandleFunc("POST /api/preview", s.preview)
	mux.HandleFunc("POST /api/rescan", s.write(s.rescan))
	mux.HandleFunc("POST /api/seal", s.write(s.seal))
	mux.HandleFunc("GET /api/attestation", s.attestation)
	mux.HandleFunc("POST /api/attestation/verify", s.verifyAtt)
	mux.HandleFunc("GET /api/ledger", s.events)
	mux.HandleFunc("GET /api/audit", s.audit)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { fail(w, http.StatusNotFound, "no such endpoint") })
	if o.Static != nil {
		mux.Handle("/", spa(o.Static))
	}
	return s.guard(mux)
}

func (s *server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		if s.o.Loopback {
			host, _, err := net.SplitHostPort(r.Host)
			if err != nil {
				host = r.Host
			}
			if host != "localhost" && host != "127.0.0.1" && host != "[::1]" && host != "::1" {
				fail(w, http.StatusForbidden, "this server answers on localhost only")
				return
			}
		}
		if r.Method == http.MethodPost && !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			fail(w, http.StatusUnsupportedMediaType, "send application/json")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// spa serves files and falls back to index.html for client-side routes.
func spa(root fs.FS) http.Handler {
	files := http.FileServerFS(root)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p != "" {
			if st, err := fs.Stat(root, p); err != nil || st.IsDir() {
				r.URL.Path = "/"
			}
		}
		if r.URL.Path == "/" {
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
}

func (s *server) write(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.o.ReadOnly {
			fail(w, http.StatusForbidden, "this instance is read-only")
			return
		}
		h(w, r)
	}
}

func reply(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	e := json.NewEncoder(w)
	e.SetEscapeHTML(false)
	_ = e.Encode(v)
}

func fail(w http.ResponseWriter, code int, msg string) {
	reply(w, code, map[string]string{"error": msg})
}

func serverErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, engine.ErrInvalid), errors.Is(err, ledger.ErrNotSealable):
		fail(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, engine.ErrNoDoc):
		fail(w, http.StatusNotFound, err.Error())
	default:
		fail(w, http.StatusInternalServerError, "internal error")
	}
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		var mb *http.MaxBytesError
		if errors.As(err, &mb) {
			fail(w, http.StatusRequestEntityTooLarge, "body too large")
		} else {
			fail(w, http.StatusBadRequest, "bad JSON: "+err.Error())
		}
		return false
	}
	if _, err := d.Token(); !errors.Is(err, io.EOF) {
		fail(w, http.StatusBadRequest, "bad JSON: trailing data")
		return false
	}
	return true
}

func (s *server) scan(w http.ResponseWriter) *spec.Project {
	p, err := spec.Scan(s.o.Root)
	if err != nil {
		serverErr(w, err)
		return nil
	}
	return p
}

func (s *server) info(w http.ResponseWriter, r *http.Request) {
	n, head := s.l.E.St.Head()
	reply(w, 200, map[string]any{"name": "Specter", "read_only": s.o.ReadOnly, "key": s.l.E.PubKey(), "key_id": s.l.E.KeyID(), "events": n, "head": head})
}

func (s *server) project(w http.ResponseWriter, r *http.Request) {
	p := s.scan(w)
	if p == nil {
		return
	}
	rep, err := s.l.Status(p)
	if err != nil {
		serverErr(w, err)
		return
	}
	reply(w, 200, rep)
}

func (s *server) history(w http.ResponseWriter, r *http.Request) {
	p := s.scan(w)
	if p == nil {
		return
	}
	h, err := s.l.History(p, r.URL.Query().Get("id"))
	if err != nil {
		serverErr(w, err)
		return
	}
	reply(w, 200, h)
}

type previewIn struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type previewOut struct {
	ID      string            `json:"id"`
	Before  string            `json:"before"`
	After   string            `json:"after"`
	Changed bool              `json:"changed"`
	Change  *ledger.Change    `json:"change,omitempty"`
	Reasons []string          `json:"reasons,omitempty"`
	Tests   []ledger.TestLink `json:"tests"`
	Lint    []spec.Issue      `json:"lint"`
}

// preview answers "what would go stale if this requirement read like this?".
// Nothing is written: it edits an in-memory copy of the scan.
func (s *server) preview(w http.ResponseWriter, r *http.Request) {
	var in previewIn
	if !decode(w, r, &in) {
		return
	}
	text := strings.Join(strings.Fields(in.Text), " ")
	if text == "" || len(text) > 4000 {
		fail(w, http.StatusUnprocessableEntity, "text must be 1 to 4000 characters")
		return
	}
	p := s.scan(w)
	if p == nil {
		return
	}
	before, err := s.l.Status(p)
	if err != nil {
		serverErr(w, err)
		return
	}
	idx := -1
	for i, q := range p.Reqs {
		if q.ID == in.ID {
			idx = i
			break
		}
	}
	if idx < 0 {
		fail(w, http.StatusNotFound, "no such requirement")
		return
	}
	var b ledger.ReqStatus
	for _, q := range before.Reqs {
		if q.ID == in.ID {
			b = q
		}
	}
	reqs := append([]spec.Req(nil), p.Reqs...)
	reqs[idx].Text = text
	reqs[idx].Hash = spec.WordHash(text)
	reqs[idx].Kind, reqs[idx].Lint = spec.Classify(text)
	cp := *p
	cp.Reqs = reqs
	after, err := s.l.Status(&cp)
	if err != nil {
		serverErr(w, err)
		return
	}
	for _, q := range after.Reqs {
		if q.ID == in.ID {
			reply(w, 200, previewOut{ID: in.ID, Before: b.Status, After: q.Status, Changed: q.Change != nil, Change: q.Change,
				Reasons: q.Reasons, Tests: q.Tests, Lint: nonNil(q.Lint)})
			return
		}
	}
	fail(w, http.StatusInternalServerError, "internal error")
}

func nonNil(l []spec.Issue) []spec.Issue {
	if l == nil {
		return []spec.Issue{}
	}
	return l
}

func (s *server) rescan(w http.ResponseWriter, r *http.Request) {
	p := s.scan(w)
	if p == nil {
		return
	}
	if _, err := s.l.Record(p); err != nil {
		serverErr(w, err)
		return
	}
	rep, err := s.l.Status(p)
	if err != nil {
		serverErr(w, err)
		return
	}
	reply(w, 200, rep)
}

func (s *server) seal(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs []string `json:"ids"`
	}
	if !decode(w, r, &in) {
		return
	}
	p := s.scan(w)
	if p == nil {
		return
	}
	res, err := s.l.Seal(p, in.IDs)
	if err != nil {
		serverErr(w, err)
		return
	}
	reply(w, 200, res)
}

func (s *server) attestation(w http.ResponseWriter, r *http.Request) {
	sg, ok, err := s.l.SealOf(r.URL.Query().Get("id"))
	if err != nil {
		serverErr(w, err)
		return
	}
	if !ok {
		fail(w, http.StatusNotFound, "that requirement has no seal")
		return
	}
	reply(w, 200, sg)
}

func (s *server) verifyAtt(w http.ResponseWriter, r *http.Request) {
	var sg ledger.Signed
	if !decode(w, r, &sg) {
		return
	}
	out := map[string]any{"signature": "ok", "anchor": "ok", "ok": true}
	if err := ledger.VerifySigned(sg, ""); err != nil {
		out["signature"], out["ok"] = err.Error(), false
	}
	if sg.Key != s.l.E.PubKey() {
		out["key"] = "signed by a different key than this ledger's"
	}
	evs, err := s.l.Events()
	if err != nil {
		serverErr(w, err)
		return
	}
	if err := ledger.CheckAnchor(sg.Attestation, evs); err != nil {
		out["anchor"], out["ok"] = err.Error(), false
	}
	reply(w, 200, out)
}

type eventOut struct {
	Seq   int    `json:"seq"`
	TS    string `json:"ts"`
	Type  string `json:"type"`
	Label string `json:"label"`
	Hash  string `json:"hash"`
	Prev  string `json:"prev"`
}

func label(ev store.Event) string {
	var d struct {
		Req   string `json:"req"`
		Doc   string `json:"doc"`
		Title string `json:"title"`
		Cmd   string `json:"cmd"`
		Pass  int    `json:"pass"`
		Fail  int    `json:"fail"`
	}
	_ = json.Unmarshal(ev.Data, &d)
	switch ev.Type {
	case "spec.seal":
		return "sealed " + d.Req
	case "spec.run":
		return "test run: " + itoa(d.Pass) + " passed, " + itoa(d.Fail) + " failed"
	case "doc.version":
		return "recorded " + d.Title
	}
	return ev.Type
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

func (s *server) events(w http.ResponseWriter, r *http.Request) {
	evs, err := s.l.Events()
	if err != nil {
		serverErr(w, err)
		return
	}
	out := make([]eventOut, len(evs))
	for i, ev := range evs {
		out[i] = eventOut{ev.Seq, ev.TS, ev.Type, label(ev), ev.Hash, ev.Prev}
	}
	reply(w, 200, out)
}

func (s *server) audit(w http.ResponseWriter, r *http.Request) {
	a, err := s.l.Audit()
	if err != nil {
		serverErr(w, err)
		return
	}
	reply(w, 200, a)
}
