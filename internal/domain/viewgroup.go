package domain

import (
	"strconv"
	"strings"
)

// ViewGroup is a set of arrangements of one Machine's records that share one header (2026-10-19, Case 19 mockups PM01/PM05/PM08):
// a Board, a Table and a Calendar of the same tasks. The header says which records they are (a title, a one-line summary made
// of counts), lets the viewer move between the arrangements (a segmented switch of links) and, when the records belong to
// something larger, which one of those they are looking at (Switch project). A screen opts in with `view_group:` on its
// navigation item, so the three arrangements are three ordinary screens -- a Board is a View of the Machine, the others are
// `page:` blocks -- and none of them draws or names the header: it is this declaration, read once, drawn once.
//
// The record set a Scope picks is not filtered here. Each Dataset a page of the group reads says so itself, with an optional
// `$parameters.<scope parameter>` comparison (metadata.validateViewGroups holds every one of them to it), and the Board reads the
// same declaration -- so no screen can forget it and the choice is carried across all of them.
type ViewGroup struct {
	ID string
	// Machine is the Machine every arrangement shows: its name's records are what the summary counts.
	Machine string
	// Figures names the aggregate Dataset the summary is counted from and the two Measures it reads, so a count is the whole set
	// (a list cut by a Dataset's `limit:` would understate it) and is scoped by the same parameter the screens are.
	Figures ViewFigures
	// Words are the sentences the summary is made of; every number is substituted, never typed.
	Words ViewWords
	// Scope is optional: a group whose records are not one of several larger things has no scope, no pill and no filter.
	Scope *ViewScope
}

// ViewFigures is the Dataset and Measures a group's summary counts.
type ViewFigures struct {
	Dataset  string
	Total    string
	Complete string
}

// ViewWords are a group's summary sentences. Total is a PageCount (`{n}` is the number, One and Other the two wordings);
// Complete and Grouped carry `{n}` and `{field}` respectively.
type ViewWords struct {
	TotalOne, TotalOther string
	Complete             string
	Grouped              string
}

// SummaryCounts is the one value a navigation item's `summary:` takes.
const SummaryCounts = "counts"

// Tokens a ViewWords sentence carries.
const (
	ViewCountToken = PageCountToken
	ViewFieldToken = "{field}"
)

// ViewScope is what a group's records can be narrowed to: the records of Machine they point at through Field. The request names
// one by id under Parameter; absent, every record is shown (existing records that point at none stay visible).
type ViewScope struct {
	// Machine is the Machine whose records are offered (a Project), and Field the group Machine's relation Field to it. Dataset is
	// the records Dataset of that Machine the menu lists and the summary counts (its order is the menu's, its `limit:` the most a menu
	// offers): the records are read through a declaration, never the whole Machine.
	Machine string
	Field   string
	Dataset string
	// Parameter is the query parameter the choice travels in.
	Parameter string
	// Switch and All are the words of the control and of its "no choice" entry; Chosen and Overall are the summary's closing
	// phrase with a project picked ("one of {n} projects in this workspace") and without ("{n} projects in this workspace").
	Switch, All, Chosen, Overall string
	// More is the note under a menu cut at the Dataset's `limit:` ("Showing the first {n} projects"); empty draws none. The summary still
	// counts every record, and a record past the cut is a real choice, so a cut list is said to be one rather than mistaken for all of them.
	More string
}

// ViewGroupByID finds a group of an Application by id.
func (a Application) ViewGroupByID(id string) (ViewGroup, bool) {
	for _, g := range a.ViewGroups {
		if g.ID == id {
			return g, true
		}
	}
	return ViewGroup{}, false
}

// ViewGroupItems are the navigation items of the group, in declaration order -- the order the switch lists them.
func (a Application) ViewGroupItems(id string) []NavigationItem {
	var out []NavigationItem
	for _, n := range a.AllNavigation {
		if n.ViewGroup == id {
			out = append(out, n)
		}
	}
	return out
}

// FillCount substitutes a count into a words template: every `{n}`.
func FillCount(words string, n int) string {
	return strings.ReplaceAll(words, ViewCountToken, strconv.Itoa(n))
}
