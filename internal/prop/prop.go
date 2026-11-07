// Package prop is a tiny property-test driver: run a check over many seeded
// random cases and print the failing seed so the case can be replayed.
package prop

import (
	"math/rand"
	"testing"
)

// Check runs f for n seeds. f returns an error when the property breaks.
func Check(t *testing.T, n int, f func(r *rand.Rand) error) {
	t.Helper()
	for seed := int64(1); seed <= int64(n); seed++ {
		if err := f(rand.New(rand.NewSource(seed))); err != nil {
			t.Fatalf("property failed at seed %d: %v", seed, err)
		}
	}
}

var vocab = []string{
	"the", "people", "government", "shall", "not", "perish", "from", "earth",
	"liberty", "dedicated", "proposition", "created", "equal", "nation", "new",
	"conceived", "continent", "forth", "fathers", "brought", "our", "four",
	"score", "and", "seven", "years", "ago", "whether", "any", "long",
	"endure", "great", "civil", "war", "testing", "Lincoln", "1863", "must",
	"all", "men", "are", "born", "free", "rights", "life", "happiness",
}

// Words returns n random words with random trailing punctuation, joined by single spaces.
func Words(r *rand.Rand, n int) []string {
	out := make([]string, n)
	for i := range out {
		w := vocab[r.Intn(len(vocab))]
		switch r.Intn(8) {
		case 0:
			w += ","
		case 1:
			w += "."
		}
		out[i] = w
	}
	return out
}
