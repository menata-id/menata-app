package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"menata.app/internal/aiassist"
	"menata.app/internal/authorization"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/execution"
	"menata.app/internal/mail"
	"menata.app/internal/metadata"
	"menata.app/internal/pdf"
	"menata.app/internal/registry"
	"menata.app/internal/storage"
)

// The case corpus (roadmap KZ1; menata-app-document audits/2026-10-15-kajian-strategi §4.1). Each directory under
// testdata/cases holds one business case written the way 001-007 say it should read, plus a scenario. The harness
// loads the case through the real loader, strips every key the loader does not read yet (that list is the case's
// backlog), and plays the scenario through the real router. A scenario step that cannot pass is the measurement.
//
// It orders work; it does not admit it. A key in a case YAML is not permission to build the key (capability-lifecycle.md
// §2 still applies).

type caseScenario struct {
	Case   int         `yaml:"case"`
	Title  string      `yaml:"title"`
	Actors []caseActor `yaml:"actors"`
	Steps  []caseStep  `yaml:"steps"`
	// Reading is what the document reader answers (a stand-in for the model, which no test may call): the same answer to
	// every document, keyed by Field id, and per row kind (its index) the rows.
	Reading *caseReading `yaml:"reading"`
	// NumberFormat is how the case's Workspace writes a figure (`number_format:` in its manifest).
	NumberFormat string `yaml:"number_format"`
}

type caseReading struct {
	Values map[string]caseProposed              `yaml:"values"`
	Rows   map[string][]map[string]caseProposed `yaml:"rows"`
}

type caseProposed struct {
	Value    string `yaml:"value"`
	Source   string `yaml:"source"`
	Inferred bool   `yaml:"inferred"`
}

// caseReader is the Extractor of a case: it answers from the scenario and remembers what it was asked.
type caseReader struct {
	reading *caseReading
	asked   *[]aiassist.ExtractRequest
	// blank makes the reader find nothing: the answer for a document that is not the kind the form is for.
	blank *bool
}

func (r caseReader) Extract(_ context.Context, req aiassist.ExtractRequest) (aiassist.ExtractResult, error) {
	*r.asked = append(*r.asked, req)
	out := aiassist.ExtractResult{Values: map[string]aiassist.Proposed{}, Rows: map[string][]map[string]aiassist.Proposed{}}
	if r.reading == nil || *r.blank {
		return out, nil
	}
	for k, v := range r.reading.Values {
		out.Values[k] = aiassist.Proposed(v)
	}
	for kind, rows := range r.reading.Rows {
		for _, row := range rows {
			kept := map[string]aiassist.Proposed{}
			for k, v := range row {
				kept[k] = aiassist.Proposed(v)
			}
			out.Rows[kind] = append(out.Rows[kind], kept)
		}
	}
	return out, nil
}

// caseRead sends a document to a page's reader (POST /pages/<nav>/extract) and keeps the proposal it was answered with, which
// a later step names as `$<step id>`. Asks lists the Field ids the reader must have been asked about, exactly.
type caseRead struct {
	Nav  string   `yaml:"nav"`
	File string   `yaml:"file"`
	Asks []string `yaml:"asks"`
	// Blank is a document the reader finds nothing in.
	Blank bool `yaml:"blank"`
}

// caseSave submits the form a proposal filled (POST /pages/<nav>/extract/save) with the values a person left in it: Values
// are the form's own control names, rows.<kind>.<n>.<field> and rows.<kind>.<n>.include among them. Machine is the Machine
// the form creates, so the step's record check reads what it made.
type caseSave struct {
	Nav      string            `yaml:"nav"`
	Proposal string            `yaml:"proposal"`
	Machine  string            `yaml:"machine"`
	Values   map[string]string `yaml:"values"`
}

type caseActor struct {
	Name string `yaml:"name"`
	Role string `yaml:"role"`
}

type caseStep struct {
	ID     string      `yaml:"id"`
	As     string      `yaml:"as"`
	Create *caseCreate `yaml:"create"`
	Update *caseUpdate `yaml:"update"`
	Tick   *caseTick   `yaml:"tick"`
	Delete *caseDelete `yaml:"delete"`
	View   *caseView   `yaml:"view"`
	Read   *caseRead   `yaml:"read"`
	Save   *caseSave   `yaml:"save"`
	Expect string      `yaml:"expect"`
	// API sends the step through the JSON routes (/api/machines/...) instead of the form routes. Both end in the same
	// guards, and a case that wants a refusal proved on both says so with a second, state-neutral step.
	API      bool              `yaml:"api"`
	Record   map[string]string `yaml:"record"`
	Notified []caseNotified    `yaml:"notified"`
	// Count checks how many records a Machine holds (optionally only those whose Fields hold given values) after
	// the step, for a rule that creates records rather than changing one.
	Count *caseCount `yaml:"count"`
}

type caseCount struct {
	Machine string            `yaml:"machine"`
	Where   map[string]string `yaml:"where"`
	Equals  int               `yaml:"equals"`
}

type caseCreate struct {
	Machine string            `yaml:"machine"`
	Values  map[string]string `yaml:"values"`
	Files   map[string]string `yaml:"files"`
}

type caseUpdate struct {
	Ref    string            `yaml:"ref"`
	Values map[string]string `yaml:"values"`
	// Files sends files with a form PATCH, as a transition form that asks for a proof does.
	Files map[string]string `yaml:"files"`
	// Omit leaves Fields out of a JSON PUT, which carries the whole record otherwise: what a client that drops a Field
	// sends.
	Omit []string `yaml:"omit"`
}

// caseTick moves the clock: the runtime's scheduler runs once as if Days had passed, and the step's record check
// reads Ref's record afterwards. It is not a request, so it has no actor and no refusal.
type caseTick struct {
	Days int    `yaml:"days"`
	Ref  string `yaml:"ref"`
}

