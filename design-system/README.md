# design-system/

`tokens.json` is a **copy** of the Design System artifact "Menata App"
(`https://claude.ai/artifact/Xi4TYQPxsMQUmL3chScmaS`, file `project/tokens.json`), taken at artifact version
`1791770746-0d9d` (2026-10-12; adds the `rejected` family read by GS2). It is the token source the mockup canvases are generated from.

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
