// Package api is the HTTP face of the engine: JSON under /api, the web UI at /.
//
// The server has no accounts. It is meant to run on one machine, so by default
// it only answers requests whose Host header is a loopback name (a guard
// against DNS rebinding) and every POST must be application/json (a plain
// cross-site form cannot send that without a preflight the server never grants).
package api

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"mime"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/111Reihan/kiro/examples"
	"github.com/111Reihan/kiro/internal/engine"
	"github.com/111Reihan/kiro/internal/receipt"
	"github.com/111Reihan/kiro/internal/store"
	"github.com/111Reihan/kiro/internal/verify"
)

const maxBody = 4 << 20 // a 2 MiB document plus JSON overhead

type Options struct {
	UI       fs.FS // served at /; nil serves no UI
	Loopback bool  // refuse requests whose Host is not localhost, 127.0.0.1 or ::1
}

type srv struct{ e *engine.Engine }

// Handler returns the whole app as one http.Handler.
func Handler(e *engine.Engine, o Options) http.Handler {
	s := &srv{e}
	m := http.NewServeMux()
	m.HandleFunc("GET /api/status", s.status)
	m.HandleFunc("GET /api/audit", s.audit)
	m.HandleFunc("GET /api/events", s.events)
	m.HandleFunc("POST /api/demo", s.demo)

	m.HandleFunc("GET /api/docs", s.docs)
	m.HandleFunc("POST /api/docs", s.putDoc)
	m.HandleFunc("GET /api/docs/{id}", s.doc)
	m.HandleFunc("GET /api/docs/{id}/versions", s.versions)

	m.HandleFunc("POST /api/check", s.check)
	m.HandleFunc("POST /api/check-text", s.checkText)

	m.HandleFunc("GET /api/cites", s.cites)
	m.HandleFunc("POST /api/cites", s.addCite)
	m.HandleFunc("GET /api/haunts", s.haunts)

	m.HandleFunc("POST /api/receipts", s.issue)
	m.HandleFunc("POST /api/receipts/verify", s.verifyReceipt)
	m.HandleFunc("GET /api/receipts/{id}", s.receipt)

	m.HandleFunc("GET /api/trails", s.trails)
	m.HandleFunc("POST /api/trails", s.newTrail)
	m.HandleFunc("GET /api/trails/{id}", s.trail)
	m.HandleFunc("POST /api/trails/{id}/steps", s.addStep)
	m.HandleFunc("GET /api/trails/{id}/replay", s.replay)

	m.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { fail(w, http.StatusNotFound, "no such endpoint") })
	if o.UI != nil {
		m.Handle("/", http.FileServerFS(o.UI))
	}
	return guard(m, o)
}

func guard(next http.Handler, o Options) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if o.Loopback && !loopbackHost(r.Host) {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; "+
			"img-src 'self' data:; font-src 'self'; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Cache-Control", "no-store")
		}
		defer func() {
			if v := recover(); v != nil {
				log.Printf("panic serving %s %s: %v", r.Method, r.URL.Path, v)
				fail(w, http.StatusInternalServerError, "internal error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func loopbackHost(h string) bool {
	host, _, err := net.SplitHostPort(h)
	if err != nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// ---- plumbing ----

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write response: %v", err)
	}
}

func fail(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// httpErr maps engine errors to a status and a message that is safe to show.
func httpErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, engine.ErrInvalid):
		fail(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, engine.ErrNoDoc), errors.Is(err, store.ErrNotFound):
		fail(w, http.StatusNotFound, err.Error())
	case errors.Is(err, engine.ErrNoChange):
		fail(w, http.StatusConflict, err.Error())
	case errors.Is(err, engine.ErrNoMatch):
		fail(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, store.ErrRedacted):
		fail(w, http.StatusGone, err.Error())
	default:
		log.Printf("internal error: %v", err)
		fail(w, http.StatusInternalServerError, "internal error")
	}
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "application/json" {
		fail(w, http.StatusUnsupportedMediaType, "send Content-Type: application/json")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			fail(w, http.StatusRequestEntityTooLarge, "request body is too large")
		} else {
			fail(w, http.StatusBadRequest, "bad JSON: "+err.Error())
		}
		return false
	}
	return true
}

// qint reads an integer query parameter; a missing one is 0.
func qint(r *http.Request, name string) (int, error) {
	s := r.URL.Query().Get(name)
	if s == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, errors.New(name + " must be an integer")
	}
	return n, nil
}