// caseView opens a screen as the step's actor -- a route, with `$<step id>` filled -- and checks what it says: each of
// Shows must appear in the answer and none of Hides may. It is how a screen capability is held to a case: what the
// person reads, not how it is built. A string is matched against the HTML as sent, so `&` is written `&amp;`.
// CASE_VIEW_DUMP=<dir> writes each viewed answer to <dir>/<step id>.html, to read what a failing check saw.
type caseView struct {
	Path  string   `yaml:"path"`
	Shows []string `yaml:"shows"`
	Hides []string `yaml:"hides"`
}

// caseDelete deletes the record an earlier step created.
type caseDelete struct {
	Ref string `yaml:"ref"`
	// From is the page the delete is sent from (its HX-Current-URL), as a Button on a screen sends it; Redirect is where the
	// runtime must then send the browser (its HX-Redirect), "" meaning it must not redirect.
	From     string `yaml:"from"`
	Redirect string `yaml:"redirect"`
}

type caseNotified struct {
	Actor    string `yaml:"actor"`
	Contains string `yaml:"contains"`
	// Not turns the line round: the actor must NOT have been notified of it.
	Not bool `yaml:"not"`
}

// caseReport is what one case measured.
type caseReport struct {
	Loads    bool     // the loader accepted the case once the unread keys were stripped
	Rejected []string // "file: key", one per key stripped, sorted
	Steps    int
	Passed   int
	Results  []string // one line per step, for -v
}

// caseRatchet records each case's measurement. Both numbers move on purpose: Passed may only rise and Rejected may
// only fall, and an improvement fails until the entry is updated, so progress is locked in rather than left as room
// to regress (the shape of every ratchet in internal/conformance).
type ratchetEntry struct {
	Rejected int
	Passed   int
	Steps    int
	// Red names the steps that do not pass, exactly. The counts alone cannot see two steps swap (one starts failing
	// as another starts passing), which a mutation that broke a step the case needs and let a wished one pass by
	// accident did, so the names are the ratchet and the counts are what a person reads.
	Red []string
}

var caseRatchet = map[string]ratchetEntry{
	"case01_design_request":       {Rejected: 0, Passed: 13, Steps: 13, Red: nil},
	"case02_leave_request":        {Rejected: 0, Passed: 10, Steps: 10},
	"case17_helpdesk":             {Rejected: 0, Passed: 15, Steps: 15},
	"case19_project_tasks":        {Rejected: 0, Passed: 19, Steps: 19},
	"case04_maintenance":          {Rejected: 0, Passed: 6, Steps: 6},
	"case05_inventory":            {Rejected: 0, Passed: 6, Steps: 6},
	"case06_petty_cash":           {Rejected: 0, Passed: 15, Steps: 15},
	"case14_lending":              {Rejected: 0, Passed: 9, Steps: 9},
	"case09_accounting":           {Rejected: 0, Passed: 14, Steps: 14},
	"case22_production_contracts": {Rejected: 0, Passed: 114, Steps: 114},
	"case23_number_format":        {Rejected: 0, Passed: 4, Steps: 4},
}

var unreadKey = regexp.MustCompile(`line (\d+): "([^"]+)" is not a key`)

func TestCaseCorpusOnlyAdvances(t *testing.T) {
	dirs, err := filepath.Glob(filepath.Join("testdata", "cases", "case*"))
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no case under testdata/cases: %v", err)
	}
	pool := authTestPool(t)
	for _, dir := range dirs {
		name := filepath.Base(dir)
		rep := playCase(t, pool, dir)
		t.Logf("%s: loads=%v rejected=%d steps %d/%d passed", name, rep.Loads, len(rep.Rejected), rep.Passed, rep.Steps)
		for _, k := range rep.Rejected {
			t.Logf("  unread key  %s", k)
		}
		for _, r := range rep.Results {
			t.Logf("  %s", r)
		}
		for _, msg := range catalogueProblems(name, rep.Rejected, registry.Capabilities) {
			t.Error(msg)
		}
		want, ok := caseRatchet[name]
		if !ok {
			t.Errorf("%s has no entry in caseRatchet; add {Rejected: %d, Passed: %d, Steps: %d}", name, len(rep.Rejected), rep.Passed, rep.Steps)
			continue
		}
		if msg := ratchetProblem(name, len(rep.Rejected), rep.Passed, rep.Steps, want); msg != "" {
			t.Error(msg)
		}
		if msg := redProblem(name, redSteps(rep.Results), want.Red); msg != "" {
			t.Error(msg)
		}
	}
	for name := range caseRatchet {
		if _, err := os.Stat(filepath.Join("testdata", "cases", name)); err != nil {
			t.Errorf("caseRatchet names %s, which has no directory", name)
		}
	}
}

// redSteps lists the ids of the steps a run reported as failing or not run, sorted.
func redSteps(results []string) []string {
	var out []string
	for _, r := range results {
		if rest, ok := strings.CutPrefix(r, "FAIL "); ok {
			out = append(out, rest[:strings.Index(rest, ":")])
		}
	}
	sort.Strings(out)
	return slices.Compact(out)
}

// redProblem compares the failing steps with the recorded ones, exactly.
func redProblem(name string, got, want []string) string {
	if slices.Equal(got, want) {
		return ""
	}
	return fmt.Sprintf("%s: the steps that do not pass are %v, recorded %v; update caseRatchet.Red only for a step that now passes, and a step that starts failing is a regression", name, got, want)
}

func TestRedProblemSeesASwap(t *testing.T) {
	if redProblem("c", []string{"a", "b"}, []string{"a", "b"}) != "" {
		t.Error("equal lists were refused")
	}
	for name, got := range map[string][]string{"a swap": {"a", "c"}, "fewer": {"a"}, "more": {"a", "b", "c"}} {
		if redProblem("c", got, []string{"a", "b"}) == "" {
			t.Errorf("%s was accepted", name)
		}
	}
	if got := redSteps([]string{"ok   z", "FAIL b: expected ok", "FAIL a: x: y", "FAIL a: another line of the same step", "NOT RUN: the case does not load"}); !slices.Equal(got, []string{"a", "b"}) {
		t.Errorf("redSteps = %v", got)
	}
}

