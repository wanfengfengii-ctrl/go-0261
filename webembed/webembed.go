// Package webembed embeds the built LeafWash 联检控制台 static assets so the
// single binary serves the browser page that renders live backend state.
package webembed

import (
	"embed"
	"io/fs"
)

//go:embed dist
var dist embed.FS

// FS returns the embedded frontend as an http.FileSystem-compatible fs.FS
// rooted at the dist directory.
func FS() (fs.FS, error) {
	return fs.Sub(dist, "dist")
}
