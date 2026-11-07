// Package text folds text for matching and keeps a map back to the original
// rune offsets, so a match found in folded text can be quoted from the source
// exactly as written.
package text

import "unicode"

// Norm is folded text plus the way back to the original.
type Norm struct {
	S     string // folded text
	Runes []rune // same, as runes
	Orig  []int  // Orig[i] is the original rune offset of Runes[i]
	N     int    // rune count of the original
}

// fold maps one rune to its folded form. Smart quotes and dashes become ASCII,
// zero-width characters vanish, every kind of space becomes one space.
// shortcut: no Unicode NFC (needs x/text, offline build), combining marks compare raw.
func fold(r rune) string {
	switch r {
	case 0x2018, 0x2019, 0x201a, 0x201b, 0x2032: // single curly quotes, prime
		return "'"
	case 0x201c, 0x201d, 0x201e, 0x201f, 0x2033: // double curly quotes, double prime
		return "\""
	case 0x2010, 0x2011, 0x2012, 0x2013, 0x2014, 0x2015, 0x2212: // hyphens, dashes, minus
		return "-"
	case 0x2026: // ellipsis
		return "..."
	case 0x200b, 0x200c, 0x200d, 0xfeff, 0xad: // zero-width space/joiners, BOM, soft hyphen
		return ""
	}
	if unicode.IsSpace(r) {
		return " "
	}
	return string(r)
}

// Normalize folds s. Case is kept: a case change is drift, a curly quote is not.
func Normalize(s string) Norm {
	src := []rune(s)
	n := Norm{N: len(src)}
	var out []rune
	var orig []int
	space := false
	for i, r := range src {
		f := fold(r)
		if f == " " {
			if len(out) > 0 && !space {
				out = append(out, ' ')
				orig = append(orig, i)
				space = true
			}
			continue
		}
		for _, fr := range f {
			out = append(out, fr)
			orig = append(orig, i)
			space = false
		}
	}
	if len(out) > 0 && out[len(out)-1] == ' ' {
		out, orig = out[:len(out)-1], orig[:len(orig)-1]
	}
	n.Runes, n.Orig, n.S = out, orig, string(out)
	return n
}

// Span maps the folded rune range [a,b) back to original rune offsets [start,end).
func (n Norm) Span(a, b int) (int, int) {
	return n.Orig[a], n.Orig[b-1] + 1
}

// Token is one word with its rune range in the folded text.
type Token struct {
	Text       string
	Start, End int
}

func isWord(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.Is(unicode.M, r)
}

// Tokenize splits folded runes into words. Apostrophes and hyphens inside a
// word stay, so do separators inside numbers (1,000 and 3.5). Other
// punctuation is ignored: a dropped comma is not a changed claim.
func Tokenize(rs []rune) []Token {
	var toks []Token
	i := 0
	for i < len(rs) {
		if !isWord(rs[i]) {
			i++
			continue
		}
		j := i
		for j < len(rs) {
			if isWord(rs[j]) {
				j++
				continue
			}
			if j+1 < len(rs) && isWord(rs[j+1]) && (rs[j] == '\'' || rs[j] == '-') {
				j++
				continue
			}
			if j+1 < len(rs) && unicode.IsDigit(rs[j+1]) && unicode.IsDigit(rs[j-1]) && (rs[j] == ',' || rs[j] == '.') {
				j++
				continue
			}
			break
		}
		toks = append(toks, Token{string(rs[i:j]), i, j})
		i = j
	}
	return toks
}

// Quote is one quoted passage found in a document.
type Quote struct {
	Text  string `json:"text"`
	Start int    `json:"start"` // rune offsets of the inner text in the input
	End   int    `json:"end"`
}

const (
	MinQuoteWords = 4
	MaxQuoteRunes = 2000
)

// ExtractQuotes finds passages inside double quotes (curly or straight).
// shortcut: single quotes are skipped (apostrophes make them ambiguous), an odd
// number of straight quotes shifts every later pair; upgrade with a real parser.
func ExtractQuotes(s string) []Quote {
	rs := []rune(s)
	var qs []Quote
	open, kind := -1, rune(0)
	for i, r := range rs {
		if open < 0 {
			if r == '“' || r == '"' {
				open, kind = i, r
			}
			continue
		}
		closes := (kind == '“' && r == '”') || (kind == '"' && r == '"')
		if !closes {
			if r == '“' { // an unclosed opener is abandoned
				open, kind = i, r
			}
			continue
		}
		inner := string(rs[open+1 : i])
		if i-open-1 <= MaxQuoteRunes && len(Tokenize(Normalize(inner).Runes)) >= MinQuoteWords {
			qs = append(qs, Quote{Text: inner, Start: open + 1, End: i})
		}
		open = -1
	}
	return qs
}
