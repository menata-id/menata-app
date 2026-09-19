package action

import (
	"bytes"
	gopng "image/png"
	"testing"
	"time"

	"menata.app/internal/pdf"
)

func TestApprovalStatusBanner_growsPerApprovedStep(t *testing.T) {
	when := time.Date(2026, time.September, 20, 0, 0, 0, 0, time.UTC)

	got := ApprovalStatusBanner(nil)
	if got != "" {
		t.Errorf("ApprovalStatusBanner(nil) = %q, want empty (nothing approved yet)", got)
	}

	got = ApprovalStatusBanner([]ApprovalStatusLine{
		{Sequence: 1, Total: 2, ApproverName: "Rina Nur", ApprovedAt: when},
	})
	want := "APPROVED: Step 1/2 - Rina Nur - 20 Sep 2026"
	if got != want {
		t.Errorf("ApprovalStatusBanner(1 line) = %q, want %q", got, want)
	}

	got = ApprovalStatusBanner([]ApprovalStatusLine{
		{Sequence: 1, Total: 2, ApproverName: "Rina Nur", ApprovedAt: when},
		{Sequence: 2, Total: 2, ApproverName: "Budi", ApprovedAt: when.AddDate(0, 0, 1)},
	})
	want = "APPROVED: Step 1/2 - Rina Nur - 20 Sep 2026 | APPROVED: Step 2/2 - Budi - 21 Sep 2026"
	if got != want {
		t.Errorf("ApprovalStatusBanner(2 lines) = %q, want %q", got, want)
	}
}

// TestCompositeStatusBanner_noopOnEmptyText covers the "nothing approved yet" case signDocument
// hits before any step has been decided -- the input PDF must come back byte-for-byte unchanged,
// not just visually unchanged, since this runs on every signDocument call regardless of whether
// there's anything new to stamp.
func TestCompositeStatusBanner_noopOnEmptyText(t *testing.T) {
	doc := testPDFBytes(t)
	out, err := CompositeStatusBanner(doc, "")
	if err != nil {
		t.Fatalf("CompositeStatusBanner(empty text) error = %v", err)
	}
	if !bytes.Equal(doc, out) {
		t.Error("CompositeStatusBanner(empty text) modified the document, want byte-for-byte unchanged")
	}
}

// TestCompositeStatusBanner_rendersFullWidthTopBand is the empirical regression test for the
// banner's own geometry, in the same spirit as composite_test.go's
// TestCompositeSignatures_placesStampAtExpectedPositionAndSize: render the composited PDF back to
// a PNG (internal/pdf.RenderPagePNG) and inspect real pixels rather than trusting the pdfcpu
// watermark description string alone.
//
// Checks, on the rendered page:
//   - top-left and top-right corners (inside the expected band) are tinted purple, not white --
//     "full width", not a narrow box hugging the text.
//   - a point well below the band's own computed height is still white -- the band doesn't cover
//     the whole page.
//   - at least one near-black pixel exists inside the band -- the text actually rendered, not
//     just the background.
func TestCompositeStatusBanner_rendersFullWidthTopBand(t *testing.T) {
	doc := testPDFBytes(t)
	text := ApprovalStatusBanner([]ApprovalStatusLine{
		{Sequence: 1, Total: 1, ApproverName: "Test Approver", ApprovedAt: time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)},
	})

	out, err := CompositeStatusBanner(doc, text)
	if err != nil {
		t.Fatalf("CompositeStatusBanner() error = %v", err)
	}

	const renderW, renderH = 900, 900
	rendered, err := pdf.RenderPagePNG(out, 0, renderW, renderH)
	if err != nil {
		t.Fatalf("render composited page: %v", err)
	}
	img, err := gopng.Decode(bytes.NewReader(rendered))
	if err != nil {
		t.Fatalf("decode rendered png: %v", err)
	}
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	isPurpleish := func(x, y int) bool {
		r, g, b, _ := img.At(x, y).RGBA()
		r8, g8, b8 := int(r>>8), int(g>>8), int(b>>8)
		// Not white/near-white, and red+blue both visibly above green -- bannerBackgroundColor
		// {230,217,250} blended at opacity 0.35 over white stays light, but g stays the lowest
		// channel throughout that blend range.
		return (r8 <= 245 || g8 <= 245 || b8 <= 245) && r8 > g8+3 && b8 > g8+3
	}
	isNearBlack := func(x, y int) bool {
		r, g, b, _ := img.At(x, y).RGBA()
		return r>>8 < 60 && g>>8 < 60 && b>>8 < 60
	}

	// Sample near the very top row, at both edges -- "full width" is the property under test.
	topY := 3
	if !isPurpleish(3, topY) {
		t.Errorf("pixel near top-left (3,%d) is not purple-tinted -- band missing at the left edge", topY)
	}
	if !isPurpleish(w-4, topY) {
		t.Errorf("pixel near top-right (%d,%d) is not purple-tinted -- band missing at the right edge (not full width)", w-4, topY)
	}

	// Well below the band: blank.pdf is 144pt tall: with a single wrapped line the band is
	// (1*12.6 + 20) ≈ 32.6pt, under a quarter of the page, so the vertical midpoint is a safe
	// "definitely below the band" sample point.
	midY := h / 2
	if isPurpleish(w/2, midY) {
		t.Errorf("pixel at vertical midpoint (%d,%d) is purple-tinted -- band covers more than the top row", w/2, midY)
	}

	foundText := false
	for y := 0; y < h/4 && !foundText; y++ {
		for x := 0; x < w && !foundText; x++ {
			if isNearBlack(x, y) {
				foundText = true
			}
		}
	}
	if !foundText {
		t.Error("no near-black pixels found in the band's own region -- status text did not render")
	}
}
