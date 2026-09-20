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
//
// This header applies to router.go's plain file server for ui-sample/ too, which is easy to miss:
// those mockups loaded Tailwind from cdn.tailwindcss.com, so 'self' silently rendered every one of
// them unstyled at /ui-sample/* between that commit and 2026-09-20. Fixed the same way rather than
// by widening script-src -- Tailwind is now vendored at static/vendor/tailwind.play.js
// (ui-sample/README.md, "Assets are vendored, not loaded from a CDN"). Anything else added under a
// file server here has to be same-origin for the same reason.
func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Content-Security-Policy",
			"default-src 'self'; "+
				"script-src 'self' 'unsafe-inline'; "+
				"style-src 'self' 'unsafe-inline'; "+
				"img-src 'self'; "+
				"object-src 'none'; "+
				"frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}
