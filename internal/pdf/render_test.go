package pdf

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPrintMakesAPDFOfTheHTML(t *testing.T) {
	browser, err := FindBrowser(os.Getenv("PDF_BROWSER"))
	if err != nil || browser == "" {
		t.Skip("no headless browser on this host")
	}
	p := NewPrinter(browser, 1)
	out, err := p.Print(context.Background(), []byte(`<html><body><h1>Purchase order</h1><style>@media print{.hide{display:none}}</style><p class="hide">screen only</p></body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF-")) {
		t.Fatalf("not a PDF: %q", out[:min(len(out), 12)])
	}
	if n, err := PageCount(out); err != nil || n != 1 {
		t.Errorf("pages %d (%v), want 1", n, err)
	}
}

func TestNoBrowserIsAnError(t *testing.T) {
	if _, err := NewPrinter("", 1).Print(context.Background(), []byte("<p>x")); !errors.Is(err, ErrNoBrowser) {
		t.Fatalf("got %v, want ErrNoBrowser", err)
	}
	if _, err := FindBrowser("/nonexistent/browser"); err == nil {
		t.Error("a configured path that does not exist must be refused")
	}
}

// TestAStuckBrowserIsKilledWithItsChildren: a "browser" that forks a child and then hangs is stopped at the timeout, and
// Print does not wait for the child to let go of the pipe.
func TestAStuckBrowserIsKilledWithItsChildren(t *testing.T) {
	script := filepath.Join(t.TempDir(), "browser.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 20 &\nsleep 20\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	start := time.Now()
	if _, err := NewPrinter(script, 1).Print(ctx, []byte("<p>x")); err == nil {
		t.Fatal("a browser that never wrote a file was accepted")
	}
	if took := time.Since(start); took > 5*time.Second {
		t.Errorf("Print waited %v for a stuck browser, want it stopped near the 1s timeout", took)
	}
}

// TestAFullHostAnswersBusyInsteadOfQueueing: with every slot taken, a print waits at most queueTimeout and then says so.
func TestAFullHostAnswersBusyInsteadOfQueueing(t *testing.T) {
	p := NewPrinter("/bin/true", 1)
	p.slots <- struct{}{}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	start := time.Now()
	if _, err := p.Print(ctx, []byte("<p>x")); !errors.Is(err, ErrBusy) {
		t.Fatalf("got %v, want ErrBusy", err)
	}
	if took := time.Since(start); took > queueTimeout+3*time.Second {
		t.Errorf("waited %v", took)
	}
}