// catalogueProblems holds a case's backlog to the capability catalogue (KT1) in both directions: every key the
// loader rejects must belong to a proposed capability that names this case, and every proposed capability naming
// this case must have at least one of its keys rejected here. So a wished key cannot sit outside the catalogue, and a catalogue
// entry cannot cite a case that no longer wishes for it. A rejected entry reads "<file>: <key>".
func catalogueProblems(name string, rejected []string, rows []registry.Capability) []string {
	var out []string
	rejectedKeys := map[string]bool{}
	for _, r := range rejected {
		k := r[strings.LastIndex(r, ": ")+2:]
		rejectedKeys[k] = true
		found := false
		for _, c := range rows {
			if c.Stage != registry.StageProposed || !slices.Contains(c.Cases, name) {
				continue
			}
			found = found || slices.Contains(c.Keys, k)
		}
		if !found {
			out = append(out, fmt.Sprintf("%s: the loader does not read %q and no proposed capability naming this case lists it -- add it to registry.Capabilities", name, k))
		}
	}
	for _, c := range rows {
		if c.Stage != registry.StageProposed || !slices.Contains(c.Cases, name) {
			continue
		}
		wished := false
		for _, k := range c.Keys {
			wished = wished || rejectedKeys[k]
		}
		if !wished {
			out = append(out, fmt.Sprintf("%s: capability %s names this case, which wishes for none of %v (or the loader now reads them) -- update the catalogue", name, c.ID, c.Keys))
		}
	}
	return out
}

// outcomeProblem says why an HTTP answer is not the one a step expects, or "" when it is. A refusal is a 4xx; a 5xx
// is the server failing and satisfies neither expectation.
func outcomeProblem(code int, expect string) string {
	switch {
	case code >= 500:
		return fmt.Sprintf("the server answered %d", code)
	case expect == "refused" && code < 400:
		return fmt.Sprintf("expected refused, the router answered %d", code)
	case expect == "ok" && code >= 400:
		return fmt.Sprintf("expected ok, the router answered %d", code)
	}
	return ""
}

// ratchetProblem compares one case's measurement with its recorded entry and says what moved, or "" when nothing did.
func ratchetProblem(name string, rejected, passed, steps int, want ratchetEntry) string {
	if rejected == want.Rejected && passed == want.Passed && steps == want.Steps {
		return ""
	}
	dir := "changed"
	if passed > want.Passed || rejected < want.Rejected {
		dir = "improved: lock it in"
	}
	if passed < want.Passed || rejected > want.Rejected {
		dir = "REGRESSED"
	}
	return fmt.Sprintf("%s %s: rejected %d (recorded %d), passed %d/%d (recorded %d/%d); update caseRatchet only for an improvement",
		name, dir, rejected, want.Rejected, passed, steps, want.Passed, want.Steps)
}

// playCase stages one case in a throwaway Workspace and plays its scenario.
func playCase(t *testing.T, pool any, dir string) caseReport {
	t.Helper()
	var rep caseReport
	sc := readScenario(t, dir)
	rep.Steps = len(sc.Steps)
	root, manifest, rejected := stageCase(t, dir)
	rep.Rejected = rejected
	app, err := metadata.LoadApplication(manifest)
	if err != nil {
		rep.Results = append(rep.Results, "NOT RUN: the case does not load: "+firstLine(err.Error()))
		for _, line := range strings.Split(err.Error(), "\n")[1:] {
			rep.Results = append(rep.Results, "         "+line)
		}
		return rep
	}
	rep.Loads = true
	_ = root
	rep.Passed, rep.Results = runScenario(t, app, sc)
	return rep
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// scenarioProblem says what is wrong with a scenario as written, or "".
func scenarioProblem(sc caseScenario) string {
	seen := map[string]bool{}
	actors := map[string]bool{}
	for _, a := range sc.Actors {
		actors[a.Name] = true
	}
	for _, s := range sc.Steps {
		switch {
		case s.ID == "" || seen[s.ID]:
			return fmt.Sprintf("step id %q is empty or repeated", s.ID)
		case s.Tick == nil && !actors[s.As]:
			return fmt.Sprintf("step %s is taken by %q, who is not an actor", s.ID, s.As)
		case s.Tick != nil && (s.As != "" || s.Expect != "ok"):
			return fmt.Sprintf("step %s moves the clock; it takes no actor and expects ok", s.ID)
		case countSet(s.Create != nil, s.Update != nil, s.Tick != nil, s.Delete != nil, s.View != nil, s.Read != nil, s.Save != nil) != 1:
			return fmt.Sprintf("step %s must have exactly one of create, update, delete, view, read, save and tick", s.ID)
		case s.View != nil && (s.View.Path == "" || len(s.Record) > 0 || s.API):
			return fmt.Sprintf("step %s views a screen: it names a path, and checks no record and takes no api", s.ID)
		case s.Expect != "ok" && s.Expect != "refused":
			return fmt.Sprintf("step %s expects %q; use ok or refused", s.ID, s.Expect)
		}
		seen[s.ID] = true
	}
	return ""
}

func countSet(flags ...bool) int {
	n := 0
	for _, f := range flags {
		if f {
			n++
		}
	}
	return n
}

func readScenario(t *testing.T, dir string) caseScenario {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "scenario.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var sc caseScenario
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&sc); err != nil {
		t.Fatalf("%s: %v", dir, err)
	}
	if why := scenarioProblem(sc); why != "" {
		t.Fatalf("%s: %s", dir, why)
	}
	return sc
}

