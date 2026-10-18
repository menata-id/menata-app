package web

import (
	"bytes"
	"context"
	"errors"
	"html"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"menata.app/internal/pdf"
)

// showDeclaredPagePDF answers `/pages/<nav>[/<id>]/pdf` with the page as a PDF file (`write: pdf`). It does not draw
// anything itself: it asks the page handler for the same page the viewer would have got at the address without `/pdf`
// -- same Workspace, same viewer, same query, so the Application's roles, the record's existence and what this viewer
// may see are decided exactly where they are for the page -- and prints that. A page that does not answer 200 is
// answered as it was (404, 403 ...). A host without a browser answers 404, the route not being there to offer.
func showDeclaredPagePDF(page http.HandlerFunc, printer *pdf.Printer, staticRoot string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if !printer.Available() {
			http.NotFound(w, req)
			return
		}
		drawn := &drawnPage{header: http.Header{}, code: http.StatusOK}
		inner := req.Clone(req.Context())
		inner.URL.Path = strings.TrimSuffix(req.URL.Path, "/pdf")
		inner.RequestURI = inner.URL.RequestURI()
		page(drawn, inner)
		if drawn.code != http.StatusOK {
			for k, v := range drawn.header {
				w.Header()[k] = v
			}
			w.WriteHeader(drawn.code)
			_, _ = w.Write(drawn.body.Bytes())
			return
		}
		doc, err := printable(drawn.body.Bytes(), staticRoot)
		if err != nil {
			serverError(w, err)
			return
		}
		out, err := printer.Print(req.Context(), doc)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			if errors.Is(err, pdf.ErrBusy) {
				w.Header().Set("Retry-After", "10")
				http.Error(w, err.Error(), http.StatusServiceUnavailable)
				return
			}
			serverError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", `attachment; filename="`+pdfFilename(drawn.body.Bytes())+`"`)
		w.Header().Set("Cache-Control", "private, no-store")
		_, _ = w.Write(out)
	}
}

var (
	stylesheetLink = regexp.MustCompile(`<link[^>]*rel="stylesheet"[^>]*href="([^"?]+)[^"]*"[^>]*>`)
	scriptTag      = regexp.MustCompile(`(?is)<script\b.*?</script\s*>`)
	headOpen       = regexp.MustCompile(`(?i)<head\b[^>]*>`)
	fontURL        = regexp.MustCompile(`url\((["']?)(/(?:css|vendor|icons)/)`)
	firstHeading   = regexp.MustCompile(`(?s)<h1\b[^>]*>(.*?)</h1>`)
	anyTag         = regexp.MustCompile(`<[^>]*>`)
	fileNameUnsafe = regexp.MustCompile(`[^A-Za-z0-9._-]+`)
)

// staticRoots maps the public prefix a page names an asset by to the directory the router serves it from
// (router.go's staticAssets), so a print can read the same files by path.
var staticRoots = map[string]string{"/css/": "css", "/vendor/": "vendor", "/icons/": "icons"}

// staticDir is where those directories are, relative to the working directory the server runs from.
const staticDir = "static"

// printPolicy is the Content-Security-Policy a printed page carries: styles inline, fonts from files, nothing else.
const printPolicy = "default-src 'none'; style-src 'unsafe-inline'; font-src file:; img-src data:"

// printable makes the drawn page a document the browser can print from a file: its stylesheet inlined (the font
// addresses in it made absolute file:// paths) and its scripts dropped, since a print runs none of them.
func printable(page []byte, root string) ([]byte, error) {
	var failed error
	out := stylesheetLink.ReplaceAllFunc(page, func(tag []byte) []byte {
		href := string(stylesheetLink.FindSubmatch(tag)[1])
		css, err := readStatic(root, href)
		if err != nil {
			failed = err
			return nil
		}
		abs, err := filepath.Abs(root)
		if err != nil {
			failed = err
			return nil
		}
		// A font or image the stylesheet names by its public address is read from the file the router serves it from.
		css = fontURL.ReplaceAll(css, []byte("url(${1}file://"+abs+"${2}"))
		return append(append([]byte("<style>"), css...), []byte("</style>")...)
	})
	if failed != nil {
		return nil, failed
	}
	out = scriptTag.ReplaceAll(out, nil)
	// Nothing in a print needs to run or to be fetched but the inlined styles and their fonts: the policy says so, whatever
	// the page holds.
	return headOpen.ReplaceAll(out, []byte(`$0<meta http-equiv="Content-Security-Policy" content="`+printPolicy+`">`)), nil
}

func readStatic(root, href string) ([]byte, error) {
	for prefix, dir := range staticRoots {
		if rest, ok := strings.CutPrefix(href, prefix); ok && !strings.Contains(rest, "..") {
			return os.ReadFile(filepath.Join(root, dir, rest))
		}
	}
	return nil, errors.New("print: the page names a stylesheet outside the static roots: " + href)
}

// pdfFilename names the file after the page's own heading (a purchase order's number, say), made safe for a header,
// and falls back to "page" when the page has none.
func pdfFilename(page []byte) string {
	if m := firstHeading.FindSubmatch(page); m != nil {
		text := strings.TrimSpace(html.UnescapeString(anyTag.ReplaceAllString(string(m[1]), "")))
		if name := strings.Trim(fileNameUnsafe.ReplaceAllString(text, "-"), "-."); name != "" {
			return name + ".pdf"
		}
	}
	return "page.pdf"
}

// drawnPage collects what a page handler writes, so the page can be printed instead of sent.
type drawnPage struct {
	header http.Header
	code   int
	body   bytes.Buffer
}

func (d *drawnPage) Header() http.Header         { return d.header }
func (d *drawnPage) WriteHeader(code int)        { d.code = code }
func (d *drawnPage) Write(b []byte) (int, error) { return d.body.Write(b) }
