// Package installer installs an Application into a Workspace: it copies a template out of the
// template library (metadata/*.yaml plus metadata/applications/*.yaml) into that Workspace's own
// directory, renaming any id the Workspace already uses, and it owns the atomic,
// rolled-back-on-failure write discipline every path that edits Runtime Metadata on disk needs.
//
// **Installing is copying** (Workspace isolation, 2026-09-27). A Workspace's Applications live under
// metadata/workspaces/<slug>/ and diverge on their own; the library holds what they were copied
// *from*. Pointing two Workspaces at one file is the model this replaced, and it ended with a
// generated Application overwriting the real Document Approval Machine another Workspace had
// installed.
//
// **Renaming on install is the resolution to a collision, never the default.** Two Machines cannot
// share an id inside one Workspace (metadata.validateMachineIDsAreUnique), so a Workspace already
// holding mch_document could not take Document Approval at all until this package existed. A
// Workspace with no collision still gets a byte-identical copy, which is what keeps "install, then
// diverge" legible as a diff -- rewriting every id on the way in would have destroyed that property
// for everyone to serve the minority case.
//
// What may be renamed is exactly what no longer carries meaning in Go: Machine ids and an
// Application's own id, both freed by ROADMAP.md's Stage A (the engine reads a declared binding
// rather than matching a name) and the slice after it (a Machine is resolved by the role its
// Application casts it in). Nav ids, nav routes and Dataset ids are still named from Go --
// routeByID("nav_approval_inbox") in a .templ, composition's own ds_* constants,
// Workspace.ApplicationForRoute -- so a collision there is refused with a clear message instead of
// renamed, because renaming one would recreate exactly the coupling those two slices removed.
//
// **Nothing survives a failure.** Every file goes through WriteFileStrict (temp file, strict
// unknown-key re-parse, rename into place), every path touched is recorded in a WriteSet, and the
// whole manifest is loaded back through metadata.LoadApplication before the install is called done. A
// rename that missed a cross-reference cannot load, and what cannot load is rolled back rather than
// left on disk -- which is the entire reason attempting a rewrite is reasonable at all.
//
// This package holds no pool, renders nothing, and speaks no HTTP: internal/web hands it a Workspace
// and two paths, and reloads the route table itself once it returns.
package installer
