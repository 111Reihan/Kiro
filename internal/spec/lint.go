package spec

import (
	"regexp"
	"strings"
)

// Issue is one thing wrong with how a requirement is written.
type Issue struct {
	Code string `json:"code"`
	Msg  string `json:"msg"`
}

var (
	reShall   = regexp.MustCompile(`(?i)\b(shall|must)\b`)
	reWeak    = regexp.MustCompile(`(?i)\b(should|may|might|could|will)\b`)
	reWhen    = regexp.MustCompile(`(?i)^\W*(when|whenever)\b`)
	reWhile   = regexp.MustCompile(`(?i)\bwhile\b`)
	reIf      = regexp.MustCompile(`(?i)^\W*if\b`)
	reWhere   = regexp.MustCompile(`(?i)^\W*where\b`)
	reVague   = regexp.MustCompile(`(?i)\b(fast|quick(?:ly)?|slow|easy|easily|user[- ]friendly|appropriate(?:ly)?|adequate(?:ly)?|reasonabl[ey]|efficient(?:ly)?|robust|flexible|seamless(?:ly)?|intuitive|as needed|if necessary|as soon as possible|etc|tbd|and/or)\b`)
	reNumber  = regexp.MustCompile(`\d`)
	reTimeish = regexp.MustCompile(`(?i)\b(within|no more than|at most|at least|under|over)\b`)
)

// Classify names the EARS pattern of a clause and lists writing problems.
// It reads keywords only; it does not understand the sentence.
func Classify(t string) (kind string, issues []Issue) {
	switch {
	case reIf.MatchString(t):
		kind = "unwanted"
	case reWhen.MatchString(t) && reWhile.MatchString(t):
		kind = "complex"
	case reWhen.MatchString(t):
		kind = "event"
	case reWhile.MatchString(t):
		kind = "state"
	case reWhere.MatchString(t):
		kind = "optional"
	case reShall.MatchString(t):
		kind = "ubiquitous"
	default:
		kind = "unclassified"
	}
	switch {
	case reShall.MatchString(t):
	case reWeak.MatchString(t):
		issues = append(issues, Issue{"weak-verb", "uses " + strings.ToLower(reWeak.FindString(t)) + " instead of SHALL, so it is unclear whether this is required"})
	default:
		issues = append(issues, Issue{"no-shall", "no SHALL: this reads as a description, not a requirement"})
	}
	if v := reVague.FindString(t); v != "" {
		issues = append(issues, Issue{"vague", "\"" + v + "\" cannot be tested; give a number or a condition"})
	}
	if reTimeish.MatchString(t) && !reNumber.MatchString(t) {
		issues = append(issues, Issue{"unquantified", "states a limit without a number"})
	}
	return kind, issues
}