// stageCase copies the case's files beside the library's three shared Machines, then loads with the real loader,
// removing every key it reports as unread, until it loads or fails for another reason. It returns the manifest path
// and the removed keys.
func stageCase(t *testing.T, dir string) (root, manifest string, rejected []string) {
	t.Helper()
	root = t.TempDir()
	for _, f := range []string{"user.yaml", "activity.yaml", "notification.yaml"} {
		copyFile(t, filepath.Join("..", "..", "metadata", f), filepath.Join(root, f))
	}
	machines, err := filepath.Glob(filepath.Join(dir, "machines", "*.yaml"))
	if err != nil || len(machines) == 0 {
		t.Fatalf("%s: no machines/*.yaml", dir)
	}
	var manifestText strings.Builder
	manifestText.WriteString("workspace: casecorpus\nmachines:\n  - ../activity.yaml\n  - ../notification.yaml\n  - ../user.yaml\n")
	files := []string{}
	for _, m := range machines {
		base := filepath.Base(m)
		copyFile(t, m, filepath.Join(root, base))
		manifestText.WriteString("  - ../" + base + "\n")
		files = append(files, filepath.Join(root, base))
	}
	copyFile(t, filepath.Join(dir, "application.yaml"), filepath.Join(root, "application.yaml"))
	files = append(files, filepath.Join(root, "application.yaml"))
	manifestText.WriteString("applications:\n  - ../application.yaml\n")
	// A scenario may say how its Workspace writes a figure (`number_format:`), which a manifest declares.
	if raw, err := os.ReadFile(filepath.Join(dir, "scenario.yaml")); err == nil {
		var header struct {
			NumberFormat string `yaml:"number_format"`
		}
		if yaml.Unmarshal(raw, &header) == nil && header.NumberFormat != "" {
			manifestText.WriteString("number_format: " + header.NumberFormat + "\n")
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "workspaces"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest = filepath.Join(root, "workspaces", "casecorpus.yaml")
	if err := os.WriteFile(manifest, []byte(manifestText.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	for round := 0; round < 100; round++ {
		_, err := metadata.LoadApplication(manifest)
		if err == nil {
			break
		}
		hits := unreadKey.FindAllStringSubmatch(err.Error(), -1)
		if len(hits) == 0 {
			break
		}
		file := fileNamedIn(err.Error(), files)
		if file == "" {
			break
		}
		var drop []keyAt
		for _, h := range hits {
			n, _ := strconv.Atoi(h[1])
			drop = append(drop, keyAt{n, h[2]})
		}
		if !stripKeys(t, file, drop) {
			t.Fatalf("%s: could not locate every unread key in %v", file, drop)
		}
		for _, d := range drop {
			rejected = append(rejected, filepath.Base(file)+": "+d.key)
		}
	}
	sort.Strings(rejected)
	return root, manifest, rejected
}

// fileNamedIn finds which staged file a loader error is about. The error names the file by its manifest path.
func fileNamedIn(msg string, files []string) string {
	for _, f := range files {
		if strings.Contains(msg, filepath.Base(f)) {
			return f
		}
	}
	return ""
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	b, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(to, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

type keyAt struct {
	line int
	key  string
}

// stripKeys removes every named mapping key (with its value) from one file in a single pass, because removing one
// and rewriting the file would move the lines the others were reported at.
func stripKeys(t *testing.T, path string, drop []keyAt) bool {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	for _, d := range drop {
		if !removeKey(&doc, d.line, d.key) {
			return false
		}
	}
	out, err := yaml.Marshal(&doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}
	return true
}

func removeKey(n *yaml.Node, line int, key string) bool {
	if n.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(n.Content); i += 2 {
			k := n.Content[i]
			if k.Value == key && k.Line == line {
				n.Content = append(n.Content[:i], n.Content[i+2:]...)
				return true
			}
		}
	}
	for _, c := range n.Content {
		if removeKey(c, line, key) {
			return true
		}
	}
	return false
}

// runScenario plays the steps through the real router and returns how many held and one line per step.
func runScenario(t *testing.T, app *metadata.App, sc caseScenario) (int, []string) {
	t.Helper()
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	slug := fmt.Sprintf("case-corpus-%d", sc.Case)
	ws, err := store.CreateWorkspace(ctx, "Case corpus "+strconv.Itoa(sc.Case), slug)
	if err != nil {
		t.Fatal(err)
	}
	var firstEmail string
	if len(sc.Actors) > 0 {
		firstEmail = fmt.Sprintf("case%d_%s@example.com", sc.Case, sc.Actors[0].Name)
	}
	cleanupAuthTest(t, pool, ws.ID, firstEmail)
	wsCtx := data.WithWorkspaceScope(ctx, ws.ID)
	installed := app.Workspace
	installed.Slug = ws.Slug
	if len(installed.Applications) == 0 {
		t.Fatal("the case installs no Application")
	}
	appID := installed.Applications[0].ID

	ids := map[string]string{}
	cookies := map[string]string{}
	// Five documents an hour: the sixth reading of a case is refused (K01c's ceiling against a loop).
	cfg := config.Config{SessionSecret: slug + "-secret", TemplatePath: realLibrary(t), MaxReadingsPerHour: 5}
	for _, a := range sc.Actors {
		email := fmt.Sprintf("case%d_%s@example.com", sc.Case, a.Name)
		u, err := store.CreateRecord(wsCtx, "mch_user", map[string]any{"fld_email": email})
		if err != nil {
			t.Fatal(err)
		}
		if err := store.AddMember(ctx, ws.ID, u.ID, email, "member", ""); err != nil {
			t.Fatal(err)
		}
		if err := store.SetMemberAppRole(ctx, ws.ID, u.ID, appID, a.Role); err != nil {
			t.Fatal(err)
		}
		ids[a.Name] = u.ID
		cookies[a.Name] = sessionCookieValueForTest(t, cfg, u.ID, 0)
	}
	files, err := storage.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var asked []aiassist.ExtractRequest
	var blank bool
	h := Routes(Deps{UserMachine: userMachineOf(t, app), Store: store, Files: files, Cfg: cfg,
		Extractor: caseReader{reading: sc.Reading, asked: &asked, blank: &blank},
		// A printer the harness only asks whether it exists: a page then draws its PDF button, and nothing is printed.
		Printer:    pdf.NewPrinter("/bin/true", 1),
		Workspaces: map[string]domain.Workspace{slug: installed}, DefaultWorkspaceID: ws.ID})

	machineOf := map[string]string{} // step id -> machine
	recordOf := map[string]string{}  // step id -> record id
	subst := func(v string) string { return substituteRefs(v, ids, recordOf) }
	// currentURL is the page a request says it was sent from (HX-Current-URL); lastRedirect is the HX-Redirect it was answered with.
	var currentURL, lastRedirect string
	send := func(method, path, actor string, values, fileFields map[string]string) int {
		tok := csrfTokenFor(t, h, path)
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		for k, v := range values {
			mw.WriteField(k, subst(v))
		}
		for k, name := range fileFields {
			fw, err := mw.CreateFormFile(k, name)
			if err != nil {
				t.Fatal(err)
			}
			content := "case corpus attachment"
			if strings.HasSuffix(name, ".pdf") {
				content = "%PDF-1.4\n" + content // what a reader accepts is judged by the bytes
			}
			fw.Write([]byte(content))
		}
		mw.Close()
		req := httptest.NewRequest(method, path, &body)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req.Header.Set("X-CSRF-Token", tok.value)
		req.Header.Set("HX-Request", "true")
		if currentURL != "" {
			req.Header.Set("HX-Current-URL", "http://localhost"+currentURL)
		}
		req.AddCookie(tok.cookie)
		req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: cookies[actor]})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		lastRedirect = rec.Header().Get("HX-Redirect")
		return rec.Code
	}

	// sendJSON is the JSON route's twin of send: an update overlays the changed Fields on the stored record (a PUT carries
	// the whole record), numbers go as numbers, and the answer is the status code alone.
	sendJSON := func(method, path, actor string, stored map[string]any, changes map[string]string, machine string, omit ...string) int {
		tok := csrfTokenFor(t, h, path)
		values := map[string]any{}
		for k, v := range stored {
			values[k] = v
		}
		for _, k := range omit {
			delete(values, k)
		}
		var m *domain.Machine
		for _, cand := range installed.Machines {
			if cand.ID == machine {
				m = cand
			}
		}
		for k, v := range changes {
			v = subst(v)
			values[k] = v
			if f, ok := m.FieldByID(k); ok && f.Type.IsNumeric() {
				if n, err := strconv.ParseFloat(v, 64); err == nil {
					values[k] = n
				}
			}
		}
		raw, err := json.Marshal(values)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(method, path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-CSRF-Token", tok.value)
		req.AddCookie(tok.cookie)
		req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: cookies[actor]})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	var lines []string
	passed := 0
	for _, s := range sc.Steps {
		fail := func(format string, a ...any) {
			lines = append(lines, "FAIL "+s.ID+": "+fmt.Sprintf(format, a...))
		}
		var code int
		var machine, id, redirectProblem string
		var snapshot map[string]any
		if s.View != nil {
			path := subst(s.View.Path)
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: cookies[s.As]})
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if why := outcomeProblem(rec.Code, s.Expect); why != "" {
				fail("%s %s", path, why)
				continue
			}
			body, ok := rec.Body.String(), true
			if dir := os.Getenv("CASE_VIEW_DUMP"); dir != "" {
				os.WriteFile(filepath.Join(dir, s.ID+".html"), []byte(body), 0o644)
			}
			for _, want := range s.View.Shows {
				if !strings.Contains(body, subst(want)) {
					fail("%s does not show %q", path, subst(want))
					ok = false
				}
			}
			for _, not := range s.View.Hides {
				if strings.Contains(body, subst(not)) {
					fail("%s shows %q", path, subst(not))
					ok = false
				}
			}
			if s.Count != nil {
				if n := countRecords(t, store, wsCtx, s.Count.Machine, substAll(s.Count.Where, subst)); n != s.Count.Equals {
					fail("%s holds %d matching records, want %d", s.Count.Machine, n, s.Count.Equals)
					ok = false
				}
			}
			if ok {
				passed++
				lines = append(lines, "ok   "+s.ID)
			}
			continue
		}
		if s.Read != nil {
			asked, blank = nil, s.Read.Blank
			code := send(http.MethodPost, "/pages/"+s.Read.Nav+"/extract", s.As, nil, map[string]string{domain.ExtractFileParam: s.Read.File})
			if why := outcomeProblem(code, s.Expect); why != "" {
				fail("%s", why)
				continue
			}
			ok := true
			if s.Expect == "ok" {
				proposal := ""
				if u, err := url.Parse(lastRedirect); err == nil {
					proposal = u.Query().Get(domain.ExtractParam)
				}
				if proposal == "" || !strings.HasPrefix(lastRedirect, "/pages/"+s.Read.Nav+"?") {
					fail("the reading was answered with a redirect to %q, want the page with a proposal", lastRedirect)
					ok = false
				}
				recordOf[s.ID] = proposal
				var got []string
				if len(asked) == 1 {
					for _, f := range asked[0].Fields {
						got = append(got, f.ID)
					}
				}
				if len(asked) != 1 || !slices.Equal(got, s.Read.Asks) {
					fail("the reader was asked %v, want exactly %v", got, s.Read.Asks)
					ok = false
				}
			}
			if ok {
				passed++
				lines = append(lines, "ok   "+s.ID)
			}
			continue
		}
		if s.Tick != nil {
			machine, id = machineOf[s.Tick.Ref], recordOf[s.Tick.Ref]
			if id == "" {
				fail("step %s created no record to watch", s.Tick.Ref)
				continue
			}
			code = http.StatusOK
			if err := execution.RunScheduledEvents(wsCtx, store, mail.LogMailer{}, installed.Machines, time.Now().AddDate(0, 0, s.Tick.Days)); err != nil {
				fail("the scheduler failed: %v", err)
				continue
			}
		} else if s.Delete != nil {
			machine, id = machineOf[s.Delete.Ref], recordOf[s.Delete.Ref]
			if id == "" {
				fail("step %s created no record to delete", s.Delete.Ref)
				continue
			}
			if rec, err := store.GetRecord(wsCtx, machine, id); err == nil {
				snapshot = map[string]any{}
				for k, v := range rec.Values {
					snapshot[k] = v
				}
			}
			if s.API {
				code = sendJSON(http.MethodDelete, "/api/machines/"+machine+"/records/"+id, s.As, nil, nil, machine)
			} else {
				currentURL = subst(s.Delete.From)
				code = send(http.MethodDelete, "/machines/"+machine+"/records/"+id, s.As, nil, nil)
				currentURL = ""
				if s.Expect == "ok" && lastRedirect != s.Delete.Redirect {
					redirectProblem = fmt.Sprintf("the delete was answered with a redirect to %q, want %q", lastRedirect, s.Delete.Redirect)
				}
			}
			if s.Expect == "refused" && !(code >= 400 && code < 500) && snapshot != nil {
				// The runtime deleted what the case says it must keep: put it back so later steps stay meaningful. It
				// comes back under a new id, which the steps after this one now name.
				restored, err := store.CreateRecord(wsCtx, machine, snapshot)
				if err != nil {
					t.Fatalf("restoring after %s: %v", s.ID, err)
				}
				for ref, rid := range recordOf {
					if rid == id {
						recordOf[ref] = restored.ID
					}
				}
				id, snapshot = restored.ID, nil
			}
		} else if s.Save != nil {
			machine = s.Save.Machine
			values := map[string]string{domain.ExtractParam: recordOf[s.Save.Proposal]}
			for k, v := range s.Save.Values {
				values[k] = v
			}
			code = send(http.MethodPost, "/pages/"+s.Save.Nav+"/extract/save", s.As, values, nil)
			if s.Expect == "ok" {
				id = lastRedirect[strings.LastIndex(lastRedirect, "/")+1:]
			}
		} else if s.Create != nil {
			machine = s.Create.Machine
			before := recordIDs(t, store, wsCtx, machine)
			if s.API {
				code = sendJSON(http.MethodPost, "/api/machines/"+machine+"/records", s.As, nil, s.Create.Values, machine)
			} else {
				code = send(http.MethodPost, "/machines/"+machine+"/records", s.As, s.Create.Values, s.Create.Files)
			}
			for rid := range recordIDs(t, store, wsCtx, machine) {
				if !before[rid] {
					id = rid
				}
			}
		} else {
			machine, id = machineOf[s.Update.Ref], recordOf[s.Update.Ref]
			if id == "" {
				fail("step %s created no record to update", s.Update.Ref)
				continue
			}
			if rec, err := store.GetRecord(wsCtx, machine, id); err == nil {
				snapshot = map[string]any{}
				for k, v := range rec.Values {
					snapshot[k] = v
				}
			}
			if s.API {
				code = sendJSON(http.MethodPut, "/api/machines/"+machine+"/records/"+id, s.As, snapshot, s.Update.Values, machine, s.Update.Omit...)
			} else {
				code = send(http.MethodPatch, "/machines/"+machine+"/records/"+id, s.As, s.Update.Values, s.Update.Files)
			}
		}
		machineOf[s.ID], recordOf[s.ID] = machine, id
		refused := code >= 400 && code < 500
		if s.Expect == "refused" && !refused && snapshot != nil {
			// The runtime took a write the case says it must refuse. Undo it, so the steps after this one measure
			// themselves and not the damage this one did.
			if _, err := store.UpdateRecord(wsCtx, machine, id, snapshot); err != nil {
				t.Fatalf("restoring %s after %s: %v", id, s.ID, err)
			}
		}
		if why := outcomeProblem(code, s.Expect); why != "" {
			fail("%s", why)
			continue
		}
		ok := true
		if redirectProblem != "" {
			fail("%s", redirectProblem)
			ok = false
		}
		if s.Expect == "ok" && len(s.Record) > 0 {
			rec, err := store.GetRecord(wsCtx, machine, id)
			if err != nil {
				fail("record %s unreadable: %v", id, err)
				continue
			}
			for f, want := range s.Record {
				got := fmt.Sprint(rec.Values[f])
				if want == "*" && got != "" && got != "<nil>" {
					continue
				}
				if pre, isPrefix := strings.CutPrefix(want, "prefix:"); isPrefix && strings.HasPrefix(got, pre) {
					continue
				}
				if got != subst(want) {
					fail("%s is %q, want %q", f, got, subst(want))
					ok = false
				}
			}
		}
		if s.Count != nil {
			if n := countRecords(t, store, wsCtx, s.Count.Machine, substAll(s.Count.Where, subst)); n != s.Count.Equals {
				fail("%s holds %d matching records, want %d", s.Count.Machine, n, s.Count.Equals)
				ok = false
			}
		}
		for _, n := range s.Notified {
			got := hasNotification(t, store, wsCtx, ids[n.Actor], n.Contains)
			if got && n.Not {
				fail("%s was notified of %q", n.Actor, n.Contains)
				ok = false
			} else if !got && !n.Not {
				fail("%s was not notified of %q", n.Actor, n.Contains)
				ok = false
			}
		}
		if ok {
			passed++
			lines = append(lines, "ok   "+s.ID)
		}
	}
	return passed, lines
}

// substituteRefs fills `$<actor>` with that actor's user record id and `$<step id>` with the record that step created,
// so a later step can point a relation at it. The longest step id goes first, so `$open-period` is not read as `$open`.
func substituteRefs(v string, actors, steps map[string]string) string {
	for name, id := range actors {
		v = strings.ReplaceAll(v, "$"+name, id)
	}
	stepIDs := make([]string, 0, len(steps))
	for sid := range steps {
		stepIDs = append(stepIDs, sid)
	}
	sort.Slice(stepIDs, func(i, j int) bool { return len(stepIDs[i]) > len(stepIDs[j]) })
	for _, sid := range stepIDs {
		v = strings.ReplaceAll(v, "$"+sid, steps[sid])
	}
	return v
}

func TestSubstituteRefs(t *testing.T) {
	actors := map[string]string{"rina": "usr_rina"}
	steps := map[string]string{"open": "rec_1", "open-period": "rec_2"}
	for in, want := range map[string]string{
		"$rina":            "usr_rina",
		"$open":            "rec_1",
		"$open-period":     "rec_2",
		"x $open-period y": "x rec_2 y",
		"$nobody":          "$nobody",
		"plain":            "plain",
	} {
		if got := substituteRefs(in, actors, steps); got != want {
			t.Errorf("substituteRefs(%q) = %q, want %q", in, got, want)
		}
	}
}

// substAll is a count's wanted values with the refs of earlier steps ($contract) put in.
func substAll(where map[string]string, subst func(string) string) map[string]string {
	out := make(map[string]string, len(where))
	for k, v := range where {
		out[k] = subst(v)
	}
	return out
}

func countRecords(t *testing.T, store *data.Store, ctx context.Context, machine string, where map[string]string) int {
	t.Helper()
	recs, err := store.ListRecords(ctx, machine)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, r := range recs {
		match := true
		for f, want := range where {
			if fmt.Sprint(r.Values[f]) != want {
				match = false
			}
		}
		if match {
			n++
		}
	}
	return n
}

func recordIDs(t *testing.T, store *data.Store, ctx context.Context, machine string) map[string]bool {
	t.Helper()
	recs, err := store.ListRecords(ctx, machine)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, r := range recs {
		out[r.ID] = true
	}
	return out
}

func hasNotification(t *testing.T, store *data.Store, ctx context.Context, recipient, contains string) bool {
	t.Helper()
	recs, err := store.ListRecords(ctx, "mch_notification")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range recs {
		if fmt.Sprint(r.Values["fld_recipient"]) == recipient && strings.Contains(fmt.Sprint(r.Values["fld_message"]), contains) {
			return true
		}
	}
	return false
}

// TestCaseHarnessReportsEachKindOfMismatch shows the harness can fail. Every step below is wrong in exactly one way,
// except the first, which is right; a harness that accepted everything would still pass the corpus ratchet only by
// leaving its numbers where they are, so each check is proved here by being handed a wrong expectation.
func TestCaseHarnessReportsEachKindOfMismatch(t *testing.T) {
	authTestPool(t)
	_, manifest, _ := stageCase(t, filepath.Join("testdata", "cases", "case01_design_request"))
	app, err := metadata.LoadApplication(manifest)
	if err != nil {
		t.Fatal(err)
	}
	ok := func(id, as string, vals map[string]string) caseStep {
		return caseStep{ID: id, As: as, Expect: "ok", Create: &caseCreate{Machine: "mch_design_request", Values: vals}}
	}
	good := map[string]string{"fld_title": "Poster", "fld_coordinator": "$kiki"}
	steps := []caseStep{
		ok("right", "rina", good),
		{ID: "expected-refused-got-ok", As: "rina", Expect: "refused", Create: &caseCreate{Machine: "mch_design_request", Values: good}},
		{ID: "expected-ok-got-refused", As: "rina", Expect: "ok", Create: &caseCreate{Machine: "mch_design_request", Values: map[string]string{"fld_format": "poster"}}},
		{ID: "wrong-record-value", As: "rina", Expect: "ok", Record: map[string]string{"fld_status": "selesai"}, Create: &caseCreate{Machine: "mch_design_request", Values: good}},
		{ID: "missing-notification", As: "rina", Expect: "ok", Notified: []caseNotified{{Actor: "dedi", Contains: "Poster"}}, Create: &caseCreate{Machine: "mch_design_request", Values: good}},
		{ID: "assigned", As: "kiki", Expect: "ok", Update: &caseUpdate{Ref: "right", Values: map[string]string{"fld_designer": "$dedi", "fld_status": "ditugaskan"}}},
		{ID: "unwanted-notification", As: "kiki", Expect: "ok", Notified: []caseNotified{{Actor: "dedi", Contains: "Poster", Not: true}}, Update: &caseUpdate{Ref: "right", Values: map[string]string{"fld_description": "x"}}},
		{ID: "wrong-recipient", As: "kiki", Expect: "ok", Notified: []caseNotified{{Actor: "rina", Contains: "Kamu ditugaskan"}}, Update: &caseUpdate{Ref: "right", Values: map[string]string{"fld_description": "y"}}},
		{ID: "count-right", As: "rina", Expect: "ok", Count: &caseCount{Machine: "mch_design_request", Where: map[string]string{"fld_title": "Poster"}, Equals: 99}, Create: &caseCreate{Machine: "mch_design_request", Values: good}},
		{ID: "unknown-ref", As: "kiki", Expect: "ok", Update: &caseUpdate{Ref: "nowhere", Values: map[string]string{"fld_description": "x"}}},
	}
	sc := caseScenario{Case: 9001, Title: "harness self-test", Steps: steps, Actors: []caseActor{
		{Name: "rina", Role: "requester"}, {Name: "kiki", Role: "koordinator desain"}, {Name: "dedi", Role: "desainer"}}}
	passed, lines := runScenario(t, app, sc)
	out := strings.Join(lines, "\n")
	for _, want := range []string{
		"ok   right",
		"FAIL expected-refused-got-ok: expected refused",
		"FAIL expected-ok-got-refused: expected ok",
		"FAIL wrong-record-value: fld_status is",
		"FAIL missing-notification: dedi was not notified",
		"ok   assigned",
		"FAIL unwanted-notification: dedi was notified",
		"FAIL wrong-recipient: rina was not notified",
		"FAIL count-right: mch_design_request holds",
		"FAIL unknown-ref: step nowhere created no record",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the report lacks %q:\n%s", want, out)
		}
	}
	if passed != 2 {
		t.Errorf("%d steps passed, want 2 (right, assigned):\n%s", passed, out)
	}
}

