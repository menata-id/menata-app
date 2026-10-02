package conformance

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestClaimMatrixCitesRealArtifacts holds 007 §40 to citing this repository.
//
// §40 is the matrix every other claim in that document defers to for its status, and `CLAUDE.md` tells
// every session to read 001-007 before any architectural decision. **Until 2026-10-02 it described a
// different repository**: dated 2026-09-11, its evidence column cited `CAP-` rows, `CR-` gap numbers,
// `composable-runtime-blueprint.md` and `internal/metadata/compile.go` — none of which exist here — and it
// was wrong in both directions. It claimed "CEL expression evaluation — PROVEN" for a runtime with no CEL,
// and it listed Dataset, Relation and Projection as PROPOSED after all three had shipped.
//
// So this gate checks the one thing a scan can: every file path and every test name the matrix cites
// **exists**. **It cannot check that a status is true** — PROVEN versus PARTIAL is a judgement, and a green
// run means the citations resolve, never that the claims are right. That limit is stated in §40 itself too,
// so a reader meeting the gate and the table separately gets the same warning.
func TestClaimMatrixCitesRealArtifacts(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(repoRoot(), "007-composable-runtime-architecture.md"))
	if err != nil {
		t.Fatalf("read 007: %v", err)
	}
	matrix := string(body)
	if i := strings.Index(matrix, "# 40. Claim Citation Matrix"); i >= 0 {
		matrix = matrix[i:]
	} else {
		t.Fatal("007 has no §40 -- either it was renamed or this gate stopped looking")
	}

	// **Only the table rows, and that scope is precise rather than convenient.** The preamble quotes the
	// citations this rewrite *removed* ("`internal/metadata/compile.go` — none of which exist here"), and a
	// gate that read the whole section would fail on its own retraction -- the fifth time in this repo a
	// gate's text would have sat inside the data it reads. What must resolve is the evidence column.
	var rows []string
	for _, line := range strings.Split(matrix, "\n") {
		if strings.HasPrefix(line, "|") && !strings.HasPrefix(line, "|---") {
			rows = append(rows, line)
		}
	}
	if len(rows) == 0 {
		t.Fatal("§40 has no table rows -- either the matrix changed shape or this gate stopped looking")
	}

	checkedPaths, checkedTests := 0, 0
	for _, row := range rows {
		for _, m := range regexp.MustCompile("`([A-Za-z0-9_./-]+)`").FindAllStringSubmatch(row, -1) {
			tok := m[1]
			switch {
			case strings.HasPrefix(tok, "audits/"):
				// menata-app-document's, cited with that repo named in the same cell. A different
				// repository's path is not this gate's to resolve, and pretending otherwise would make it
				// fail on a correct citation.
				continue
			case isSourceFile(tok) && (strings.HasPrefix(tok, "internal/") || strings.HasPrefix(tok, "metadata/")):
			case regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z.-]*\.md$`).MatchString(tok):
			case strings.HasPrefix(tok, "internal/"):
				// A package-qualified identifier, `internal/<pkg>.<Symbol>` -- the matrix cites these far
				// more often than file paths, and checking only the directory would let a renamed symbol
				// sit here forever. So both halves are checked.
				pkg, symbol, hasSymbol := strings.Cut(tok, ".")
				checkedPaths++
				if _, err := os.Stat(filepath.Join(repoRoot(), pkg)); err != nil {
					t.Errorf("007 §40 cites package %q, which does not exist", pkg)
					continue
				}
				if hasSymbol {
					found, err := identifierExistsIn(filepath.Join(repoRoot(), pkg), symbol)
					if err != nil {
						t.Fatalf("search %s for %s: %v", pkg, symbol, err)
					}
					if !found {
						t.Errorf("007 §40 cites %q, and %s declares no %s -- a renamed or deleted symbol leaves the matrix citing something a reader cannot find", tok, pkg, symbol)
					}
				}
				continue
			default:
				// A bare filename in prose ("machine.templ does it generically") names something real but
				// gives no path to check. Deliberately out of scope rather than resolved by searching --
				// a search would make the gate pass on any file of that name anywhere, which is weaker
				// than not checking.
				continue
			}
			checkedPaths++
			if _, err := os.Stat(filepath.Join(repoRoot(), tok)); err != nil {
				t.Errorf("007 §40 cites %q in its evidence column, and it does not exist in this repository -- the matrix must cite artifacts a reader can open. If the claim is about something unbuilt, say so in the status column instead of citing a file", tok)
			}
		}
		for _, m := range regexp.MustCompile(`\b(Test[A-Z][A-Za-z0-9]+)`).FindAllStringSubmatch(row, -1) {
			name := m[1]
			checkedTests++
			found, err := testFunctionExists(name)
			if err != nil {
				t.Fatalf("search for %s: %v", name, err)
			}
			if !found {
				t.Errorf("007 §40 cites conformance test %s, which no file in internal/ declares -- a status backed by a test that does not exist is the shape this gate was written to stop", name)
			}
		}
	}

	if checkedPaths == 0 || checkedTests == 0 {
		t.Fatalf("checked %d paths and %d test names -- a matrix citing neither is one this gate cannot hold", checkedPaths, checkedTests)
	}
}

func testFunctionExists(name string) (bool, error) {
	var found bool
	err := filepath.WalkDir(filepath.Join(repoRoot(), "internal"), func(path string, d os.DirEntry, err error) error {
		if err != nil || found || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		b, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(b), "func "+name+"(") {
			found = true
		}
		return nil
	})
	return found, err
}

// isSourceFile reports whether a token names a file rather than a package-qualified identifier.
func isSourceFile(tok string) bool {
	for _, ext := range []string{".go", ".templ", ".md", ".yaml"} {
		if strings.HasSuffix(tok, ext) {
			return true
		}
	}
	return false
}

// identifierExistsIn reports whether a package declares a given exported name, by text search over its
// own files. Text rather than go/types because the matrix cites types, funcs, methods, vars and struct
// fields interchangeably, and a reader opening the package to find the name is doing exactly this search.
func identifierExistsIn(dir, symbol string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		if e.IsDir() || !(strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), ".templ")) {
			continue
		}
		b, readErr := os.ReadFile(filepath.Join(dir, e.Name()))
		if readErr != nil {
			return false, readErr
		}
		if regexp.MustCompile(`\b` + regexp.QuoteMeta(symbol) + `\b`).Match(b) {
			return true, nil
		}
	}
	return false, nil
}
