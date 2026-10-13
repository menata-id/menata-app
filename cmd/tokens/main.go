// Command tokens exports a Workspace's Theme as W3C DTCG 2025.10 design tokens (rencana 3.4), for mockups and design
// tools: four files, a resolver document and the three token documents it names (see rendering.ExportDesignTokens).
//
//	go run ./cmd/tokens -out design-system/dtcg/default              # the default Theme (make tokens)
//	go run ./cmd/tokens -workspace <slug> -out <dir>                 # one Workspace's own theme: block
//
// Values are read from the compiled static/css/app.css, so run `make generate` first if a .templ or input.css changed.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"menata.app/internal/domain"
	"menata.app/internal/metadata"
	"menata.app/internal/rendering"
)

func main() {
	slug := flag.String("workspace", "", "a Workspace slug (metadata/workspaces/<slug>.yaml); empty exports the default Theme")
	manifests := flag.String("manifests", "metadata/workspaces", "the Workspace manifest directory")
	css := flag.String("css", "static/css/app.css", "the compiled stylesheet the values are read from")
	out := flag.String("out", "", "the directory to write the four files into (created if missing)")
	flag.Parse()
	if *out == "" {
		fail(fmt.Errorf("-out is required"))
	}

	var ws domain.Workspace
	if *slug != "" {
		loaded, err := metadata.LoadWorkspaces(*manifests)
		if err != nil {
			fail(err)
		}
		app, ok := loaded[*slug]
		if !ok {
			fail(fmt.Errorf("no Workspace %q in %s", *slug, *manifests))
		}
		ws = app.Workspace
	}
	b, err := os.ReadFile(*css)
	if err != nil {
		fail(err)
	}
	files, err := rendering.ExportDesignTokens(ws, string(b))
	if err != nil {
		fail(err)
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fail(err)
	}
	for name, body := range map[string][]byte{
		rendering.DesignTokensBaseFile: files.Base, rendering.DesignTokensLightFile: files.Light,
		rendering.DesignTokensDarkFile: files.Dark, rendering.DesignTokensResolverFile: files.Resolver,
	} {
		if err := os.WriteFile(filepath.Join(*out, name), body, 0o644); err != nil {
			fail(err)
		}
	}
	fmt.Printf("wrote %s (%s)\n", *out, map[bool]string{true: "Workspace " + *slug, false: "default Theme"}[*slug != ""])
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "tokens:", err)
	os.Exit(1)
}