func TestCaseOutcomeAndRatchetJudgements(t *testing.T) {
	for _, c := range []struct {
		code   int
		expect string
		bad    bool
	}{
		{200, "ok", false}, {303, "ok", false}, {403, "refused", false}, {422, "refused", false},
		{200, "refused", true}, {403, "ok", true}, {422, "ok", true},
		{500, "ok", true}, {500, "refused", true}, {503, "refused", true},
	} {
		if got := outcomeProblem(c.code, c.expect) != ""; got != c.bad {
			t.Errorf("outcomeProblem(%d, %s) problem=%v, want %v", c.code, c.expect, got, c.bad)
		}
	}
	good := caseStep{ID: "a", As: "x", Expect: "ok", Create: &caseCreate{}}
	for name, sc := range map[string]caseScenario{
		"repeated id":            {Actors: []caseActor{{Name: "x"}}, Steps: []caseStep{good, good}},
		"empty id":               {Actors: []caseActor{{Name: "x"}}, Steps: []caseStep{{As: "x", Expect: "ok", Create: &caseCreate{}}}},
		"stranger":               {Steps: []caseStep{good}},
		"neither":                {Actors: []caseActor{{Name: "x"}}, Steps: []caseStep{{ID: "a", As: "x", Expect: "ok"}}},
		"both":                   {Actors: []caseActor{{Name: "x"}}, Steps: []caseStep{{ID: "a", As: "x", Expect: "ok", Create: &caseCreate{}, Update: &caseUpdate{}}}},
		"unknown wants":          {Actors: []caseActor{{Name: "x"}}, Steps: []caseStep{{ID: "a", As: "x", Expect: "maybe", Create: &caseCreate{}}}},
		"tick with an actor":     {Actors: []caseActor{{Name: "x"}}, Steps: []caseStep{{ID: "a", As: "x", Expect: "ok", Tick: &caseTick{}}}},
		"tick expecting refusal": {Steps: []caseStep{{ID: "a", Expect: "refused", Tick: &caseTick{}}}},
		"delete and update":      {Actors: []caseActor{{Name: "x"}}, Steps: []caseStep{{ID: "a", As: "x", Expect: "ok", Delete: &caseDelete{}, Update: &caseUpdate{}}}},
		"tick and create":        {Actors: []caseActor{{Name: "x"}}, Steps: []caseStep{{ID: "a", Expect: "ok", Tick: &caseTick{}, Create: &caseCreate{}}}},
	} {
		if scenarioProblem(sc) == "" {
			t.Errorf("a scenario with %s was accepted", name)
		}
	}
	if why := scenarioProblem(caseScenario{Steps: []caseStep{{ID: "t", Expect: "ok", Tick: &caseTick{Days: 1, Ref: "a"}}}}); why != "" {
		t.Errorf("a sound tick was refused: %s", why)
	}
	if why := scenarioProblem(caseScenario{Actors: []caseActor{{Name: "x"}}, Steps: []caseStep{good}}); why != "" {
		t.Errorf("a sound scenario was refused: %s", why)
	}
	rec := ratchetEntry{Rejected: 6, Passed: 9, Steps: 12}
	for _, c := range []struct {
		rejected, passed, steps int
		want                    string
	}{
		{6, 9, 12, ""},
		{6, 10, 12, "improved"},
		{5, 9, 12, "improved"},
		{6, 8, 12, "REGRESSED"},
		{7, 9, 12, "REGRESSED"},
		{6, 9, 13, "changed"},
	} {
		got := ratchetProblem("c", c.rejected, c.passed, c.steps, rec)
		if (c.want == "") != (got == "") || (c.want != "" && !strings.Contains(got, c.want)) {
			t.Errorf("ratchetProblem(%d,%d,%d) = %q, want %q", c.rejected, c.passed, c.steps, got, c.want)
		}
	}
}

