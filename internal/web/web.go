// Package web embeds the browser UI so the whole app is one binary.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:static
var files embed.FS

// FS is the UI, rooted at its index.html.
func FS() fs.FS {
	s, err := fs.Sub(files, "static")
	if err != nil {
		panic(err) // the directory is embedded at build time
	}
	return s
}
