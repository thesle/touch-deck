// Package ui unit tests for the pure value logic of the two reusable widgets
// (task 13.3, Requirements 10.3, 10.4, 10.5). These exercise only the
// parsing/mapping functions — no Gio window is constructed and Layout is never
// called, so they run without a display server (design "Testing Strategy":
// unit tests cover the pure value API of the widgets).
package ui

import (
	"image/color"
	"testing"
)

// --- ColorPicker: ParseHexColor -------------------------------------------

func TestParseHexColor(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want color.NRGBA
		ok   bool
	}{
		{name: "valid 6-digit with hash", in: "#1f2937", want: color.NRGBA{R: 0x1f, G: 0x29, B: 0x37, A: 0xff}, ok: true},
		{name: "valid 6-digit without hash", in: "ffffff", want: color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}, ok: true},
		{name: "3-digit shorthand", in: "#f0a", want: color.NRGBA{R: 0xff, G: 0x00, B: 0xaa, A: 0xff}, ok: true},
		{name: "3-digit shorthand without hash", in: "f0a", want: color.NRGBA{R: 0xff, G: 0x00, B: 0xaa, A: 0xff}, ok: true},
		{name: "uppercase", in: "#ABCDEF", want: color.NRGBA{R: 0xab, G: 0xcd, B: 0xef, A: 0xff}, ok: true},
		{name: "surrounding whitespace", in: "  #123456  ", want: color.NRGBA{R: 0x12, G: 0x34, B: 0x56, A: 0xff}, ok: true},
		{name: "black", in: "#000000", want: color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0xff}, ok: true},

		{name: "empty", in: "", ok: false},
		{name: "too short 2 digits", in: "#12", ok: false},
		{name: "non-hex chars", in: "#gggggg", ok: false},
		{name: "5 chars", in: "12345", ok: false},
		{name: "7 chars", in: "#1234567", ok: false},
		{name: "4 chars", in: "#1234", ok: false},
		{name: "non-hex in 3-digit", in: "#zzz", ok: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ParseHexColor(tc.in)
			if ok != tc.ok {
				t.Fatalf("ParseHexColor(%q) ok = %v, want %v", tc.in, ok, tc.ok)
			}
			if !tc.ok {
				return // value undefined on failure; only ok matters
			}
			if got != tc.want {
				t.Errorf("ParseHexColor(%q) = %+v, want %+v", tc.in, got, tc.want)
			}
		})
	}
}

// --- ColorPicker: NormalizeHex --------------------------------------------

