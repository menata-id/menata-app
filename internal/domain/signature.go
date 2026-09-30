package domain

// SignaturePlacement and SignatureStore are the two halves of "which Fields hold a signature", and
// they exist because nothing else in metadata answers that question.
//
// Stages A-C of the approval engine removed literals by *deriving* them from declarations that were
// already there: which Machine plays which role from `workflow:`, what an Action writes from
// `actions:`, which records get composited from the Service's own trigger config. The signature shape
// had no such declaration anywhere -- `fld_signature_x` was a string in internal/action and nothing
// more -- so an Application could cast any Machine in the `step` role, under any names, and the
// approval would still fail to composite unless its Fields happened to be called what this repo's own
// template library calls them. That is a capability gap rather than a reading one (audit
// 2026-09-28, §5), and these two blocks are the capability.
//
// Machine-level, beside `fields:`, following Sequencing's own precedent: a block naming which Fields
// play which role in one capability. Flat rather than nested, for the same reason Sequencing is.
//
// **Two blocks rather than one** because they answer different questions for different Machines --
// where a box sits on *this* step, versus where a person's reusable image is kept. A single block
// whose valid keys depend on which role its Machine was cast in is the shape that gets mis-declared.
//
// **Nil means the Machine declares none**, and there is no fallback to internal/action's constants.
// Same contract as action.EngineFields and for the same reason: a silent fallback makes a Machine
// that declares less than the template library look like it worked, while matching the wrong records.

// SignaturePlacement names the Fields on an Approval Step holding its signature image and where that
// image sits on the document's page. Declared as `signature_placement:` on the Machine an Application
// casts in the engine's `step` role.
type SignaturePlacement struct {
	// ImageField is the file Field holding the one-time signature image captured when this step was
	// decided -- used only when its approver chose not to save a reusable one (SignatureStore below).
	ImageField string
	// PageField is the 1-based page number the box sits on. A record holding no page has no
	// placement at all, which is the rule every reader of this block already used.
	PageField string
	// XField and YField are the box's own centre, as a percentage of the rendered page, origin
	// top-left with Y growing down -- matching the drag math the placement screen computes.
	XField string
	YField string
	// WidthField is the box's width as a percentage of the page's own width. Height is derived from
	// the image's aspect ratio at composite time rather than stored, so no height Field exists.
	WidthField string
}

// Fields returns every Field id this block names, in declaration order, skipping the ones left
// undeclared. It is what a caller wanting "the signature Fields of this Machine" asks instead of
// listing them -- the placement route's write set and the detail page's hidden set are both exactly
// this list.
func (p *SignaturePlacement) Fields() []string {
	if p == nil {
		return nil
	}
	var out []string
	for _, id := range []string{p.ImageField, p.PageField, p.XField, p.YField, p.WidthField} {
		if id != "" {
			out = append(out, id)
		}
	}
	return out
}

// PlacementFields returns just the four positioning Fields, without the image. That is the set the
// placement screen writes: moving a box must never touch the image a decision already captured,
// which is the bug the whole-record update route once had (development-history.md Fase 6c-3).
func (p *SignaturePlacement) PlacementFields() []string {
	if p == nil {
		return nil
	}
	var out []string
	for _, id := range []string{p.PageField, p.XField, p.YField, p.WidthField} {
		if id != "" {
			out = append(out, id)
		}
	}
	return out
}

// SignatureStore names the Fields on a Machine holding people's own reusable signature images --
// declared as `signature_store:` on the Machine an Application casts in the optional `signature`
// role. An Application casting none is normal: the one-time image on the step is then the whole
// feature.
type SignatureStore struct {
	// OwnerField is the person Field naming whose signature this is; a lookup by it is how the
	// compositing finds an approver's saved image.
	OwnerField string
	// ImageField is the file Field holding the image itself.
	ImageField string
}
