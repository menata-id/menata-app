package action

import (
	"bytes"
	"image"
	"image/color"
	gopng "image/png"
	"os"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"

	"menata.app/internal/data"
	"menata.app/internal/pdf"
)

func testPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.NRGBA{R: 200, G: 30, B: 30, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := gopng.Encode(&buf, img); err != nil {
		t.Fatalf("encode test png: %v", err)
	}
	return buf.Bytes()
}

func testPDFBytes(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/blank.pdf")
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}
	return data
}

func TestStampFor_completePlacement(t *testing.T) {
	s := &data.Record{Values: map[string]any{
		FieldStepSignaturePage:  float64(2),
		FieldStepSignatureX:     float64(30),
		FieldStepSignatureY:     float64(70),
		FieldStepSignatureWidth: float64(25),
	}}
	stamp, ok := StampFor(s, []byte("fake-image-bytes"))
	if !ok {
		t.Fatal("StampFor() ok = false, want true")
	}
	if stamp.Page != 2 || stamp.X != 30 || stamp.Y != 70 || stamp.Width != 25 {
		t.Errorf("StampFor() = %+v, unexpected fields", stamp)
	}
}

func TestStampFor_missingPlacement(t *testing.T) {
	s := &data.Record{Values: map[string]any{}}
	if _, ok := StampFor(s, []byte("img")); ok {
		t.Error("StampFor() ok = true, want false: no placement fields set")
	}
}

func TestStampFor_missingImage(t *testing.T) {
	s := &data.Record{Values: map[string]any{
		FieldStepSignaturePage:  float64(1),
		FieldStepSignatureX:     float64(50),
		FieldStepSignatureY:     float64(50),
		FieldStepSignatureWidth: float64(20),
	}}
	if _, ok := StampFor(s, nil); ok {
		t.Error("StampFor() ok = true, want false: no signature image")
	}
}

func TestStampFor_zeroWidthSkipped(t *testing.T) {
	s := &data.Record{Values: map[string]any{
		FieldStepSignaturePage:  float64(1),
		FieldStepSignatureX:     float64(50),
		FieldStepSignatureY:     float64(50),
		FieldStepSignatureWidth: float64(0),
	}}
	if _, ok := StampFor(s, []byte("img")); ok {
		t.Error("StampFor() ok = true, want false: zero width is not a real placement")
	}
}

// TestCompositeSignatures_placesStampAtExpectedPositionAndSize is a regression test for the exact
// pdfcpu parameter formula in compositeOne, verified empirically (composite a known image at a
// known percentage, render the result back with internal/pdf, and measure the rendered stamp's
// bounding box) rather than derived from documentation alone -- see compositeOne's own doc
// comment for why. A 2:1 aspect ratio image at 50% page width should render at 50% width and a
// proportional height, centered on the requested x/y.
func TestCompositeSignatures_placesStampAtExpectedPositionAndSize(t *testing.T) {
	doc := testPDFBytes(t)
	img := testPNG(t, 400, 200) // 2:1 aspect ratio

	out, err := CompositeSignatures(doc, []Stamp{{Page: 1, X: 50, Y: 50, Width: 50, Image: img}})
	if err != nil {
		t.Fatalf("CompositeSignatures() error = %v", err)
	}
	if len(out) == 0 {
		t.Fatal("CompositeSignatures() returned empty output")
	}

	rendered, err := pdf.RenderPagePNG(out, 0, 900, 900)
	if err != nil {
		t.Fatalf("render composited page: %v", err)
	}
	minX, minY, maxX, maxY, w, h := redBoundingBox(t, rendered)

	widthPct := float64(maxX-minX) / float64(w) * 100
	heightPct := float64(maxY-minY) / float64(h) * 100
	centerXPct := float64(minX+maxX) / 2 / float64(w) * 100
	centerYPct := float64(minY+maxY) / 2 / float64(h) * 100

	dims, err := api.PageDims(bytes.NewReader(doc), nil)
	if err != nil {
		t.Fatalf("read page dimensions: %v", err)
	}
	// The rendered PNG preserves the page's own aspect ratio, so "percent of rendered image"
	// equals "percent of page" for both dimensions.
	wantWidthPtPct := 50.0
	targetWidthPt := wantWidthPtPct / 100 * dims[0].Width
	targetHeightPt := targetWidthPt * (200.0 / 400.0) // image's own 2:1 aspect ratio
	wantHeightPct := targetHeightPt / dims[0].Height * 100

	const tol = 3.0 // percentage points
	if diff := widthPct - wantWidthPtPct; diff < -tol || diff > tol {
		t.Errorf("stamp width = %.1f%% of page, want ~%.1f%%", widthPct, wantWidthPtPct)
	}
	if diff := heightPct - wantHeightPct; diff < -tol || diff > tol {
		t.Errorf("stamp height = %.1f%% of page, want ~%.1f%%", heightPct, wantHeightPct)
	}
	if diff := centerXPct - 50; diff < -tol || diff > tol {
		t.Errorf("stamp center X = %.1f%%, want ~50%%", centerXPct)
	}
	if diff := centerYPct - 50; diff < -tol || diff > tol {
		t.Errorf("stamp center Y = %.1f%%, want ~50%%", centerYPct)
	}
}

func TestCompositeSignatures_pageOutOfRange(t *testing.T) {
	doc := testPDFBytes(t)
	img := testPNG(t, 100, 100)
	if _, err := CompositeSignatures(doc, []Stamp{{Page: 5, X: 50, Y: 50, Width: 20, Image: img}}); err == nil {
		t.Fatal("CompositeSignatures() error = nil, want error for out-of-range page")
	}
}

// redBoundingBox finds the pixel bounding box of the test stamp's own solid red fill within a
// rendered page, plus the rendered image's own dimensions.
func redBoundingBox(t *testing.T, pngBytes []byte) (minX, minY, maxX, maxY, w, h int) {
	t.Helper()
	img, err := gopng.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		t.Fatalf("decode rendered png: %v", err)
	}
	bounds := img.Bounds()
	w, h = bounds.Dx(), bounds.Dy()
	minX, minY = w, h
	maxX, maxY = 0, 0
	found := false
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			r8, g8, b8 := r>>8, g>>8, b>>8
			if r8 > 180 && g8 < 80 && b8 < 80 {
				found = true
				if x < minX {
					minX = x
				}
				if x > maxX {
					maxX = x
				}
				if y < minY {
					minY = y
				}
				if y > maxY {
					maxY = y
				}
			}
		}
	}
	if !found {
		t.Fatal("no red pixels found in rendered output -- stamp did not render")
	}
	return minX, minY, maxX, maxY, w, h
}
