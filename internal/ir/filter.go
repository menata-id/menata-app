package ir

import (
	"fmt"
	"net/url"
	"slices"
	"strings"

	"menata.app/internal/domain"
)

// keepQuery is the request's parameters other than the ones named, as a query string in key order: what a filter's link
// or form carries so that choosing a chip does not drop the words in the search box, nor a search drop the chip. The
// parameters the runtime sets itself (`application`, a record page's `id`) are never carried -- they are the route's.
func keepQuery(r Resolver, except ...string) string {
	if r.Params == nil {
		return ""
	}
	v := url.Values{}
	for k, val := range r.Params() {
		if val == "" || slices.Contains(except, k) || k == "application" || k == "id" {
			continue
		}
		v.Set(k, val)
	}
	return v.Encode() // sorted by key
}

// oneField is a value made safe to sit in one tab-separated field of one line: a label or value comes from the data (a
// related record's title, a stored status), and a tab or a line break in it would otherwise start a field or a line of
// its own -- a chip with an address the data chose (the review of step 1 drew `javascript:` that way).
func oneField(s string) string {
	return lineBreaker.Replace(s)
}

var lineBreaker = strings.NewReplacer("\t", " ", "\n", " ", "\r", " ")

// filterHref is the address of one choice: the kept parameters, and the filter's own set to value ("" clears it).
func filterHref(keep, query, value string) string {
	v, _ := url.ParseQuery(keep)
	if value != "" {
		v.Set(query, value)
	}
	return "?" + v.Encode()
}

// filterRows is a filter's bound Dimension: its rows, and the total of the bound Measure over every record (the count an
// "All" choice says, which a sum of the rows would get wrong for a record with no value on the Dimension).
func filterRows(n domain.PageNode, r Resolver, path string) ([]Row, string, error) {
	b := n.Binding
	switch {
	case b == nil || b.Rows != domain.PageRowsDimension:
		return nil, "", fmt.Errorf("%s: a %s is bound with rows: %s -- its choices are a Dimension's values and their counts", path, n.Type, domain.PageRowsDimension)
	case len(n.Children) > 0 || len(n.From) > 0 || n.To != "" || n.Param != "":
		return nil, "", fmt.Errorf("%s: a %s holds no children and takes no from:, to: or param: -- every choice links to this page", path, n.Type)
	case r.Rows == nil || r.Total == nil:
		return nil, "", fmt.Errorf("%s: no resolver for dimension rows", path)
	}
	rows, err := r.Rows(*b)
	if err != nil {
		return nil, "", err
	}
	total, err := r.Total(domain.PageBinding{Dataset: b.Dataset, Measure: b.Measure, Rows: domain.PageRowsTotal})
	if err != nil {
		return nil, "", err
	}
	return rows, total, nil
}

// lowerFilterChips produces a FilterChips' `chips`: one line per chip -- label, count, address, and "1" for the one the
// request selected -- from the bound Dimension's rows, or from `options:` in their own order when the page names a subset
// (a value the Dimension holds no record of counts 0). `all:` puts the clearing chip first.
func lowerFilterChips(n domain.PageNode, r Resolver, path string) ([]UINode, error) {
	for _, p := range []string{"chips"} {
		if _, written := n.Props[p]; written {
			return nil, fmt.Errorf("%s: %s: is produced by lowering from the bound Dimension and is never written", path, p)
		}
	}
	rows, total, err := filterRows(n, r, path)
	if err != nil {
		return nil, err
	}
	query := n.Props["query"]
	selected := ""
	if r.Query != nil {
		selected = r.Query(query)
	}
	keep := keepQuery(r, query)
	counts := map[string]string{}
	type choice struct{ value, label string }
	var choices []choice
	for _, row := range rows {
		counts[row.linkValue()] = row.Value
		choices = append(choices, choice{row.linkValue(), row.Label})
	}
	if opts := n.Props["options"]; opts != "" {
		choices = nil
		for _, o := range domain.ParseSegmentOptions(opts) {
			choices = append(choices, choice{o.Value, o.Label})
		}
		if len(choices) == 0 {
			return nil, fmt.Errorf("%s: options: lists value=Label pairs, one per line", path)
		}
	}
	var lines []string
	line := func(label, count, href string, current bool) {
		flag := "0"
		if current {
			flag = "1"
		}
		lines = append(lines, strings.Join([]string{oneField(label), oneField(count), href, flag}, "\t"))
	}
	if all := n.Props["all"]; all != "" {
		line(all, total, filterHref(keep, query, ""), selected == "")
	}
	for _, c := range choices {
		count := counts[c.value]
		if count == "" {
			count = "0"
		}
		line(c.label, count, filterHref(keep, query, c.value), selected == c.value)
	}
	props := map[string]string{"label": n.Props["label"], "query": query, "chips": strings.Join(lines, "\n")}
	return []UINode{{Kind: NodeComponent, Type: n.Type, Props: props}}, nil
}

// lowerFilterSelect produces a FilterSelect's `choices` (one `value<TAB>label` line per Dimension row: a relation's
// records by their titles, carrying the id), the request's `selected` value and the `keep` parameters.
func lowerFilterSelect(n domain.PageNode, r Resolver, path string) ([]UINode, error) {
	for _, p := range []string{"choices", "selected", "keep"} {
		if _, written := n.Props[p]; written {
			return nil, fmt.Errorf("%s: %s: is produced by lowering from the bound Dimension and the request, and is never written", path, p)
		}
	}
	rows, _, err := filterRows(n, r, path)
	if err != nil {
		return nil, err
	}
	query := n.Props["query"]
	var lines []string
	for _, row := range rows {
		lines = append(lines, oneField(row.linkValue())+"\t"+oneField(row.Label))
	}
	props := map[string]string{"label": n.Props["label"], "query": query, "all": n.Props["all"], "choices": strings.Join(lines, "\n"), "keep": keepQuery(r, query)}
	if r.Query != nil {
		props["selected"] = r.Query(query)
	}
	return []UINode{{Kind: NodeComponent, Type: n.Type, Props: props}}, nil
}