func TestCatalogueProblemsJudgesABacklogBothWays(t *testing.T) {
	rows := []registry.Capability{
		{ID: "p", Struct: "fieldDoc", Keys: []string{"a", "b"}, Stage: registry.StageProposed, Cases: []string{"c1"}},
		{ID: "q", Struct: "fieldDoc", Keys: []string{"z"}, Stage: registry.StageProposed, Cases: []string{"c2"}},
		{ID: "s", Struct: "fieldDoc", Keys: []string{"y"}, Stage: registry.StageSupported},
	}
	if p := catalogueProblems("c1", []string{"f.yaml: a", "f.yaml: b", "g.yaml: a"}, rows); len(p) != 0 {
		t.Fatalf("a backlog the catalogue covers was refused: %v", p)
	}
	cases := []struct {
		name     string
		rejected []string
		want     string
	}{
		{"c1", []string{"f.yaml: a", "f.yaml: b", "f.yaml: nope"}, `"nope"`},
		{"c2", []string{"f.yaml: nothing"}, "wishes for none"},
		{"c1", []string{"f.yaml: a", "f.yaml: b", "f.yaml: z"}, `"z"`}, // z belongs to another case
		{"c1", []string{"f.yaml: a", "f.yaml: b", "f.yaml: y"}, `"y"`}, // a supported key is not a wish
	}
	for _, c := range cases {
		got := strings.Join(catalogueProblems(c.name, c.rejected, rows), "; ")
		if !strings.Contains(got, c.want) {
			t.Errorf("%v: want a problem containing %s, got %q", c.rejected, c.want, got)
		}
	}
}