func TestNormalizeHex(t *testing.T) {
	tests := []struct {
		name string
		in   color.NRGBA
		want string
	}{
		{name: "typical color", in: color.NRGBA{R: 0x1f, G: 0x29, B: 0x37, A: 0xff}, want: "#1f2937"},
		{name: "white", in: color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}, want: "#ffffff"},
		{name: "black", in: color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0xff}, want: "#000000"},
		{name: "shorthand-derived", in: color.NRGBA{R: 0xff, G: 0x00, B: 0xaa, A: 0xff}, want: "#ff00aa"},
		{name: "alpha is dropped", in: color.NRGBA{R: 0xab, G: 0xcd, B: 0xef, A: 0x00}, want: "#abcdef"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeHex(tc.in); got != tc.want {
				t.Errorf("NormalizeHex(%+v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestNormalizeHexRoundTrip verifies that parsing then normalizing yields the
// canonical lowercase form regardless of the input casing / hash / shorthand.
func TestNormalizeHexRoundTrip(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "#1F2937", want: "#1f2937"},
		{in: "ABCDEF", want: "#abcdef"},
		{in: "#f0a", want: "#ff00aa"},
		{in: "  #123456  ", want: "#123456"},
	}

	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			col, ok := ParseHexColor(tc.in)
			if !ok {
				t.Fatalf("ParseHexColor(%q) unexpectedly failed", tc.in)
			}
			if got := NormalizeHex(col); got != tc.want {
				t.Errorf("NormalizeHex(ParseHexColor(%q)) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// --- FontSizeSelector: offsetForIndex / indexForOffset --------------------

func TestOffsetForIndex(t *testing.T) {
	tests := []struct {
		name string
		i    int
		want int
	}{
		{name: "index 0", i: 0, want: -2},
		{name: "index 1", i: 1, want: -1},
		{name: "index 2 (default)", i: 2, want: 0},
		{name: "index 3", i: 3, want: 1},
		{name: "index 4", i: 4, want: 2},
		{name: "negative index -> default", i: -1, want: 0},
		{name: "too-large index -> default", i: 5, want: 0},
		{name: "far out-of-range index -> default", i: 99, want: 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := offsetForIndex(tc.i); got != tc.want {
				t.Errorf("offsetForIndex(%d) = %d, want %d", tc.i, got, tc.want)
			}
		})
	}
}

func TestIndexForOffset(t *testing.T) {
	tests := []struct {
		name string
		off  int
		want int
	}{
		{name: "offset -2", off: -2, want: 0},
		{name: "offset -1", off: -1, want: 1},
		{name: "offset 0 (default)", off: 0, want: 2},
		{name: "offset +1", off: 1, want: 3},
		{name: "offset +2", off: 2, want: 4},
		{name: "out-of-range +5 -> default index", off: 5, want: 2},
		{name: "out-of-range -9 -> default index", off: -9, want: 2},
		{name: "out-of-range +3 -> default index", off: 3, want: 2},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := indexForOffset(tc.off); got != tc.want {
				t.Errorf("indexForOffset(%d) = %d, want %d", tc.off, got, tc.want)
			}
		})
	}
}

// TestIndexOffsetRoundTrip verifies the mapping is self-inverse for every valid
// index/offset pair.
func TestIndexOffsetRoundTrip(t *testing.T) {
	for i := 0; i < 5; i++ {
		off := offsetForIndex(i)
		if got := indexForOffset(off); got != i {
			t.Errorf("indexForOffset(offsetForIndex(%d)) = %d, want %d", i, got, i)
		}
	}
}

// --- FontSizeSelector: NewFontSizeSelector / Value / SetValue -------------

func TestNewFontSizeSelectorValue(t *testing.T) {
	tests := []struct {
		name    string
		initial int
		want    int
	}{
		{name: "valid -2", initial: -2, want: -2},
		{name: "valid -1", initial: -1, want: -1},
		{name: "valid 0", initial: 0, want: 0},
		{name: "valid +1", initial: 1, want: 1},
		{name: "valid +2", initial: 2, want: 2},
		{name: "out-of-range +3 -> default 0", initial: 3, want: 0},
		{name: "out-of-range -5 -> default 0", initial: -5, want: 0},
		{name: "far out-of-range 99 -> default 0", initial: 99, want: 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := NewFontSizeSelector(tc.initial)
			if got := f.Value(); got != tc.want {
				t.Errorf("NewFontSizeSelector(%d).Value() = %d, want %d", tc.initial, got, tc.want)
			}
		})
	}
}

func TestFontSizeSelectorSetValue(t *testing.T) {
	f := NewFontSizeSelector(3) // out-of-range initial normalizes to Default (0)
	if got := f.Value(); got != 0 {
		t.Fatalf("NewFontSizeSelector(3).Value() = %d, want 0", got)
	}

	tests := []struct {
		name string
		set  int
		want int
	}{
		{name: "set +2", set: 2, want: 2},
		{name: "set -2", set: -2, want: -2},
		{name: "set 0", set: 0, want: 0},
		{name: "set out-of-range 99 -> default 0", set: 99, want: 0},
		{name: "set out-of-range -3 -> default 0", set: -3, want: 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f.SetValue(tc.set)
			if got := f.Value(); got != tc.want {
				t.Errorf("after SetValue(%d), Value() = %d, want %d", tc.set, got, tc.want)
			}
		})
	}
}
