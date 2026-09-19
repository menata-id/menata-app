// Signature compositing (ROADMAP.md Phase 17): burns each approved Approval Step's own signature
// image onto the Document's PDF, at that step's declared page/x/y/width. This is genuinely new
// *capability*, not new presentation -- Phase 15 deliberately excluded it for that reason. It
// belongs here, alongside CanDecide/DocumentStatus, as a second hardcoded function: fires on one
// triggering write, hardcoded to mch_document/mch_approval_step/mch_signature's own field ids, not
// a generic PDF-processing service (no second case needs one yet).
package action

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"strconv"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"menata.app/internal/data"
)

// Stamp is one signature image to burn onto one page of a Document's PDF, at a percentage
// position/size relative to that page -- Phase 15's own fld_signature_page/x/y/width convention.
// X/Y are the stamp's own center point, origin top-left, Y growing down (matching the placement
// screen's own drag math, ROADMAP.md Phase 15 Step 4); Width is a percentage of the page's own
// width. Height is derived from Image's own aspect ratio at composite time, not stored separately
// -- a redundant height Field isn't forced when the image's own dimensions already answer it.
type Stamp struct {
	Page  int
	X, Y  float64
	Width float64
	Image []byte
}

// StampFor builds one approved step's Stamp, or ok=false if the step has no placement yet or no
// signature image was found -- both are silently skippable, not an error: Phase 17 composites
// whatever is ready rather than blocking a real Approve on a placement someone forgot to make.
func StampFor(step *data.Record, signatureImage []byte) (Stamp, bool) {
	page, hasPage := step.Values[FieldStepSignaturePage].(float64)
	x, hasX := step.Values[FieldStepSignatureX].(float64)
	y, hasY := step.Values[FieldStepSignatureY].(float64)
	width, hasWidth := step.Values[FieldStepSignatureWidth].(float64)
	if !hasPage || !hasX || !hasY || !hasWidth || width <= 0 || len(signatureImage) == 0 {
		return Stamp{}, false
	}
	return Stamp{Page: int(page), X: x, Y: y, Width: width, Image: signatureImage}, true
}

// CompositeSignatures burns each Stamp onto docBytes in turn, returning the resulting PDF. Pure
// over bytes -- no I/O, same posture as CanDecide/DocumentStatus; callers fetch the Document's PDF
// and each signature image via internal/storage.
func CompositeSignatures(docBytes []byte, stamps []Stamp) ([]byte, error) {
	current := docBytes
	for _, s := range stamps {
		next, err := compositeOne(current, s)
		if err != nil {
			return nil, fmt.Errorf("composite signature on page %d: %w", s.Page, err)
		}
		current = next
	}
	return current, nil
}

// compositeOne places one Stamp using pdfcpu's anchor+offset+absolute-scale watermark scheme:
// position:bl anchors the image's own bottom-left corner at the page's bottom-left corner (0,0),
// offset then moves that corner to the stamp's actual bottom-left in points, and an absolute scale
// factor is the ratio of the target width to the image's native pixel width (pdfcpu treats one
// image pixel as one PDF point at scale 1.0). This exact formula was verified empirically --
// composite a known image at a known percentage, render the result back with internal/pdf, and
// measure the rendered stamp's bounding box -- not derived from documentation alone, since
// pdfcpu's own docs don't spell out the image-pixel-to-point convention. rotation:0 is required:
// pdfcpu's default watermark angle is diagonal, which a stamp must not have.
func compositeOne(docBytes []byte, s Stamp) ([]byte, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(s.Image))
	if err != nil {
		return nil, fmt.Errorf("decode signature image: %w", err)
	}
	dims, err := api.PageDims(bytes.NewReader(docBytes), nil)
	if err != nil {
		return nil, fmt.Errorf("read page dimensions: %w", err)
	}
	if s.Page < 1 || s.Page > len(dims) {
		return nil, fmt.Errorf("page %d out of range (document has %d pages)", s.Page, len(dims))
	}
	pageW := dims[s.Page-1].Width
	pageH := dims[s.Page-1].Height

	targetW := s.Width / 100 * pageW
	targetH := targetW * float64(cfg.Height) / float64(cfg.Width)
	centerX := s.X / 100 * pageW
	centerYFromBottom := pageH - s.Y/100*pageH
	bottomLeftX := centerX - targetW/2
	bottomLeftY := centerYFromBottom - targetH/2
	scaleAbs := targetW / float64(cfg.Width)

	desc := fmt.Sprintf("position:bl, offset:%.4f %.4f, scalefactor:%.6f abs, rotation:0",
		bottomLeftX, bottomLeftY, scaleAbs)
	wm, err := api.ImageWatermarkForReader(bytes.NewReader(s.Image), desc, true, false, types.POINTS)
	if err != nil {
		return nil, fmt.Errorf("build watermark: %w", err)
	}

	var out bytes.Buffer
	if err := api.AddWatermarks(bytes.NewReader(docBytes), &out, []string{strconv.Itoa(s.Page)}, wm, nil); err != nil {
		return nil, fmt.Errorf("apply watermark: %w", err)
	}
	return out.Bytes(), nil
}
