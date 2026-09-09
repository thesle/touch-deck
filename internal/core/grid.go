package core

import "strconv"

// Pure grid-math and content-resolution helpers for TouchDeck (task 4.1).
//
// Everything in this file is a free function operating on Config / PageConfig /
// ButtonConfig values (or plain ints). There are NO Gio types, no I/O, and no
// *Backend receiver, so these helpers can be unit- and property-tested without a
// display server (see the design's Testing Strategy). Font sizes are returned as
// plain float32 point/sp values; the UI layer is responsible for wrapping them in
// unit.Sp so this package stays Gio-free.
//
// This file owns ONLY the grid-math / content-resolution / indicator helpers.
// Pagination wrap, pruneToGrid, moveButton, and writeSlot belong to task 4.2 in a
// separate file.

// --- Grid index <-> row/col ---

// SlotToRowCol converts a flat slot index into its (row, col) coordinates for a
// grid with the given number of columns. row = index/cols, col = index%cols.
//
// cols is expected to be >= 1 (per the min-grid constraint enforced by the
// Config_View). To avoid a divide-by-zero panic on a malformed grid, cols <= 0
// returns (0, 0).
func SlotToRowCol(index, cols int) (row, col int) {
	if cols <= 0 {
		return 0, 0
	}
	return index / cols, index % cols
}

// RowColToSlot converts (row, col) coordinates back into a flat slot index for a
// grid with the given number of columns: row*cols + col.
//
// cols is expected to be >= 1. For symmetry with SlotToRowCol, cols <= 0 returns
// 0 rather than producing a nonsensical index.
func RowColToSlot(row, col, cols int) int {
	if cols <= 0 {
		return 0
	}
	return row*cols + col
}

// --- Pagination slot identification ---
//
// The last two slots of the grid are reserved for page navigation
// (Pagination_Slots): index rows*cols-2 is the Prev Page control and
// rows*cols-1 is the Next Page control. These are not editable and never render
// a configured button.

// PrevPageSlot returns the slot index reserved for the Prev Page control:
// rows*cols - 2.
func PrevPageSlot(rows, cols int) int {
	return rows*cols - 2
}

// NextPageSlot returns the slot index reserved for the Next Page control:
// rows*cols - 1.
func NextPageSlot(rows, cols int) int {
	return rows*cols - 1
}

// IsPaginationSlot reports whether the given slot index is one of the two
// reserved pagination slots (Prev or Next) for a rows x cols grid.
func IsPaginationSlot(index, rows, cols int) bool {
	return index == PrevPageSlot(rows, cols) || index == NextPageSlot(rows, cols)
}

// --- Font-size offset -> concrete point size ---

// DefaultFontSizePt is the base label point size (in sp/points) used for a
// fontSize offset of 0. The offset values {-2,-1,0,+1,+2} map to concrete sizes
// around this base. The chosen sizes mirror the intent of the Svelte
// getFontSizeClass Tailwind classes (text-xs / text-sm / text-lg-xl / text-xl-2xl
// / text-2xl-3xl) as concrete numbers.
const DefaultFontSizePt float32 = 18

// FontSizeForOffset maps a fontSize offset to a concrete label size in
// sp/points (a plain float32; the UI layer wraps it in unit.Sp). The supported
// offsets are {-2, -1, 0, +1, +2}; any out-of-range value is treated as 0
// (Requirement 4.6). The returned values are:
//
//	-2 -> 12  (tiny,   ~text-xs)
//	-1 -> 14  (small,  ~text-sm)
//	 0 -> 18  (default,~text-lg/xl)  == DefaultFontSizePt
//	+1 -> 24  (large,  ~text-xl/2xl)
//	+2 -> 30  (huge,   ~text-2xl/3xl)
func FontSizeForOffset(offset int) float32 {
	switch offset {
	case -2:
		return 12
	case -1:
		return 14
	case 1:
		return 24
	case 2:
		return 30
	default: // 0 and any out-of-range value
		return DefaultFontSizePt
	}
}

// --- Content resolution ---

// ButtonAtSlot returns the button on the given page whose Order equals slot,
// mirroring the Svelte getButtonAtSlot. The bool result is false when no button
// occupies that slot (Requirement 4.8).
func ButtonAtSlot(page PageConfig, slot int) (ButtonConfig, bool) {
	for _, b := range page.Buttons {
		if b.Order == slot {
			return b, true
		}
	}
	return ButtonConfig{}, false
}

// ButtonAtSlotOnPage is a Config-level convenience: it finds the page whose
// PageIndex equals pageIndex, then returns the button at the given slot on that
// page. The bool result is false when there is no such page or no button at the
// slot.
func ButtonAtSlotOnPage(cfg Config, pageIndex, slot int) (ButtonConfig, bool) {
	for _, p := range cfg.Pages {
		if p.PageIndex == pageIndex {
			return ButtonAtSlot(p, slot)
		}
	}
	return ButtonConfig{}, false
}

// IsRenderable reports whether a button has any content worth rendering: a
// non-empty Label, BgImage, or Command. Matches the Svelte
// `button && (button.label || button.bgImage || button.command)` check
// (Requirement 4.2). An empty/zero button is not renderable and is drawn as a
// dashed empty placeholder.
func IsRenderable(b ButtonConfig) bool {
	return b.Label != "" || b.BgImage != "" || b.Command != ""
}

// --- Pagination indicator ---

// PageIndicator formats the Next Page indicator string as "current / total"
// with a 1-based current page, matching the Svelte
// `(currentPage + 1) + " / " + totalPages` (Requirement 6.5). For example
// currentPage=0, totalPages=5 yields "1 / 5".
func PageIndicator(currentPage, totalPages int) string {
	return strconv.Itoa(currentPage+1) + " / " + strconv.Itoa(totalPages)
}
