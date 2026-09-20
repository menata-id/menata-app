package rendering

import (
	"context"

	"menata.app/internal/domain"
)

type currentAppKey struct{}

// WithCurrentApplication puts the Application a request is in onto ctx. internal/web's
// currentApplication middleware is the only caller; every shell and page reads it back through
// CurrentApplication below.
//
// This is context rather than a parameter threaded through ~20 Page functions for the reason
// pageShell's own doc comment already gives for navigation, with one difference that matters:
// navigation is boot-time-static and can be package state, while *which* Application a request
// is in genuinely varies per request. Resolving it costs no database access at all -- it is a
// lookup over the boot-time Workspace (domain.Workspace.ApplicationForMachine / ForRoute) -- so
// unlike the Workspace name and viewer initials in internal/web.resolveChrome, doing it for every
// request is free. That is the whole distinction: ctx for a per-request value that is free to
// compute and needed everywhere; explicit parameters for values that cost queries and are needed
// by three screens.
func WithCurrentApplication(ctx context.Context, app domain.Application) context.Context {
	return context.WithValue(ctx, currentAppKey{}, app)
}

// CurrentApplication returns the Application this request is in, and whether one was resolved.
//
// Not every request has one: the Workspace-level screens (Home, Members) belong to no
// Application, and neither do routes concerning a shared Machine (mch_user, mch_activity). A
// caller that needs a display name should use CurrentApplicationName, which degrades to the
// Workspace's own name rather than to empty.
func CurrentApplication(ctx context.Context) (domain.Application, bool) {
	app, ok := ctx.Value(currentAppKey{}).(domain.Application)
	return app, ok
}

// CurrentApplicationName is what the chrome renders where it used to render a single hardcoded
// AppName: the current Application's name, or the Workspace's own when the request belongs to no
// Application. It never returns empty for a configured Workspace, so no page has to handle a
// blank heading.
func CurrentApplicationName(ctx context.Context) string {
	if app, ok := CurrentApplication(ctx); ok {
		return app.Name
	}
	return workspace.Name
}
