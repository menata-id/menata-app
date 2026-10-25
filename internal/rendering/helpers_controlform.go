package rendering

import (
	"context"

	"github.com/a-h/templ"
)

type controlFormKey struct{}

// withControlForm names the <form> a row's controls belong to. A <form> cannot be a child of a <tr> -- the parser closes it at once, so
// the cells land outside it and its Save sends nothing -- so a table row keeps its form in one cell and its controls say `form=<id>`.
func withControlForm(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, controlFormKey{}, id)
}

// controlForm is the `form=` attribute for a control drawn inside a table row, and nothing anywhere else.
func controlForm(ctx context.Context) templ.Attributes {
	id, _ := ctx.Value(controlFormKey{}).(string)
	if id == "" {
		return nil
	}
	return templ.Attributes{"form": id}
}
