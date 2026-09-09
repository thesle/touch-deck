package core

// Pure pagination and slot-mutation helpers for TouchDeck (task 4.2).
//
// Like grid.go, everything in this file is a free function that operates on
// Config / PageConfig / ButtonConfig values (or plain ints). There are NO Gio
// types, no I/O, and no *Backend receiver, so these helpers can be unit- and
// property-tested without a display server (see the design's Testing Strategy).
//
// The functions are pure in the referential-transparency sense: they never
// mutate a caller-owned slice in place. Any function that "changes" a page or
// config copies the affected slices first and returns a NEW value, so property
// tests can rely on the input being unchanged and on self-inverse behavior.
//
// These helpers mirror the Svelte logic in frontend/src/App.svelte exactly:
//   - nextPage/prevPage      <- the currentPage wrap expressions
//   - PruneToGrid            <- adjustGridSize's out-of-bounds filter
//   - MoveButton             <- moveButton(direction)
//   - WriteSlot / ClearSlot  <- saveButton / deleteButton
//
// grid.go owns the grid-math / content-resolution / indicator helpers; this
// file CALLS those (IsPaginationSlot, SlotToRowCol) rather than recomputing.

// --- Pagination wrap (Property 5, Requirements 6.3, 6.4) ---

// NextPage returns the page index reached by advancing one page, wrapping from
// the last page back to the first: (current+1) mod totalPages. It mirrors the
// Svelte `(currentPage + 1) % totalPages`.
//
// totalPages is expected to be >= 1 (the UI uses a const TotalPages = 5); the
// helper is parameterized so tests can vary it. To avoid a mod-by-zero panic on
// a degenerate totalPages <= 0, the current index is returned unchanged.
func NextPage(current, totalPages int) int {
	if totalPages <= 0 {
		return current
	}
	return (current + 1) % totalPages
}

// PrevPage returns the page index reached by going back one page, wrapping from
// the first page to the last: (current-1+totalPages) mod totalPages. It mirrors
// the Svelte `(currentPage - 1 + totalPages) % totalPages`.
//
// totalPages is expected to be >= 1. To avoid a mod-by-zero panic on a
// degenerate totalPages <= 0, the current index is returned unchanged.
func PrevPage(current, totalPages int) int {
	if totalPages <= 0 {
		return current
	}
	return (current - 1 + totalPages) % totalPages
}

// --- Grid-size prune (Property 3, Requirements 9.3, 9.4) ---

// PruneToGrid returns a copy of cfg in which every page has had all buttons with
// Order >= rows*cols removed, mirroring the Svelte adjustGridSize prune
// (`page.buttons.filter(b => b.order < maxSlots)`). The returned Config keeps
// cfg.Rows/cfg.Cols as passed in cfg (this helper only prunes; callers set the
// new rows/cols on the value they pass in), and its Pages/Buttons slices are
// fresh copies so the input Config is left untouched (referential transparency
// for property tests).
//
// After PruneToGrid no page contains a button whose Order is >= rows*cols
// (Property 3). A rows*cols <= 0 removes every button (all Orders are >= 0),
// which is consistent with the invariant.
func PruneToGrid(cfg Config, rows, cols int) Config {
	maxSlots := rows * cols

	out := cfg
	out.Pages = make([]PageConfig, len(cfg.Pages))
	for i, page := range cfg.Pages {
		np := page // copy scalar fields (PageIndex)
		np.Buttons = make([]ButtonConfig, 0, len(page.Buttons))
		for _, b := range page.Buttons {
			if b.Order < maxSlots {
				np.Buttons = append(np.Buttons, b)
			}
		}
		out.Pages[i] = np
	}
	return out
}

// --- Slot move / swap (Property 4, Requirements 11.2, 11.3) ---

// Direction is a move direction for MoveButton / AdjacentSlot. The four valid
// directions mirror the Svelte moveButton('left'|'up'|'down'|'right') controls.
type Direction int

const (
	// Left moves toward a lower column (selectedSlot - 1) when col > 0.
	Left Direction = iota
	// Up moves toward a lower row (selectedSlot - cols) when row > 0.
	Up
	// Down moves toward a higher row (selectedSlot + cols) when row < rows-1.
	Down
	// Right moves toward a higher column (selectedSlot + 1) when col < cols-1.
	Right
)

// AdjacentSlot computes the slot index adjacent to selectedSlot in the given
// direction for a rows x cols grid. It mirrors the Svelte edge guards:
//
//	Left:  col > 0        -> selectedSlot - 1
//	Right: col < cols-1   -> selectedSlot + 1
//	Up:    row > 0        -> selectedSlot - cols
//	Down:  row < rows-1   -> selectedSlot + cols
//
// The bool result is false when the move would leave the grid (the guard fails),
// when the direction is unrecognized, or when the grid is degenerate
// (rows/cols <= 0) or selectedSlot is outside 0 .. rows*cols-1. When ok is
// false the returned slot equals selectedSlot.
func AdjacentSlot(selectedSlot, rows, cols int, dir Direction) (int, bool) {
	if rows <= 0 || cols <= 0 {
		return selectedSlot, false
	}
	if selectedSlot < 0 || selectedSlot >= rows*cols {
		return selectedSlot, false
	}

	row, col := SlotToRowCol(selectedSlot, cols)

	switch dir {
	case Left:
		if col > 0 {
			return selectedSlot - 1, true
		}
	case Right:
		if col < cols-1 {
			return selectedSlot + 1, true
		}
	case Up:
		if row > 0 {
			return selectedSlot - cols, true
		}
	case Down:
		if row < rows-1 {
			return selectedSlot + cols, true
		}
	}
	return selectedSlot, false
}

