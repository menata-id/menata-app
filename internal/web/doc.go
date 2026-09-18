// Package web is the HTTP transport layer: it maps routes onto the planes and renders their
// results, and owns nothing else (ROADMAP.md Phase 19).
//
// It exists because cmd/server had become the place this work accumulated. By Phase 18's own
// audit main.go was "the de facto composition layer"; Phase 18 moved the composition functions
// into internal/composition but left the handlers behind, and the file kept growing -- 1512
// lines at Phase 18's commit, 1621 once Phase 15 Step 6 landed. The cost was not the line count
// but what the line count made impossible: `go test` reported `cmd/server [no test files]`,
// because a package whose only job is to be a binary has nowhere to put a unit test.
//
// A handler here parses the request, calls a plane, and renders the answer. Deriving a view
// model from records is internal/composition's job, evaluating a business rule is
// internal/behavior's or internal/action's, and deciding who may act is internal/authorization's.
// What legitimately belongs here is the part that is genuinely about HTTP: query parameters,
// status codes, HTMX fragment-versus-page selection, and redirects.
//
// This package holds no connection pool and loads no Runtime Metadata: both belong to the
// composition root, which builds them once at startup (005-runtime-lifecycle.md Phase 3-4) and
// passes the results in as Deps. internal/conformance enforces that as an import rule, so the
// boundary is checked rather than merely described here.
package web