func ints(w http.ResponseWriter, r *http.Request, names ...string) ([]int, bool) {
	out := make([]int, len(names))
	for i, n := range names {
		v, err := qint(r, n)
		if err != nil {
			fail(w, http.StatusBadRequest, err.Error())
			return nil, false
		}
		out[i] = v
	}
	return out, true
}

// ---- service ----

func (s *srv) status(w http.ResponseWriter, r *http.Request) {
	if err := s.e.St.Sync(); err != nil {
		httpErr(w, err)
		return
	}
	docs, err := s.e.Docs()
	if err != nil {
		httpErr(w, err)
		return
	}
	n, head := s.e.St.Head()
	writeJSON(w, 200, map[string]any{"key": s.e.KeyID(), "pub": s.e.PubKey(), "docs": len(docs), "events": n, "head": head})
}

func (s *srv) audit(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{"ok": true}
	if err := s.e.Integrity(); err != nil {
		out["ok"], out["error"] = false, err.Error()
	}
	if err := s.e.St.Sync(); err == nil {
		out["events"], out["head"] = s.e.St.Head()
	}
	writeJSON(w, 200, out)
}

// events lists the newest log entries (without their data) so the UI can draw the chain.
func (s *srv) events(w http.ResponseWriter, r *http.Request) {
	if err := s.e.St.Sync(); err != nil {
		httpErr(w, err)
		return
	}
	type row struct {
		Seq  int    `json:"seq"`
		TS   string `json:"ts"`
		Type string `json:"type"`
		Hash string `json:"hash"`
		Prev string `json:"prev"`
	}
	evs := s.e.St.Events()
	if len(evs) > 60 {
		evs = evs[len(evs)-60:]
	}
	out := make([]row, len(evs))
	for i, ev := range evs {
		out[i] = row{ev.Seq, ev.TS, ev.Type, ev.Hash, ev.Prev}
	}
	writeJSON(w, 200, out)
}

func (s *srv) demo(w http.ResponseWriter, r *http.Request) {
	ids, err := examples.Load(s.e)
	if err != nil {
		httpErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"loaded": ids, "report": examples.Report()})
}

// ---- documents ----

func (s *srv) docs(w http.ResponseWriter, r *http.Request) {
	d, err := s.e.Docs()
	if err != nil {
		httpErr(w, err)
		return
	}
	writeJSON(w, 200, d)
}

func (s *srv) putDoc(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID    string `json:"id"`
		Title string `json:"title"`
		Text  string `json:"text"`
	}
	if !decode(w, r, &in) {
		return
	}
	v, err := s.e.PutDoc(in.ID, in.Title, in.Text)
	if err != nil {
		httpErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (s *srv) doc(w http.ResponseWriter, r *http.Request) {
	n, ok := ints(w, r, "ver")
	if !ok {
		return
	}
	v, err := s.e.Doc(r.PathValue("id"), n[0])
	if err != nil {
		httpErr(w, err)
		return
	}
	writeJSON(w, 200, v)
}

func (s *srv) versions(w http.ResponseWriter, r *http.Request) {
	v, err := s.e.Versions(r.PathValue("id"))
	if err != nil {
		httpErr(w, err)
		return
	}
	writeJSON(w, 200, v)
}

// ---- checking ----

func (s *srv) check(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Quote string   `json:"quote"`
		Docs  []string `json:"docs"`
		Ver   int      `json:"ver"`
	}
	if !decode(w, r, &in) {
		return
	}
	res, err := s.e.Check(in.Quote, engine.Scope{Docs: in.Docs, Ver: in.Ver})
	if err != nil {
		httpErr(w, err)
		return
	}
	writeJSON(w, 200, res)
}

func (s *srv) checkText(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Text string   `json:"text"`
		Docs []string `json:"docs"`
		Ver  int      `json:"ver"`
	}
	if !decode(w, r, &in) {
		return
	}
	found, err := s.e.CheckText(in.Text, engine.Scope{Docs: in.Docs, Ver: in.Ver})
	if err != nil {
		httpErr(w, err)
		return
	}
	counts := map[string]int{}
	for _, f := range found {
		counts[string(f.Result.Verdict)]++
		if f.Result.Severity == verify.Material {
			counts["material"]++
		}
	}
	writeJSON(w, 200, map[string]any{"found": found, "counts": counts})
}

