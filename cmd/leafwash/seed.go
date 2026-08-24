package main

import (
	"io/fs"

	"leafwash-packaging-release-gate/catalog"
	"leafwash-packaging-release-gate/webembed"
)

// seedCatalog builds the in-memory read-side catalog with the fictional base
// lots, wash-rule revisions, personnel, and templates used by the demo.
func seedCatalog() *catalog.Memory {
	c := catalog.NewMemory()
	catalog.Seed(c)
	return c
}

// webembedFS returns the embedded frontend file system.
func webembedFS() (fs.FS, error) {
	return webembed.FS()
}
