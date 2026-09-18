// Package pdf rasterizes a PDF page to a PNG image (ROADMAP.md Phase 15 Step 3), the preview
// step the signature-coordinate placement screen (Phase 15 Step 4) needs a real page image to
// place markers against. Thin wrapper over github.com/richardwilkes/pdfview -- the pure-Go
// PDF rasterizer chosen to match 007-composable-runtime-architecture.md §4.10's single-binary,
// no-native-dependency constraint (no cgo/MuPDF, no external pdftoppm/poppler process).
package pdf

import (
	"bytes"
	"fmt"
	"image/png"

	"github.com/richardwilkes/pdfview"
)

// PageCount returns the number of pages in the PDF held by data.
func PageCount(data []byte) (int, error) {
	doc, err := pdfview.New(data, 0)
	if err != nil {
		return 0, fmt.Errorf("open pdf: %w", err)
	}
	defer doc.Release()
	return doc.PageCount(), nil
}

// RenderPagePNG rasterizes the 0-indexed page of the PDF held by data, scaled to fit within
// maxWidth x maxHeight, and returns it PNG-encoded.
func RenderPagePNG(data []byte, page, maxWidth, maxHeight int) ([]byte, error) {
	doc, err := pdfview.New(data, 0)
	if err != nil {
		return nil, fmt.Errorf("open pdf: %w", err)
	}
	defer doc.Release()

	rendered, err := doc.RenderPageForSize(page, maxWidth, maxHeight, 0, "")
	if err != nil {
		return nil, fmt.Errorf("render page %d: %w", page, err)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, rendered.Image); err != nil {
		return nil, fmt.Errorf("encode png: %w", err)
	}
	return buf.Bytes(), nil
}
