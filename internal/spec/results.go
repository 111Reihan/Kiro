package spec

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"path"
	"path/filepath"
	"strings"
)

// Result is the outcome of one test in a run.
type Result struct {
	Name   string `json:"name"`
	Pkg    string `json:"pkg,omitempty"` // Go package, or JUnit classname
	Status string `json:"status"`        // pass, fail, skip
}

const (
	Pass = "pass"
	Fail = "fail"
	Skip = "skip"
)

// ParseResults reads either `go test -json` output or a JUnit XML report.
func ParseResults(raw []byte) ([]Result, error) {
	t := bytes.TrimSpace(raw)
	switch {
	case len(t) == 0:
		return nil, errors.New("no test results in the input")
	case t[0] == '<':
		return parseJUnit(t)
	default:
		return parseGoJSON(t)
	}
}

func parseGoJSON(raw []byte) ([]Result, error) {
	type ev struct{ Action, Package, Test string }
	var out []Result
	idx := map[string]int{}
	for _, ln := range bytes.Split(raw, []byte("\n")) {
		var e ev
		if json.Unmarshal(bytes.TrimSpace(ln), &e) != nil || e.Test == "" || strings.Contains(e.Test, "/") {
			continue // build chatter, package lines and subtests: the parent test carries the verdict
		}
		var st string
		switch e.Action {
		case "pass":
			st = Pass
		case "fail":
			st = Fail
		case "skip":
			st = Skip
		default:
			continue
		}
		k := e.Package + "\x00" + e.Test
		if i, ok := idx[k]; ok {
			out[i].Status = st
			continue
		}
		idx[k] = len(out)
		out = append(out, Result{Name: e.Test, Pkg: e.Package, Status: st})
	}
	if len(out) == 0 {
		return nil, errors.New("no tests found: expected `go test -json` output or a JUnit XML file")
	}
	return out, nil
}

func parseJUnit(raw []byte) ([]Result, error) {
	d := xml.NewDecoder(bytes.NewReader(raw))
	d.Strict = false
	var out []Result
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, errors.New("not valid JUnit XML: " + err.Error())
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "testcase" {
			continue
		}
		r := Result{Status: Pass}
		for _, a := range se.Attr {
			switch a.Name.Local {
			case "name":
				r.Name = a.Value
			case "classname":
				r.Pkg = a.Value
			}
		}
		for depth := 1; depth > 0; {
			t, err := d.Token()
			if err != nil {
				return nil, errors.New("not valid JUnit XML: " + err.Error())
			}
			switch x := t.(type) {
			case xml.StartElement:
				depth++
				switch x.Name.Local {
				case "failure", "error":
					r.Status = Fail
				case "skipped":
					if r.Status != Fail {
						r.Status = Skip
					}
				}
			case xml.EndElement:
				depth--
			}
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil, errors.New("no testcase elements in the XML")
	}
	return out, nil
}

// Status finds how a test did in a run. "none" means the run has no such test.
// Several matches (the same name in two packages) are resolved by the test's
// folder first; any failure among what is left wins.
func Status(t Test, rs []Result) string {
	var hit []Result
	for _, r := range rs {
		if r.Name == t.Name || strings.HasSuffix(r.Name, " "+t.Name) || strings.HasSuffix(r.Name, "."+t.Name) {
			hit = append(hit, r)
		}
	}
	if len(hit) > 1 {
		dir := path.Base(filepath.ToSlash(filepath.Dir(t.File)))
		var near []Result
		for _, r := range hit {
			if strings.HasSuffix(r.Pkg, "/"+dir) || r.Pkg == dir {
				near = append(near, r)
			}
		}
		if len(near) > 0 {
			hit = near
		}
	}
	best := "none"
	for _, r := range hit {
		switch {
		case r.Status == Fail:
			return Fail
		case r.Status == Pass:
			best = Pass
		case best == "none":
			best = Skip
		}
	}
	return best
}
