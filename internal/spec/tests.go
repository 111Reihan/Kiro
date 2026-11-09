package spec

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"regexp"
	"strings"
)

// Test is one test function that says which requirements it covers, with a
// comment such as  // covers: R3, R4  directly above it.
type Test struct {
	Name   string   `json:"name"`
	File   string   `json:"file"`
	Line   int      `json:"line"`
	Covers []string `json:"covers"` // global requirement ids, resolved by Scan
	Body   string   `json:"body"`   // hash of the test's code, comments left out
	raw    []string
}

var (
	annotRe = regexp.MustCompile(`(?i)^\s*(?://|#|/\*|\*|--)\s*@?covers?\b:?\s*(.+?)\s*(?:\*/)?\s*$`)
	goDecl  = regexp.MustCompile(`^func\s+((?:Test|Fuzz)\w*)\s*\(`)
	pyDecl  = regexp.MustCompile(`^\s*(?:async\s+)?def\s+(test\w*)\s*\(`)
	jsDecl  = regexp.MustCompile("^\\s*(?:it|test)(?:\\.(?:only|skip))?\\(\\s*(?:'([^']+)'|\"([^\"]+)\"|`([^`]+)`)")
	jvDecl  = regexp.MustCompile(`^\s*(?:(?:public|private|protected|static|final|suspend)\s+)*(?:void|fun)\s+(\w+)\s*\(`)
	rsDecl  = regexp.MustCompile(`^\s*(?:pub\s+)?(?:async\s+)?fn\s+(\w+)\s*\(`)
	jvAttr  = regexp.MustCompile(`^\s*@(?:Test|ParameterizedTest|RepeatedTest)\b`)
	rsAttr  = regexp.MustCompile(`^\s*#\[(?:\w+::)?test\b`)
)

// testExts maps a file extension to the language its tests are read as.
var testExts = map[string]string{
	".go": "go", ".py": "py", ".js": "js", ".jsx": "js", ".mjs": "js", ".cjs": "js",
	".ts": "js", ".tsx": "js", ".java": "jv", ".kt": "jv", ".rs": "rs",
}

func splitIDs(s string) []string {
	var out []string
	for _, id := range idSplit.Split(s, -1) {
		if id = strings.Trim(id, "*_`.:\"'"); id != "" {
			out = append(out, id)
		}
	}
	return out
}

func indent(s string) int {
	n := 0
	for _, r := range s {
		if r == ' ' {
			n++
		} else if r == '\t' {
			n += 4
		} else {
			break
		}
	}
	return n
}

// ParseTests lists the test functions in one source file that carry a covers: note.
// shortcut: bodies end by indentation, not by parsing the language; exotic layouts can
// end a body early, which only makes the "test changed" hash less precise.
func ParseTests(file, src string) []Test {
	lang := testExts[strings.ToLower(filepath.Ext(file))]
	if lang == "" {
		return nil
	}
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	var out []Test
	type note struct {
		line int
		ids  []string
	}
	var pending []note
	cur := -1 // index in out of the test whose first lines still accept a note
	curLine, curIndent := 0, 0
	attr := -10    // line of the last @Test / #[test] attribute
	inRaw := false // inside a backtick string: fixtures in a test file are not tests
	for i, ln := range lines {
		if lang == "go" || lang == "js" {
			odd := strings.Count(ln, "`")%2 == 1
			if inRaw {
				inRaw = !odd
				continue
			}
			inRaw = odd
		}
		if m := annotRe.FindStringSubmatch(ln); m != nil {
			ids := splitIDs(m[1])
			if cur >= 0 && i-curLine <= 4 && indent(ln) > curIndent {
				out[cur].raw = append(out[cur].raw, ids...)
			} else {
				pending = append(pending, note{i, ids})
			}
			continue
		}
		if lang == "jv" && jvAttr.MatchString(ln) || lang == "rs" && rsAttr.MatchString(ln) {
			attr = i
			continue
		}
		name := ""
		switch lang {
		case "go":
			if m := goDecl.FindStringSubmatch(ln); m != nil {
				name = m[1]
			}
		case "py":
			if m := pyDecl.FindStringSubmatch(ln); m != nil {
				name = m[1]
			}
		case "js":
			if m := jsDecl.FindStringSubmatch(ln); m != nil {
				name = m[1] + m[2] + m[3]
			}
		case "jv":
			if m := jvDecl.FindStringSubmatch(ln); m != nil && i-attr <= 3 {
				name = m[1]
			}
		case "rs":
			if m := rsDecl.FindStringSubmatch(ln); m != nil && i-attr <= 3 {
				name = m[1]
			}
		}
		if name == "" {
			continue
		}
		t := Test{Name: name, File: file, Line: i + 1, Body: bodyHash(lines, i)}
		for _, n := range pending {
			if i-n.line <= 8 {
				t.raw = append(t.raw, n.ids...)
			}
		}
		pending = pending[:0]
		out = append(out, t)
		cur, curLine, curIndent = len(out)-1, i, indent(ln)
	}
	kept := out[:0]
	for _, t := range out {
		if len(t.raw) > 0 {
			kept = append(kept, t)
		}
	}
	return kept
}

// bodyHash fingerprints a test's code, ignoring comment-only lines and indentation,
// so editing a comment does not count as changing the test.
func bodyHash(lines []string, start int) string {
	base := indent(lines[start])
	h := sha256.New()
	for j := start; j < len(lines); j++ {
		t := strings.TrimSpace(lines[j])
		if j > start && t != "" && indent(lines[j]) <= base {
			if strings.HasPrefix(t, "}") || strings.HasPrefix(t, ")") || strings.HasPrefix(t, "]") {
				h.Write([]byte(t + "\n"))
			}
			break
		}
		if t == "" || strings.HasPrefix(t, "//") || strings.HasPrefix(t, "#") && !strings.HasPrefix(t, "#[") || strings.HasPrefix(t, "*") || strings.HasPrefix(t, "/*") {
			continue
		}
		h.Write([]byte(t + "\n"))
	}
	return hex.EncodeToString(h.Sum(nil)[:8])
}