// MoveButton swaps the button at selectedSlot with the button in the adjacent
// slot in the given direction on the supplied page. It mirrors the Svelte
// moveButton: it computes the target via AdjacentSlot, rejects invalid moves,
// then rewrites the two slots' Order fields.
//
// It returns the (possibly updated) page, the slot that should now be selected,
// and ok. A move is invalid — page returned unchanged, newSel == selectedSlot,
// ok == false — when any of the following hold (Requirement 11.3):
//   - selectedSlot is outside the grid (or the grid is degenerate),
//   - the target slot does not exist (edge of grid / unrecognized direction),
//   - the target slot is a Pagination_Slot (the reserved last two slots),
//   - the selected slot is itself a Pagination_Slot.
//
// On a valid move the two buttons are swapped by exchanging their Order fields:
// the button currently at selectedSlot (if any) takes the target Order and the
// button at the target (if any) takes the selected Order; every other button is
// left untouched. Empty slots are handled exactly as in Svelte — only the
// button(s) actually present are re-added with the swapped Order, so moving a
// button onto an empty slot simply relocates it and vacates the source. Because
// the swap is symmetric, applying the opposite move restores the page (Property
// 4, self-inverse). The returned page's Buttons slice is a fresh copy; the input
// page is never mutated.
func MoveButton(page PageConfig, selectedSlot, rows, cols int, dir Direction) (PageConfig, int, bool) {
	// Selected slot must be a real, non-pagination slot.
	if selectedSlot < 0 || rows <= 0 || cols <= 0 || selectedSlot >= rows*cols {
		return page, selectedSlot, false
	}
	if IsPaginationSlot(selectedSlot, rows, cols) {
		return page, selectedSlot, false
	}

	target, ok := AdjacentSlot(selectedSlot, rows, cols, dir)
	if !ok || target == selectedSlot {
		return page, selectedSlot, false
	}
	// Never swap with a pagination slot.
	if IsPaginationSlot(target, rows, cols) {
		return page, selectedSlot, false
	}

	// Rebuild the button slice: keep everything except the two slots involved,
	// then re-add whichever of the two buttons exist with swapped Order values.
	updated := make([]ButtonConfig, 0, len(page.Buttons))
	var btnAtSelected, btnAtTarget *ButtonConfig
	for i := range page.Buttons {
		b := page.Buttons[i]
		switch b.Order {
		case selectedSlot:
			bb := b
			btnAtSelected = &bb
		case target:
			bb := b
			btnAtTarget = &bb
		default:
			updated = append(updated, b)
		}
	}
	if btnAtSelected != nil {
		btnAtSelected.Order = target
		updated = append(updated, *btnAtSelected)
	}
	if btnAtTarget != nil {
		btnAtTarget.Order = selectedSlot
		updated = append(updated, *btnAtTarget)
	}

	out := page
	out.Buttons = updated
	// Follow the selection to the target slot (Requirement 11.4).
	return out, target, true
}

// --- Slot write / clear (Property 7 & 6, Requirements 8.6, 10.6, 11.6) ---

// WriteSlot writes btn into the page at the given slot, replacing any button
// previously at that slot. It mirrors the Svelte saveButton: force the button's
// Order to slot, drop any existing button whose Order == slot, then append the
// new button. This is used both for editor "save" and clipboard "paste".
//
// The returned page's Buttons slice is a fresh copy; the input page is never
// mutated. After WriteSlot the page contains exactly one button at Order == slot
// carrying btn's label/command/bgImage/bgColor/fontColor/fontSize/id
// (Property 7), and — because the prior occupant of that slot is removed —
// per-page Order uniqueness is preserved (Property 6).
func WriteSlot(page PageConfig, slot int, btn ButtonConfig) PageConfig {
	btn.Order = slot

	updated := make([]ButtonConfig, 0, len(page.Buttons)+1)
	for _, b := range page.Buttons {
		if b.Order != slot {
			updated = append(updated, b)
		}
	}
	updated = append(updated, btn)

	out := page
	out.Buttons = updated
	return out
}

// ClearSlot removes any button at Order == slot from the page, mirroring the
// Svelte deleteButton (`page.buttons.filter(b => b.order !== selectedSlot)`).
// The returned page's Buttons slice is a fresh copy; the input page is never
// mutated. If no button occupies the slot the page is returned with an
// equivalent (copied) Buttons slice and no other change.
func ClearSlot(page PageConfig, slot int) PageConfig {
	updated := make([]ButtonConfig, 0, len(page.Buttons))
	for _, b := range page.Buttons {
		if b.Order != slot {
			updated = append(updated, b)
		}
	}

	out := page
	out.Buttons = updated
	return out
}
