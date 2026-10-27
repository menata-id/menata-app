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
