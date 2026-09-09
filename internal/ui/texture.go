package ui

import (
	"bytes"
	"image"
	"os"
	"sync"

	// Blank imports register the standard-library decoders so image.Decode can
	// handle .png/.jpg/.jpeg/.gif. The Go standard library does NOT decode
	// .webp or .svg; those simply fail to decode and are recorded as "bad" so
	// the Deck_View falls back to color + label (see design Error Handling).
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"gioui.org/op/paint"
)

// TextureCache decodes and caches image files as Gio paint.ImageOps, keyed by
// ABSOLUTE image path. It replaces the old Svelte base64Cache (which was keyed
// by button id): keying by path means identical images shared across multiple
// buttons decode only once.
//
// Concurrency: all access is guarded by mu. For simplicity we decode the image
// while holding the lock — this app decodes a handful of small deck icons, so
// serializing the occasional cold-miss decode is not a bottleneck, and it keeps
// the cache logic trivially correct (no duplicate in-flight decodes, no partial
// state). If decode cost ever mattered, Get could be reworked to release the
// lock across file I/O with a double-checked insert.
type TextureCache struct {
	mu    sync.Mutex
	items map[string]paint.ImageOp // abs path -> decoded op
	bad   map[string]bool          // paths that failed to decode (avoid retry storms)
}

// NewTextureCache returns an empty, ready-to-use TextureCache.
func NewTextureCache() *TextureCache {
	return &TextureCache{
		items: make(map[string]paint.ImageOp),
		bad:   make(map[string]bool),
	}
}

// Get returns the decoded paint.ImageOp for the image at path.
//
// Behavior:
//   - empty path            -> (zero ImageOp, false); not cached.
//   - cache hit in items    -> (op, true).
//   - previously failed path -> (zero ImageOp, false); not retried.
//   - cache miss            -> read + decode the file. On success the op is
//     stored and returned with true. On any read/decode error (including
//     undecodable formats like .webp/.svg or corrupt files) the path is
//     recorded in bad and (zero ImageOp, false) is returned so the caller can
//     render the color + label fallback.
func (c *TextureCache) Get(path string) (paint.ImageOp, bool) {
	if path == "" {
		return paint.ImageOp{}, false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if op, ok := c.items[path]; ok {
		return op, true
	}
	if c.bad[path] {
		return paint.ImageOp{}, false
	}

	data, err := os.ReadFile(path)
	if err != nil {
		c.bad[path] = true
		return paint.ImageOp{}, false
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		c.bad[path] = true
		return paint.ImageOp{}, false
	}

	op := paint.NewImageOp(img)
	c.items[path] = op
	return op, true
}

// Invalidate drops any cached decode (success or failure) for path, so a
// subsequent Get re-reads and re-decodes it. Used when an image at a known path
// is replaced.
func (c *TextureCache) Invalidate(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, path)
	delete(c.bad, path)
}
