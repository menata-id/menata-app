# DTCG 2025.10 JSON Schemas

`format.json` and `resolver.json` are the W3C Design Tokens Community Group's own schemas for the 2025.10 release,
downloaded unchanged on 2026-10-13 from:

- https://www.designtokens.org/schemas/2025.10/format.json (sha256 `32e93b78…42915f`)
- https://www.designtokens.org/schemas/2025.10/resolver.json (sha256 `a5acd143…1da977`)

`TestDesignTokensValidateAgainstTheDTCGSchemas` (dtcg_test.go) compiles them offline, under their own URLs, and
validates every file `rendering.ExportDesignTokens` writes (rencana 3.4). Never edit them; replace both when the
Community Group publishes a new version, and change `dtcgFormatSchema`/`dtcgResolverSchema` in dtcg.go with them.
