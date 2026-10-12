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
