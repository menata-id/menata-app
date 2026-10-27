package rendering

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"menata.app/internal/domain"
	"menata.app/internal/ir"
)

// The Frame draws a path on this host and nothing else, whichever role fed it: the renderer is the last place an address can be refused.
func TestFrame_drawsOnlyAPathOnThisHost(t *testing.T) {
	draw := func(src string) string {
		var buf bytes.Buffer
		n := ir.UINode{Kind: ir.NodeComponent, Type: string(domain.ComponentFrame), Props: map[string]string{"label": "P", "src": src, "query": "node"}}
		if err := uiNode(n).Render(context.Background(), &buf); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}
	if got := draw("/machines/x?builder=1"); !strings.Contains(got, `<iframe title="P" src="/machines/x?builder=1"`) {
		t.Errorf("a path on this host is not framed: %s", got)
	}
	for _, src := range []string{"javascript:alert(1)", "https://example.com/", "//example.com/", "/\\example.com", "", "x"} {
		if got := draw(src); strings.Contains(got, "<iframe") || strings.Contains(got, "menataFrameListener") {
			t.Errorf("src %q was framed: %s", src, got)
		}
	}
}
