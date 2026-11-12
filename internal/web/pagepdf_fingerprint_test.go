package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPrintReadsAFingerprintedStylesheet: a page names its stylesheet by the fingerprinted address the router
// serves (app.<hash>.css), and a print reads the file by path, so it must read the plain file as the router does.
// Without it every `write: pdf` answered 500 from the first deploy that fingerprinted the sheet.
func TestPrintReadsAFingerprintedStylesheet(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "css"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "css", "app.css"), []byte("body{color:#000}"), 0o600); err != nil {
		t.Fatal(err)
	}
	page := []byte(`<html><head><link rel="stylesheet" href="/css/app.27313997.css"></head><body><h1>x</h1></body></html>`)
	out, err := printable(page, root)
	if err != nil {
		t.Fatalf("printable: %v", err)
	}
	if !strings.Contains(string(out), "body{color:#000}") {
		t.Fatalf("stylesheet not inlined: %s", out)
	}
}
