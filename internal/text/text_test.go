package text

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/111Reihan/kiro/internal/prop"
)

func TestExtractQuotes(t *testing.T) {
	in := "He said “government of the people, by the people” and then \"short one\" and \"for the people, shall not perish\"."
	qs := ExtractQuotes(in)
	if len(qs) != 2 {
		t.Fatalf("got %d quotes: %+v", len(qs), qs)
	}
	if qs[0].Text != "government of the people, by the people" {
		t.Fatalf("text %q", qs[0].Text)
	}
	rs := []rune(in)
	if string(rs[qs[0].Start:qs[0].End]) != qs[0].Text {
		t.Fatal("offsets do not point at the quote")
	}
}

func TestNormalizeCollapsesSpace(t *testing.T) {
	n := Normalize("  a \n\t b c  ")
	if n.S != "a b c" {
		t.Fatalf("got %q", n.S)
	}
}

// Property: every token of the folded text maps back to original text that
// folds to the same token.
func TestPropSpanRoundTrip(t *testing.T) {
	pieces := []string{"don’t", "well—known", "wait…", "  ", "\n", "“quoted”", "plain", "x​y", "1,000", "é"}
	prop.Check(t, 300, func(r *rand.Rand) error {
		var s string
		for i := 0; i < 5+r.Intn(30); i++ {
			s += pieces[r.Intn(len(pieces))] + " "
		}
		n := Normalize(s)
		src := []rune(s)
		for i := 1; i < len(n.Orig); i++ {
			if n.Orig[i] < n.Orig[i-1] || n.Orig[i] >= len(src) {
				return fmt.Errorf("map not monotonic or out of range at %d", i)
			}
		}
		for _, tok := range Tokenize(n.Runes) {
			a, b := n.Span(tok.Start, tok.End)
			if got := Normalize(string(src[a:b])).S; got != tok.Text {
				return fmt.Errorf("token %q maps back to %q", tok.Text, got)
			}
		}
		return nil
	})
}
