package web

import (
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"menata.app/internal/storage"
)

// A file stored for a write the guards then refuse is removed; one stored for a write that was accepted stays; and a read
// touches nothing.
func TestDiscardRefusedUploadsRemovesOnlyWhatARefusedWriteStored(t *testing.T) {
	files, err := storage.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, method string
		status       int
		kept         bool
	}{
		{"a refused write", http.MethodPost, http.StatusUnprocessableEntity, false},
		{"a failed write", http.MethodPatch, http.StatusInternalServerError, false},
		{"an accepted write", http.MethodPost, http.StatusOK, true},
		{"an accepted write that redirects", http.MethodPost, http.StatusSeeOther, true},
		{"a read", http.MethodGet, http.StatusBadRequest, true},
	} {
		var key string
		h := discardRefusedUploads(files)(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			var err error
			if key, err = files.Save("mch_x", "fld_file", "proof.pdf", strings.NewReader("%PDF-1.4")); err != nil {
				t.Fatal(err)
			}
			noteUpload(req.Context(), key)
			w.WriteHeader(tc.status)
		}))
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(tc.method, "/", nil))
		path, err := files.Path(key)
		if err != nil {
			t.Fatal(err)
		}
		_, statErr := os.Stat(path)
		if kept := statErr == nil; kept != tc.kept {
			t.Errorf("%s: file kept = %v, want %v", tc.name, kept, tc.kept)
		}
	}
}

// The middleware does nothing unless the router mounts it on the authenticated group, where every write that stores a file lives.
func TestRouterMountsTheUploadLedger(t *testing.T) {
	src, err := os.ReadFile("router.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "pr.Use(discardRefusedUploads(") {
		t.Error("router.go does not mount discardRefusedUploads on the authenticated routes: a refused write would leave its upload on disk")
	}
}

// A PATCH is logged by the names of the Fields it sent and never their values, so a dispute about what a Save did is settled from
// the log: an Edit form sends its own controls, a transition Button sends the status Field alone.
func TestLogPatchNamesTheFieldsAndNeverTheirValues(t *testing.T) {
	var buf strings.Builder
	prev := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(prev)
	logPatch("mch_contract", "rec_1", []string{"fld_party_name", "fld_value"})
	got := buf.String()
	if !strings.Contains(got, "RECORD-PATCH machine=mch_contract record=rec_1 fields=fld_party_name,fld_value") {
		t.Errorf("the line does not name the Fields: %q", got)
	}
}
