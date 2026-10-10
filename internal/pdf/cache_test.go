package pdf

import "testing"

func TestCache_secondRequestIsServedFromMemory(t *testing.T) {
	c := NewCache(10 << 20)
	data := testPDF(t)
	first, hit, err := c.RenderPagePNG(data, 0, 600, 800)
	if err != nil || hit {
		t.Fatalf("first call: hit=%v err=%v, want a miss", hit, err)
	}
	second, hit, err := c.RenderPagePNG(data, 0, 600, 800)
	if err != nil || !hit {
		t.Fatalf("second call: hit=%v err=%v, want a hit", hit, err)
	}
	if string(first) != string(second) {
		t.Error("a cached page differs from the rendered one")
	}
	if _, hit, _ := c.RenderPagePNG(data, 0, 300, 400); hit {
		t.Error("a different size was answered from the 600x800 entry")
	}
}

func TestCache_differentBytesAreADifferentEntry(t *testing.T) {
	c := NewCache(10 << 20)
	data := testPDF(t)
	if _, _, err := c.RenderPagePNG(data, 0, 600, 800); err != nil {
		t.Fatal(err)
	}
	other := append(append([]byte(nil), data...), '\n') // trailing byte: same page, different file
	if _, hit, _ := c.RenderPagePNG(other, 0, 600, 800); hit {
		t.Error("a replaced file was served the old file's page")
	}
}

func TestCache_failuresAreNotRemembered(t *testing.T) {
	c := NewCache(10 << 20)
	data := testPDF(t)
	if _, _, err := c.RenderPagePNG(data, 5, 600, 800); err == nil {
		t.Fatal("rendering a missing page succeeded")
	}
	if len(c.entries) != 0 {
		t.Error("a failed render was cached")
	}
}

func TestCache_evictsLeastRecentlyUsedWithinItsByteBudget(t *testing.T) {
	data := testPDF(t)
	probe, _, err := NewCache(10<<20).RenderPagePNG(data, 0, 600, 800)
	if err != nil {
		t.Fatal(err)
	}
	c := NewCache(len(probe)*2 + len(probe)/2) // room for two entries of this size, not three
	sizes := [][2]int{{600, 800}, {601, 800}, {602, 800}}
	for _, s := range sizes {
		if _, _, err := c.RenderPagePNG(data, 0, s[0], s[1]); err != nil {
			t.Fatal(err)
		}
	}
	if c.bytes > c.maxBytes {
		t.Errorf("cache holds %d bytes over a %d budget", c.bytes, c.maxBytes)
	}
	if _, hit, _ := c.RenderPagePNG(data, 0, 602, 800); !hit {
		t.Error("the most recent entry was evicted")
	}
	if _, hit, _ := c.RenderPagePNG(data, 0, 600, 800); hit {
		t.Error("the least recently used entry survived eviction")
	}
}

func TestCache_anEntryLargerThanTheBudgetIsServedButNotKept(t *testing.T) {
	c := NewCache(10)
	if _, hit, err := c.RenderPagePNG(testPDF(t), 0, 600, 800); err != nil || hit {
		t.Fatalf("hit=%v err=%v", hit, err)
	}
	if len(c.entries) != 0 || c.bytes != 0 {
		t.Errorf("an oversized entry was kept: %d entries, %d bytes", len(c.entries), c.bytes)
	}
}
