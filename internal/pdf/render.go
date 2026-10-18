package pdf

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

// ErrNoBrowser marks a host with no headless browser to print with: PDF output is then not offered at all (a page hides
// its PDF button), rather than offered and failing.
var ErrNoBrowser = errors.New("no headless browser is configured to print PDF")

// renderTimeout bounds one print. A page that has not settled by then is a page the browser is stuck on.
const renderTimeout = 30 * time.Second

// queueTimeout bounds the wait for a free slot: a host busy printing answers "busy" rather than holding requests (and
// the pages they already drew) open.
const queueTimeout = 10 * time.Second

// ErrBusy marks a print that found every slot taken for queueTimeout.
var ErrBusy = errors.New("PDF output is busy; try again shortly")

// maxStderr caps what is kept of the browser's stderr for an error message.
const maxStderr = 4 << 10

// capped keeps the first maxStderr bytes written and discards the rest, so a chatty browser cannot grow a buffer.
type capped struct{ buf bytes.Buffer }

func (c *capped) Write(p []byte) (int, error) {
	if room := maxStderr - c.buf.Len(); room > 0 {
		c.buf.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}

// browserNames are the executables looked for on PATH when none is configured, most specific first.
var browserNames = []string{"chrome-headless-shell", "chromium", "chromium-browser", "google-chrome", "google-chrome-stable"}

// FindBrowser returns the headless browser to print with: the configured path when it is set (and it must exist -- a
// configured path that does not is an error worth seeing, not a silent fallback), else the first known name on PATH.
// "" with a nil error is a host without one.
func FindBrowser(configured string) (string, error) {
	if configured != "" {
		if _, err := os.Stat(configured); err != nil {
			return "", fmt.Errorf("PDF_BROWSER %q: %w", configured, err)
		}
		return configured, nil
	}
	for _, name := range browserNames {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	return "", nil
}

// Printer prints an HTML document to PDF through a headless browser. At most Limit prints run at once (a browser is
// hundreds of MB), the rest wait their turn or give up with the request's own context.
type Printer struct {
	Browser string
	slots   chan struct{}
}

// NewPrinter returns a Printer for the browser at path; limit < 1 means one at a time.
func NewPrinter(browser string, limit int) *Printer {
	if limit < 1 {
		limit = 1
	}
	return &Printer{Browser: browser, slots: make(chan struct{}, limit)}
}

// Available says whether this host can print.
func (p *Printer) Available() bool { return p != nil && p.Browser != "" }

// Print renders html (a complete document, its subresources named by absolute file:// addresses) as the PDF the browser
// makes of its print layout -- the page's `@media print` rules, its `@page` size -- and returns the bytes.
func (p *Printer) Print(ctx context.Context, html []byte) ([]byte, error) {
	if !p.Available() {
		return nil, ErrNoBrowser
	}
	select {
	case p.slots <- struct{}{}:
		defer func() { <-p.slots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(queueTimeout):
		return nil, ErrBusy
	}
	dir, err := os.MkdirTemp("", "menata-pdf-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	src, out := filepath.Join(dir, "page.html"), filepath.Join(dir, "page.pdf")
	if err := os.WriteFile(src, html, 0o600); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, renderTimeout)
	defer cancel()
	// --no-sandbox: the service runs as root, where the sandbox refuses to start; the document is the application's own
	// rendering of its own page, not a stranger's.
	cmd := exec.CommandContext(ctx, p.Browser, "--headless", "--no-sandbox", "--disable-gpu", "--no-pdf-header-footer",
		// The page is printed, not run: no script (the document also carries a CSP that allows none), and file access only
		// so the inlined stylesheet's fonts load.
		"--disable-javascript", "--allow-file-access-from-files", "--user-data-dir="+filepath.Join(dir, "profile"), "--print-to-pdf="+out, "file://"+src)
	// The browser forks renderers; killing only the main process leaves them holding the stderr pipe, so Run would wait
	// for them past the timeout. Its own process group is killed whole, and Run stops waiting a moment after the kill.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 2 * time.Second
	var stderr capped
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("print pdf: %w: %s", err, bytes.TrimSpace(stderr.buf.Bytes()))
	}
	b, err := os.ReadFile(out)
	if err != nil {
		return nil, fmt.Errorf("print pdf: the browser wrote no file: %w", err)
	}
	return b, nil
}
