package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/111Reihan/kiro/internal/canon"
	"github.com/111Reihan/kiro/internal/receipt"
	"github.com/111Reihan/kiro/internal/verify"
)

func itoa(n int) string { return strconv.Itoa(n) }

// ---- receipts ----

// Issue checks quote against one version of doc and, if it is found, signs a
// receipt and stores it. Quotes that are not found get no receipt: there is
// nothing to bind them to.
func (e *Engine) Issue(quote, doc string, ver int) (receipt.Envelope, string, error) {
	if utf8.RuneCountInString(quote) > MaxQuoteRunes {
		return receipt.Envelope{}, "", invalid("quote is over %d characters", MaxQuoteRunes)
	}
	v, _, err := e.pick(doc, ver)
	if err != nil {
		return receipt.Envelope{}, "", err
	}
	p, err := e.prepared(doc, v)
	if err != nil {
		return receipt.Envelope{}, "", err
	}
	r := verify.Check(quote, p)
	if r.Verdict != verify.Verbatim && r.Verdict != verify.Drifted {
		return receipt.Envelope{}, "", ErrNoMatch
	}
	env, err := receipt.Sign(receipt.Body{
		V: 1, Kind: "revenant.receipt", Issued: time.Now().UTC().Format(time.RFC3339),
		Quote: quote, Verdict: r.Verdict, Severity: r.Severity, Sim: r.Sim, Elided: r.Elided,
		Doc: doc, Ver: v.Ver, Blob: v.Blob, LogSeq: v.Seq, LogHash: v.Hash,
		Spans: r.Spans, Ops: r.Ops, Notes: r.Notes,
	}, e.key)
	if err != nil {
		return receipt.Envelope{}, "", err
	}
	raw, err := canon.Marshal(env)
	if err != nil {
		return receipt.Envelope{}, "", err
	}
	blob, err := e.St.PutBlob(raw)
	if err != nil {
		return receipt.Envelope{}, "", err
	}
	if sum := sha256.Sum256(raw); hex.EncodeToString(sum[:]) != blob {
		return receipt.Envelope{}, "", errors.New("receipt blob hash mismatch")
	}
	_, err = e.St.Append("receipt.issue", map[string]any{"blob": blob, "doc": doc, "ver": v.Ver, "verdict": string(r.Verdict)})
	return env, blob, err
}

func (e *Engine) GetReceipt(id string) (receipt.Envelope, error) {
	b, err := e.St.GetBlob(id)
	if err != nil {
		return receipt.Envelope{}, err
	}
	var env receipt.Envelope
	return env, json.Unmarshal(b, &env)
}

// ---- trails ----

// Step is one hop of a trail: a claim, the quote that backs it, and the
// receipt that was signed when the quote was checked.
type Step struct {
	Claim   string `json:"claim"`
	Quote   string `json:"quote"`
	Doc     string `json:"doc"`
	Ver     int    `json:"ver"`
	Receipt string `json:"receipt"`
	TS      string `json:"ts,omitempty"`
}

// Trail is a named path from claims to the exact source versions behind them
// (Bush's Memex trail, with each step proven).
type Trail struct {
	ID    string `json:"id,omitempty"`
	Title string `json:"title"`
	TS    string `json:"ts,omitempty"`
	Steps []Step `json:"steps"`
}

func (e *Engine) NewTrail(title string) (Trail, error) {
	title = strings.TrimSpace(title)
	if title == "" || utf8.RuneCountInString(title) > 200 {
		return Trail{}, invalid("title must be 1-200 characters")
	}
	ev, err := e.St.Append("trail.create", Trail{Title: title})
	if err != nil {
		return Trail{}, err
	}
	return e.TrailByID("t" + itoa(ev.Seq))
}

func (e *Engine) TrailByID(id string) (Trail, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.sync(); err != nil {
		return Trail{}, err
	}
	t := e.st.trails[id]
	if t == nil {
		return Trail{}, ErrNoDoc
	}
	cp := *t
	cp.Steps = append([]Step{}, t.Steps...)
	return cp, nil
}

func (e *Engine) Trails() ([]Trail, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.sync(); err != nil {
		return nil, err
	}
	out := make([]Trail, 0, len(e.st.torder))
	for _, id := range e.st.torder {
		t := *e.st.trails[id]
		t.Steps = append([]Step{}, t.Steps...)
		out = append(out, t)
	}
	return out, nil
}

// AddStep checks the quote against doc@ver, signs a receipt, and appends the step.
func (e *Engine) AddStep(trail, claim, quote, doc string, ver int) (Step, error) {
	if _, err := e.TrailByID(trail); err != nil {
		return Step{}, err
	}
	claim = strings.TrimSpace(claim)
	if claim == "" || utf8.RuneCountInString(claim) > 500 {
		return Step{}, invalid("claim must be 1-500 characters")
	}
	env, id, err := e.Issue(quote, doc, ver)
	if err != nil {
		return Step{}, err
	}
	s := Step{Claim: claim, Quote: quote, Doc: doc, Ver: env.Body.Ver, Receipt: id}
	_, err = e.St.Append("trail.step", struct {
		Trail string `json:"trail"`
		Step
	}{trail, s})
	return s, err
}

// StepReplay is one step re-checked: did the receipt hold up, and does the
// quote still match the latest version of its source?
type StepReplay struct {
	Step
	SigOK   bool          `json:"sig_ok"`
	Err     string        `json:"err,omitempty"`
	Then    verify.Result `json:"then"`
	Now     verify.Result `json:"now"`
	Changed bool          `json:"changed"`
}

// Replay walks a trail and re-verifies every step.
func (e *Engine) Replay(id string) ([]StepReplay, error) {
	t, err := e.TrailByID(id)
	if err != nil {
		return nil, err
	}
	out := make([]StepReplay, 0, len(t.Steps))
	for _, s := range t.Steps {
		r := StepReplay{Step: s}
		if env, err := e.GetReceipt(s.Receipt); err != nil {
			r.Err = "receipt: " + err.Error()
		} else if _, err := receipt.Verify(env); err != nil {
			r.Err = "receipt: " + err.Error()
		} else {
			r.SigOK = true
		}
		then, err1 := e.Check(s.Quote, Scope{Docs: []string{s.Doc}, Ver: s.Ver})
		now, err2 := e.Check(s.Quote, Scope{Docs: []string{s.Doc}})
		if err := errors.Join(err1, err2); err != nil {
			r.Err = strings.TrimSpace(r.Err + " " + err.Error())
		}
		r.Then, r.Now = then, now
		r.Changed = then.Verdict != now.Verdict || then.Severity != now.Severity
		out = append(out, r)
	}
	return out, nil
}
