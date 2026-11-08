package spec

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const maxFile = 1 << 20

// File is a requirements file as read from disk.
type File struct {
	Path string `json:"path"` // relative to the root, slash separated
	Spec string `json:"spec"`
	Text string `json:"-"`
}

// Project is everything found under a root.
type Project struct {
	Root     string   `json:"root"`
	Files    []File   `json:"files"`
	Reqs     []Req    `json:"reqs"`
	Tasks    []Task   `json:"tasks"`
	Tests    []Test   `json:"tests"`
	Warnings []string `json:"warnings"`
}

var skipDir = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, "dist": true, "build": true, "target": true,
	"data": true, ".venv": true, "venv": true, "__pycache__": true, ".next": true,
}

// Scan reads requirements.md files, the tasks.md beside them, and every test
// file under root. It never writes.
func Scan(root string) (*Project, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", root)
	}
	p := &Project{Root: root}
	var testFiles []string
	err = filepath.WalkDir(root, func(fp string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // an unreadable corner should not stop the scan
		}
		if d.IsDir() {
			if fp != root && (skipDir[d.Name()] || strings.HasPrefix(d.Name(), ".") && d.Name() != ".kiro") {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(root, fp)
		rel = filepath.ToSlash(rel)
		switch {
		case d.Name() == "requirements.md":
			if txt, ok := readText(fp); ok {
				p.Files = append(p.Files, File{Path: rel, Text: txt})
			} else {
				p.Warnings = append(p.Warnings, rel+": skipped (not UTF-8 text, or over 1 MiB)")
			}
		case testExts[strings.ToLower(filepath.Ext(d.Name()))] != "":
			testFiles = append(testFiles, rel)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// name each spec after its folder; two folders with one name get their full path
	count := map[string]int{}
	for _, f := range p.Files {
		count[specName(f.Path, false)]++
	}
	for i := range p.Files {
		f := &p.Files[i]
		f.Spec = specName(f.Path, count[specName(f.Path, false)] > 1)
		p.Reqs = append(p.Reqs, ParseRequirements(f.Spec, f.Path, f.Text)...)
		tp := path.Join(path.Dir(f.Path), "tasks.md")
		if txt, ok := readText(filepath.Join(root, filepath.FromSlash(tp))); ok {
			p.Tasks = append(p.Tasks, ParseTasks(f.Spec, tp, txt)...)
		}
	}
	seen := map[string]bool{}
	for _, r := range p.Reqs {
		if seen[r.ID] {
			p.Warnings = append(p.Warnings, fmt.Sprintf("%s:%d: duplicate requirement id %s; the first one is used", r.File, r.Line, r.ID))
		}
		seen[r.ID] = true
	}
	for _, rel := range testFiles {
		if txt, ok := readText(filepath.Join(root, filepath.FromSlash(rel))); ok {
			p.Tests = append(p.Tests, ParseTests(rel, txt)...)
		}
	}
	p.resolve()
	return p, nil
}

func specName(rel string, full bool) string {
	dir := path.Dir(rel)
	switch {
	case dir == ".":
		return "project"
	case full:
		return strings.ReplaceAll(dir, "/", ".")
	}
	return path.Base(dir)
}

func readText(fp string) (string, bool) {
	st, err := os.Stat(fp)
	if err != nil || st.Size() > maxFile {
		return "", false
	}
	b, err := os.ReadFile(fp)
	if err != nil || !utf8.Valid(b) {
		return "", false
	}
	return string(b), true
}

// resolve turns the ids written in tests and tasks into global requirement ids.
// A bare id (R3) works when exactly one spec has it; a qualified one (spec/R3) always does.
func (p *Project) resolve() {
	global := map[string]bool{}
	byLocal := map[string][]string{}
	for _, r := range p.Reqs {
		if !global[r.ID] {
			byLocal[r.Local] = append(byLocal[r.Local], r.ID)
		}
		global[r.ID] = true
	}
	lookup := func(tok, prefer string) (string, string) { // id, problem
		if global[tok] {
			return tok, ""
		}
		if prefer != "" && global[prefer+"/"+tok] {
			return prefer + "/" + tok, ""
		}
		switch c := byLocal[tok]; len(c) {
		case 1:
			return c[0], ""
		case 0:
			return "", "no requirement " + tok
		default:
			return "", tok + " is ambiguous (" + strings.Join(c, ", ") + "); write it as spec/" + tok
		}
	}
	for i := range p.Tests {
		t := &p.Tests[i]
		for _, tok := range t.raw {
			id, bad := lookup(tok, "")
			if bad != "" {
				p.Warnings = append(p.Warnings, fmt.Sprintf("%s:%d (%s): %s", t.File, t.Line, t.Name, bad))
				continue
			}
			t.Covers = appendOnce(t.Covers, id)
		}
	}
	for i := range p.Tasks {
		t := &p.Tasks[i]
		for _, tok := range t.raw {
			id, bad := lookup(tok, t.Spec)
			if bad != "" {
				p.Warnings = append(p.Warnings, fmt.Sprintf("%s:%d: task refers to a missing requirement: %s", t.File, t.Line, bad))
				continue
			}
			t.Reqs = appendOnce(t.Reqs, id)
		}
	}

}

func appendOnce(l []string, s string) []string {
	for _, x := range l {
		if x == s {
			return l
		}
	}
	return append(l, s)
}

// TestsFor lists the tests that say they cover a requirement.
func (p *Project) TestsFor(id string) []Test {
	var out []Test
	for _, t := range p.Tests {
		for _, c := range t.Covers {
			if c == id {
				out = append(out, t)
				break
			}
		}
	}
	return out
}

// TasksFor lists the tasks that name a requirement.
func (p *Project) TasksFor(id string) []Task {
	var out []Task
	for _, t := range p.Tasks {
		for _, c := range t.Reqs {
			if c == id {
				out = append(out, t)
				break
			}
		}
	}
	return out
}
