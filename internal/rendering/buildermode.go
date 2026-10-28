package rendering

import (
	"context"

	"github.com/a-h/templ"
)

type builderModeKey struct{}

// WithBuilderMode marks a request as the Application Builder's preview: the page is the runtime's own page, drawn for an admin inside the
// builder's frame, and every part of it that is one piece of the Application's metadata says which (`data-node-path`). internal/web's builderMode
// middleware is the only caller, and only for a Workspace admin who asked with `?builder=1`.
func WithBuilderMode(ctx context.Context) context.Context {
	return context.WithValue(ctx, builderModeKey{}, true)
}

// BuilderMode reports whether this request is a builder preview.
func BuilderMode(ctx context.Context) bool {
	on, _ := ctx.Value(builderModeKey{}).(bool)
	return on
}

// NodePathAttr is the one attribute that names a part of the page: "<Application id>/<metadata path>", the same id the builder's tree gives that
// part (`web.builderNodesProvider`). It is emitted in builder mode and never otherwise, so a page a person uses is byte-identical to what it was; it
// carries no class and no colour. Outside an Application, or for an empty path, there is nothing to name.
const NodePathAttr = "data-node-path"

// nodeAttrs is NodePathAttr for path, or nil when the request is not a builder preview.
func nodeAttrs(ctx context.Context, path string) templ.Attributes {
	if path == "" || !BuilderMode(ctx) {
		return nil
	}
	app, ok := CurrentApplication(ctx)
	if !ok {
		return nil
	}
	return templ.Attributes{NodePathAttr: app.ID + "/" + path}
}

// PreviewNoRole is the previewed role of someone who holds no role in the Application: what a request names that the Application does not declare.
const PreviewNoRole = "(none)"

type previewRoleKey struct{}

// WithPreviewRole puts the Application role a builder preview is drawn as onto ctx: the admin looking at the screen as someone who holds only that
// role there. internal/web's builderMode middleware is the only caller, and only on a builder preview (GET, an admin, `?builder=1&as=<role>`).
func WithPreviewRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, previewRoleKey{}, role)
}

// PreviewRole is the role this request is previewed as, and whether it is previewed as one.
func PreviewRole(ctx context.Context) (string, bool) {
	role, ok := ctx.Value(previewRoleKey{}).(string)
	return role, ok && role != ""
}
