package engine

import (
	"errors"
	"unicode/utf8"

	"github.com/111Reihan/kiro/internal/align"
	"github.com/111Reihan/kiro/internal/store"
	"github.com/111Reihan/kiro/internal/text"
	"github.com/111Reihan/kiro/internal/verify"
)

const ctxRunes = 32 // context kept on each side of a cited passage

// Cite records that something quoted a passage of a source. It is Xanadu's
// transclusion link: a pointer to (document, version, range), not a copy.
type Cite struct {
	ID     string `json:"id,omitempty"`
	Label  string `json:"label"` // where the quote is used
	Quote  string `json:"quote"` // the citing side's wording
	Doc    string `json:"doc"`
	Ver    int    `json:"ver"`
	Start  int    `json:"start"` // rune offsets in that version
	End    int    `json:"end"`
	Exact  string `json:"exact"` // the source's own text at that range
	Prefix string `json:"prefix"`
	Suffix string `json:"suffix"`
	Mode   string `json:"mode"` // pinned: stays on Ver; follow: tracks the latest version
	TS     string `json:"ts,omitempty"`
}

// Loc says where a cited passage is in some version, and how it got there.
//
//	intact    same words, same place
//	moved     same words, elsewhere in the text
//	drifted   similar words; Ops shows what changed (the "ghost" diff)
//	orphaned  nothing close enough is left
//	redacted  the source bytes were removed from the store
type Loc struct {
	Status  string     `json:"status"`
	AtVer   int        `json:"at_ver"`
	AtStart int        `json:"at_start"`
	AtEnd   int        `json:"at_end"`
	Now     string     `json:"now"`
	Sim     int        `json:"sim"`
	Ops     []align.Op `json:"ops,omitempty"`
}

type CiteView struct {
	Cite
	Loc
}

func (e *Engine) locate(c *Cite, v *dv) Loc {
	p, err := e.prepared(c.Doc, v)
	switch {
	case errors.Is(err, store.ErrRedacted):
		return Loc{Status: "redacted", AtVer: v.Ver}
	case err != nil:
		return Loc{Status: "unreadable", AtVer: v.Ver}
	}
	if v.Ver == c.Ver {
		if c.End <= len(p.Src) && string(p.Src[c.Start:c.End]) == c.Exact {
			return Loc{Status: "intact", AtVer: v.Ver, AtStart: c.Start, AtEnd: c.End, Now: c.Exact, Sim: 1000}
		}
		return Loc{Status: "unreadable", AtVer: v.Ver}
	}
	if occ := verify.Occurrences(p, c.Exact); len(occ) > 0 {
		qlen := len(text.Normalize(c.Exact).Runes)
		a := occ[pickOcc(p, occ, qlen, c)]
		s, en := p.Norm.Span(a, a+qlen)
		st := "moved"
		if s == c.Start && en == c.End {
			st = "intact"
		}
		return Loc{Status: st, AtVer: v.Ver, AtStart: s, AtEnd: en, Now: string(p.Src[s:en]), Sim: 1000}
	}
	r := verify.Check(c.Exact, p)
	if r.Verdict != verify.Drifted || len(r.Spans) == 0 {
		return Loc{Status: "orphaned", AtVer: v.Ver, Sim: r.Sim}
	}
	last := r.Spans[len(r.Spans)-1]
	return Loc{Status: "drifted", AtVer: v.Ver, AtStart: r.Spans[0].Start, AtEnd: last.End,
		Now: string(p.Src[r.Spans[0].Start:last.End]), Sim: r.Sim, Ops: r.Ops}
}

// pickOcc chooses among repeated copies of a passage by how well the text
// around each copy matches the context saved with the cite.
func pickOcc(p *verify.Prepared, occ []int, qlen int, c *Cite) int {
	pre, suf := []rune(c.Prefix), []rune(c.Suffix)
	best, bestScore, bestDist := 0, -1, 0
	for i, a := range occ {
		s, en := p.Norm.Span(a, a+qlen)
		score := tail(p.Src[max(0, s-len(pre)):s], pre) + head(p.Src[en:min(len(p.Src), en+len(suf))], suf)
		dist := s - c.Start
		if dist < 0 {
			dist = -dist
		}
		if score > bestScore || score == bestScore && dist < bestDist {
			best, bestScore, bestDist = i, score, dist
		}
	}
	return best
}

