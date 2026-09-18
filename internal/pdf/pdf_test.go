package pdf

import (
	"bytes"
	"image/png"
	"os"
	"testing"
)

func testPDF(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/blank.pdf")
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}
	return data
}

func TestPageCount(t *testing.T) {
	n, err := PageCount(testPDF(t))
	if err != nil {
		t.Fatalf("PageCount: %v", err)
	}
	if n != 1 {
		t.Fatalf("PageCount = %d, want 1", n)
	}
}

func TestRenderPagePNG(t *testing.T) {
	out, err := RenderPagePNG(testPDF(t), 0, 600, 800)
	if err != nil {
		t.Fatalf("RenderPagePNG: %v", err)
	}

	img, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("decode rendered png: %v", err)
	}
	bounds := img.Bounds()
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
		t.Fatalf("rendered image has empty bounds: %v", bounds)
	}
	if bounds.Dx() > 600 || bounds.Dy() > 800 {
		t.Fatalf("rendered image %v exceeds requested max 600x800", bounds)
	}
}

func TestRenderPagePNG_InvalidPage(t *testing.T) {
	if _, err := RenderPagePNG(testPDF(t), 5, 600, 800); err == nil {
		t.Fatal("expected error for out-of-range page, got nil")
	}
}
