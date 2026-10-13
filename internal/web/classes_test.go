package web

import (
	"html"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"menata.app/internal/authorization"
)

// renderedClassAttr is a class attribute in served HTML.
var renderedClassAttr = regexp.MustCompile(`\bclass="([^"]*)"`)

// compiledClassNames is every class the committed app.css has a selector for, unescaped.
func compiledClassNames(t *testing.T) map[string]bool {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "static", "css", "app.css"))
	if err != nil {
		t.Fatal(err)
	}
	unescape := regexp.MustCompile(`\\(.)`)
	out := map[string]bool{}
	for _, m := range regexp.MustCompile(`\.((?:\\.|[A-Za-z0-9_-])+)`).FindAllStringSubmatch(string(b), -1) {
		out[unescape.ReplaceAllString(m[1], "$1")] = true
	}
	return out
}

// classHooks are classes a page writes for something other than CSS: a selector a script looks up. Closed both ways,
// each with what reads it. `reached` says whether this fixture serves a page that draws it: a reached hook no page
// draws fails (stale), and an unreached one a page now draws fails too (set it, so the next loss of that page shows).
// The two signature-marker hooks are unreached because no seeded Document has a placed marker -- that state is outside
// this sweep, which the 3.5 review found by reading the template.
var classHooks = map[string]struct {
	reached bool
	why     string
}{
	"approver-row":   {true, "documentsubmit's Hyperscript moves and removes a step by `closest <.approver-row/>`"},
	"approver-user":  {true, "documentsubmit's Hyperscript shows the person picker by `next <.approver-user/>`"},
	"approver-group": {true, "documentsubmit's Hyperscript shows the Group picker by `next <.approver-group/>`"},
	"sig-canvas":     {true, "signatureplacement's drag script finds the page by `closest(\".sig-canvas\")`"},
	"sig-marker":     {false, "signatureplacement's drag script finds a marker by `closest(\".sig-marker\")`"},
	"sig-resize":     {false, "signatureplacement's drag script tells a resize from a move by `closest(\".sig-resize\")`"},
}

// TestEveryRenderedClassIsCompiled is G2's rendered half (rencana 3.5): every class on every page this fixture can
// serve must have a selector in app.css. It covers what conformance.TestEveryClassTokenIsCompiled cannot: a class
// assembled in Go, and a bare word (`flx`) the static scan cannot tell from prose. The pages are every authenticated
// GET route without a path parameter, every per-record case TestPerRecordGetRoutes builds, and the pre-auth screens.
//
// 3.2d is why it exists: two readers joined without a space drew a filter chip and two empty states without a fill,
// every test stayed green, and a Chromium diff found it. What it cannot see is a page or a state this fixture does not
// reach (an open dialog's server-rendered variant, a parameter the sweep does not pass).
func TestEveryRenderedClassIsCompiled(t *testing.T) {
	h, cookie, fx := newPerRecordSweepSetup(t, "classsweep")
	compiled := compiledClassNames(t)

	type page struct {
		url    string
		authed bool
	}
	var pages []page
	for _, r := range authenticatedGetRoutes(t) {
		pages = append(pages, page{r, true})
	}
	for _, c := range fx.cases(t) {
		if c.wantStatus == http.StatusOK {
			pages = append(pages, page{c.url, true})
		}
	}
	for _, r := range []string{"/login", "/register", "/forgot-password", "/resend-verification", "/delete-account"} {
		pages = append(pages, page{r, false})
	}

	missing, seen := map[string][]string{}, map[string]bool{}
	served, checked := 0, 0
	for _, p := range pages {
		req := httptest.NewRequest(http.MethodGet, p.url, nil)
		if p.authed {
			req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: cookie})
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		// A handler rendering templ does not always set Content-Type, and the recorder does not sniff, so HTML is
		// recognised by having a class attribute at all. JSON escapes its quotes and never matches.
		ct := rec.Header().Get("Content-Type")
		if rec.Code != http.StatusOK || (ct != "" && !strings.HasPrefix(ct, "text/html")) || !strings.Contains(rec.Body.String(), `class="`) {
			continue
		}
		served++
		for _, m := range renderedClassAttr.FindAllStringSubmatch(rec.Body.String(), -1) {
			for _, tok := range strings.Fields(html.UnescapeString(m[1])) {
				checked++
				if compiled[tok] {
					continue
				}
				if _, hook := classHooks[tok]; hook {
					seen[tok] = true
					continue
				}
				missing[tok] = append(missing[tok], p.url)
			}
		}
	}

	toks := make([]string, 0, len(missing))
	for tok := range missing {
		toks = append(toks, tok)
	}
	sort.Strings(toks)
	for _, tok := range toks {
		urls := missing[tok]
		if len(urls) > 3 {
			urls = append(urls[:3:3], "...")
		}
		t.Errorf("%q is drawn on %s but app.css has no such class -- the element draws without it. A colour outside the closed palette, a typo, two classes run together, or a class assembled in Go that no .templ names literally (Tailwind compiles only what it reads); a class kept for a script goes in classHooks with what reads it",
			tok, strings.Join(urls, ", "))
	}
	for hook, h := range classHooks {
		switch {
		case h.reached && !seen[hook]:
			t.Errorf("classHooks names %q, which no served page draws -- remove the entry, or the sweep stopped reaching the page that uses it", hook)
		case !h.reached && seen[hook]:
			t.Errorf("classHooks marks %q unreached, but a served page draws it now -- set reached", hook)
		}
		if compiled[hook] {
			t.Errorf("classHooks names %q, which app.css does compile -- it is a class, not only a hook; remove the entry", hook)
		}
	}
	t.Logf("served %d HTML pages, %d class tokens checked", served, checked)
	if served < 60 {
		t.Fatalf("only %d HTML pages were served (want >= 60) -- this sweep is measuring far less than it appears to", served)
	}
}
