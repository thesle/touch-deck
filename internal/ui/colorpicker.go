package ui

// ColorPicker is the reusable Config_View color control (design:
// "Components and Interfaces -> Widgets"). It matches the Svelte editor's color
// controls, which pair an <input type=color> with a hex text field.
//
// Rather than hand-roll an SV-square + hue-strip, this wraps the maintained
// gioui.org/x/colorpicker widget, which renders R/G/B/A sliders, a small hex
// text field, and a live color-sample swatch. The app only stores opaque RGB
// hex values (#rrggbb), so this wrapper forces the color opaque every frame
// (pinning A to 0xff) and translates between the library's slider color and the
// app's #rrggbb string form. Parsing/normalization go through the package-level
// ParseHexColor / NormalizeHex helpers so the app keeps talking #rrggbb.
//
// This widget is self-contained and holds no reference to AppState; the caller
// (task 15.1) reads Hex()/NormalizedHex() to persist bgColor / fontColor and
// calls SetHex when the selected slot changes.

import (
	"image/color"
	"strings"

	"gioui.org/layout"
	"gioui.org/widget/material"
	"gioui.org/x/colorpicker"
)

// ColorPicker is a reusable hex color widget backed by gioui.org/x/colorpicker
// (RGB sliders + hex field + live sample). Construct it with NewColorPicker (or
// a zero value + SetHex) and drive it each frame via Layout. The picker's color
// is always kept opaque; the app stores only RGB hex values.
type ColorPicker struct {
	// state holds the library colorpicker's slider/hex/editor state. It is the
	// single source of truth for the current color; the caller reads it through
	// Hex()/NormalizedHex() and updates it via SetHex.
	state colorpicker.State
}

// NewColorPicker returns a ColorPicker seeded with initialHex (for example
// "#1f2937"). The value is parsed via ParseHexColor; an unparseable seed leaves
// the picker at its zero (opaque black) color rather than failing.
func NewColorPicker(initialHex string) *ColorPicker {
	c := &ColorPicker{}
	c.SetHex(initialHex)
	return c
}

// SetHex sets the picker's color from hex. Use this when the selected slot
// changes so the picker reflects the newly loaded button's color. The color is
// forced opaque (A=0xff) so the A slider can never make tiles translucent. An
// invalid/unparseable hex is ignored, leaving the picker as-is (which, on a
// freshly constructed zero-value picker, means opaque black).
func (c *ColorPicker) SetHex(hex string) {
	col, ok := ParseHexColor(hex)
	if !ok {
		return
	}
	col.A = 0xff
	c.state.SetColor(col)
}

// Hex returns the current color as a canonical opaque "#rrggbb" string. This is
// the same format the app stored before: config.go writes it straight into
// Editor.BgColor/FontColor and the live preview reads it. Because the backing
// slider color is always a valid RGB triple, this is always a well-formed
// 6-digit hex value.
func (c *ColorPicker) Hex() string {
	col := color.NRGBA{R: c.state.Red(), G: c.state.Green(), B: c.state.Blue(), A: 0xff}
	return NormalizeHex(col)
}

// NormalizedHex returns the canonical "#rrggbb" form of the current color with
// ok=true. It parses the value produced by Hex(); since the slider color is
// always valid RGB this returns ok=true in practice, but the parse guards
// against any future change to Hex().
func (c *ColorPicker) NormalizedHex() (string, bool) {
	col, ok := ParseHexColor(c.Hex())
	if !ok {
		return "", false
	}
	return NormalizeHex(col), true
}

// Layout renders the library colorpicker (label, hex field, R/G/B/A sliders and
// a live color-sample swatch) and returns its dimensions. Before laying out it
// forces the current color opaque so the A slider cannot make tiles
// translucent; the library's Picker.Layout calls state.Update internally, so
// this pins alpha back to 0xff each frame after any slider input.
func (c *ColorPicker) Layout(gtx layout.Context, th *material.Theme, label string) layout.Dimensions {
	if col := c.state.Color(); col.A != 0xff {
		col.A = 0xff
		c.state.SetColor(col)
	}
	return colorpicker.Picker(th, &c.state, label).Layout(gtx)
}

// ParseHexColor parses a hex color string into a color.NRGBA (fully opaque).
//
// Accepted forms (case-insensitive, optional leading '#', surrounding
// whitespace tolerated):
//   - "#RRGGBB" / "RRGGBB"  (6 hex digits)
//   - "#RGB"    / "RGB"     (3 hex digits, each nibble doubled: "#f0a" -> ff00aa)
//
// On any other input (wrong length, non-hex characters, empty) it returns
// (zero NRGBA, false). The alpha channel is always set to 0xff on success; this
// app stores only RGB hex values.
func ParseHexColor(s string) (color.NRGBA, bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "#")

	switch len(s) {
	case 6:
		r, ok1 := parseHexByte(s[0], s[1])
		g, ok2 := parseHexByte(s[2], s[3])
		b, ok3 := parseHexByte(s[4], s[5])
		if !ok1 || !ok2 || !ok3 {
			return color.NRGBA{}, false
		}
		return color.NRGBA{R: r, G: g, B: b, A: 0xff}, true
	case 3:
		r, ok1 := parseHexByte(s[0], s[0])
		g, ok2 := parseHexByte(s[1], s[1])
		b, ok3 := parseHexByte(s[2], s[2])
		if !ok1 || !ok2 || !ok3 {
			return color.NRGBA{}, false
		}
		return color.NRGBA{R: r, G: g, B: b, A: 0xff}, true
	default:
		return color.NRGBA{}, false
	}
}

// NormalizeHex renders col as a canonical lower-case "#rrggbb" string (alpha is
// dropped, matching the RGB hex values this app stores).
func NormalizeHex(col color.NRGBA) string {
	const hexDigits = "0123456789abcdef"
	var b [7]byte
	b[0] = '#'
	b[1] = hexDigits[col.R>>4]
	b[2] = hexDigits[col.R&0x0f]
	b[3] = hexDigits[col.G>>4]
	b[4] = hexDigits[col.G&0x0f]
	b[5] = hexDigits[col.B>>4]
	b[6] = hexDigits[col.B&0x0f]
	return string(b[:])
}

// parseHexByte combines two hex-digit bytes (high nibble, low nibble) into a
// single byte value, reporting false if either character is not a hex digit.
func parseHexByte(hi, lo byte) (byte, bool) {
	h, ok := hexNibble(hi)
	if !ok {
		return 0, false
	}
	l, ok := hexNibble(lo)
	if !ok {
		return 0, false
	}
	return h<<4 | l, true
}

// hexNibble maps a single ASCII hex digit to its 0–15 value, reporting false for
// any non-hex character.
func hexNibble(ch byte) (byte, bool) {
	switch {
	case ch >= '0' && ch <= '9':
		return ch - '0', true
	case ch >= 'a' && ch <= 'f':
		return ch - 'a' + 10, true
	case ch >= 'A' && ch <= 'F':
		return ch - 'A' + 10, true
	default:
		return 0, false
	}
}
