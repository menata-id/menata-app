package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"menata.app/internal/pdf"
)

const printablePage = `<html><head><link rel="stylesheet" href="/css/app.css?v=abc"/><script src="/vendor/htmx.min.js" defer></script></head>` +
	`<body><h1>PO/2026/0087 &amp; co</h1><script>alert(1)</script><SCRIPT >alert(2)</SCRIPT ><p>Rp 180,000,000</p></body></html>`

// TestPrintableInlinesTheStylesheetAndDropsTheScripts: the browser prints from a file, so the page must carry its own
// CSS (font addresses made file paths) and no script.
func TestPrintableInlinesTheStylesheetAndDropsTheScripts(t *testing.T) {
	out, err := printable([]byte(printablePage), "../../static")
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	for _, bad := range []string{"<script", "<SCRIPT", `rel="stylesheet"`, "url(/vendor/"} {
		if strings.Contains(got, bad) {
			t.Errorf("the printable page still holds %q", bad)
		}
	}
	if !strings.Contains(got, `http-equiv="Content-Security-Policy"`) || !strings.Contains(got, "default-src 'none'") {
		t.Errorf("the printable page carries no policy that forbids script and fetches")
	}
	if !strings.Contains(got, "<style>") || !strings.Contains(got, "url(file://") {
		t.Errorf("the stylesheet was not inlined with file:// font addresses")
	}
	if _, err := printable([]byte(`<link rel="stylesheet" href="/elsewhere/x.css">`), "../../static"); err == nil {
		t.Error("a stylesheet outside the static roots was read")
	}
	if _, err := printable([]byte(`<link rel="stylesheet" href="/css/../css/app.css">`), "../../static"); err == nil {
		t.Error("a stylesheet named with .. was read")
	}
}

func TestPDFFilenameComesFromTheHeading(t *testing.T) {
	if got := pdfFilename([]byte(printablePage)); got != "PO-2026-0087-co.pdf" {
		t.Errorf("filename %q", got)
	}
	if got := pdfFilename([]byte("<p>no heading</p>")); got != "page.pdf" {
		t.Errorf("filename %q", got)
	}
}

// TestThePDFRouteAnswersWhatThePageAnswersAndOtherwisePrintsIt: a refusal of the page is the refusal of its PDF; no
// browser is a 404; with one, the answer is the PDF with a name.
func TestThePDFRouteAnswersWhatThePageAnswersAndOtherwisePrintsIt(t *testing.T) {
	page := func(code int) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/pages/nav_x/abc" {
				t.Errorf("the page was asked for %q, want the address without /pdf", r.URL.Path)
			}
			w.WriteHeader(code)
			_, _ = w.Write([]byte(printablePage))
		}
	}
	get := func(h http.HandlerFunc) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest(http.MethodGet, "/pages/nav_x/abc/pdf?q=1", nil))
		return rec
	}
	if rec := get(showDeclaredPagePDF(page(200), pdf.NewPrinter("", 1), "../../static")); rec.Code != http.StatusNotFound {
		t.Errorf("a host without a browser answered %d, want 404", rec.Code)
	}
	if rec := get(showDeclaredPagePDF(page(403), pdf.NewPrinter("/bin/true", 1), "../../static")); rec.Code != http.StatusForbidden {
		t.Errorf("a page that refused was printed (%d)", rec.Code)
	}
	browser, err := pdf.FindBrowser(os.Getenv("PDF_BROWSER"))
	if err != nil || browser == "" {
		t.Skip("no headless browser on this host")
	}
	rec := get(showDeclaredPagePDF(page(200), pdf.NewPrinter(browser, 1), "../../static"))
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Body.String(), "%PDF-") {
		t.Fatalf("status %d, body %.20q", rec.Code, rec.Body.String())
	}
	if d := rec.Header().Get("Content-Disposition"); d != `attachment; filename="PO-2026-0087-co.pdf"` {
		t.Errorf("Content-Disposition %q", d)
	}
}
