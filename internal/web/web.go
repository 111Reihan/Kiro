// Package web embeds the built UI (web/ is the source; `npm run build` in web/
// writes here).
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// FS is the built site, rooted at its index.html.
func FS() fs.FS {
	f, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // dist is embedded at build time; it cannot be missing
	}
	return f
}
