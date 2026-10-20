package web

import (
	"net/http"
	"net/url"
	"strings"

	"menata.app/internal/rendering"
)

// BackParam is the query parameter a record's page carries to say which screen it was opened from: the board, table or calendar, with
// the project or week they showed. It travels in the page's own address, so a reload, an edit swapped into the page and a delete all
// read the same answer (HX-Current-URL), and nothing is stored.
const backParam = "back"

// validBack is raw as a screen of this Workspace to return to, or false. Only an address the runtime draws a list screen at counts: a
// Machine's own list (`/machines/<id>`) or a navigation item's route -- never another site, a protocol-relative address, a record's own
// page or a runtime route -- because the value comes from the address bar and is redirected to.
func validBack(req *http.Request, raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || raw == "" || u.Scheme != "" || u.Host != "" || !strings.HasPrefix(u.Path, "/") || strings.HasPrefix(u.Path, "//") {
		return "", false
	}
	ws := rendering.CurrentWorkspace(req.Context())
	if machineID, ok := machineIDFromPath(u.Path); ok && u.Path == "/machines/"+machineID {
		if _, found := ws.ApplicationForMachine(machineID); !found {
			return "", false
		}
	} else if _, found := ws.ApplicationForRoute(u.Path); !found {
		return "", false
	}
	if u.RawQuery != "" {
		return u.Path + "?" + u.RawQuery, true
	}
	return u.Path, true
}

// backOf is the screen the request's page was opened from: `back` in its own address, or in the address of the page an HTMX request
// was sent from.
func backOf(req *http.Request) (string, bool) {
	if v := req.URL.Query().Get(backParam); v != "" {
		return validBack(req, v)
	}
	if current, err := url.Parse(req.Header.Get("HX-Current-URL")); err == nil {
		if v := current.Query().Get(backParam); v != "" {
			return validBack(req, v)
		}
	}
	return "", false
}

// openedFrom is the screen a browser navigation came from, read from Referer: only this site's own, and only one validBack accepts.
func openedFrom(req *http.Request) (string, bool) {
	ref, err := url.Parse(req.Header.Get("Referer"))
	if err != nil || ref.Host != req.Host {
		return "", false
	}
	return validBack(req, ref.RequestURI())
}
