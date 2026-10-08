package ir

import (
	"strings"
	"testing"

	"menata.app/internal/domain"
)

func init() {
	RegisterComponentTypes([]string{string(domain.ComponentStatusBadge), string(domain.ComponentAvatar)})
}

func stack(children ...UINode) UINode {
	return UINode{Kind: NodeLayout, Type: string(domain.LayoutStack), Props: map[string]string{"gap": "tight"}, Children: children}
}
func heading(text string) UINode {
	return UINode{Kind: NodeStatic, Type: string(domain.StaticHeading), Props: map[string]string{"text": text}}
}

// TestValidateRejectsAllFiveMandatoryFaults covers 007 §15.3's list exactly, one case each. The section says
// the compiler **MUST** reject these, so each is a separate named case rather than a table row, and the
// message each produces is asserted -- a rejection that does not say which fault it found is a compiler that
// makes a metadata author guess.
func TestValidateRejectsAllFiveMandatoryFaults(t *testing.T) {
	for _, tc := range []struct {
		fault string
		tree  UINode
		want  string
	}{
		{
			fault: "cyclic experience tree",
			// A Go value tree cannot contain itself, so the reachable form is an identity repeating on its own
			// ancestor path -- which is what a tree assembled from references produces.
			tree: UINode{Kind: NodeLayout, Type: "stack", Identity: "a", Props: map[string]string{"gap": "tight"},
				Children: []UINode{{Kind: NodeLayout, Type: "stack", Identity: "a", Props: map[string]string{"gap": "tight"}, Children: []UINode{heading("x")}}}},
			want: `identity "a" appears inside itself`,
		},
		{
			fault: "unresolved required child reference",
			tree:  stack(),
			want:  `has no children`,
		},
		{
			fault: "slot/type mismatch",
			tree:  stack(UINode{Kind: NodeStatic, Type: "heading", Props: map[string]string{"text": "x"}, Children: []UINode{heading("y")}}),
			want:  `static content is a leaf and exposes no slot`,
		},
		{
			fault: "binding outside the permitted scope",
			tree:  stack(UINode{Kind: NodeStatic, Type: "heading", Props: map[string]string{"text": "x", "onclick": "alert(1)"}}),
			want:  `carries property "onclick"`,
		},
		{
			fault: "recursion beyond the depth limit",
			tree: func() UINode {
				n := heading("deep")
				for i := 0; i < MaxDepth+2; i++ {
					n = stack(n)
				}
				return n
			}(),
			want: `composition depth exceeds the limit of 8`,
		},
	} {
		t.Run(tc.fault, func(t *testing.T) {
			issues := Validate(tc.tree)
			if !strings.Contains(strings.Join(issues, " | "), tc.want) {
				t.Fatalf("Validate() = %v, want an issue containing %q", issues, tc.want)
			}
		})
	}
}

// TestValidateAcceptsTheHeaderTree is the positive half: the one tree this runtime actually builds must pass.
// A validator that rejects everything is not a validator, and the five cases above would all still pass if it
// did.
func TestValidateAcceptsTheHeaderTree(t *testing.T) {
	ok := UINode{Kind: NodeLayout, Type: "stack", Identity: "page_header", Props: map[string]string{"gap": "tight"},
		Children: []UINode{
			{Kind: NodeStatic, Type: "eyebrow", Props: map[string]string{"text": "Document Approval"}},
			{Kind: NodeStatic, Type: "heading", Props: map[string]string{"text": "Approval Inbox"}},
			{Kind: NodeStatic, Type: "paragraph", Props: map[string]string{"text": "Documents waiting on you."}},
		}}
	if issues := Validate(ok); len(issues) != 0 {
		t.Fatalf("the header tree rendered on eight screens failed validation: %v", issues)
	}
}

// heldOutStaticKinds are the Static kinds a node may not yet carry text for, each with the reason. It is empty
// since `link` gained `to:` (Tahap 2a, 2026-10-08) and stays declared: the next kind that cannot be declared
// yet is named here instead of being absent from allowedProps unnoticed. An entry whose kind has since gained
// a property entry fails, so the list cannot outlive its reason.
var heldOutStaticKinds = map[domain.StaticKind]string{}

// TestValidateAcceptsEveryStaticKindWithItsText closes the gap that made a drawn Static kind unreachable from
// YAML: `ir.Validate` accepts a kind as a type via domain.KnownStaticKinds, but a node's properties are checked
// against allowedProps, a second list. Seven kinds were in the first and not the second, so a page could not
// declare a subheading although the renderer drew one -- no gate saw it, because each list was individually valid.
func TestValidateAcceptsEveryStaticKindWithItsText(t *testing.T) {
	for kind := range domain.KnownStaticKinds {
		node := UINode{Kind: NodeLayout, Type: "stack", Props: map[string]string{"gap": "tight"},
			Children: []UINode{{Kind: NodeStatic, Type: string(kind), Props: map[string]string{"text": "x"}}}}
		issues := Validate(node)
		if reason, held := heldOutStaticKinds[kind]; held {
			if len(issues) == 0 {
				t.Errorf("static kind %q is listed as held out (%s) but now validates with text -- remove it from heldOutStaticKinds", kind, reason)
			}
			continue
		}
		if len(issues) != 0 {
			t.Errorf("static kind %q is drawn by the renderer but a node of it with text fails validation: %v -- add static/%s to allowedProps", kind, issues, kind)
		}
	}
	for kind := range heldOutStaticKinds {
		if !domain.KnownStaticKinds[kind] {
			t.Errorf("heldOutStaticKinds names %q, which is no longer a Static kind", kind)
		}
	}
}

// TestValidateRefusesUnknownTypes is the fail-closed half, which 007 §9.2 states for expression context and
// §14 states for the registry: an unknown type string is refused, never resolved. Without this a typo renders
// a blank node and nothing reports it.
func TestValidateRefusesUnknownTypes(t *testing.T) {
	for _, n := range []UINode{
		{Kind: NodeKind("widget"), Type: "stack"},
		stack(UINode{Kind: NodeStatic, Type: "marquee"}),
		stack(UINode{Kind: NodeComponent, Type: "SparklineCard"}),
		{Kind: NodeLayout, Type: "carousel", Children: []UINode{heading("x")}},
	} {
		if issues := Validate(n); len(issues) == 0 {
			t.Errorf("Validate(%s/%s) accepted a type this runtime does not realize", n.Kind, n.Type)
		}
	}
}

func TestValidateRefusesAnUnknownLayoutPropertyValue(t *testing.T) {
	grid := func(props map[string]string) UINode {
		return UINode{Kind: NodeLayout, Type: "grid", Props: props, Children: []UINode{heading("x")}}
	}
	for _, props := range []map[string]string{
		{"columns": "9"}, {"mobile": "3"}, {"gap": "huge"}, {"columns": "four"},
	} {
		if issues := Validate(grid(props)); len(issues) == 0 {
			t.Errorf("Validate accepted grid %v -- the renderer would fall back to its default and the declaration would silently mean something else", props)
		}
	}
	for _, props := range []map[string]string{
		nil, {"gap": "default"}, {"columns": "4", "mobile": "2"}, {"gap": ""},
	} {
		if issues := Validate(grid(props)); len(issues) != 0 {
			t.Errorf("Validate refused grid %v: %v", props, issues)
		}
	}
}
