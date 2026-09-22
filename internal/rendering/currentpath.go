package rendering

import "context"

type currentPathKey struct{}

// currentPath is what CurrentPath returns: the request's own path and raw query, kept together
// because deciding which Application-menu item is "active" needs both -- Approval Inbox's own
// "My Documents" item is a query on the same path as "Approval Inbox" (nav_my_documents's declared
// route is literally "/approval-inbox?tab=mine"), so path alone cannot tell them apart.
type currentPath struct {
	Path     string
	RawQuery string
}

// WithCurrentPath puts the request's own URL (path + raw query) on ctx, the same pattern
// WithCurrentApplication already uses -- internal/web's currentApplication middleware is the only
// caller, and appShell reads it back through CurrentPath to mark the active entry in its own
// Application-menu row/bottom bar without any client-side script (CLAUDE.md's own "menu, bottom
// bar, modal: prefer Tailwind's built-ins" -- pageShell's aria-current used a small script for
// exactly this, which this replaces with a server-computed value).
func WithCurrentPath(ctx context.Context, path, rawQuery string) context.Context {
	return context.WithValue(ctx, currentPathKey{}, currentPath{Path: path, RawQuery: rawQuery})
}

// CurrentPath returns the request's own path and raw query, as WithCurrentPath set them. Both
// empty for a ctx no middleware has touched (every real request always sets this, unlike
// CurrentApplication, which legitimately has no value for a Workspace-level screen).
func CurrentPath(ctx context.Context) (path, rawQuery string) {
	cp, _ := ctx.Value(currentPathKey{}).(currentPath)
	return cp.Path, cp.RawQuery
}
