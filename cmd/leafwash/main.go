// Command leafwash is the single-node executable entry point for the LeafWash
// 联检闭环 backend. It wires the seeded read-side catalog, the SQLite WAL
// persistence directory, the transactional service layer, the JSON API, and the
// embedded frontend into one HTTP service. Open tasks, leases, adapter retries,
// idempotency records, and state are recovered from SQLite after a restart.
package main

import (
	"log"
	"net/http"
	"os"

	"leafwash-packaging-release-gate/api"
	"leafwash-packaging-release-gate/store"
)

func main() {
	addr := envOr("LEAFWASH_ADDR", ":8080")
	dataPath := envOr("LEAFWASH_DATA", "leafwash.db")

	cat := seedCatalog()
	backend, err := store.OpenSQLite(dataPath, cat)
	if err != nil {
		log.Fatalf("open sqlite: %v", err)
	}
	defer backend.Close()

	svc := store.NewService(backend)

	srv := api.New(svc)

	fe, err := webembedFS()
	if err != nil {
		log.Fatalf("load embedded frontend: %v", err)
	}
	srv.WithFrontend(http.FS(fe))

	log.Printf("LeafWash backend listening on %s (data=%s)", addr, dataPath)
	if err := http.ListenAndServe(addr, srv.Handler()); err != nil {
		log.Fatalf("server: %v", err)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
