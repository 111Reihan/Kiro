// Package verify grades a quote against a source text.
//
// Four verdicts, on purpose:
//
//	verbatim    found word for word (after folding quotes, dashes, spaces)
//	drifted     found, but words differ; severity says how much it matters
//	unsupported not found in the sources that were loaded
//	no-source   nothing was loaded to check against (set by the caller)
//
// "unsupported" is not "fabricated": the real source may simply not be loaded.
package verify

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/111Reihan/kiro/internal/align"
	"github.com/111Reihan/kiro/internal/text"
)

type Verdict string

const (
	Verbatim    Verdict = "verbatim"
	Drifted     Verdict = "drifted"
	Unsupported Verdict = "unsupported"
	NoSource    Verdict = "no-source"
)

type Severity string

const (
	None     Severity = ""
	Cosmetic Severity = "cosmetic" // punctuation or case only
	Minor    Severity = "minor"    // words changed, nothing that flips meaning
	Material Severity = "material" // negation, number, name, modal or quantifier changed
)

const (
	SimThreshold   = 600        // permille; below this a fuzzy hit is not trusted
	MinFuzzyTokens = 4          // shorter quotes must match exactly or not at all
	MaxQuoteTokens = 400        // alignment guard
	MaxCells       = 40_000_000 // alignment guard: quote tokens x source tokens
)

// Prepared is a source version, folded and tokenized once.
type Prepared struct {
	Doc     string
	Ver     int
	Src     []rune
	Norm    text.Norm
	Toks    []text.Token
	TokText []string
}

func Prepare(doc string, ver int, txt string) *Prepared {
	n := text.Normalize(txt)
	toks := text.Tokenize(n.Runes)
	tt := make([]string, len(toks))
	for i, t := range toks {
		tt[i] = t.Text
	}
	return &Prepared{Doc: doc, Ver: ver, Src: []rune(txt), Norm: n, Toks: toks, TokText: tt}
}

// Span is a matched passage in the original source text (rune offsets).
type Span struct {
	Start   int    `json:"start"`
	End     int    `json:"end"`
	Excerpt string `json:"excerpt"`
}

type Result struct {
	Verdict  Verdict    `json:"verdict"`
	Severity Severity   `json:"severity,omitempty"`
	Sim      int        `json:"sim"` // permille, 1000 = identical
	Doc      string     `json:"doc,omitempty"`
	Ver      int        `json:"ver,omitempty"`
	Spans    []Span     `json:"spans,omitempty"`
	Ops      []align.Op `json:"ops,omitempty"`
	Elided   bool       `json:"elided,omitempty"`
	Notes    []string   `json:"notes,omitempty"`
}

func rankV(v Verdict) int {
	switch v {
	case Verbatim:
		return 0
	case Drifted:
		return 1
	}
	return 2
}

func rankS(s Severity) int {
	switch s {
	case Cosmetic:
		return 1
	case Minor:
		return 2
	case Material:
		return 3
	}
	return 0
}

// Better reports whether a is a stronger match than b (used to pick across sources).
func Better(a, b Result) bool {
	if rankV(a.Verdict) != rankV(b.Verdict) {
		return rankV(a.Verdict) < rankV(b.Verdict)
	}
	return a.Sim > b.Sim
}

type seg struct {
	verdict Verdict
	sev     Severity
	sim     int
	span    *Span
	ops     []align.Op
	notes   []string
}

// Check grades quote against one prepared source.
func Check(quote string, p *Prepared) Result {
	res := Result{Doc: p.Doc, Ver: p.Ver}
	segs, elided := split(quote)
	if len(segs) == 0 {
		res.Verdict = Unsupported
		res.Notes = []string{"quote has no words"}
		return res
	}
	res.Elided = elided
	res.Verdict, res.Sim = Verbatim, 1000
	for _, s := range segs {
		r := checkSeg(s, p)
		if rankV(r.verdict) > rankV(res.Verdict) {
			res.Verdict = r.verdict
		}
		if rankS(r.sev) > rankS(res.Severity) {
			res.Severity = r.sev
		}
		if r.sim < res.Sim {
			res.Sim = r.sim
		}
		if r.span != nil {
			res.Spans = append(res.Spans, *r.span)
		}
		res.Ops = append(res.Ops, r.ops...)
		res.Notes = append(res.Notes, r.notes...)
	}
	for i := 1; i < len(res.Spans); i++ { // ellipsis pieces must keep their order
		if res.Spans[i].Start < res.Spans[i-1].End {
			res.Notes = append(res.Notes, "quoted pieces appear in a different order in the source")
			if res.Verdict == Verbatim {
				res.Verdict = Drifted
			}
			res.Severity = Material
			break
		}
	}
	if res.Verdict == Verbatim {
		res.Severity = None
	}
	if res.Verdict == Unsupported {
		res.Spans = nil
	}
	return res
}

