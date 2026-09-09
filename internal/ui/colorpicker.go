package ui

// ColorPicker is the reusable Config_View color control (design:
// "Components and Interfaces -> Widgets"). It matches the Svelte editor's color
// controls, which pair an <input type=color> with a hex text field. Gio has no
// native color-input widget, so this reimplements the pairing as:
//
//   [swatch preview][hex text editor]
//
// The swatch is filled live with the color parsed from whatever the user has
// typed, giving immediate feedback as the hex text changes (Requirements 10.3,
// 10.4). Parsing is tolerant (see ParseHexColor); the swatch falls back to a
// neutral gray when the current text is not a valid hex color, but Hex() still
// returns the raw editor text so the caller owns validation.
//
// This widget is self-contained and holds no reference to AppState; the caller
// (task 15.1) reads Hex()/NormalizedHex() to persist bgColor / fontColor and
// calls SetHex when the selected slot changes.

import (
	"image"
	"image/color"
	"strings"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// swatchSizeDp is the edge length (in dp) of the square color preview drawn to
// the left of the hex editor.
const swatchSizeDp = 24

// swatchRadiusDp is the corner radius (in dp) of the swatch, giving it the same
// gently rounded look as the Svelte tile preview.
const swatchRadiusDp = 4

// invalidSwatch is the neutral fill drawn when the current editor text is not a
// valid hex color, so the preview never disappears while the user is mid-typing.
var invalidSwatch = color.NRGBA{R: 0x88, G: 0x88, B: 0x88, A: 0xff}

// ColorPicker is a reusable hex color widget: an editable hex text field plus a
// live swatch preview. Construct it with NewColorPicker (or zero-value + SetHex)
// and drive it each frame via Layout.
type ColorPicker struct {
	// editor holds the hex text (e.g. "#1f2937"). It is single-line; the caller
	// reads its value through Hex()/NormalizedHex() and updates it via SetHex.
	editor widget.Editor
}

// NewColorPicker returns a ColorPicker whose hex editor is initialized to
// initialHex (for example "#1f2937"). The value is stored verbatim; it is not
// validated here so callers may seed it with whatever default they use.
func NewColorPicker(initialHex string) *ColorPicker {
	c := &ColorPicker{}
	c.editor.SingleLine = true
	c.editor.SetText(initialHex)
	return c
}

// SetHex replaces the editor text with hex. Use this when the selected slot
// changes so the picker reflects the newly loaded button's color.
func (c *ColorPicker) SetHex(hex string) {
	// Ensure SingleLine is set even when the ColorPicker was created as a
	// zero value rather than via NewColorPicker.
	c.editor.SingleLine = true
	c.editor.SetText(hex)
}

// Hex returns the current, raw editor text (trimmed of surrounding
// whitespace). It is intentionally not normalized or validated: the caller
// decides whether to accept it, and can call NormalizedHex for a canonical form.
func (c *ColorPicker) Hex() string {
	return strings.TrimSpace(c.editor.Text())
}

// NormalizedHex returns the canonical "#rrggbb" form of the current text when it
// is a valid hex color, along with ok=true. When the text is not a valid hex
// color it returns ("", false). This is the value a caller should persist to
// satisfy Requirements 10.3/10.4 ("produces a 6-digit hex color value").
func (c *ColorPicker) NormalizedHex() (string, bool) {
	col, ok := ParseHexColor(c.Hex())
	if !ok {
		return "", false
	}
	return NormalizeHex(col), true
}

// Layout draws the widget as [swatch preview][hex editor] on a single row.
// label is used as the editor's placeholder hint. The swatch is filled with the
// color parsed from the current editor text, or a neutral gray when the text is
// not a valid hex color.
func (c *ColorPicker) Layout(gtx layout.Context, th *material.Theme, label string) layout.Dimensions {
	col, ok := ParseHexColor(c.Hex())
	if !ok {
		col = invalidSwatch
	}

	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		// Swatch preview.
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return c.layoutSwatch(gtx, col)
		}),
		// Small gap between the swatch and the editor.
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		// Hex text field, taking the remaining horizontal space.
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			// c.editor.SingleLine is set in NewColorPicker/SetHex; the
			// EditorStyle does not expose it, so we rely on the widget field.
			c.editor.SingleLine = true
			return material.Editor(th, &c.editor, label).Layout(gtx)
		}),
	)
}

// layoutSwatch draws a fixed-size, slightly rounded square filled with col and
// reports its size to the layout.
func (c *ColorPicker) layoutSwatch(gtx layout.Context, col color.NRGBA) layout.Dimensions {
	size := gtx.Dp(unit.Dp(swatchSizeDp))
	radius := gtx.Dp(unit.Dp(swatchRadiusDp))
	rect := image.Rect(0, 0, size, size)

	defer clip.UniformRRect(rect, radius).Push(gtx.Ops).Pop()
	paint.Fill(gtx.Ops, col)

	return layout.Dimensions{Size: image.Pt(size, size)}
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
