# design-system/

`tokens.json` is a **copy** of the Design System artifact "Menata App"
(`https://claude.ai/artifact/Xi4TYQPxsMQUmL3chScmaS`, file `project/tokens.json`), taken at artifact version
`1792028479-b5e8` (2026-10-15; Popover page, `count-badge`, lane and gap sync; before it `1792024156-16be`: `accent-ring` and `primary-hover` follow the code as opacities of a role, `radius-inner` 4px added; before it `1792019598-beab`, 2026-10-14: token sync with the code: the four accepted tokens lost their status, four tone edges were added, usage strings corrected; before it `1791992280-76db` deepened the light steps of four Tag hues to reach 4.5:1, and before that `1791884364-e589` added the `dark` theme and the categorical colour tokens, owner decisions DK1–DK11). It is the token source the mockup canvases are generated from.

It is here so a test can hold the artifact and the code to the same values without reading the network:
`internal/rendering`'s `TestDesignSystemTokensMatchTheCode` (GS1 of `menata-app-document`'s
`guides/menjaga-design-system.md`). That test checks every token against the default Theme's readers and the
compiled `static/css/app.css`. A token must be classified as one of three things, or the test fails:
- checked;
- accepted but not yet in code;
- unmapped with a reason.

**Never edit this file by hand.** Change the artifact, then copy its `project/tokens.json` here unchanged and update the
version above, in the same commit as the code change that matches it (R8 of the guide).

## `canvas.py` (GS6)

Since 2026-10-13 a colour token's `value` may be a map of theme id to value (`{"light": …, "dark": …}`); a plain string is
the first theme's, and a theme missing from the map inherits it. The canvases are light, so `canvas.py` writes the first
theme's value. GS1 checks both themes: light against the default `app.css`, dark against its `prefers-color-scheme: dark`
block.

`canvas.py` generates and checks the mockup canvas's `:root` token blocks from `tokens.json`. The canvas is not in this
repo, so download its `project/*.dc.html` first (Artifact `read` with `paths`).

- `python3 -I design-system/canvas.py generate design-system/tokens.json <folder> --version <artifact-version>`:
  rewrites every artboard's block, and refuses if anything outside the block would change.
- `python3 -I design-system/canvas.py check design-system/tokens.json <folder>`: exit 1 on any stale block. It also
  reports adoption: `var(--)` references, and colour literals that do or do not equal a token value.
  - Measured 2026-10-12 on canvas version 155: 0 stale blocks; 130 references, all in one artboard (U04); 1,623
    literals equal a token value and 6,728 do not. Most of the 6,728 are the same palette written in old hex notation
    (`#e2e8f0` for slate-200); that is the canvas's remaining R3 debt.

`canvas_vars.py` (rencana 0.6) replaces oklch colour literals in the canvas artboards that equal a token with
`var(--token)`. The name comes from the CSS property's family: `color` reads ink, `background` reads fill, and
`border`/`outline` read edge. Every file is verified by resolving each `var()` back to its value. Hex is out of
scope; see the script's docstring and decision K1. The first run, on 2026-10-12, covered canvas version 157: 466
literals in the five Builder artboards (A01, U01–U04). 26 were left, because their colour belongs to another
family (a dot, a tab underline, a chevron).

## `canvas_k1.py` (owner decision K1)

`canvas_k1.py` replaced the mockup canvas's Tailwind v3 hex colours with the Tailwind v4 oklch of the same palette
step (UI canvas version 159, 2026-10-12): 6,958 values in 84 artboards. `k1-v3-to-v4.json` records each of the 53
colours with its OKLab distance (×100; about 2 is a just-noticeable difference). The mean, weighted by use, is 0.64.
The largest are the blues and violets (blue-600 2.98 over 235 uses, violet-600 3.44). Afterwards `canvas_vars.py`
turned 5,653 of the new values into `var(--token)`. Four hex outside the v3 palette (18 uses) were left alone.
White and black were never touched.

## `dtcg/` (rencana 3.4)

`dtcg/default/` is the runtime's **default Theme exported as W3C DTCG 2025.10 tokens**: `menata.resolver.json` (the
Resolver Module document), `base.tokens.json` (spacing, radius, typography), and `light.tokens.json`/`dark.tokens.json`
(colours under the same names). It is **generated, never edited**: `make tokens` writes it from the committed
`static/css/app.css`, and `internal/rendering`'s `TestCommittedDesignTokensAreFresh` fails when it is stale. One
Workspace's own `theme:` is exported with `go run ./cmd/tokens -workspace <slug> -out <dir>`.

The difference from `tokens.json`: `tokens.json` is the **Design System artifact** (what the mockups decided, with the
tokens no reader answers for, such as brand, focus-ring and the shadows). The DTCG export is **what the runtime draws**
for one Theme, so it carries only the tokens a reader answers for (GS1's mapping, `rendering.designTokenClasses`), with
each value resolved through the Workspace's override and app.css's dark block. For the default Theme the two agree;
`TestDesignTokensMatchTheDesignSystemArtifact` and `canvas.py compare` both check that.

`canvas.py` reads a DTCG resolver wherever it takes a tokens.json, so a mockup for one Workspace is generated from that
Workspace's export:

    python3 -I design-system/canvas.py generate <dir>/menata.resolver.json <canvas>/project --version <label>
