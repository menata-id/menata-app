package composition

import (
	"context"
	"fmt"
	"strings"

	"menata.app/internal/aiassist"
	"menata.app/internal/domain"
	"menata.app/internal/ir"
)

// This file is the two halves of "fill a form from a document": asking (the Form's `extract:` and the Machines' own Fields
// become the questions a reader is given, and its answers are held to the controls they are for) and filling (a stored
// proposal becomes the starting values, notes and flags of the controls a page draws). Neither names a Machine or a Field:
// everything is read from the declaration and from the Fields.

// WithReader says whether this host can read a document; a page offers its reader only where one would work.
func (l *Loader) WithReader(available bool) *Loader {
	l.reader = available
	return l
}

// extractInputs are the controls of a Machine's create form for the listed Fields, in the order listed, with their
// choices filled (a relation's records by label).
func extractInputs(ctx context.Context, l *Loader, m *domain.Machine, ids []string) ([]domain.FormInput, error) {
	all, _ := m.CreateFormInputs()
	if err := fillChoices(ctx, l, m, all); err != nil {
		return nil, err
	}
	byID := map[string]domain.FormInput{}
	for _, in := range all {
		byID[in.FieldID] = in
	}
	var out []domain.FormInput
	for _, id := range ids {
		in, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("composition: extract names field %q, which %s's create form does not ask for", id, m.ID)
		}
		out = append(out, in)
	}
	return out, nil
}

// maxChoicesAsked is how many related records' labels one question carries to the model.
const maxChoicesAsked = 150

// question is the reader's question for one control: its Field's name, its kind of answer and the choices it allows.
func question(in domain.FormInput) aiassist.ExtractField {
	q := aiassist.ExtractField{ID: in.FieldID, Label: in.Label, Type: "text"}
	switch {
	case in.Kind == domain.InputNumber:
		q.Type = "number"
	case in.Kind == domain.InputDate:
		q.Type = "date"
	case in.Reference != "":
		// The related records' labels are sent to the model as the allowed answers, so they are bounded: a relation with more
		// records than that is asked as words and matched to a record by its label afterwards (domain.Fit).
		if len(in.Choices) <= maxChoicesAsked {
			q.Type = "choice"
			for _, c := range in.Choices {
				q.Options = append(q.Options, c.Label)
			}
		}
	case in.Kind == domain.InputSelect:
		q.Type, q.Options = "choice", in.Options
	}
	return q
}

// ExtractProposal has a reader read a document for a Form's `extract:` and returns what it proposed, each answer held to its
// control. Nothing is written: the proposal is what a person is shown to check.
func ExtractProposal(ctx context.Context, l *Loader, reader aiassist.Extractor, x domain.FormExtract, source *domain.Machine, doc []byte, mime string) (domain.ExtractionProposal, error) {
	fields, err := extractInputs(ctx, l, source, x.Fields)
	if err != nil {
		return domain.ExtractionProposal{}, err
	}
	req := aiassist.ExtractRequest{Instruction: x.Instruction, Document: doc, MIME: mime}
	for _, in := range fields {
		req.Fields = append(req.Fields, question(in))
	}
	rowInputs := make([][]domain.FormInput, len(x.Rows))
	for i, r := range x.Rows {
		child := l.Machine(r.Machine)
		if child == nil {
			return domain.ExtractionProposal{}, fmt.Errorf("composition: extract rows name machine %q, which this Workspace does not install", r.Machine)
		}
		if rowInputs[i], err = extractInputs(ctx, l, child, r.Fields); err != nil {
			return domain.ExtractionProposal{}, err
		}
		q := aiassist.ExtractRowsQuestion{Key: fmt.Sprint(i), Label: r.Label}
		for _, in := range rowInputs[i] {
			q.Fields = append(q.Fields, question(in))
		}
		req.Rows = append(req.Rows, q)
	}
	got, err := reader.Extract(ctx, req)
	if err != nil {
		return domain.ExtractionProposal{}, err
	}
	hold := func(in domain.FormInput, p aiassist.Proposed) domain.ProposedValue {
		value, unfit := domain.Fit(in, p.Value)
		return domain.ProposedValue{Value: value, Source: strings.TrimSpace(p.Source), Review: unfit || (p.Inferred && value != "")}
	}
	out := domain.ExtractionProposal{Values: map[string]domain.ProposedValue{}, Rows: make([][]map[string]domain.ProposedValue, len(x.Rows))}
	for _, in := range fields {
		if p, ok := got.Values[in.FieldID]; ok {
			out.Values[in.FieldID] = hold(in, p)
		}
	}
	for i := range x.Rows {
		for _, row := range got.Rows[fmt.Sprint(i)] {
			kept := map[string]domain.ProposedValue{}
			for _, in := range rowInputs[i] {
				if p, ok := row[in.FieldID]; ok {
					kept[in.FieldID] = hold(in, p)
				}
			}
			out.Rows[i] = append(out.Rows[i], kept)
		}
	}
	return out, nil
}

// proposed gives a control the proposal's answer: the value it starts as, where it was read and, when a person should look
// twice, the words that say so.
func proposed(in domain.FormInput, p domain.ProposedValue, flag string) domain.FormInput {
	in.Default, in.Note = p.Value, p.Source
	if p.Review {
		in.Flag = flag
	}
	return in
}

// applyProposal fills a create form's controls from a proposal and returns the proposed rows as controls, each kind with its
// rows. A control the proposal does not mention is left as it was.
func applyProposal(ctx context.Context, l *Loader, x domain.FormExtract, inputs []domain.FormInput, p domain.ExtractionProposal) ([]ir.FormRows, error) {
	for i, in := range inputs {
		if v, ok := p.Values[in.FieldID]; ok {
			inputs[i] = proposed(in, v, x.Review)
		}
	}
	var rows []ir.FormRows
	for i, r := range x.Rows {
		if i >= len(p.Rows) || len(p.Rows[i]) == 0 {
			continue
		}
		child := l.Machine(r.Machine)
		if child == nil {
			return nil, fmt.Errorf("composition: extract rows name machine %q, which this Workspace does not install", r.Machine)
		}
		controls, err := extractInputs(ctx, l, child, r.Fields)
		if err != nil {
			return nil, err
		}
		kind := ir.FormRows{Label: r.Label}
		for _, row := range p.Rows[i] {
			cells := make([]domain.FormInput, len(controls))
			for j, in := range controls {
				cells[j] = in
				if v, ok := row[in.FieldID]; ok {
					cells[j] = proposed(in, v, x.Review)
				}
			}
			kind.Rows = append(kind.Rows, cells)
		}
		rows = append(rows, kind)
	}
	return rows, nil
}