// ---- citations ----

func (s *srv) cites(w http.ResponseWriter, r *http.Request) {
	c, err := s.e.Cites(r.URL.Query().Get("doc"))
	if err != nil {
		httpErr(w, err)
		return
	}
	writeJSON(w, 200, c)
}

func (s *srv) addCite(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Label string `json:"label"`
		Doc   string `json:"doc"`
		Ver   int    `json:"ver"`
		Quote string `json:"quote"`
		Mode  string `json:"mode"`
	}
	if !decode(w, r, &in) {
		return
	}
	c, err := s.e.AddCite(in.Label, in.Doc, in.Ver, in.Quote, in.Mode)
	if err != nil {
		httpErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (s *srv) haunts(w http.ResponseWriter, r *http.Request) {
	n, ok := ints(w, r, "ver", "start", "end")
	if !ok {
		return
	}
	c, err := s.e.Haunts(r.URL.Query().Get("doc"), n[0], n[1], n[2])
	if err != nil {
		httpErr(w, err)
		return
	}
	writeJSON(w, 200, c)
}

// ---- receipts ----

func (s *srv) issue(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Quote string `json:"quote"`
		Doc   string `json:"doc"`
		Ver   int    `json:"ver"`
	}
	if !decode(w, r, &in) {
		return
	}
	env, id, err := s.e.Issue(in.Quote, in.Doc, in.Ver)
	if err != nil {
		httpErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "receipt": env})
}

func (s *srv) receipt(w http.ResponseWriter, r *http.Request) {
	env, err := s.e.GetReceipt(r.PathValue("id"))
	if err != nil {
		httpErr(w, err)
		return
	}
	writeJSON(w, 200, env)
}

// verifyReceipt answers "does this receipt hold up?". A failed check is a
// normal answer (ok:false), not an HTTP error.
func (s *srv) verifyReceipt(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Receipt receipt.Envelope `json:"receipt"`
		Source  *string          `json:"source"`
		Pin     string           `json:"pin"` // expected public key, base64; empty means do not pin
	}
	if !decode(w, r, &in) {
		return
	}
	rep, err := receipt.Verify(in.Receipt)
	out := map[string]any{"ok": err == nil}
	switch {
	case err != nil:
		out["error"] = err.Error()
	case in.Pin != "" && in.Pin != in.Receipt.Pub:
		out["ok"], out["error"] = false, "receipt was signed by a different key than the one pinned"
	case in.Source != nil:
		if err := receipt.CheckSource(in.Receipt, []byte(*in.Source)); err != nil {
			out["ok"], out["error"] = false, err.Error()
		} else {
			rep.Checked = append(rep.Checked, "source bytes match the receipt's source hash and excerpts")
		}
	}
	if in.Pin != "" && out["ok"] == true {
		rep.Checked = append(rep.Checked, "signed by the pinned key")
	}
	out["report"] = rep
	writeJSON(w, 200, out)
}

// ---- trails ----

func (s *srv) trails(w http.ResponseWriter, r *http.Request) {
	t, err := s.e.Trails()
	if err != nil {
		httpErr(w, err)
		return
	}
	writeJSON(w, 200, t)
}

func (s *srv) newTrail(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Title string `json:"title"`
	}
	if !decode(w, r, &in) {
		return
	}
	t, err := s.e.NewTrail(in.Title)
	if err != nil {
		httpErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

func (s *srv) trail(w http.ResponseWriter, r *http.Request) {
	t, err := s.e.TrailByID(r.PathValue("id"))
	if err != nil {
		httpErr(w, err)
		return
	}
	writeJSON(w, 200, t)
}

func (s *srv) addStep(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Claim string `json:"claim"`
		Quote string `json:"quote"`
		Doc   string `json:"doc"`
		Ver   int    `json:"ver"`
	}
	if !decode(w, r, &in) {
		return
	}
	st, err := s.e.AddStep(r.PathValue("id"), in.Claim, in.Quote, in.Doc, in.Ver)
	if err != nil {
		httpErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, st)
}

func (s *srv) replay(w http.ResponseWriter, r *http.Request) {
	out, err := s.e.Replay(r.PathValue("id"))
	if err != nil {
		httpErr(w, err)
		return
	}
	writeJSON(w, 200, out)
}
