package storage

import (
	"os"
	"strings"
	"testing"
)

func TestStore_SaveAndPathRoundTrip(t *testing.T) {
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	key, err := s.Save("mch_task", "fld_attachment", "contract.pdf", strings.NewReader("pdf bytes"))
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	path, err := s.Path(key)
	if err != nil {
		t.Fatalf("Path() error = %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	if string(got) != "pdf bytes" {
		t.Errorf("file content = %q, want %q", got, "pdf bytes")
	}
}

func TestStore_Save_sanitizesFilename(t *testing.T) {
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	key, err := s.Save("mch_task", "fld_attachment", "../../etc/passwd", strings.NewReader("x"))
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if strings.Contains(key, "..") {
		t.Errorf("key = %q, want no path-traversal segments from a malicious filename", key)
	}
	if _, err := s.Path(key); err != nil {
		t.Errorf("Path(%q) error = %v, want the saved key to resolve cleanly", key, err)
	}
}

func TestStore_Path_rejectsTraversal(t *testing.T) {
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	cases := []string{
		"../../../etc/passwd",
		"mch_task/fld_attachment/../../../etc/passwd",
	}
	for _, key := range cases {
		if _, err := s.Path(key); err == nil {
			t.Errorf("Path(%q) error = nil, want an error rejecting escape from the store root", key)
		}
	}
}

func TestDisplayName(t *testing.T) {
	cases := []struct{ key, want string }{
		{"mch_task/fld_attachment/a1b2c3d4__contract.pdf", "contract.pdf"},
		{"mch_task/fld_attachment/a1b2c3d4__report_final.pdf", "report_final.pdf"},
		{"no-double-underscore.pdf", "no-double-underscore.pdf"},
	}
	for _, c := range cases {
		if got := DisplayName(c.key); got != c.want {
			t.Errorf("DisplayName(%q) = %q, want %q", c.key, got, c.want)
		}
	}
}
