// Package examples holds public-domain sources and a draft report with a few
// bad quotes in it, so the app has something to show on first run.
package examples

import (
	"embed"
	"errors"

	"github.com/111Reihan/kiro/internal/engine"
)

//go:embed sources/*.txt report.md
var fsys embed.FS

type Source struct{ ID, Title, File string }

var Sources = []Source{
	{"gettysburg", "Gettysburg Address (Bliss copy, 1863)", "sources/gettysburg.txt"},
	{"declaration", "Declaration of Independence, opening (1776)", "sources/declaration.txt"},
}

// Report is a draft essay with six quotes: three exact, two altered, one invented.
func Report() string {
	b, _ := fsys.ReadFile("report.md")
	return string(b)
}

// Load adds the sample sources. Sources that are already loaded are left alone.
func Load(e *engine.Engine) ([]string, error) {
	var ids []string
	for _, s := range Sources {
		b, err := fsys.ReadFile(s.File)
		if err != nil {
			return nil, err
		}
		if _, err := e.PutDoc(s.ID, s.Title, string(b)); err != nil && !errors.Is(err, engine.ErrNoChange) {
			return nil, err
		}
		ids = append(ids, s.ID)
	}
	return ids, nil
}
