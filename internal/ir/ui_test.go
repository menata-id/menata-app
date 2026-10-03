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
