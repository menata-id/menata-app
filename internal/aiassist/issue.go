package aiassist

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"menata.app/internal/metadata"
)

// Issue is one thing wrong with a GeneratedChange, said where it is and, when the fault is a choice among known
// values, what would have been accepted.
//
// Before this type every fault was a sentence in a []string, so the model that had to fix it (the automatic check
// feeds it back, web/newapplication.go) and the Apps Builder inspector that would show it beside a control both had
// to find the place by reading. Module Builder's form attaches each violation to its element by address
// (ComponentSectionForm::validateForm); this is the same idea, with the addresses Menata's builder tree already uses
// for its parts (web/builderscreen.go: "machines/<m>/fields/<f>", "navigation/<item>"), so an Issue's Address names a
// node of the tree.
//
// The Message is unchanged from when it was a bare string: the pairing of the assistant's refusals with the
// builder's (builder.TestOperationsRefuseWhatTheConversationRefuses and its kind) compares those words.
//
// Issues are only made by at / atf and the *At helpers below, never by a literal elsewhere
// (TestEveryIssueIsMadeWithAnAddress reads the sources), so an Issue without an Address cannot be added by a new
// check without failing that gate.
type Issue struct {
	// Address is the part of the Application the fault is in, relative to it: AddressApplication, or one of the
	// shapes machineAt, fieldAt, permissionAt, transitionAt, eventAt, viewAt and menuAt make.
	Address string `json:"address"`
	// Message says what is wrong, and usually what to do.
	Message string `json:"message"`
	// Allowed are the values that would have been accepted, when the fault is a choice (a role the Application
	// declares, a field of the right type, a closed word list). Sorted when it came from a set.
	Allowed []string `json:"allowed,omitempty"`
}

// AddressApplication addresses a fault in the Application as a whole, or one that no narrower part owns.
const AddressApplication = "application"

func machineAt(machine string) string { return "machines/" + machine }
func fieldAt(machine, field string) string {
	return machineAt(machine) + "/fields/" + field
}
func permissionAt(machine, permission string) string {
	return machineAt(machine) + "/permissions/" + permission
}
func transitionAt(machine, transition string) string {
	return machineAt(machine) + "/transitions/" + transition
}
func eventAt(machine, event string) string {
	return machineAt(machine) + "/events/" + event
}
func viewAt(machine, view string) string {
	return machineAt(machine) + "/views/" + view
}

// menuAt addresses a menu entry: by its id when it has one (an installed item), by its 1-based place in the list
// when it is new and has no id yet ("navigation/#2").
func menuAt(id string, position int) string {
	if id != "" {
		return "navigation/" + id
	}
	return "navigation/#" + strconv.Itoa(position)
}

// at makes an Issue. The message is a plain string; atf formats one.
func at(address, message string) Issue { return Issue{Address: address, Message: message} }

func atf(address, format string, args ...any) Issue {
	return Issue{Address: address, Message: fmt.Sprintf(format, args...)}
}

// allowing is the Issue with the values that would have been accepted. The values are copied, so a caller's slice
// is not aliased into the error.
func (i Issue) allowing(values ...string) Issue {
	i.Allowed = append([]string(nil), values...)
	return i
}

// allowingSet is allowing for a set, in a stable order.
func (i Issue) allowingSet(set map[string]bool) Issue {
	values := make([]string, 0, len(set))
	for v, ok := range set {
		if ok {
			values = append(values, v)
		}
	}
	sort.Strings(values)
	return i.allowing(values...)
}

// Messages are the words of the issues, in order.
func Messages(issues []Issue) []string {
	out := make([]string, len(issues))
	for i, is := range issues {
		out[i] = is.Message
	}
	return out
}

// Line is the issue as one line of a report to the model: where, what, and what would have been accepted.
func (i Issue) Line() string {
	line := i.Address + ": " + i.Message
	if len(i.Allowed) > 0 {
		line += " (accepted: " + strings.Join(i.Allowed, ", ") + ")"
	}
	return line
}

// Report is the issues as a list, one line each, for the automatic check that sends them back to the model. Unlike
// Error it names the place of each fault and the values that would have been accepted.
func (e *ValidationError) Report() string {
	lines := make([]string, len(e.Issues))
	for i, is := range e.Issues {
		lines[i] = "- " + is.Line()
	}
	return strings.Join(lines, "\n")
}

// CheckReport is the turn the automatic check sends back to the model when its change does not validate: the faults,
// each at its address and with what would have been accepted when it is a choice (a ValidationError), or the error's
// own words for any other. The web handler and the replay harness (eval_test.go) both use it, so a replay is the
// turn production sends.
func CheckReport(err error) string {
	body := err.Error()
	var ve *ValidationError
	if errors.As(err, &ve) {
		body = ve.Report()
	}
	return "Automatic check: the change in your last reply does not validate, so it cannot be reviewed yet:\n\n" +
		body + "\n\nCorrect it, or ask me what you need to know."
}

// loaderPlace reads where a loader message is. The loader (metadata.Validate, ValidateEdgeRules) states each fault as
// one sentence that opens with what it is about and its id: `field "fld_x": ...`, `machine "mch_m": transition
// "trn_t": ...`. Those openings are what the builder's tree is made of, so the address is read from them, not
// guessed: a fault whose opening is not one of the known parts stays at its Machine.
var loaderPlace = regexp.MustCompile(`^(?:machine "[^"]*": )?(field|permission|event|transition|view) "([^"]+)"`)

var loaderPartAt = map[string]func(machine, id string) string{
	"field": fieldAt, "permission": permissionAt, "event": eventAt, "transition": transitionAt, "view": viewAt,
}

// LoaderIssues are the faults the loader found in one Machine, each at its own address. A ValidationError from the
// loader is split into its sentences (one Issue each) instead of being wrapped whole into one; any other error is one
// Issue at the Machine. A sentence that does not already name the Machine is given its name, so it reads alone.
func LoaderIssues(machine string, err error) []Issue {
	var sentences []string
	var le *metadata.ValidationError
	if errors.As(err, &le) {
		sentences = le.Issues
	} else {
		sentences = []string{err.Error()}
	}
	out := make([]Issue, 0, len(sentences))
	for _, s := range sentences {
		address := machineAt(machine)
		if m := loaderPlace.FindStringSubmatch(s); m != nil {
			address = loaderPartAt[m[1]](machine, m[2])
		}
		if !strings.HasPrefix(s, "machine ") {
			s = fmt.Sprintf("machine %q: %s", machine, s)
		}
		out = append(out, Issue{Address: address, Message: s})
	}
	return out
}
