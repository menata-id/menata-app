// Command server is the Menata App entrypoint: a single binary that will grow to realize
// Runtime Metadata into a running application, per 002-architecture.md's Runtime Realization
// pipeline. Phase 0 only proves the binary builds, listens, and reports health.
package main

import (
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"

	"menata.app/internal/config"
)

func main() {
	cfg := config.Load()

	r := chi.NewRouter()
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	log.Printf("menata-app listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, r); err != nil {
		log.Fatal(err)
	}
}
