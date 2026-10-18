package web

import (
	"context"
	"log"
	"net/http"
	"sync"

	chimw "github.com/go-chi/chi/v5/middleware"

	"menata.app/internal/storage"
)

// A file is stored while a write's body is read, before the guards that may refuse the write have run -- a missing Permission, a
// frozen record, a failed validation. Each refusal used to leave the file on disk with nothing pointing at it. The ledger
// notes every file a request stored, and discardRefusedUploads removes them when the request ends in an error status, so a
// refused write leaves no file behind and an accepted one keeps its own.
type uploadLedgerKey struct{}

type uploadLedger struct {
	mu   sync.Mutex
	keys []string
}

func (l *uploadLedger) note(key string) {
	l.mu.Lock()
	l.keys = append(l.keys, key)
	l.mu.Unlock()
}

// noteUpload records a stored file in the request's ledger; a request with none (a test calling a handler bare) records nothing.
func noteUpload(ctx context.Context, key string) {
	if l, ok := ctx.Value(uploadLedgerKey{}).(*uploadLedger); ok {
		l.note(key)
	}
}

// discardRefusedUploads is the middleware: it gives the request a ledger and, when the response is an error (4xx or 5xx),
// removes the files the request stored. A write that was accepted answers 2xx or 3xx and keeps them.
func discardRefusedUploads(files *storage.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if files == nil || req.Method == http.MethodGet || req.Method == http.MethodHead {
				next.ServeHTTP(w, req)
				return
			}
			ledger := &uploadLedger{}
			ww := chimw.NewWrapResponseWriter(w, req.ProtoMajor)
			next.ServeHTTP(ww, req.WithContext(context.WithValue(req.Context(), uploadLedgerKey{}, ledger)))
			if ww.Status() < http.StatusBadRequest {
				return
			}
			for _, key := range ledger.keys {
				if err := files.Remove(key); err != nil {
					log.Printf("upload cleanup: removing %q after a refused write: %v", key, err)
				}
			}
		})
	}
}
