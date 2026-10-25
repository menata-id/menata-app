//go:build aieval

// The baseline measurement of the assistant (plan 2026-10-25, C1): it puts a fixed set of requests to the real
// model and reports how well it builds what it can and records what it cannot. It is behind a build tag because
// it calls a paid API, is not deterministic, and has no business in `make test` or the pre-push hook:
//
//	make aieval                       # all cases, 3 runs each
//	AIEVAL_RUNS=1 AIEVAL_ONLY=helpdesk,calendar make aieval
//	AIEVAL_RECORD=dir make aieval     # also write every conversation, one JSON file per run (the replay fixtures)
//	AIEVAL_NO_REGISTER=1 make aieval  # the prompt without the register's list of known gaps, to see what it does
//
// It only measures. Nothing here changes the prompt, and no number it prints is a gate.
//
// The assistant is a conversation: it asks what it needs (the person's role, the menu) before it sets
// "change". The harness therefore plays the person with one fixed, neutral follow-up and lets the model take up
// to evalMaxTurns turns; gaps are collected across all of them, as the web handler would record them.
package aiassist

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const evalMaxTurns = 5

// knownGapsForEval is the register as the test binary started, kept so a run without it can restore it.
var knownGapsForEval = append([]KnownGap(nil), KnownGaps...)

// evalFollowUp is what the harness answers whenever the model asks a question instead of setting "change".
// It chooses nothing in favour of recording gaps: it only declines to be asked again.
const evalFollowUp = "Terserah kamu, pilih saja yang paling masuk akal untuk semua pertanyaan tadi. Saya sendiri memegang peran pertama. Semua masuk ke menu sesuai urutan usulanmu. Silakan langsung buatkan."

type evalRun struct {
	Case      string `json:"case"`
	Group     string `json:"group"`
	Run       int    `json:"run"`
	Turns     int    `json:"turns"`
	ChangeSet bool   `json:"change_set"`
	// ValidateError is the refusal that stood after the one automatic correction round internal/web gives a
	// change that does not validate (runAssistantTurn); FirstRefusal is what the model's first change drew.
	// Recovered is true when the first change was refused and the corrected one was accepted.
	ValidateError string          `json:"validate_error,omitempty"`
	FirstRefusal  string          `json:"first_refusal,omitempty"`
	Recovered     bool            `json:"recovered,omitempty"`
	Gaps          []CapabilityGap `json:"gaps"`
	Messages      []string        `json:"messages"`
	Error         string          `json:"error,omitempty"`
	Satisfied     bool            `json:"satisfied"`
	// Prompt and Replies are the whole conversation as the model gave it, written out one file per run when
	// AIEVAL_RECORD is set, so a later test can replay it without the model (plan C2).
	Prompt  string  `json:"-"`
	Replies []Reply `json:"-"`
}

