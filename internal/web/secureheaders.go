package web

import "net/http"

// secureHeaders sets baseline security response headers on every response (security audit
// 2026-09-19, M3 -- and the missing X-Content-Type-Options is specifically what turned H2's
// upload gap into a High rather than a contained issue, since the browser was left free to sniff
// an uploaded file's declared Content-Type). Registered first in Routes() so it applies uniformly,
// including to /uploads/*.
//
// script-src/style-src need 'unsafe-inline' (machine.templ's own inline <script>/<style> blocks,
// used across most pages) -- this means CSP's script-src is not the thing that stops an uploaded
// HTML/SVG payload's own inline script from running if it were ever served as text/html; that
// protection comes from record.go's upload-time content validation and serveUpload's forced
// attachment disposition (H2's other two layers), not from this header. CSP still earns its place
// here for frame-ancestors (clickjacking, alongside X-Frame-Options for older browsers) and
// object-src (blocks plugin-based content embedding) regardless.
//
// script-src no longer needs https://unpkg.com (2026-09-19): htmx/hyperscript are vendored under
// static/vendor and served same-origin (pageHead's own doc comment) rather than loaded from that
// CDN, closing the failure mode where a CDN outage or client-side block silently took down every
// hx-* interaction on every page at once.
func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		// The Application Builder frames the application's own screen, asked for with `?builder=1`: that request alone may be framed, and only by this
		// origin ('self'), so the page stays unframeable by any other site. (Whether the request gets builder markup is builderMode's decision, for an
		// admin; the framing rule does not depend on it because a frame of this origin cannot be made by anyone but this origin.)
		frameAncestors, frameOptions := "'none'", "DENY"
		if r.Method == http.MethodGet && r.URL.Query().Get("builder") == "1" {
			frameAncestors, frameOptions = "'self'", "SAMEORIGIN"
		}
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", frameOptions)
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Content-Security-Policy",
			"default-src 'self'; "+
				"script-src 'self' 'unsafe-inline'; "+
				"style-src 'self' 'unsafe-inline'; "+
				"img-src 'self'; "+
				"object-src 'none'; "+
				"frame-ancestors "+frameAncestors)
		next.ServeHTTP(w, r)
	})
}