func checkSeg(s string, p *Prepared) seg {
	qn := text.Normalize(s)
	if occ := Occurrences(p, s); len(occ) > 0 {
		a := occ[0]
		st, en := p.Norm.Span(a, a+len(qn.Runes))
		r := seg{verdict: Verbatim, sim: 1000, span: mkSpan(p, st, en)}
		if len(occ) > 1 {
			r.notes = []string{"passage appears more than once in the source; first one used"}
		}
		return r
	}
	qt := text.Tokenize(qn.Runes)
	switch {
	case len(qt) < MinFuzzyTokens:
		return seg{verdict: Unsupported, notes: []string{"too short for fuzzy matching and not found exactly"}}
	case len(qt) > MaxQuoteTokens || len(qt)*len(p.TokText) > MaxCells:
		return seg{verdict: Unsupported, notes: []string{"too large to align; split the quote or the source"}}
	}
	qs := make([]string, len(qt))
	for i, t := range qt {
		qs[i] = t.Text
	}
	m := align.Locate(qs, p.TokText)
	w := m.End - m.Start
	if w == 0 {
		return seg{verdict: Unsupported}
	}
	sim := 1000 - 1000*m.Dist/max(len(qs), w)
	if sim < SimThreshold {
		return seg{verdict: Unsupported, sim: sim}
	}
	ops := align.Diff(qs, p.TokText[m.Start:m.End])
	st, en := p.Norm.Span(p.Toks[m.Start].Start, p.Toks[m.End-1].End)
	return seg{verdict: Drifted, sev: severity(ops), sim: sim, span: mkSpan(p, st, en), ops: ops}
}

func mkSpan(p *Prepared, st, en int) *Span {
	return &Span{Start: st, End: en, Excerpt: string(p.Src[st:en])}
}

// Occurrences lists folded-rune offsets where q appears word for word.
// A hit that begins or ends in the middle of a word does not count.
func Occurrences(p *Prepared, q string) []int {
	qn := text.Normalize(q)
	if qn.S == "" {
		return nil
	}
	var out []int
	from := 0
	for {
		i := strings.Index(p.Norm.S[from:], qn.S)
		if i < 0 {
			return out
		}
		i += from
		a := utf8.RuneCountInString(p.Norm.S[:i])
		b := a + len(qn.Runes)
		if bounded(p.Norm.Runes, qn.Runes, a, b) {
			out = append(out, a)
		}
		from = i + utf8.RuneLen(qn.Runes[0])
	}
}

func bounded(hay, q []rune, a, b int) bool {
	if isW(q[0]) && a > 0 && isW(hay[a-1]) {
		return false
	}
	if isW(q[len(q)-1]) && b < len(hay) && isW(hay[b]) {
		return false
	}
	return true
}

func isW(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

// split cuts a quote at ellipses; each piece is checked on its own.
func split(q string) (segs []string, elided bool) {
	r := strings.NewReplacer("[...]", "\x00", "[…]", "\x00", "...", "\x00", "…", "\x00", ". . .", "\x00")
	parts := strings.Split(r.Replace(q), "\x00")
	for _, p := range parts {
		// closing punctuation inside quote marks is house style, not a claim
		p = strings.TrimLeft(p, " \t\r\n,;:\"“”")
		p = strings.TrimRight(p, " \t\r\n,;:.!?\"“”")
		if len(text.Tokenize(text.Normalize(p).Runes)) > 0 {
			segs = append(segs, p)
		}
	}
	return segs, len(parts) > 1
}

var sensitive = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`
		not no never none neither nor cannot without nobody nothing nowhere
		may might must shall should will would can could
		all any every each some only most few many always sometimes often rarely
		more less fewer least
		zero one two three four five six seven eight nine ten eleven twelve
		thirteen fourteen fifteen sixteen seventeen eighteen nineteen twenty
		thirty forty fifty sixty seventy eighty ninety hundred thousand million billion`) {
		sensitive[w] = true
	}
}

func severity(ops []align.Op) Severity {
	sev := Cosmetic
	for _, o := range ops {
		if o.Kind == "eq" || o.Kind == "sub" && strings.EqualFold(o.Q, o.S) {
			continue // a case change alone stays cosmetic
		}
		sev = Minor
		if material(o) {
			return Material
		}
	}
	return sev
}

func material(o align.Op) bool {
	for _, t := range []string{o.Q, o.S} {
		if t == "" {
			continue
		}
		l := strings.ToLower(t)
		if sensitive[l] || strings.HasSuffix(l, "n't") || strings.IndexFunc(l, unicode.IsDigit) >= 0 {
			return true
		}
	}
	// one name swapped for another
	return o.Kind == "sub" && startsUpper(o.Q) && startsUpper(o.S)
}

func startsUpper(s string) bool {
	r, _ := utf8.DecodeRuneInString(s)
	return unicode.IsUpper(r)
}
