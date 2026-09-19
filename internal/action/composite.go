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
	"image/color"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"math"
	"strconv"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
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

// Approval-status banner geometry constants (owner request, 2026-09-19): black Courier ("computer
// font") text on a light-purple translucent band, full page width, top-aligned, page 1 only,
// sized to fit exactly the wrapped status text -- no more, no less.
const (
	bannerFontSize   = 9
	bannerMargin     = 10.0 // points, all four sides
	bannerLineHeight = float64(bannerFontSize) * 1.4
)

var bannerBackgroundColor = color.RGBA{R: 230, G: 217, B: 250, A: 255}

// CompositeStatusBanner stamps text at the top of the Document PDF's page 1 only, full page
// width, sized to fit. A no-op (docBytes returned unchanged) when text is empty -- signDocument
// calls this with action.ApprovalStatusBanner's own output, which is empty until the first step
// is approved.
//
// Implemented as two separate watermark passes -- a background-color image band, then black text
// laid on top -- rather than pdfcpu's own single-pass text watermark + backgroundcolor, because
// that single-pass form shares one opacity between the background AND the text (there is no
// independent alpha for each): the owner asked for solid black text on a *translucent* purple
// band, which a single shared opacity cannot produce. The background image's pixel dimensions are
// chosen 1:1 with the target size in points (matching compositeOne's own established "one image
// pixel = one point at scale 1.0" convention), so no scale-factor math is needed to hit an exact
// width/height -- unlike a Stamp's image, whose native aspect ratio is fixed by the uploaded file.
func CompositeStatusBanner(docBytes []byte, text string) ([]byte, error) {
	if text == "" {
		return docBytes, nil
	}

	dims, err := api.PageDims(bytes.NewReader(docBytes), nil)
	if err != nil {
		return nil, fmt.Errorf("read page dimensions: %w", err)
	}
	if len(dims) == 0 {
		return nil, fmt.Errorf("document has no pages")
	}
	pageW := dims[0].Width
	pageH := dims[0].Height
	maxTextWidth := pageW - 2*bannerMargin
	if maxTextWidth <= 0 {
		return nil, fmt.Errorf("page too narrow for a status banner: %.1fpt wide", pageW)
	}

	lines, err := model.WordWrap(text, "Courier", bannerFontSize, maxTextWidth)
	if err != nil {
		return nil, fmt.Errorf("wrap status banner text: %w", err)
	}
	if len(lines) == 0 {
		return docBytes, nil
	}
	bannerHeight := float64(len(lines))*bannerLineHeight + 2*bannerMargin

	withBackground, err := compositeStatusBackground(docBytes, pageW, pageH, bannerHeight)
	if err != nil {
		return nil, fmt.Errorf("composite status banner background: %w", err)
	}
	withText, err := compositeStatusText(withBackground, text, pageH, bannerHeight, maxTextWidth)
	if err != nil {
		return nil, fmt.Errorf("composite status banner text: %w", err)
	}
	return withText, nil
}

// compositeStatusBackground stamps a light-purple translucent band spanning the full page width,
// top-aligned, bannerHeight tall, onto page 1 only.
func compositeStatusBackground(docBytes []byte, pageW, pageH, bannerHeight float64) ([]byte, error) {
	pxW := int(math.Round(pageW))
	pxH := int(math.Round(bannerHeight))
	if pxW < 1 || pxH < 1 {
		return nil, fmt.Errorf("degenerate banner size: %dx%d px", pxW, pxH)
	}
	img := image.NewRGBA(image.Rect(0, 0, pxW, pxH))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: bannerBackgroundColor}, image.Point{}, draw.Src)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("encode background band: %w", err)
	}

	bottomLeftY := pageH - bannerHeight
	desc := fmt.Sprintf("position:bl, offset:0 %.4f, scalefactor:1.0 abs, opacity:0.35, rotation:0", bottomLeftY)
	wm, err := api.ImageWatermarkForReader(bytes.NewReader(buf.Bytes()), desc, true, false, types.POINTS)
	if err != nil {
		return nil, fmt.Errorf("build background watermark: %w", err)
	}
	var out bytes.Buffer
	if err := api.AddWatermarks(bytes.NewReader(docBytes), &out, []string{"1"}, wm, nil); err != nil {
		return nil, fmt.Errorf("apply background watermark: %w", err)
	}
	return out.Bytes(), nil
}

// compositeStatusText lays black Courier text on top of the band compositeStatusBackground just
// drew, inset by bannerMargin on every side so glyphs never touch the band's own edge.
func compositeStatusText(docBytes []byte, text string, pageH, bannerHeight, maxTextWidth float64) ([]byte, error) {
	bottomLeftX := bannerMargin
	bottomLeftY := pageH - bannerHeight + bannerMargin
	desc := fmt.Sprintf(
		"fontname:Courier, points:%d, color:#000000, position:bl, offset:%.4f %.4f, aligntext:l, opacity:1, rotation:0, maxWidth:%.4f",
		bannerFontSize, bottomLeftX, bottomLeftY, maxTextWidth,
	)
	wm, err := api.TextWatermark(text, desc, true, false, types.POINTS)
	if err != nil {
		return nil, fmt.Errorf("build text watermark: %w", err)
	}
	var out bytes.Buffer
	if err := api.AddWatermarks(bytes.NewReader(docBytes), &out, []string{"1"}, wm, nil); err != nil {
		return nil, fmt.Errorf("apply text watermark: %w", err)
	}
	return out.Bytes(), nil
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
