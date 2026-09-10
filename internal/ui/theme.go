// This file implements the color-emoji theme wiring for tile labels
// (Enhancement 10, tasks 14.1 + 14.2) per gio-ui-polish/tasks.md.
//
// Goal: tile labels such as 📱 render in COLOR through the standard
// material.Label / widget.Label path — no custom glyph drawing.
//
// Why this works in Gio v0.10.2 (verified against the module source): the
// standard label paint loop (widget/label.go paintGlyph) draws the vector
// outline AND then calls shaper.Bitmaps(line), adding the returned op.CallOp.
// So material.Label already renders color BITMAP glyphs automatically, PROVIDED
// the theme's text.Shaper has a font face that supplies bitmap glyphs for those
// runes. We therefore only need to register a color-emoji FALLBACK face in the
// shaper — no custom glyph-draw path is required.
//
// The emoji font (Noto Color Emoji, a CBDT color-bitmap font) is embedded via
// go:embed from assets/NotoColorEmoji.ttf so the binary is portable (the user
// accepted the ~10MB binary-size increase).
package ui

import (
	_ "embed"
	"log"
	"sync"

	"gioui.org/font"
	"gioui.org/font/gofont"
	"gioui.org/font/opentype"
	"gioui.org/text"
	"gioui.org/widget/material"
)

//go:embed assets/NotoColorEmoji.ttf
var notoColorEmojiTTF []byte

// A single shared shaper is built once and reused by every theme that wants
// color emoji. Constructing a shaper parses its font faces (including the
// ~10MB emoji font), so we do that exactly once. The shaper is stateless with
// respect to the palette, so sharing it between the deck theme (r.th) and the
// Config_View theme (cfgTheme) is intentional and safe.
var (
	emojiShaperOnce sync.Once
	emojiShaper     *text.Shaper
)

// emojiFontCollection returns the gofont collection with Noto Color Emoji
// appended as a FALLBACK face. gofont faces come first so normal text keeps
// using Go fonts; the emoji face is only used for runes the Go fonts lack
// (e.g. 📱), where go-text's fallback selects it. Noto Color Emoji is a CBDT
// color-bitmap font, and widget.Label's paint loop calls shaper.Bitmaps(...),
// so those glyphs render in COLOR through the standard material.Label path —
// no custom glyph drawing required (Gio v0.10.2, Enhancement 10).
//
// If the embedded font fails to parse, we log the error and return the plain
// gofont collection unchanged: normal text still works, only color emoji is
// unavailable (graceful degradation).
func emojiFontCollection() []font.FontFace {
	coll := gofont.Collection()

	face, err := opentype.Parse(notoColorEmojiTTF)
	if err != nil {
		log.Printf("ui: failed to parse embedded Noto Color Emoji font, color emoji disabled: %v", err)
		return coll
	}

	coll = append(coll, font.FontFace{
		Font: font.Font{Typeface: "Noto Color Emoji"},
		Face: face,
	})
	return coll
}

// sharedEmojiShaper lazily builds and returns the process-wide shaper that
// includes the Noto Color Emoji fallback face. All themes share this one
// shaper (see the emojiShaper var block for why that is safe).
func sharedEmojiShaper() *text.Shaper {
	emojiShaperOnce.Do(func() {
		emojiShaper = text.NewShaper(text.WithCollection(emojiFontCollection()))
	})
	return emojiShaper
}

// newThemeWithEmoji builds a material.Theme whose Shaper includes the Noto
// Color Emoji fallback face so tile labels can render color emoji. Callers may
// override the returned theme's Palette (e.g. the Config_View dark palette)
// without affecting the shared shaper.
func newThemeWithEmoji() *material.Theme {
	th := material.NewTheme()
	th.Shaper = sharedEmojiShaper()
	return th
}
