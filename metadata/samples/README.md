# Sample Workspaces

Frozen copies of installed Workspaces that the tests need (the real ones in `metadata/workspaces/` are data and are
git-ignored, except `default`). `conformance` and `web` read them next to `metadata/workspaces/`
(`loadInstalledWorkspaces`, `installedManifest`).

A sample is **not** regenerated when the template library changes -- that is the point: an installed copy is
independent of the library. What keeps one honest is that every gate loads it with the current loader, so a metadata
key the loader starts to demand turns the sample red. Fix it the way a Workspace would be fixed: update the sample's
own files. Never point a sample back at `metadata/*.yaml`.
