package verify

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/111Reihan/kiro/internal/prop"
)

const gb = "Four score and seven years ago our fathers brought forth on this continent, a new nation, " +
	"conceived in Liberty, and dedicated to the proposition that all men are created equal. " +
	"Now we are engaged in a great civil war, testing whether that nation, or any nation so conceived " +
	"and so dedicated, can long endure."

func check(q string) Result { return Check(q, Prepare("gb", 1, gb)) }

func TestVerbatim(t *testing.T) {
	r := check("our fathers brought forth on this continent, a new nation")
	if r.Verdict != Verbatim || r.Sim != 1000 || len(r.Spans) != 1 {
		t.Fatalf("got %+v", r)
	}
	if got := r.Spans[0].Excerpt; got != "our fathers brought forth on this continent, a new nation" {
		t.Fatalf("excerpt %q", got)
	}
}

func TestFoldingIsNotDrift(t *testing.T) {
	src := Prepare("x", 1, "It’s the people—all of them—who decide, said the speaker.")
	r := Check(`It's the people-all of them-who decide`, src)
	if r.Verdict != Verbatim {
		t.Fatalf("got %+v", r)
	}
}

func TestClosingPunctuationIsNotDrift(t *testing.T) {
	if r := check("dedicated to the proposition that all men are created equal."); r.Verdict != Verbatim {
		t.Fatalf("got %+v", r)
	}
}

// covers: specter-core/2.2
func TestNegationInsertedIsMaterial(t *testing.T) {
	r := check("testing whether that nation, or any nation so conceived and so dedicated, can not long endure")
	if r.Verdict != Drifted || r.Severity != Material {
		t.Fatalf("got %+v", r)
	}
}

// covers: specter-core/2.2
func TestSpelledNumberChangeIsMaterial(t *testing.T) {
	r := check("Four score and eight years ago our fathers brought forth on this continent")
	if r.Verdict != Drifted || r.Severity != Material {
		t.Fatalf("got %+v", r)
	}
}

func TestCaseOnlyIsCosmetic(t *testing.T) {
	r := check("four score and seven years ago our fathers brought forth")
	if r.Verdict != Drifted || r.Severity != Cosmetic {
		t.Fatalf("got %+v", r)
	}
}

func TestSynonymIsMinor(t *testing.T) {
	r := check("our fathers brought forth upon this continent, a new nation")
	if r.Verdict != Drifted || r.Severity != Minor {
		t.Fatalf("got %+v", r)
	}
}

func TestUnsupported(t *testing.T) {
	r := check("the quick brown fox jumps over the lazy dog today")
	if r.Verdict != Unsupported {
		t.Fatalf("got %+v", r)
	}
}

func TestShortQuoteNeverFuzzy(t *testing.T) {
	if r := check("a old nation"); r.Verdict != Unsupported {
		t.Fatalf("got %+v", r)
	}
}

func TestMidWordIsNotVerbatim(t *testing.T) {
	p := Prepare("x", 1, "that government of the people, by the people, for the people")
	if r := Check("ernment of the people, by the people", p); r.Verdict == Verbatim {
		t.Fatalf("mid-word hit passed as verbatim: %+v", r)
	}
}

func TestEllipsis(t *testing.T) {
	r := check("Four score and seven years ago ... conceived in Liberty")
	if r.Verdict != Verbatim || !r.Elided || len(r.Spans) != 2 {
		t.Fatalf("got %+v", r)
	}
	r = check("conceived in Liberty ... Four score and seven years ago")
	if r.Verdict != Drifted || r.Severity != Material {
		t.Fatalf("reordered pieces: %+v", r)
	}
}

func TestRepeatedPassageIsNoted(t *testing.T) {
	p := Prepare("x", 1, "to be or not to be, that is the question. to be or not to be, again.")
	r := Check("to be or not to be", p)
	if r.Verdict != Verbatim || len(r.Notes) == 0 {
		t.Fatalf("got %+v", r)
	}
}

func TestEmpty(t *testing.T) {
	if r := check("  ... "); r.Verdict != Unsupported {
		t.Fatalf("got %+v", r)
	}
}

// ---- properties ----

func randomSource(r *rand.Rand) (words []string, src string) {
	words = prop.Words(r, 40+r.Intn(80))
	return words, strings.Join(words, " ")
}

func TestPropSpanOfSourceIsVerbatim(t *testing.T) {
	prop.Check(t, 300, func(r *rand.Rand) error {
		words, src := randomSource(r)
		n := 4 + r.Intn(10)
		i := r.Intn(len(words) - n)
		q := strings.Join(words[i:i+n], " ")
		got := Check(q, Prepare("s", 1, src))
		if got.Verdict != Verbatim || got.Sim != 1000 {
			return fmt.Errorf("quote %q graded %s", q, got.Verdict)
		}
		return nil
	})
}

func TestPropChangedWordIsNeverVerbatim(t *testing.T) {
	prop.Check(t, 300, func(r *rand.Rand) error {
		words, src := randomSource(r)
		n := 4 + r.Intn(10)
		i := r.Intn(len(words) - n)
		span := append([]string(nil), words[i:i+n]...)
		span[r.Intn(n)] = "zzyzx"
		q := strings.Join(span, " ")
		if got := Check(q, Prepare("s", 1, src)); got.Verdict == Verbatim {
			return fmt.Errorf("altered quote %q graded verbatim", q)
		}
		return nil
	})
}

// covers: specter-core/2.2
func TestPropInsertedNegationIsMaterial(t *testing.T) {
	prop.Check(t, 300, func(r *rand.Rand) error {
		words, src := randomSource(r)
		n := 6 + r.Intn(8)
		i := r.Intn(len(words) - n)
		span := append([]string(nil), words[i:i+n]...)
		k := 1 + r.Intn(n-2) // keep first and last words so the span still anchors
		span = append(span[:k], append([]string{"never"}, span[k:]...)...)
		q := strings.Join(span, " ")
		got := Check(q, Prepare("s", 1, src))
		if got.Verdict == Verbatim || got.Verdict == Drifted && got.Severity != Material {
			return fmt.Errorf("quote with inserted negation %q graded %s/%s", q, got.Verdict, got.Severity)
		}
		return nil
	})
}