func tail(a, b []rune) int { // length of the common suffix
	n := 0
	for n < len(a) && n < len(b) && a[len(a)-1-n] == b[len(b)-1-n] {
		n++
	}
	return n
}

func head(a, b []rune) int { // length of the common prefix
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return n
}

// AddCite records that label quotes doc. The quote must be found in the
// source (verbatim or drifted); what is stored is the source's own text.
func (e *Engine) AddCite(label, doc string, ver int, quote, mode string) (CiteView, error) {
	if mode == "" {
		mode = "pinned"
	}
	if mode != "pinned" && mode != "follow" {
		return CiteView{}, invalid("mode must be pinned or follow")
	}
	if utf8.RuneCountInString(label) > 200 || utf8.RuneCountInString(quote) > MaxQuoteRunes {
		return CiteView{}, invalid("label or quote is too long")
	}
	v, _, err := e.pick(doc, ver)
	if err != nil {
		return CiteView{}, err
	}
	p, err := e.prepared(doc, v)
	if err != nil {
		return CiteView{}, err
	}
	r := verify.Check(quote, p)
	if (r.Verdict != verify.Verbatim && r.Verdict != verify.Drifted) || len(r.Spans) == 0 {
		return CiteView{}, ErrNoMatch
	}
	start, end := r.Spans[0].Start, r.Spans[len(r.Spans)-1].End
	c := Cite{
		Label: label, Quote: quote, Doc: doc, Ver: v.Ver, Start: start, End: end,
		Exact:  string(p.Src[start:end]),
		Prefix: string(p.Src[max(0, start-ctxRunes):start]),
		Suffix: string(p.Src[end:min(len(p.Src), end+ctxRunes)]),
		Mode:   mode,
	}
	ev, err := e.St.Append("cite.add", c)
	if err != nil {
		return CiteView{}, err
	}
	return e.viewByID("c" + itoa(ev.Seq))
}

func (e *Engine) viewByID(id string) (CiteView, error) {
	e.mu.Lock()
	if err := e.sync(); err != nil {
		e.mu.Unlock()
		return CiteView{}, err
	}
	var c *Cite
	for _, x := range e.st.cites {
		if x.ID == id {
			c = x
		}
	}
	var vers []*dv
	if c != nil {
		vers = e.st.docs[c.Doc]
	}
	e.mu.Unlock()
	if c == nil {
		return CiteView{}, ErrNoDoc
	}
	return e.view(c, vers), nil
}

func (e *Engine) view(c *Cite, vers []*dv) CiteView {
	v := vers[len(vers)-1]
	if c.Mode == "pinned" {
		v = vers[c.Ver-1]
	}
	return CiteView{*c, e.locate(c, v)}
}

// Cites lists citations of one document (all documents if doc is empty) with
// where each one stands now.
func (e *Engine) Cites(doc string) ([]CiteView, error) {
	cs, vers, err := e.snapshotCites(doc)
	if err != nil {
		return nil, err
	}
	out := make([]CiteView, 0, len(cs))
	for _, c := range cs {
		out = append(out, e.view(c, vers[c.Doc]))
	}
	return out, nil
}

func (e *Engine) snapshotCites(doc string) ([]*Cite, map[string][]*dv, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.sync(); err != nil {
		return nil, nil, err
	}
	var cs []*Cite
	vers := map[string][]*dv{}
	for _, c := range e.st.cites {
		if doc == "" || c.Doc == doc {
			cs = append(cs, c)
			vers[c.Doc] = append([]*dv(nil), e.st.docs[c.Doc]...)
		}
	}
	return cs, vers, nil
}

// Haunts answers "who haunts me?": every citation that lands on [start,end) of
// one version of doc, however it was cited. ver 0 means the latest version.
func (e *Engine) Haunts(doc string, ver, start, end int) ([]CiteView, error) {
	if start < 0 || end <= start {
		return nil, invalid("need 0 <= start < end")
	}
	v, _, err := e.pick(doc, ver)
	if err != nil {
		return nil, err
	}
	cs, _, err := e.snapshotCites(doc)
	if err != nil {
		return nil, err
	}
	out := []CiteView{}
	for _, c := range cs {
		l := e.locate(c, v)
		switch l.Status {
		case "intact", "moved", "drifted":
			if l.AtStart < end && l.AtEnd > start {
				out = append(out, CiteView{*c, l})
			}
		}
	}
	return out, nil
}