func TestAIEvalBaseline(t *testing.T) {
	key := os.Getenv("GEMINI_API_KEY")
	if key == "" {
		t.Skip("set GEMINI_API_KEY (make aieval does)")
	}
	runs := 3
	if v, err := strconv.Atoi(os.Getenv("AIEVAL_RUNS")); err == nil && v > 0 {
		runs = v
	}
	only := map[string]bool{}
	for _, id := range strings.Split(os.Getenv("AIEVAL_ONLY"), ",") {
		if id = strings.TrimSpace(id); id != "" {
			only[id] = true
		}
	}
	out := os.Getenv("AIEVAL_OUT")
	if out == "" {
		out = t.TempDir()
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}

	if os.Getenv("AIEVAL_NO_REGISTER") != "" {
		KnownGaps = nil // the prompt then carries no list of known gaps; restored when the test ends
		saved := append([]KnownGap(nil), knownGapsForEval...)
		t.Cleanup(func() { KnownGaps = saved })
	}
	client := GeminiClient{APIKey: key, HTTPClient: &http.Client{Timeout: 90 * time.Second}}
	prompt := SystemPromptFor(nil, nil)
	existing := ExistingState{
		MachineIDs:     map[string]bool{"mch_user": true, "mch_activity": true, "mch_notification": true},
		ApplicationIDs: map[string]bool{},
		Applications:   map[string]ExistingApplicationState{},
		NavIDs:         map[string]bool{},
	}

	type job struct {
		c   evalCase
		run int
	}
	var jobs []job
	for _, c := range evalCases {
		if len(only) > 0 && !only[c.ID] {
			continue
		}
		for r := 1; r <= runs; r++ {
			jobs = append(jobs, job{c, r})
		}
	}
	results := make([]evalRun, len(jobs))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for i, j := range jobs {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, j job) {
			defer wg.Done()
			defer func() { <-sem }()
			results[i] = runEvalCase(client, prompt, existing, j.c, j.run)
		}(i, j)
	}
	wg.Wait()

	f, err := os.Create(filepath.Join(out, "runs.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	w := bufio.NewWriter(f)
	for _, r := range results {
		b, _ := json.Marshal(r)
		_, _ = w.Write(append(b, '\n'))
	}
	_ = w.Flush()
	_ = f.Close()

	if dir := os.Getenv("AIEVAL_RECORD"); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, r := range results {
			b, err := json.MarshalIndent(struct {
				Case    string  `json:"case"`
				Run     int     `json:"run"`
				Group   string  `json:"group"`
				Prompt  string  `json:"prompt"`
				Replies []Reply `json:"replies"`
			}{r.Case, r.Run, r.Group, r.Prompt, r.Replies}, "", " ")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%s-%d.json", r.Case, r.Run)), b, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}

	summary := summariseEval(results)
	if err := os.WriteFile(filepath.Join(out, "summary.md"), []byte(summary), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("results in %s\n%s", out, summary)
}

func runEvalCase(client Client, prompt string, existing ExistingState, c evalCase, run int) evalRun {
	r := evalRun{Case: c.ID, Group: c.Group, Run: run, Prompt: c.Prompt}
	turns := []Turn{{Role: "user", Text: c.Prompt}}
	for r.Turns < evalMaxTurns {
		reply, err := generateWithRetry(client, prompt, turns)
		if err != nil {
			r.Error = err.Error()
			break
		}
		r.Turns++
		r.Replies = append(r.Replies, reply)
		r.Messages = append(r.Messages, reply.Message)
		r.Gaps = append(r.Gaps, reply.Gaps()...)
		if reply.Change != nil {
			r.ChangeSet = true
			err := Validate(*reply.Change, existing)
			if err != nil && r.FirstRefusal == "" {
				// One correction round, worded as internal/web's handler words it: the model answers once more.
				r.FirstRefusal = err.Error()
				raw, _ := json.Marshal(reply)
				report := "Automatic check: the change in your last reply does not validate, so it cannot be reviewed yet:\n\n" +
					err.Error() + "\n\nCorrect it, or ask me what you need to know."
				turns = append(turns, Turn{Role: "model", Text: string(raw)}, Turn{Role: "user", Text: report})
				continue
			}
			if err != nil {
				r.ValidateError = err.Error()
			} else if r.FirstRefusal != "" {
				r.Recovered = true
			}
			break
		}
		raw, _ := json.Marshal(reply)
		turns = append(turns, Turn{Role: "model", Text: string(raw)}, Turn{Role: "user", Text: evalFollowUp})
	}
	r.Satisfied = evalSatisfied(c, r)
	return r
}

func generateWithRetry(client Client, prompt string, turns []Turn) (Reply, error) {
	var err error
	for attempt, wait := 0, 5*time.Second; attempt < 4; attempt, wait = attempt+1, wait*3 {
		var reply Reply
		reply, err = client.Generate(context.Background(), prompt, turns)
		if err == nil {
			return reply, nil
		}
		if !strings.Contains(err.Error(), "429") && !strings.Contains(err.Error(), "503") && !strings.Contains(err.Error(), "timeout") {
			break
		}
		time.Sleep(wait)
	}
	return Reply{}, err
}

// evalSatisfied is the per-run verdict: a control case is satisfied by recording no gap, a gap case by a gap
// whose key or class the case names, a template case by pointing at the template or recording any gap.
func evalSatisfied(c evalCase, r evalRun) bool {
	if r.Error != "" {
		return false
	}
	switch c.Group {
	case "control":
		return len(r.Gaps) == 0
	case "table":
		if len(r.Gaps) > 0 {
			return false
		}
		for _, reply := range r.Replies {
			if reply.Change == nil || reply.Change.Application == nil {
				continue
			}
			for _, n := range reply.Change.Application.Navigation {
				if n.List.isTable() {
					return true
				}
			}
		}
		return false
	case "template":
		for _, m := range r.Messages {
			if strings.Contains(m, c.WantTemplate) {
				return true
			}
		}
		return len(r.Gaps) > 0
	}
	for _, g := range r.Gaps {
		for _, k := range c.WantKeys {
			if g.CapabilityKey == k {
				return true
			}
		}
		for _, cl := range c.WantClasses {
			if g.Class == cl {
				return true
			}
		}
	}
	return false
}

func pct(n, d int) string {
	if d == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%d%% (%d/%d)", n*100/d, n, d)
}

func summariseEval(rs []evalRun) string {
	byCase := map[string][]evalRun{}
	var order []string
	for _, r := range rs {
		if _, ok := byCase[r.Case]; !ok {
			order = append(order, r.Case)
		}
		byCase[r.Case] = append(byCase[r.Case], r)
	}
	known := map[string]bool{}
	for _, g := range KnownGaps {
		known[g.Key] = true
	}
	var total, errs, changes, valid, refusedFirst, recovered int
	var ctlRuns, ctlFalse, ctlRecords, ctlFromRegister, gapRuns, gapHit, gapOther, gapNone, tblRuns, tblHit int
	var keyRuns, keyHit int
	var gaps, gapsIdeal, gapsKnown int
	want := map[string]evalCase{}
	for _, c := range evalCases {
		want[c.ID] = c
	}
	for _, r := range rs {
		total++
		if r.Error != "" {
			errs++
			continue
		}
		if r.ChangeSet {
			changes++
			if r.ValidateError == "" {
				valid++
			}
			if r.FirstRefusal != "" {
				refusedFirst++
				if r.Recovered {
					recovered++
				}
			}
		}
		for _, g := range r.Gaps {
			gaps++
			if len(strings.TrimSpace(g.IdealCapability)) >= 40 {
				gapsIdeal++
			}
			if known[g.CapabilityKey] {
				gapsKnown++
			}
		}
		switch r.Group {
		case "control":
			ctlRuns++
			if len(r.Gaps) > 0 {
				ctlFalse++
			}
			for _, g := range r.Gaps {
				ctlRecords++
				if known[g.CapabilityKey] {
					ctlFromRegister++
				}
			}
		case "table":
			tblRuns++
			if r.Satisfied {
				tblHit++
			}
		case "gap":
			gapRuns++
			switch {
			case r.Satisfied:
				gapHit++
			case len(r.Gaps) == 0:
				gapNone++
			default:
				gapOther++
			}
			if wk := want[r.Case].WantKeys; len(wk) > 0 {
				keyRuns++
				for _, g := range r.Gaps {
					for _, k := range wk {
						if g.CapabilityKey == k {
							keyHit++
							goto counted
						}
					}
				}
			counted:
			}
		}
	}
	var b strings.Builder
	b.WriteString("# Baseline of the AI assistant (C1)\n\n")
	fmt.Fprintf(&b, "Model %s, %d runs over %d cases, up to %d turns each.\n\n", geminiModel, total, len(order), evalMaxTurns)
	b.WriteString("| Measure | Result |\n|---|---|\n")
	fmt.Fprintf(&b, "| Runs that failed outright (API error) | %s |\n", pct(errs, total))
	fmt.Fprintf(&b, "| Runs that reached a change | %s |\n", pct(changes, total-errs))
	fmt.Fprintf(&b, "| Changes refused by Validate the first time | %s |\n", pct(refusedFirst, changes))
	fmt.Fprintf(&b, "| ...of those, recovered by the one correction round | %s |\n", pct(recovered, refusedFirst))
	fmt.Fprintf(&b, "| Changes that pass Validate after it (what a person gets) | %s |\n", pct(valid, changes))
	fmt.Fprintf(&b, "| Gap cases: a gap of the wanted key or class recorded (recall) | %s |\n", pct(gapHit, gapRuns))
	fmt.Fprintf(&b, "| Gap cases: gaps recorded, but none of the wanted key or class | %s |\n", pct(gapOther, gapRuns))
	fmt.Fprintf(&b, "| Gap cases: no gap at all (a silent drop) | %s |\n", pct(gapNone, gapRuns))
	fmt.Fprintf(&b, "| Cases that want a register key: that key used | %s |\n", pct(keyHit, keyRuns))
	fmt.Fprintf(&b, "| Table cases: a table written, no gap (the slot is used) | %s |\n", pct(tblHit, tblRuns))
	fmt.Fprintf(&b, "| Control cases: any gap recorded (false gap) | %s |\n", pct(ctlFalse, ctlRuns))
	fmt.Fprintf(&b, "| Control cases: false gap records that are a register key | %s |\n", pct(ctlFromRegister, ctlRecords))
	fmt.Fprintf(&b, "| Gaps whose ideal_capability has 40+ characters | %s |\n", pct(gapsIdeal, gaps))
	fmt.Fprintf(&b, "| Gaps that reuse a key of the register | %s |\n", pct(gapsKnown, gaps))
	b.WriteString("\n## Per case\n\nKeys that differ between runs of one case are the same need counted under several names.\n\n| Case | Group | Satisfied | Change | Valid | Gap keys recorded (all runs) |\n|---|---|---|---|---|---|\n")
	for _, id := range order {
		runs := byCase[id]
		var sat, chg, val int
		keys := map[string]int{}
		for _, r := range runs {
			if r.Satisfied {
				sat++
			}
			if r.ChangeSet {
				chg++
				if r.ValidateError == "" {
					val++
				}
			}
			for _, g := range r.Gaps {
				keys[g.CapabilityKey+"/"+g.Class]++
			}
		}
		var ks []string
		for k, n := range keys {
			ks = append(ks, fmt.Sprintf("%s x%d", k, n))
		}
		sort.Strings(ks)
		fmt.Fprintf(&b, "| %s | %s | %d/%d | %d/%d | %d/%d | %s |\n", id, runs[0].Group, sat, len(runs), chg, len(runs), val, chg, strings.Join(ks, "; "))
	}
	b.WriteString("\n## Validate failures that stood after the correction round\n\n")
	n := 0
	for _, r := range rs {
		if r.ValidateError != "" {
			n++
			fmt.Fprintf(&b, "- %s run %d: %s\n", r.Case, r.Run, strings.ReplaceAll(r.ValidateError, "\n", " "))
		}
	}
	if n == 0 {
		b.WriteString("none\n")
	}
	return b.String()
}
