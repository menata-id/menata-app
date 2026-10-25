package aiassist

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"menata.app/internal/installer"
)

// Replays (plan 2026-10-25, C2) are conversations the real model actually had, recorded by `make aieval` with
// AIEVAL_RECORD and kept in testdata/replays, one file per run. They let everything downstream of the model --
// the reply's decoding, Validate, the writer and the loader behind it, the repair of a gap -- be tested against
// what the model really produces, without calling it. The fixtures are the model's own words, not hand-written
// examples shaped to pass: that is the point of keeping them.
//
// What they pin is the state of this package, not the model's quality (that is `make aieval`). A replay whose
// outcome changes is not wrong; it says a rule moved, and the fixture or the ratchet below moves with it,
// knowingly.

type replayFile struct {
	Case    string  `json:"case"`
	Run     int     `json:"run"`
	Group   string  `json:"group"`
	Prompt  string  `json:"prompt"`
	Replies []Reply `json:"replies"`
}

func loadReplays(t *testing.T) map[string]replayFile {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("testdata", "replays", "*.json"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no replay fixtures (err %v): run `AIEVAL_RECORD=internal/aiassist/testdata/replays make aieval`", err)
	}
	out := map[string]replayFile{}
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		// Unknown fields fail: a fixture the current Reply cannot represent is the schema having drifted.
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		var f replayFile
		if err := dec.Decode(&f); err != nil {
			t.Fatalf("%s no longer decodes into Reply: %v", p, err)
		}
		out[strings.TrimSuffix(filepath.Base(p), ".json")] = f
	}
	return out
}

func (f replayFile) change() *GeneratedChange {
	for _, r := range f.Replies {
		if r.Change != nil {
			return r.Change
		}
	}
	return nil
}

// replayKnownInvalid holds the replays whose change Validate refuses today, each with the start of the refusal.
// It only shrinks, both ways, like every ratchet here: a replay that starts to pass must be removed (the defect
// it recorded was fixed), and one that starts to fail must not be added without saying why. These are the
// writer-quality defects `make aieval` counts as failed validations; fixing one is what empties this map.
var replayKnownInvalid = map[string]string{
	"csv-import-1":    `card_fields: role "money" needs a field of type`,
	"leave-request-2": `card_fields: role "title" needs a field of type`,
	"lending-1":       `card_fields: role "title" needs a field of type`,
	"petty-cash-2":    `type status requires at least one option`,
}

func TestReplaysValidate(t *testing.T) {
	library, err := filepath.Abs(filepath.Join("..", "..", "metadata"))
	if err != nil {
		t.Fatal(err)
	}
	replays := loadReplays(t)
	var names []string
	for n := range replays {
		names = append(names, n)
	}
	sort.Strings(names)

	for _, name := range names {
		f := replays[name]
		change := f.change()
		if change == nil {
			t.Errorf("%s: the conversation never reached a change", name)
			continue
		}
		dir := t.TempDir()
		ws, _, err := installer.EnsureWorkspaceManifest(dir, library, "replay")
		if err != nil {
			t.Fatalf("%s: a fresh workspace: %v", name, err)
		}
		err = Validate(*change, ExistingStateFrom(ws))
		want, known := replayKnownInvalid[name]
		switch {
		case err != nil && !known:
			t.Errorf("%s: Validate refuses a change it accepted when recorded, or this replay is a new defect: %v", name, err)
		case err == nil && known:
			t.Errorf("%s: Validate now accepts it; remove it from replayKnownInvalid (it recorded: %s)", name, want)
		case err != nil && !strings.Contains(err.Error(), want):
			t.Errorf("%s: refused for a different reason than recorded (%q): %v", name, want, err)
		}
		if err != nil {
			continue
		}
		// What Validate accepts must be writable and reloadable: Write loads the files it wrote and rolls
		// back on failure, so an error here is a hole between the two.
		manifest := filepath.Join(dir, "replay.yaml")
		if _, err := Write(manifest, *change, FileMachineResolver{WorkspaceManifestPath: manifest}); err != nil {
			t.Errorf("%s: Validate accepted it but Write refuses: %v", name, err)
		}
	}
}

// Every gap a replay recorded survives the repair Gaps() does, in a form a store accepts: a kind and a class from
// the closed sets, a slug key, an ideal capability. A gap the model named is never lost, and the record is a
// specification (the schema asks for the ideal capability; here is what arrived).
func TestReplayGapsAreStorable(t *testing.T) {
	var seen int
	for name, f := range loadReplays(t) {
		for _, r := range f.Replies {
			for _, g := range r.Gaps() {
				seen++
				if g.Kind != GapRuntime && g.Kind != GapAssistant && g.Kind != GapUnclassified {
					t.Errorf("%s: gap %q has kind %q", name, g.Requested, g.Kind)
				}
				if !validGapClass(g.Class) && g.Class != ClassUnclassified {
					t.Errorf("%s: gap %q has class %q", name, g.Requested, g.Class)
				}
				if g.CapabilityKey == "" || GapSlug(g.CapabilityKey) != g.CapabilityKey {
					t.Errorf("%s: gap %q has key %q, not a slug", name, g.Requested, g.CapabilityKey)
				}
			}
		}
	}
	if seen == 0 {
		t.Error("no replay records a gap, so this test checks nothing")
	}
}

// The replays are only worth keeping while they cover what they were kept for: both kinds of conversation, and
// a spread of cases rather than one.
func TestReplaysCoverTheEvaluationCases(t *testing.T) {
	replays := loadReplays(t)
	have := map[string]bool{}
	for _, f := range replays {
		have[f.Case] = true
	}
	for _, c := range replayCaseIDs {
		if !have[c] {
			t.Errorf("no replay of case %q", c)
		}
	}
}

// replayCaseIDs mirrors the evaluation cases (eval_cases_test.go is behind a build tag and cannot be read here).
var replayCaseIDs = []string{
	"helpdesk", "leave-request", "petty-cash", "maintenance", "inventory", "lending", "visitor-log", "asset-register",
	"feedback", "training", "pm-full", "settings-statuses", "card-in-column", "explain-blocked", "calendar",
	"dashboard-chart", "voting-approval", "multilevel-approval", "csv-import", "pdf-invoice", "auto-number",
	"whatsapp", "progress-rollup", "own-records-only",
}
