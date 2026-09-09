// This file implements the reusable FontSizeSelector widget (task 13.2) per the
// design's "Components and Interfaces -> Widgets" section: a segmented control
// offering the font-size offsets -2 / -1 / Default (0) / +1 / +2, matching the
// Svelte editor's Font Size Modifier control (Requirement 10.5). It is a
// self-contained Gio component with a small, pure value API so the index<->offset
// mapping can be unit-tested in isolation (task 13.3).
package ui

import (
	"image/color"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// Unselected-segment colors are defined here so the widget stays readable under
// ANY theme. Previously the unselected text used th.Palette.Fg, which becomes a
// light color under the Config view's dark theme and rendered light-on-light
// (unreadable) against the light muted fill. Using a self-contained dark fill
// with light text keeps unselected segments clearly readable regardless of the
// theme's Fg.
var (
	// fsUnselectedBg is a dark slate fill for unselected segments (#374151).
	fsUnselectedBg = color.NRGBA{R: 0x37, G: 0x41, B: 0x51, A: 0xff}
	// fsUnselectedFg is a light text color for unselected segments (#e5e7eb).
	fsUnselectedFg = color.NRGBA{R: 0xe5, G: 0xe7, B: 0xeb, A: 0xff}
)

// fontSizeOffsets is the ordered list of selectable offsets, one per segment.
// Index -> offset mapping: [0]->-2, [1]->-1, [2]->0, [3]->+1, [4]->+2.
var fontSizeOffsets = [5]int{-2, -1, 0, 1, 2}

// fontSizeLabels are the human-readable segment labels, kept short but clear.
var fontSizeLabels = [5]string{"-2", "-1", "Default", "+1", "+2"}

// offsetForIndex returns the font-size offset for a segment index. Indices are
// expected to be 0..4; out-of-range indices map to the Default offset (0).
func offsetForIndex(i int) int {
	if i < 0 || i >= len(fontSizeOffsets) {
		return 0
	}
	return fontSizeOffsets[i]
}

// indexForOffset returns the segment index for a font-size offset. Offsets
// outside the set {-2,-1,0,+1,+2} map to the Default segment (index 2, offset 0).
func indexForOffset(off int) int {
	for i, v := range fontSizeOffsets {
		if v == off {
			return i
		}
	}
	return 2 // index of offset 0
}

// FontSizeSelector is a reusable segmented control bound to an integer font-size
// offset in the set {-2, -1, 0, +1, +2}. The currently selected offset's segment
// is highlighted; the others render muted. It is self-contained: construct it
// with NewFontSizeSelector, read/write the offset with Value/SetValue, and draw
// it each frame with Layout (which also processes segment clicks).
type FontSizeSelector struct {
	buttons [5]widget.Clickable
	value   int // current offset, always one of {-2,-1,0,+1,+2}
}

// NewFontSizeSelector builds a selector initialized to the given offset. The
// initial value is normalized to the nearest valid slot: any value outside
// {-2,-1,0,+1,+2} falls back to the Default offset (0).
func NewFontSizeSelector(initial int) *FontSizeSelector {
	f := &FontSizeSelector{}
	f.SetValue(initial)
	return f
}

// Value returns the currently selected font-size offset (one of -2..+2).
func (f *FontSizeSelector) Value() int {
	return f.value
}

// SetValue sets the current font-size offset. Values outside the valid set
// {-2,-1,0,+1,+2} are clamped to the Default offset (0). This keeps the widget's
// value in sync when the selected slot changes.
func (f *FontSizeSelector) SetValue(v int) {
	// Round-tripping through the index helper normalizes out-of-range input to
	// the Default offset while keeping the index<->offset mapping the single
	// source of truth.
	f.value = offsetForIndex(indexForOffset(v))
}

// Layout draws the five segments in a horizontal row and processes clicks: if a
// segment was clicked, the selector's value becomes that segment's offset. The
// active segment uses the theme's ContrastBg/ContrastFg (primary) colors; the
// others use a self-contained dark fill with light text (fsUnselectedBg/
// fsUnselectedFg) so the selection is obvious and unselected labels stay
// readable under any theme.
func (f *FontSizeSelector) Layout(gtx layout.Context, th *material.Theme) layout.Dimensions {
	// Process clicks first so the highlight reflects the latest selection in the
	// same frame.
	for i := range f.buttons {
		for f.buttons[i].Clicked(gtx) {
			f.value = offsetForIndex(i)
		}
	}

	selected := indexForOffset(f.value)

	children := make([]layout.FlexChild, 0, len(f.buttons)*2-1)
	for i := range f.buttons {
		i := i
		if i > 0 {
			// Small spacer between segments.
			children = append(children, layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout))
		}
		children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			btn := material.Button(th, &f.buttons[i], fontSizeLabels[i])
			if i == selected {
				btn.Background = th.Palette.ContrastBg
				btn.Color = th.Palette.ContrastFg
			} else {
				// Self-contained dark-with-light-text styling so unselected
				// segments stay readable regardless of the theme's Fg.
				btn.Background = fsUnselectedBg
				btn.Color = fsUnselectedFg
			}
			return btn.Layout(gtx)
		}))
	}

	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
}
