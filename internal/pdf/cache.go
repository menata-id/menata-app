package pdf

import (
	"container/list"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"sync"
)

// Cache keeps rendered pages so the same page of the same file is rasterised once (K19, 007 §18.8 / §28
// invariant 8: bounded interactive latency). Rendering is the slow part of every preview request -- median
// 170 ms and up to 984 ms against the 100 ms interactive budget -- and the same page is asked for again on
// every reload and every pager click.
//
// The key is the SHA-256 of the file's bytes plus page and size, never a record id or a storage path: a
// replaced upload is different bytes and so a different entry, and two records holding one file share one.
// There is nothing to invalidate. Bounded by bytes, evicting least-recently-used; an entry bigger than the
// whole budget is rendered but not kept.
type Cache struct {
	mu       sync.Mutex
	maxBytes int
	bytes    int
	order    *list.List // front = most recently used
	entries  map[key]*list.Element
}

type key struct {
	file       [sha256.Size]byte
	page, w, h int
}

type entry struct {
	key key
	png []byte
}

// NewCache returns a cache holding at most maxBytes of PNG data.
func NewCache(maxBytes int) *Cache {
	return &Cache{maxBytes: maxBytes, order: list.New(), entries: map[key]*list.Element{}}
}

// Key identifies one rendering of one file; ETag uses it so a browser revalidating costs no render at all.
func Key(data []byte, page, maxWidth, maxHeight int) string {
	h := sha256.New()
	h.Write(data)
	var b [24]byte
	binary.BigEndian.PutUint64(b[0:], uint64(page))
	binary.BigEndian.PutUint64(b[8:], uint64(maxWidth))
	binary.BigEndian.PutUint64(b[16:], uint64(maxHeight))
	h.Write(b[:])
	return hex.EncodeToString(h.Sum(nil))
}

// RenderPagePNG is the package-level RenderPagePNG through the cache; hit reports whether this call was
// answered from memory.
func (c *Cache) RenderPagePNG(data []byte, page, maxWidth, maxHeight int) (png []byte, hit bool, err error) {
	k := key{file: sha256.Sum256(data), page: page, w: maxWidth, h: maxHeight}

	c.mu.Lock()
	if el, ok := c.entries[k]; ok {
		c.order.MoveToFront(el)
		png := el.Value.(*entry).png
		c.mu.Unlock()
		return png, true, nil
	}
	c.mu.Unlock()

	// Rendered outside the lock: two concurrent first requests may both render, which costs one redundant
	// render and nothing else, where holding the lock would serialise every preview behind the slowest.
	png, err = RenderPagePNG(data, page, maxWidth, maxHeight)
	if err != nil {
		return nil, false, err // failures are not cached: a corrupt page should be retried, not remembered
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.entries[k]; ok || len(png) > c.maxBytes {
		return png, false, nil
	}
	c.entries[k] = c.order.PushFront(&entry{key: k, png: png})
	c.bytes += len(png)
	for c.bytes > c.maxBytes {
		oldest := c.order.Back()
		e := oldest.Value.(*entry)
		c.order.Remove(oldest)
		delete(c.entries, e.key)
		c.bytes -= len(e.png)
	}
	return png, false, nil
}
