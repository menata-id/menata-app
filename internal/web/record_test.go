package web

import (
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"menata.app/internal/domain"
	"menata.app/internal/storage"
)

func fileFieldMachine() *domain.Machine {
	return &domain.Machine{
		ID: "mch_record_test",
		Fields: []domain.Field{
			{ID: "fld_title", Name: "Title", Type: domain.FieldTypeText, Required: true},
			{ID: "fld_file", Name: "File", Type: domain.FieldTypeFile},
		},
	}
}

// TestHandleFileUploads_nonMultipartBody is the regression test for ROADMAP.md's Operational
// backlog item "Non-multipart form bodies fail on any Machine with a file field" (pre-existing
// since Phase 19): a plain url-encoded POST to a Machine with a FieldTypeFile field previously
// 400'd, because only http.ErrMissingFile was tolerated from req.FormFile, not
// http.ErrNotMultipart.
func TestHandleFileUploads_nonMultipartBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("fld_title=hello"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := req.ParseForm(); err != nil {
		t.Fatalf("ParseForm: %v", err)
	}

	files, err := storage.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("storage.NewStore: %v", err)
	}

	uploaded, err := handleFileUploads(req, fileFieldMachine(), files)
	if err != nil {
		t.Fatalf("handleFileUploads(non-multipart body) error = %v, want nil", err)
	}
	if len(uploaded) != 0 {
		t.Errorf("handleFileUploads(non-multipart body) = %v, want empty (no file possible in a non-multipart body)", uploaded)
	}
}

// TestHandleFileUploads_multipartWithFile confirms the fix didn't break the real upload path --
// a genuinely multipart body with a file present still saves it.
func TestHandleFileUploads_multipartWithFile(t *testing.T) {
	var body strings.Builder
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("fld_title", "hello"); err != nil {
		t.Fatalf("WriteField: %v", err)
	}
	part, err := writer.CreateFormFile("fld_file", "doc.pdf")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write([]byte("pdf bytes")); err != nil {
		t.Fatalf("write file part: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body.String()))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if err := req.ParseMultipartForm(maxUploadBytes); err != nil {
		t.Fatalf("ParseMultipartForm: %v", err)
	}

	files, err := storage.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("storage.NewStore: %v", err)
	}

	uploaded, err := handleFileUploads(req, fileFieldMachine(), files)
	if err != nil {
		t.Fatalf("handleFileUploads(multipart with file) error = %v, want nil", err)
	}
	if _, ok := uploaded["fld_file"]; !ok {
		t.Errorf("handleFileUploads(multipart with file) = %v, want a key for fld_file", uploaded)
	}
}
