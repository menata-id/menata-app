package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"menata.app/internal/aiassist"
	"menata.app/internal/registry"
)

// TestPromptNamesEveryRegisteredCapability holds the AI assistant's own grounding prompt against the
// registries it describes.
//
// `internal/aiassist.composableSurface` is prose handed to a model as the boundary of what it may
// generate. Its own comment used to claim that keeping it in sync was "a review discipline ... not a new
// kind of drift risk". **Measured 2026-09-30: it had drifted three ways**, and the consequence was not
// untidy documentation -- the assistant declined work, and gave reasons that were false. The sharpest of
// the three was right in its conclusion and wrong in its reason, which is the kind of error no amount of
// re-reading prose catches, because the sentence reads as settled.
//
// **What this gate can check, and what it cannot.** It checks *naming*: every Service and workflow engine
// the registry declares must appear somewhere in the prompt, so a capability cannot be added to the
// runtime and left unmentioned. It cannot check whether what the prompt *says* about a capability is
// true -- "you may generate X" versus "X exists but you cannot emit it" is a claim about
// aiassist.GeneratedEvent's own shape, and only reading that shape settles it. So a green run means the
// prompt mentions everything, never that it describes it correctly.
//
// The field-type list is deliberately outside this gate's scope, and the reason is a flaw mutation found
// here: a textual check for it was **vacuous**. The condition was "the prompt mentions FieldTypeLabels OR
// contains this label", and prompt.go mentions FieldTypeLabels in a *comment*, so the arm could never fire
// -- breaking the generation entirely left it green. The assertion belongs where it can call the function
// instead of reading its source: aiassist.TestComposableSurfaceRendersEveryFieldType. Third time today a
// gate's own text sat inside the data it read.
func TestPromptNamesEveryRegisteredCapability(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(repoRoot(), "internal", "aiassist", "prompt.go"))
	if err != nil {
		t.Fatalf("read prompt.go: %v", err)
	}
	prompt := string(body)

	for _, name := range sortedKeys(registry.Services) {
		if !strings.Contains(prompt, name) {
			t.Errorf("registry.Services declares %q and the assistant's prompt never names it -- the model is grounded to a boundary that has stopped describing the runtime, which is how it came to refuse three shipped capabilities. Say what the model may do with it, or say plainly that it cannot emit one and why", name)
		}
	}
	for name := range registry.KnownWorkflowEngines {
		if !strings.Contains(prompt, name) {
			t.Errorf("registry.KnownWorkflowEngines declares %q and the prompt never names it -- same reason", name)
		}
	}

	// The claims the drift produced, each asserted gone by its own words rather than by a count: a
	// regression here would be someone restoring a sentence, not adding one.
	for _, gone := range []string{
		"no such capability exists in this runtime yet",
		"hardcoded to mch_document/mch_approval_step specifically",
		"You cannot emit any but the first",
	} {
		if strings.Contains(strings.SplitN(prompt, "func composableSurface", 2)[1], gone) {
			t.Errorf("the prompt has regained the false claim %q -- it was retracted 2026-09-30; see composableSurface's own comment for what is true instead", gone)
		}
	}
}

// TestPromptDoesNotDenyWhatTheAssistantMerelyCannotWriteYet holds W0's claims. The prompt lists what the
// runtime does and the assistant cannot yet write (aiassist.UnwritableByTheAssistant), and each entry
// that names a grammar key must still be catalogued unwritable (registry.Capabilities): the day a slot is built the grammar gate
// forces the entry's AIWritable true, and this one forces it out of the prompt, so the model is never told
// it lacks something it now has. The templates the prompt recommends must exist in the library.
func TestPromptDoesNotDenyWhatTheAssistantMerelyCannotWriteYet(t *testing.T) {
	if len(aiassist.UnwritableByTheAssistant) == 0 {
		t.Fatal("the list is empty, so this gate would measure nothing")
	}
	for _, e := range aiassist.UnwritableByTheAssistant {
		if e.Doc == "" {
			continue // a permission key or a menu item's page:, which the grammar gate does not cover; held by review
		}
		if unwritableReason(e.Doc, e.Key) == "" {
			t.Errorf("the prompt tells the model it cannot write %s.%s, but the catalogue no longer marks it unwritable -- the slot exists now, so remove it from aiassist.UnwritableByTheAssistant", e.Doc, e.Key)
		}
	}
	library, err := filepath.Glob(filepath.Join(repoRoot(), "metadata", "applications", "*.yaml"))
	if err != nil || len(library) == 0 {
		t.Fatalf("found no library templates (%v) -- the gate would pass by measuring nothing", err)
	}
	var all strings.Builder
	for _, f := range library {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		all.Write(b)
	}
	for _, name := range aiassist.ReadyMadeTemplates {
		if !strings.Contains(all.String(), "\nname: "+name+"\n") && !strings.HasPrefix(all.String(), "name: "+name+"\n") {
			t.Errorf("the prompt recommends installing %q, which is not in metadata/applications", name)
		}
	}
}
