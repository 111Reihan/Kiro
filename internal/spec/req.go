// Package spec reads a project's requirements, tasks and tests from disk and
// links them. It touches no log and no network: scanning the same tree twice
// gives the same answer.
package spec

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

// Req is one requirement: a single EARS-style clause with an id.
type Req struct {
	ID    string  `json:"id"`    // spec/local, e.g. quote-verification/R3
	Spec  string  `json:"spec"`  // name of the folder holding requirements.md
	Local string  `json:"local"` // R3, or 1.2 in Kiro's numbered style
	Text  string  `json:"text"`  // the clause, on one line, markdown kept
	File  string  `json:"file"`  // path relative to the project root
	Line  int     `json:"line"`
	Kind  string  `json:"kind"` // EARS pattern
	Hash  string  `json:"hash"` // of the folded words, so spacing and case do not matter
	Lint  []Issue `json:"lint,omitempty"`
}

var (
	// **R3** text   or   - **G10** text   or   REQ-4: text
	boldID  = regexp.MustCompile(`^\s*(?:[-*]\s+)?\*\*([A-Za-z][A-Za-z0-9-]*\d+(?:\.\d+)*)\*\*:?\s+(\S.*)$`)
	plainID = regexp.MustCompile(`^\s*(?:[-*]\s+)?([A-Z]{1,6}-\d+(?:\.\d+)*):\s+(\S.*)$`)
	// Kiro's own layout: "### Requirement 2" then numbered acceptance criteria.
	kiroHead = regexp.MustCompile(`(?i)^#{1,6}\s*requirement\s+(\d+)\b`)
	kiroItem = regexp.MustCompile(`^\s*(\d+)\.\s+(\S.*)$`)
	heading  = regexp.MustCompile(`^\s*#{1,6}\s`)
)

// ParseRequirements finds the requirement items in one requirements.md.
// spec is the folder name; file is the path recorded on each item.
func ParseRequirements(spec, file, src string) []Req {
	var out []Req
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	kiro := "" // current "Requirement N" number, in Kiro layout
	cur := -1  // index in out of the item being continued
	inFence := false
	for i, ln := range lines {
		if strings.HasPrefix(strings.TrimSpace(ln), "```") {
			inFence = !inFence
			cur = -1
			continue
		}
		if inFence {
			continue
		}
		if m := kiroHead.FindStringSubmatch(ln); m != nil {
			kiro, cur = m[1], -1
			continue
		}
		if heading.MatchString(ln) {
			cur = -1
			continue
		}
		local, text := "", ""
		if m := boldID.FindStringSubmatch(ln); m != nil {
			local, text = m[1], m[2]
		} else if m := plainID.FindStringSubmatch(ln); m != nil {
			local, text = m[1], m[2]
		} else if m := kiroItem.FindStringSubmatch(ln); m != nil && kiro != "" {
			local, text = kiro+"."+m[1], m[2]
		}
		if local != "" {
			out = append(out, Req{ID: spec + "/" + local, Spec: spec, Local: local, Text: oneLine(text), File: file, Line: i + 1})
			cur = len(out) - 1
			continue
		}
		// an indented continuation belongs to the item above it
		if cur >= 0 && strings.TrimSpace(ln) != "" && (strings.HasPrefix(ln, "  ") || strings.HasPrefix(ln, "\t")) {
			out[cur].Text += " " + oneLine(ln)
			continue
		}
		cur = -1
	}
	for i := range out {
		r := &out[i]
		r.Kind, r.Lint = Classify(r.Text)
		r.Hash = WordHash(r.Text)
	}
	return out
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// WordHash fingerprints the words of a clause, ignoring case, punctuation and spacing.
func WordHash(s string) string {
	h := sha256.Sum256([]byte(strings.Join(words(s), " ")))
	return hex.EncodeToString(h[:8])
}

var wordRe = regexp.MustCompile(`[\p{L}\p{N}]+(?:['’.,-][\p{L}\p{N}]+)*`)

func words(s string) []string {
	return wordRe.FindAllString(strings.ToLower(s), -1)
}

// Task is one checkbox line from a Kiro tasks.md.
type Task struct {
	Spec string   `json:"spec"`
	File string   `json:"file"`
	Line int      `json:"line"`
	Text string   `json:"text"`
	Done bool     `json:"done"`
	Reqs []string `json:"reqs"` // global ids, resolved by Scan
	raw  []string
}

var (
	taskLine = regexp.MustCompile(`^\s*[-*]\s+\[([ xX-])\]\s+(\S.*)$`)
	reqsLine = regexp.MustCompile(`(?i)_?\s*requirements?\s*:\s*([^_\n]+?)_?\s*$`)
	idSplit  = regexp.MustCompile(`[,\s;]+`)
)

// ParseTasks reads Kiro's checkbox tasks and the "_Requirements: 1.1, 2.3_" line under each.
func ParseTasks(spec, file, src string) []Task {
	var out []Task
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	for i := 0; i < len(lines); i++ {
		m := taskLine.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		t := Task{Spec: spec, File: file, Line: i + 1, Text: oneLine(m[2]), Done: m[1] == "x" || m[1] == "X"}
		for j := i + 1; j < len(lines) && j <= i+6 && !taskLine.MatchString(lines[j]); j++ {
			if r := reqsLine.FindStringSubmatch(strings.TrimSpace(lines[j])); r != nil {
				for _, id := range idSplit.Split(r[1], -1) {
					if id = strings.Trim(id, "*_`"); id != "" {
						t.raw = append(t.raw, id)
					}
				}
			}
		}
		out = append(out, t)
	}
	return out
}
