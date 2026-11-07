// Package align finds and compares token sequences. Edit distance over words,
// not characters: a changed word is one change, however long the word.
package align

// Match is a window [Start,End) of source tokens and its edit distance to the query.
type Match struct{ Start, End, Dist int }

// Locate finds the window of s closest to q (semi-global alignment: the query
// may start and end anywhere in s). Memory is O(len(s)); time is O(len(q)*len(s)),
// so callers cap both sides.
func Locate(q, s []string) Match {
	m, n := len(q), len(s)
	pd, ps := make([]int, n+1), make([]int, n+1)
	cd, cs := make([]int, n+1), make([]int, n+1)
	for j := 0; j <= n; j++ {
		ps[j] = j // an alignment may begin at any source position for free
	}
	for i := 1; i <= m; i++ {
		cd[0], cs[0] = i, 0
		for j := 1; j <= n; j++ {
			sub := 1
			if q[i-1] == s[j-1] {
				sub = 0
			}
			d, st := pd[j-1]+sub, ps[j-1]
			if v := pd[j] + 1; v < d { // query word missing from source
				d, st = v, ps[j]
			}
			if v := cd[j-1] + 1; v < d { // extra source word
				d, st = v, cs[j-1]
			}
			cd[j], cs[j] = d, st
		}
		pd, cd = cd, pd
		ps, cs = cs, ps
	}
	best := 0
	for j := 1; j <= n; j++ {
		if pd[j] < pd[best] {
			best = j
		}
	}
	return Match{Start: ps[best], End: best, Dist: pd[best]}
}

// Op is one step of a word diff between a quote and its source.
type Op struct {
	Kind string `json:"k"`           // eq, sub, del (only in quote), ins (only in source)
	Q    string `json:"q,omitempty"` // quote word
	S    string `json:"s,omitempty"` // source word
}

// Diff aligns q to s end to end. Both are short (one quote and its matched window).
func Diff(q, s []string) []Op {
	m, n := len(q), len(s)
	d := make([][]int, m+1)
	for i := range d {
		d[i] = make([]int, n+1)
		d[i][0] = i
	}
	for j := 0; j <= n; j++ {
		d[0][j] = j
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			sub := 1
			if q[i-1] == s[j-1] {
				sub = 0
			}
			d[i][j] = min(d[i-1][j-1]+sub, d[i-1][j]+1, d[i][j-1]+1)
		}
	}
	var ops []Op
	i, j := m, n
	for i > 0 || j > 0 {
		switch {
		case i > 0 && j > 0 && q[i-1] == s[j-1] && d[i][j] == d[i-1][j-1]:
			ops = append(ops, Op{"eq", q[i-1], s[j-1]})
			i, j = i-1, j-1
		case i > 0 && j > 0 && d[i][j] == d[i-1][j-1]+1:
			ops = append(ops, Op{"sub", q[i-1], s[j-1]})
			i, j = i-1, j-1
		case i > 0 && d[i][j] == d[i-1][j]+1:
			ops = append(ops, Op{Kind: "del", Q: q[i-1]})
			i--
		default:
			ops = append(ops, Op{Kind: "ins", S: s[j-1]})
			j--
		}
	}
	for l, r := 0, len(ops)-1; l < r; l, r = l+1, r-1 {
		ops[l], ops[r] = ops[r], ops[l]
	}
	return ops
}
