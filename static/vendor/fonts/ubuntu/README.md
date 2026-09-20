# Ubuntu (vendored)

The typeface `ui-sample/case-03-flow1`'s boards are designed in, self-hosted rather than loaded
from Google Fonts — the same reason htmx, hyperscript and Tailwind are vendored here:
`secureheaders.go`'s CSP is `default-src 'self'`, and `font-src` falls back to it, so a
cross-origin font would simply be refused.

Six files, not the design's own eighteen: weights 400/500/700 × the `latin` and `latin-ext`
unicode ranges. The remaining twelve cover cyrillic, cyrillic-ext, greek and greek-ext, which
nothing in this app renders. The `@font-face` rules that point at these files, including the
`unicode-range` each one is scoped to, live in `static/css/input.css`.

Licensed under the Ubuntu Font Licence 1.0 (`LICENCE.txt`, verbatim from the upstream
`google/fonts` repository). Copyright 2010–2011 Canonical Ltd.
