// This file implements the Deck_View VISUAL rendering (task 8.1) per the
// design's "Detailed Component Design -> Deck_View" (grid layout math, content
// resolution, render precedence) and Requirements 4.1-4.8.
//
// Scope of task 8.1: draw the grid. It lays out rows x cols equal cells, resolves
// each slot's content via the pure core helpers, and paints each tile with the
// design's precedence (decodable bgImage -> bgColor -> default) plus a centered
// label. Pagination slots render their APPEARANCE here (Prev/Next tiles with the
// page indicator); their click BEHAVIOR — primary-click page navigation with
// wrap — is task 9.2, also implemented in this file (layoutPrevTile /
// layoutNextTile drain the deck's singleton pagination Clickables and update
// AppState.CurrentPage via the core pagination helpers; they run no command and
// start no flash). Unconfigured slots render as muted empty placeholders
// (Requirement 4.7).
//
// Tap-to-run + the 150ms tactile flash are task 9.1 (implemented in this file
// alongside the 8.1 rendering). Explicitly NOT implemented here (later tasks):
// pagination click behavior (9.2), long-press / right-click / context menu (11),
// and full-screen (10). The flash-highlight rendering SEAM (see isFlashing /
// layoutButtonTile) is honored by 9.1: onDeckButtonTapped sets AppState.Flash[id]
// = now+150ms and layoutDeck schedules an invalidate at the deadline so the
// highlight appears and then clears without requiring pointer movement.
package ui

import (
	"image"
	"image/color"
	"log"
	"math"

	"time"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"touchdeck/internal/core"
)

// deckState holds the Deck_View's transient Gio widget state, mirroring how
// configState holds the Config_View's. It lives on the Renderer (r.deck) so
// AppState stays a pure data model of "what to draw".
//
// taps maps a button id to the widget.Clickable that owns that button tile's
// pointer/tap area. The deck grid is dynamic (buttons vary per page), so the
// Clickables are keyed by stable button id and allocated lazily on first render
// of each button (see tapClickable). Task 11 (long-press / right-click) may
// refactor to raw pointer input for secondary-button + long-press detection;
// 9.1 uses widget.Clickable for the primary tap only. The two pagination
// controls use their own singleton Clickables (prevPage/nextPage) rather than
// the taps map since there is exactly one of each per deck (task 9.2).
type deckState struct {
	taps map[string]*widget.Clickable

	// prevPage/nextPage are the Clickables backing the two pagination tiles
	// (task 9.2). Unlike button tiles, the Prev/Next controls are SINGLETONS —
	// there is exactly one of each per deck regardless of page — so they are
	// plain fields rather than entries in the per-id taps map. They are stable
	// across frames and pages, which is exactly what widget.Clickable needs to
	// track a press-to-release click. Their handlers navigate pages via the
	// core pagination helpers; they never run a command and never flash.
	prevPage widget.Clickable
	nextPage widget.Clickable

	// --- Context_Menu detection tags (task 11.1, Requirements 8.1, 8.2, 8.8) ---
	//
	// widget.Clickable only handles a PRIMARY tap (press→release); it cannot
	// detect a secondary-button (right) click nor a timed long-press. So each
	// NON-pagination content tile ALSO registers a RAW pointer input area whose
	// event.Op tag is a stable per-slot pointer allocated here. Keying by SLOT
	// index (not button id) matches the design's "each non-pagination slot
	// registers a pointer input area" and keeps the tag stable across pages even
	// as buttons move between slots. Pagination slots get NO menuTag entry, so
	// they register no pointer area and can never open the menu (Requirement
	// 8.8). Tags are allocated lazily in menuTag, mirroring tapClickable.
	//
	// The pointer.Event.PointerID makes the tag a stable event.Tag; we store a
	// *int per slot (its address is the tag) so routing stays consistent frame
	// to frame.
	menuTags map[int]*int

	// consumedTap records button ids whose in-progress primary gesture was
	// "consumed" by a long-press that opened the menu, so the widget.Clickable's
	// Clicked() on release does NOT also run the command (double-action
	// prevention — see layoutButtonTile / onDeckButtonTapped). A right-click
	// never triggers the primary Clickable, so it needs no entry. Entries are
	// cleared once the corresponding Clicked() has been drained-and-ignored.
	consumedTap map[string]bool

	// --- Context_Menu overlay widgets (task 11.2, Requirements 8.3–8.9) ---
	//
	// The three menu items and a full-window backdrop that dismisses the menu on
	// an outside click (Requirement 8.9). All are singletons (there is exactly
	// one open menu at a time) and stable across frames, so plain Clickables are
	// correct. Their Clicked() events are drained BEFORE the widgets are laid
	// out (drain-before-layout), the same ordering the rest of the deck uses.
	editMenuBtn  widget.Clickable
	copyMenuBtn  widget.Clickable
	pasteMenuBtn widget.Clickable
	menuBackdrop widget.Clickable

	// --- Deck grid geometry for Context_Menu positioning (bug fix) ---
	//
	// The Context_Menu panel is offset within the DECK's coordinate space (the
	// gtx passed to layoutDeck / layoutContextMenu), but the pointer positions
	// captured in detectMenuGesture are LOCAL to each tile's pushed pointer area
	// (relative to that tile's origin, since event.Op is declared at the tile's
	// clip.Rect within the nested Flex). Storing that small local offset in
	// Menu.Position made the panel always render near the deck's top-left instead
	// of under the clicked slot.
	//
	// To convert to deck space we capture the deck's grid geometry each frame at
	// the top of layoutDeck: deckSize is gtx.Constraints.Max (the full grid
	// area), and deckRows/deckCols/deckGap are the row/column counts and the
	// inter-tile gap in px. Because the grid is an equal-sized rows×cols layout
	// filling the deck area with fixed gaps, layoutContextMenu can compute any
	// slot's on-screen rectangle from its index and anchor the panel there. This
	// avoids threading each tile's offset through the per-slot pointer handler.
	deckSize image.Point
	deckRows int
	deckCols int
	deckGap  int
}

// longPressThreshold is how long a stationary primary press must be held before
// it opens the Context_Menu (Requirement 8.1).
const longPressThreshold = 500 * time.Millisecond

// longPressMoveTolerance is the maximum distance (in dp) the pointer may drift
// from the press origin before the long-press is cancelled and the gesture is
// treated as a normal tap/drag (Requirement 8.1).
const longPressMoveTolerance = 10

// flashDuration is how long a tapped button stays highlighted (Requirement 5.2).
const flashDuration = 150 * time.Millisecond

// Deck palette. These mirror the Svelte deck's Tailwind colors so the native
// render matches the WebView it replaces (frontend/src/App.svelte):
//   - deckBg          #0b0f19  the area behind the grid
//   - defaultTileBg   #1f2937  a configured tile with no bgColor/bgImage
//   - navTileBg       #1f2937  the Prev/Next pagination tiles
//   - flashHighlight  #3b82f6  the active-tap highlight (Requirement 5.2)
//   - tileBorder      #374151  configured/nav tile border
//   - emptyBg         #111827  unconfigured placeholder fill (~40% alpha)
//   - emptyBorder     #374151  unconfigured placeholder border (muted)
//   - navGlyph        #60a5fa  blue-400 Prev/Next arrows + labels
//   - navLabel        #d1d5db  gray-300 "Prev Page"/"Next Page" text
//   - indicatorFg     #9ca3af  gray-400 "current / total" text
//   - imageOverlay    black @ ~40% alpha, drawn over a bgImage for text contrast
//   - defaultFontFg   #ffffff  fallback label color when fontColor is unset
var (
	deckBg         = color.NRGBA{R: 0x0b, G: 0x0f, B: 0x19, A: 0xff}
	defaultTileBg  = color.NRGBA{R: 0x1f, G: 0x29, B: 0x37, A: 0xff}
	navTileBg      = color.NRGBA{R: 0x1f, G: 0x29, B: 0x37, A: 0xff}
	flashHighlight = color.NRGBA{R: 0x3b, G: 0x82, B: 0xf6, A: 0xff}
	tileBorder     = color.NRGBA{R: 0x37, G: 0x41, B: 0x51, A: 0xff}
	emptyBg        = color.NRGBA{R: 0x11, G: 0x18, B: 0x27, A: 0x66}
	emptyBorder    = color.NRGBA{R: 0x37, G: 0x41, B: 0x51, A: 0x99}
	navGlyph       = color.NRGBA{R: 0x60, G: 0xa5, B: 0xfa, A: 0xff}
	navLabel       = color.NRGBA{R: 0xd1, G: 0xd5, B: 0xdb, A: 0xff}
	indicatorFg    = color.NRGBA{R: 0x9c, G: 0xa3, B: 0xaf, A: 0xff}
	imageOverlay   = color.NRGBA{A: 0x66}
	defaultFontFg  = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}

	// Context_Menu palette (task 11.2). The menu is a small rounded panel drawn
	// over the deck; the backdrop is a faint scrim so the deck stays visible
	// while making the outside-click dismiss target obvious.
	//   - menuPanelBg    #1f2937  panel fill
	//   - menuPanelBorder#374151  panel border
	//   - menuItemFg     #e5e7eb  enabled item text (gray-200)
	//   - menuItemDisFg  #6b7280  disabled item text (gray-600) — Paste when empty
	//   - menuBackdropBg black @ ~25% — the dismiss scrim behind the panel
	menuPanelBg     = color.NRGBA{R: 0x1f, G: 0x29, B: 0x37, A: 0xff}
	menuPanelBorder = color.NRGBA{R: 0x37, G: 0x41, B: 0x51, A: 0xff}
	menuItemFg      = color.NRGBA{R: 0xe5, G: 0xe7, B: 0xeb, A: 0xff}
	menuItemDisFg   = color.NRGBA{R: 0x6b, G: 0x72, B: 0x80, A: 0xff}
	menuBackdropBg  = color.NRGBA{A: 0x40}
)

const (
	// tileGapDp is the gap between adjacent tiles, matching the Svelte gap-4.
	tileGapDp = 8
	// tileRadiusDp is the tile corner radius (~ Tailwind rounded-2xl).
	tileRadiusDp = 12
	// tileInsetDp pads the label away from the tile edge (~ Svelte p-3).
	tileInsetDp = 8
	// tileBorderDp is the tile border stroke width.
	tileBorderDp = 1
)

// layoutDeck renders the runtime deck grid (Requirement 4.1). It fills the
// available area behind the grid with the deck background, then lays out
// rows x cols equal cells using nested layout.Flex: a vertical Flex of R flexed
// rows, each a horizontal Flex of C flexed cells, so every cell is equal-sized
// and fills the space (matching the Svelte 1fr grid).
//
// Slot index i = row*cols + col. The last two indices are the Pagination_Slots
// (prev = rows*cols-2, next = rows*cols-1); every other slot resolves a button
// via the core content-resolution helpers.
func (r *Renderer) layoutDeck(gtx layout.Context) layout.Dimensions {
	rows := r.state.Config.Rows
	cols := r.state.Config.Cols

	// Paint the deck background behind the whole grid area.
	fillBackground(gtx, deckBg)

	// A malformed/empty grid has nothing to lay out; keep the painted
	// background and report the area so the frame is still valid.
	if rows < 1 || cols < 1 {
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}

	prevSlot := core.PrevPageSlot(rows, cols)
	nextSlot := core.NextPageSlot(rows, cols)

	gap := gtx.Dp(unit.Dp(tileGapDp))

	// Capture the deck grid geometry for Context_Menu positioning (bug fix).
	// gtx.Constraints.Max is the full deck area in the SAME coordinate space
	// layoutContextMenu / layoutMenuPanel offset within. Recording it (plus
	// rows/cols/gap) here lets layoutContextMenu compute the clicked slot's
	// deck-space rectangle from Menu.Slot so the panel anchors under the slot
	// the user right-clicked / long-pressed instead of at the top-left.
	r.deck.deckSize = gtx.Constraints.Max
	r.deck.deckRows = rows
	r.deck.deckCols = cols
	r.deck.deckGap = gap

	// Build R vertical children (rows). Each row is a horizontal Flex of C
	// cells. Both axes use Flexed(1, ...) so cells share the space equally.
	rowChildren := make([]layout.FlexChild, 0, rows*2-1)
	for rIdx := 0; rIdx < rows; rIdx++ {
		rIdx := rIdx
		if rIdx > 0 {
			rowChildren = append(rowChildren, layout.Rigid(layout.Spacer{Height: unit.Dp(tileGapDp)}.Layout))
		}
		rowChildren = append(rowChildren, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return r.layoutDeckRow(gtx, rIdx, cols, prevSlot, nextSlot, gap)
		}))
	}

	dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx, rowChildren...)

	// Flash scheduling (Requirement 5.2): a tap sets Flash[id] = now+150ms and
	// layoutButtonTile fills the tile with the highlight while the deadline is
	// still in the future. Gio only redraws on events, so without a scheduled
	// redraw the highlight would linger until the next unrelated frame. Request
	// a redraw exactly at the earliest live deadline so the frame that clears
	// the highlight runs on time, with no pointer movement required. Passed
	// (or absent) deadlines are ignored.
	if deadline, ok := r.earliestFlashDeadline(); ok {
		gtx.Execute(op.InvalidateCmd{At: deadline})
	}

	// Long-press scheduling (task 11.1, Requirement 8.1): while a primary press
	// is being tracked, request a redraw at the 500ms threshold so the frame
	// that opens the menu runs on time even if no further pointer input arrives
	// (mirrors the flash scheduling above). detectMenuGesture opens the menu on
	// the frame where elapsed ≥ 500ms.
	if r.state.LongPress.Active {
		gtx.Execute(op.InvalidateCmd{At: r.state.LongPress.Start.Add(longPressThreshold)})
	}

	// Context_Menu overlay (task 11.2): when open, draw the menu ON TOP of the
	// grid at Menu.Position. Because it is laid out AFTER the grid within the
	// same deck area, its ops paint above every tile. A full-area backdrop under
	// the panel captures outside clicks to dismiss (Requirement 8.9).
	if r.state.Menu.Open {
		r.layoutContextMenu(gtx)
	}

	return dims
}

// openMenu opens the Context_Menu for the given slot at pos (Requirement 8.1,
// 8.2). Pagination slots never reach here (they register no detection area), so
// no extra guard is needed, but IsPaginationSlot is checked defensively so a
// stray call can never open the menu on a Prev/Next slot (Requirement 8.8). A
// redraw is requested so the overlay paints immediately.
func (r *Renderer) openMenu(slot int, pos f32.Point) {
	if core.IsPaginationSlot(slot, r.state.Config.Rows, r.state.Config.Cols) {
		return
	}
	r.state.Menu = MenuState{Open: true, Slot: slot, Position: pos}
	if r.w != nil {
		r.w.Invalidate()
	}
}

// closeMenu closes the Context_Menu with no action (Requirement 8.9) and
// requests a redraw so the overlay disappears immediately.
func (r *Renderer) closeMenu() {
	r.state.Menu.Open = false
	if r.w != nil {
		r.w.Invalidate()
	}
}

// earliestFlashDeadline returns the soonest still-future flash deadline across
// all buttons, or ok=false when no flash is currently active. It is used to
// schedule a single redraw (op.InvalidateCmd) that lands when the nearest flash
// should clear.
func (r *Renderer) earliestFlashDeadline() (time.Time, bool) {
	if r.state.Flash == nil {
		return time.Time{}, false
	}
	now := time.Now()
	var earliest time.Time
	found := false
	for _, until := range r.state.Flash {
		if until.After(now) && (!found || until.Before(earliest)) {
			earliest = until
			found = true
		}
	}
	return earliest, found
}

// layoutDeckRow lays out a single grid row: C equal cells separated by gaps.
func (r *Renderer) layoutDeckRow(gtx layout.Context, row, cols, prevSlot, nextSlot, gap int) layout.Dimensions {
	cellChildren := make([]layout.FlexChild, 0, cols*2-1)
	for cIdx := 0; cIdx < cols; cIdx++ {
		i := core.RowColToSlot(row, cIdx, cols)
		if cIdx > 0 {
			cellChildren = append(cellChildren, layout.Rigid(layout.Spacer{Width: unit.Dp(tileGapDp)}.Layout))
		}
		cellChildren = append(cellChildren, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return r.layoutDeckSlot(gtx, i, prevSlot, nextSlot)
		}))
	}
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, cellChildren...)
}

// layoutDeckSlot renders a single slot's tile. It dispatches on the slot kind:
// pagination controls render their appearance; content slots resolve a button
// and render it (or a dashed empty placeholder when unconfigured).
func (r *Renderer) layoutDeckSlot(gtx layout.Context, slot, prevSlot, nextSlot int) layout.Dimensions {
	switch slot {
	case prevSlot:
		return r.layoutPrevTile(gtx)
	case nextSlot:
		return r.layoutNextTile(gtx)
	default:
		btn, ok := core.ButtonAtSlotOnPage(r.state.Config, r.state.CurrentPage, slot)
		if ok && core.IsRenderable(btn) {
			return r.layoutButtonTile(gtx, slot, btn)
		}
		// Unconfigured content slots still register the menu pointer area so a
		// long-press / right-click can open the menu on an EMPTY slot (Copy of
		// an empty slot copies default settings; Paste fills it). Only
		// pagination slots suppress the menu (Requirement 8.8), and those are
		// dispatched above — never reaching this default branch.
		return r.layoutEmptyMenuTile(gtx, slot)
	}
}

// layoutEmptyMenuTile draws an unconfigured slot placeholder AND registers the
// context-menu pointer detection area over it, so a long-press / right-click on
// an empty content slot opens the menu (Requirements 8.1, 8.2). It wraps the
// display-only layoutEmptyTile with the same pointer-detection macro used by
// content buttons; the empty slot has no button id, so it never participates in
// the primary-tap double-action guard.
func (r *Renderer) layoutEmptyMenuTile(gtx layout.Context, slot int) layout.Dimensions {
	dims := r.layoutEmptyTile(gtx)
	r.detectMenuGesture(gtx, slot, "", dims.Size)
	return dims
}

// layoutButtonTile draws a configured button following the design's render
// precedence (Deck_View + Requirements 4.2-4.6):
//
//  1. bgImage present AND decodable -> first fill the tile with the button's
//     bgColor (its configured Tile Color, else the default tile color) as a
//     backdrop, then draw the WHOLE image scaled to CONTAIN the tile (fit
//     inside, aspect-ratio preserved, NO crop), centered, clipped to the tile's
//     rounded bounds, then a translucent dark overlay for text contrast. The
//     bgColor backdrop fills the leftover margins around a contained image
//     (previously bgColor was skipped in this branch — now it is intentionally
//     the backdrop, per the user's "fill leftover space with the Tile Color").
//  2. else bgColor a valid hex           -> fill the tile with that color.
//  3. else                               -> fill with the default tile color.
//
// While an active flash deadline exists for this button (read-only seam for task
// 9.1) the fill is replaced by the highlight color; the image branch keeps the
// image but the flash still tints via the overlay path being skipped in favor of
// the highlight fill underneath — see tileBgColor.
//
// The label is drawn centered in fontColor (default white) at the offset size
// from FontSizeForOffset wrapped in unit.Sp (Requirements 4.5, 4.6).
func (r *Renderer) layoutButtonTile(gtx layout.Context, slot int, btn core.ButtonConfig) layout.Dimensions {
	// Tap handling (task 9.1, Requirement 5.1/5.3): route the tile's pointer
	// area through a per-button widget.Clickable.
	//
	// BUG 1 fix (no grid interaction): drain Clicked() BEFORE Layout, not after.
	// widget.Clickable.Layout (v0.10.2 widget/button.go) begins with a loop
	// `for { _, ok := b.update(t, gtx); if !ok break }` that CONSUMES every
	// pending gesture event — including the click queued on the previous frame —
	// and discards it. So calling Layout first and `for c.Clicked(gtx)` after
	// meant Layout had already eaten the click and Clicked() saw nothing, so the
	// handler never ran. The header buttons work precisely because render.go
	// checks r.deckBtn.Clicked(gtx) at the TOP of the frame, before their
	// material.Button Layout runs. Mirroring that order here makes the click land
	// on the same frame's Clicked() call, before Layout drains the queue. Each
	// drained click is a primary tap that runs the button's command
	// (empty-command taps are inert — see onDeckButtonTapped).
	//
	// Double-action prevention (task 11.1): a long-press that opened the menu
	// marks the button in deck.consumedTap. widget.Clickable still reports a
	// Clicked() on the eventual release, so we DRAIN it but skip running the
	// command for a consumed gesture, clearing the flag. This way a long-press
	// opens the menu WITHOUT also running the command, while a quick tap (never
	// marked consumed) runs normally. A right-click never triggers this primary
	// Clickable at all, so it needs no guard here.
	c := r.tapClickable(btn.ID)
	for c.Clicked(gtx) {
		if r.deck.consumedTap != nil && r.deck.consumedTap[btn.ID] {
			delete(r.deck.consumedTap, btn.ID)
			continue
		}
		r.onDeckButtonTapped(btn)
	}
	dims := c.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return r.drawButtonTile(gtx, btn)
	})

	// Register the raw pointer area for long-press / right-click detection ON
	// TOP of the Clickable's area, tagged by slot. The primary tap keeps flowing
	// to the widget.Clickable (it is not pass-through, but pointer input allows
	// multiple overlapping handlers via Shared priority); the secondary button
	// and the long-press timing are handled by detectMenuGesture.
	r.detectMenuGesture(gtx, slot, btn.ID, dims.Size)
	return dims
}

// menuTag returns the stable per-slot pointer event.Tag used to route
// context-menu detection events for a content slot, allocating it (and the
// backing map) lazily on first use. Keying by slot index keeps the tag stable
// across pages even as buttons move between slots. Pagination slots never call
// this (they suppress the menu, Requirement 8.8).
func (r *Renderer) menuTag(slot int) *int {
	if r.deck.menuTags == nil {
		r.deck.menuTags = make(map[int]*int)
	}
	t, ok := r.deck.menuTags[slot]
	if !ok {
		s := slot
		t = &s
		r.deck.menuTags[slot] = t
	}
	return t
}

// detectMenuGesture registers a raw pointer input area covering the tile just
// laid out (size) and processes its pending pointer events to drive the
// Context_Menu triggers (task 11.1, Requirements 8.1, 8.2). It is called for
// EVERY non-pagination content slot (configured or empty); pagination slots do
// not call it, which is how Requirement 8.8 is satisfied.
//
// Detection rules (design "Long-press / right-click → Context_Menu"):
//   - Secondary-button press (pointer.ButtonSecondary) → open the menu
//     immediately for this slot at the event position (8.2).
//   - Primary press → start LongPress{Slot, Origin, Start}. On a subsequent
//     Move/Drag beyond longPressMoveTolerance dp, cancel (it becomes a normal
//     tap/drag). On Release before the threshold, cancel. Each frame, if the
//     press is still held, unmoved, and elapsed ≥ 500ms, open the menu at the
//     origin and mark the gesture consumed so the release does not also run the
//     command (8.1). See layoutDeck for the op.InvalidateCmd that fires the
//     500ms deadline without further input.
//
// The tag is the per-slot *int from menuTag; event.Op declares it at the
// current clip area (the tile bounds), so events only route here for this tile.
func (r *Renderer) detectMenuGesture(gtx layout.Context, slot int, btnID string, size image.Point) {
	tag := r.menuTag(slot)

	// Declare the pointer input area over the tile bounds so events route to tag.
	area := clip.Rect{Max: size}.Push(gtx.Ops)
	event.Op(gtx.Ops, tag)
	area.Pop()

	tol := float32(gtx.Dp(unit.Dp(longPressMoveTolerance)))

	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: tag,
			Kinds:  pointer.Press | pointer.Release | pointer.Move | pointer.Drag | pointer.Cancel,
		})
		if !ok {
			break
		}
		pe, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		switch pe.Kind {
		case pointer.Press:
			if pe.Buttons.Contain(pointer.ButtonSecondary) {
				// Right-click: open immediately (8.2). Do not start a
				// long-press for this secondary press.
				r.openMenu(slot, pe.Position)
				continue
			}
			// Primary press: begin tracking a potential long-press (8.1).
			r.state.LongPress = LongPressState{
				Active: true,
				Slot:   slot,
				Origin: pe.Position,
				Start:  gtx.Now,
			}
		case pointer.Move, pointer.Drag:
			// Cancel the long-press if the pointer drifted too far.
			if r.state.LongPress.Active && r.state.LongPress.Slot == slot {
				if dist(pe.Position, r.state.LongPress.Origin) > tol {
					r.state.LongPress.Active = false
				}
			}
		case pointer.Release, pointer.Cancel:
			// A release/cancel before the threshold ends the long-press; the
			// widget.Clickable handles the tap.
			if r.state.LongPress.Active && r.state.LongPress.Slot == slot {
				r.state.LongPress.Active = false
			}
		}
	}

	// Fire the long-press if the stationary press has now been held long enough.
	// This runs every frame the press is active; the op.InvalidateCmd scheduled
	// in layoutDeck guarantees a frame lands at the deadline even with no further
	// pointer input.
	if r.state.LongPress.Active && r.state.LongPress.Slot == slot {
		if gtx.Now.Sub(r.state.LongPress.Start) >= longPressThreshold {
			r.openMenu(slot, r.state.LongPress.Origin)
			r.state.LongPress.Active = false
			// Mark this button's in-progress primary gesture as consumed so the
			// eventual release does not also run the command (double-action).
			if btnID != "" {
				if r.deck.consumedTap == nil {
					r.deck.consumedTap = make(map[string]bool)
				}
				r.deck.consumedTap[btnID] = true
			}
		}
	}
}

// dist returns the Euclidean distance between two points.
func dist(a, b f32.Point) float32 {
	dx := a.X - b.X
	dy := a.Y - b.Y
	return float32(math.Hypot(float64(dx), float64(dy)))
}

// drawButtonTile draws the button tile's visuals (the 8.1 render precedence).
// It is wrapped by a widget.Clickable in layoutButtonTile so the tile is both
// drawn and tappable; separating the drawing keeps the click wiring readable.
func (r *Renderer) drawButtonTile(gtx layout.Context, btn core.ButtonConfig) layout.Dimensions {
	size := gtx.Constraints.Max
	radius := gtx.Dp(unit.Dp(tileRadiusDp))

	// Clip everything (background, image, overlay, border) to the rounded tile
	// bounds so an oversized cover image cannot overflow the tile.
	rrect := clip.UniformRRect(image.Rectangle{Max: size}, radius)
	defer rrect.Push(gtx.Ops).Pop()

	flashing := r.isFlashing(btn.ID)

	var imageOp paint.ImageOp
	imgOK := false
	if btn.BgImage != "" && !flashing {
		if op, ok := r.state.Textures.Get(btn.BgImage); ok {
			imageOp, imgOK = op, true
		}
	}

	switch {
	case imgOK:
		// (1) Image contain: fill the tile with the button's bgColor as a
		// backdrop first, so the margins left around a contained (fit-inside)
		// image show the Tile Color rather than empty space. Then draw the WHOLE
		// image scaled to CONTAIN (aspect-ratio preserved, no crop) and centered,
		// then a translucent dark overlay for text contrast. The overlay is kept
		// full-tile (as before): it only slightly darkens the color margins too,
		// which is acceptable for label contrast and keeps the branch simple.
		paint.Fill(gtx.Ops, tileFillColor(btn.BgColor))
		drawContainImage(gtx, imageOp)
		paint.Fill(gtx.Ops, imageOverlay)
	case flashing:
		// Flash-highlight seam: while an active flash deadline exists, tint the
		// tile with the highlight color (task 9.1 sets the deadline).
		paint.Fill(gtx.Ops, flashHighlight)
	default:
		// (2)/(3) Solid fill: bgColor if valid hex, else default tile color.
		paint.Fill(gtx.Ops, tileFillColor(btn.BgColor))
	}

	// Tile border, matching the Svelte configured-tile border.
	strokeRRect(gtx, size, radius, gtx.Dp(unit.Dp(tileBorderDp)), tileBorder)

	// Centered label in the button's font color at its offset size.
	r.layoutTileLabel(gtx, btn.Label, tileFontColor(btn.FontColor), core.FontSizeForOffset(btn.FontSize))

	return layout.Dimensions{Size: size}
}

// layoutTileLabel draws label centered within the current tile at the given
// color and point size. Long labels wrap and are centered (matching the Svelte
// break-words + text-center); the label is inset from the tile edge.
func (r *Renderer) layoutTileLabel(gtx layout.Context, label string, fg color.NRGBA, sizeSp float32) layout.Dimensions {
	if label == "" {
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}
	return layout.Inset{
		Top:    unit.Dp(tileInsetDp),
		Bottom: unit.Dp(tileInsetDp),
		Left:   unit.Dp(tileInsetDp),
		Right:  unit.Dp(tileInsetDp),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			lbl := material.Label(r.th, unit.Sp(sizeSp), label)
			lbl.Color = fg
			lbl.Alignment = text.Middle
			return lbl.Layout(gtx)
		})
	})
}

// layoutPrevTile renders the Prev Page control and handles its taps (task 9.2,
// Requirement 6.4). The tile is wrapped in the deck's singleton prevPage
// Clickable; each drained click moves to the previous page, wrapping from the
// first page to the last via core.PrevPage. It renders no page indicator.
func (r *Renderer) layoutPrevTile(gtx layout.Context) layout.Dimensions {
	// BUG 1 fix: drain Clicked() before Layout (see layoutButtonTile) so the
	// Clickable's own update loop in Layout does not consume the tap first.
	for r.deck.prevPage.Clicked(gtx) {
		r.onPrevPageTapped()
	}
	return r.deck.prevPage.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return r.drawNavTile(gtx, "◀", "Prev Page", "")
	})
}

// layoutNextTile renders the Next Page control and handles its taps (task 9.2,
// Requirements 6.3, 6.5). The tile is wrapped in the deck's singleton nextPage
// Clickable; each drained click advances to the next page, wrapping from the
// last page to the first via core.NextPage. It shows the "current / total" page
// indicator in its own reserved band at the bottom of the tile (see drawNavTile).
func (r *Renderer) layoutNextTile(gtx layout.Context) layout.Dimensions {
	indicator := core.PageIndicator(r.state.CurrentPage, TotalPages)
	// BUG 1 fix: drain Clicked() before Layout (see layoutButtonTile) so the
	// Clickable's own update loop in Layout does not consume the tap first.
	for r.deck.nextPage.Clicked(gtx) {
		r.onNextPageTapped()
	}
	return r.deck.nextPage.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return r.drawNavTile(gtx, "▶", "Next Page", indicator)
	})
}

// onPrevPageTapped moves the deck to the previous page, wrapping from the first
// page to the last (Requirement 6.4). Page navigation is a pure state change:
// it runs NO command and starts NO tactile flash, so a page control tap is
// visually distinct from a button tap. layoutDeckSlot reads state.CurrentPage
// afresh each frame, so the next frame re-resolves the grid for the new page.
//
// REDRAW FIX (follow-up to 9.2, found in manual GUI testing): Gio is
// immediate-mode and only produces a frame on an event or an explicit redraw
// request. The click that changes CurrentPage was rendered against the OLD page
// (the grid for this frame is already laid out / CurrentPage was read at the top
// of layoutDeck), and nothing requests the FOLLOWING frame — so the grid stays
// stale until an unrelated event (e.g. mouse move) happens to trigger the next
// frame. Requesting r.w.Invalidate() after the mutation forces exactly one more
// frame, which re-runs layoutDeck and reads the new CurrentPage, so the new
// page's buttons appear immediately. r.w is the window handle already held for
// toggleFullscreen; (*app.Window).Invalidate() (v0.10.2) requests a new frame.
func (r *Renderer) onPrevPageTapped() {
	r.state.CurrentPage = core.PrevPage(r.state.CurrentPage, TotalPages)
	r.w.Invalidate()
}

// onNextPageTapped advances the deck to the next page, wrapping from the last
// page to the first (Requirement 6.3). Like Prev, it runs no command and starts
// no flash. It requests r.w.Invalidate() after the change for the same reason as
// onPrevPageTapped: force the next frame so the grid repaints with the new
// CurrentPage instead of staying stale until an unrelated event arrives.
func (r *Renderer) onNextPageTapped() {
	r.state.CurrentPage = core.NextPage(r.state.CurrentPage, TotalPages)
	r.w.Invalidate()
}

// drawNavTile renders a pagination control's appearance (Requirement 6.2): a
// distinct dark tile with the label ("Prev Page"/"Next Page") on top and the
// glyph (◀ / ▶) directly BELOW it as a vertical stack, plus — for the Next tile
// — the "current / total" page indicator pinned in its own band at the BOTTOM
// edge of the tile. This is drawing only; the click behavior lives in
// layoutPrevTile/layoutNextTile.
//
// Layout (alignment fix, per the user's manual-verification feedback):
//   - The label sits ON TOP with the arrow glyph CENTERED BELOW it (a vertical
//     column), not beside it, so both tiles read as "Prev Page / ◀" and
//     "Next Page / ▶" stacked. Both tiles use this identical column so they stay
//     symmetric.
//   - The label+arrow column is CENTERED over the FULL tile height on BOTH tiles
//     (identical constraints), and the "current / total" indicator is drawn as a
//     stacked OVERLAY pinned to the bottom edge (layout.S) that does NOT reduce
//     the centered group's height. This replaces the earlier flexed-spacer
//     scheme, where the Next tile reserved a rigid indicator band at the bottom
//     while Prev did not: the two tiles then centered their label+arrow over
//     DIFFERENT heights (Next's was shortened by the band), so "Prev Page" and
//     "Next Page" did not line up. With the overlay approach both tiles center
//     over the same full height, so the labels sit at the same y; only the Next
//     tile adds a bottom-overlaid indicator on top. The indicator is small (11sp)
//     and hugs the very bottom, so on realistic tile sizes it does not touch the
//     centered arrow.
func (r *Renderer) drawNavTile(gtx layout.Context, glyph, label, indicator string) layout.Dimensions {
	size := gtx.Constraints.Max
	radius := gtx.Dp(unit.Dp(tileRadiusDp))

	rrect := clip.UniformRRect(image.Rectangle{Max: size}, radius)
	defer rrect.Push(gtx.Ops).Pop()

	paint.Fill(gtx.Ops, navTileBg)
	strokeRRect(gtx, size, radius, gtx.Dp(unit.Dp(tileBorderDp)), tileBorder)

	return layout.Inset{
		Top:    unit.Dp(tileInsetDp),
		Bottom: unit.Dp(tileInsetDp),
		Left:   unit.Dp(tileInsetDp),
		Right:  unit.Dp(tileInsetDp),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Stack{Alignment: layout.Center}.Layout(gtx,
			// Expanded: the label+arrow column, centered over the FULL tile
			// height. Identical on both tiles, so the labels line up. Using
			// Expanded (not Stacked) makes this child fill the stack, and the
			// stack's overall size is driven by it, so the bottom-pinned
			// indicator overlay below cannot change where this group centers.
			layout.Expanded(func(gtx layout.Context) layout.Dimensions {
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
						// Label on top.
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							l := material.Label(r.th, unit.Sp(13), label)
							l.Color = navLabel
							l.Alignment = text.Middle
							return l.Layout(gtx)
						}),
						// Small gap between label and the arrow beneath it.
						layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
						// Arrow glyph directly below the label.
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							g := material.Label(r.th, unit.Sp(24), glyph)
							g.Color = navGlyph
							g.Alignment = text.Middle
							return g.Layout(gtx)
						}),
					)
				})
			}),
			// Stacked overlay: the "current / total" indicator pinned to the
			// bottom edge (layout.S). Empty on the Prev tile (zero-size, draws
			// nothing), present on the Next tile. As a Stacked child it is
			// OVERLAID and does not affect the centered group's height, so both
			// tiles' labels remain aligned.
			layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				if indicator == "" {
					return layout.Dimensions{}
				}
				return layout.S.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					ind := material.Label(r.th, unit.Sp(11), indicator)
					ind.Color = indicatorFg
					ind.Alignment = text.Middle
					return ind.Layout(gtx)
				})
			}),
		)
	})
}

// layoutEmptyTile renders an unconfigured slot as a muted empty placeholder
// (Requirement 4.7). Gio has no built-in dashed-stroke primitive, so the dashed
// border of the Svelte placeholder is APPROXIMATED with a thin solid muted
// border over a faint fill, plus a centered gray "--" (mirroring the Svelte
// placeholder text). See the task report note on this approximation.
func (r *Renderer) layoutEmptyTile(gtx layout.Context) layout.Dimensions {
	size := gtx.Constraints.Max
	radius := gtx.Dp(unit.Dp(tileRadiusDp))

	rrect := clip.UniformRRect(image.Rectangle{Max: size}, radius)
	defer rrect.Push(gtx.Ops).Pop()

	paint.Fill(gtx.Ops, emptyBg)
	strokeRRect(gtx, size, radius, gtx.Dp(unit.Dp(tileBorderDp)), emptyBorder)

	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		l := material.Label(r.th, unit.Sp(14), "--")
		l.Color = color.NRGBA{R: 0x6b, G: 0x72, B: 0x80, A: 0xff} // gray-600
		l.Alignment = text.Middle
		return l.Layout(gtx)
	})
}

// --- Small drawing helpers ---

// tapClickable returns the widget.Clickable owning button id's tap area,
// allocating it (and the backing map) lazily on first use. Keying by stable
// button id keeps a button's click state consistent across frames even though
// the grid is re-laid out every frame and buttons move between slots/pages.
func (r *Renderer) tapClickable(id string) *widget.Clickable {
	if r.deck.taps == nil {
		r.deck.taps = make(map[string]*widget.Clickable)
	}
	c, ok := r.deck.taps[id]
	if !ok {
		c = new(widget.Clickable)
		r.deck.taps[id] = c
	}
	return c
}

// onDeckButtonTapped runs a tapped deck button's command and starts its tactile
// flash (task 9.1). Per Requirement 5.3 a button with an empty command is inert:
// no command runs and no flash starts. Otherwise the command is dispatched
// asynchronously via the Backend so the UI never blocks (Requirement 5.1); a
// start error is logged rather than surfaced (the runtime deck has no error UI).
// The flash deadline is set to now+150ms (Requirement 5.2); layoutButtonTile
// paints the highlight while the deadline is in the future and layoutDeck
// schedules the redraw that clears it.
func (r *Renderer) onDeckButtonTapped(btn core.ButtonConfig) {
	if btn.Command == "" {
		return
	}
	if err := r.state.Backend.RunCommandAsync(btn.Command); err != nil {
		log.Printf("deck: run command for button %q failed to start: %v", btn.ID, err)
	}
	if r.state.Flash == nil {
		r.state.Flash = make(map[string]time.Time)
	}
	r.state.Flash[btn.ID] = time.Now().Add(flashDuration)
}

// isFlashing reports whether button id has a live tactile-flash deadline. This
// is the seam wired by task 9.1: onDeckButtonTapped sets Flash[id] = now+150ms
// and layoutDeck invalidates; the render simply honors a still-future deadline.
func (r *Renderer) isFlashing(id string) bool {
	if id == "" || r.state.Flash == nil {
		return false
	}
	until, ok := r.state.Flash[id]
	return ok && time.Now().Before(until)
}

// tileFillColor picks the solid fill for a tile without a bgImage: the button's
// bgColor when it is a valid hex, otherwise the default tile color (#1f2937).
func tileFillColor(bgColor string) color.NRGBA {
	if col, ok := ParseHexColor(bgColor); ok {
		return col
	}
	return defaultTileBg
}

// tileFontColor picks the label color: the button's fontColor when valid,
// otherwise the default white.
func tileFontColor(fontColor string) color.NRGBA {
	if col, ok := ParseHexColor(fontColor); ok {
		return col
	}
	return defaultFontFg
}

// drawContainImage paints op scaled to CONTAIN the current tile constraints and
// centered, using widget.Image with Fit: widget.Contain and Position:
// layout.Center. Contain fits the WHOLE image inside the tile preserving aspect
// ratio with NO cropping (widget/fit.go, v0.10.2), so the long side no longer
// overflows/crops; the leftover margins are filled by the bgColor backdrop the
// caller paints first. Scale is 1/PxPerDp so one image pixel maps to one output
// pixel before the Contain fit rescales it to fit the tile (Requirement 4.3).
// The caller has already pushed a rounded-rect clip.
func drawContainImage(gtx layout.Context, op paint.ImageOp) {
	img := widget.Image{
		Src:      op,
		Fit:      widget.Contain,
		Position: layout.Center,
		Scale:    1.0 / gtx.Metric.PxPerDp,
	}
	img.Layout(gtx)
}

// fillBackground fills the entire current constraint area (Max) with col. Used
// to paint the deck background behind the grid.
func fillBackground(gtx layout.Context, col color.NRGBA) {
	paintRect(gtx, gtx.Constraints.Max, col)
}

// paintRect fills the rectangle from (0,0) to size with col, clipped to that
// rectangle so it does not bleed past the intended area.
func paintRect(gtx layout.Context, size image.Point, col color.NRGBA) {
	defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
	paint.Fill(gtx.Ops, col)
}

// strokeRRect draws a rounded-rectangle border of the given width and color
// around a size x size... area. It uses clip.Stroke over a rounded-rect outline
// so the border matches the tile's corner radius.
func strokeRRect(gtx layout.Context, size image.Point, radius, width int, col color.NRGBA) {
	rr := clip.RRect{Rect: image.Rectangle{Max: size}, SE: radius, SW: radius, NE: radius, NW: radius}
	paint.FillShape(gtx.Ops, col, clip.Stroke{
		Path:  rr.Path(gtx.Ops),
		Width: float32(width),
	}.Op())
}

// --- Context_Menu overlay + actions (task 11.2, Requirements 8.3–8.10) ---

// menuItemHeightDp / menuWidthDp size the overlay panel. The panel is small and
// fixed-width so it reads as a compact context menu.
const (
	menuWidthDp      = 160
	menuItemHeightDp = 40
	menuPadDp        = 6
)

// layoutContextMenu draws the Context_Menu overlay when Menu.Open (Requirements
// 8.3–8.9). It is called from layoutDeck AFTER the grid, so its ops paint above
// every tile. Two stacked layers:
//
//  1. A full-area backdrop (transparent-ish scrim) wired to a widget.Clickable
//     that closes the menu on any click OUTSIDE the panel (Requirement 8.9). It
//     is laid out FIRST/underneath so the panel's own click areas sit on top and
//     take priority — clicking a menu item hits the item's Clickable, not the
//     backdrop.
//  2. The panel itself, offset to Menu.Position, containing Edit / Copy / Paste.
//
// Drain-before-layout: every item's and the backdrop's Clicked() is drained
// BEFORE its widget is laid out (the established deck ordering) so the click
// lands the same frame rather than being swallowed by widget.Clickable.Layout.
func (r *Renderer) layoutContextMenu(gtx layout.Context) layout.Dimensions {
	// Drain the action clicks first (drain-before-layout). Edit/Copy always act;
	// Paste only acts when the clipboard is non-empty (Requirement 8.7).
	for r.deck.editMenuBtn.Clicked(gtx) {
		r.menuEdit()
	}
	for r.deck.copyMenuBtn.Clicked(gtx) {
		r.menuCopy()
	}
	for r.deck.pasteMenuBtn.Clicked(gtx) {
		if r.state.Clipboard != nil {
			r.menuPaste()
		}
	}
	// Drain the backdrop click LAST so it only fires when the click was outside
	// the panel (the panel's item areas, laid out on top, consume in-panel
	// clicks). Any drained backdrop click dismisses the menu (Requirement 8.9).
	for r.deck.menuBackdrop.Clicked(gtx) {
		r.closeMenu()
	}
	// If an action above already closed the menu, stop — nothing to draw.
	if !r.state.Menu.Open {
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}

	return layout.Stack{}.Layout(gtx,
		// (1) Full-area dismiss backdrop UNDER the panel.
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return r.deck.menuBackdrop.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				size := gtx.Constraints.Max
				paintRect(gtx, size, menuBackdropBg)
				return layout.Dimensions{Size: size}
			})
		}),
		// (2) The panel, offset to Menu.Position, clamped so it stays on-screen.
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return r.layoutMenuPanel(gtx)
		}),
	)
}

// slotOrigin returns the top-left corner of the given slot's tile in the deck's
// coordinate space (the space layoutMenuPanel offsets within), computed from the
// grid geometry captured in layoutDeck (bug fix).
//
// The grid is an equal-sized rows×cols layout filling deckSize with a fixed gap
// between adjacent tiles, so each cell's width/height and origin are a pure
// function of its row/col:
//
//	cellW = (deckW - (cols-1)*gap) / cols
//	cellH = (deckH - (rows-1)*gap) / rows
//	originX = col*(cellW+gap)
//	originY = row*(cellH+gap)
//
// This matches the nested Flex(1) layout in layoutDeck/layoutDeckRow (equal
// flexed cells separated by tileGapDp spacers). It returns ok=false when the
// geometry has not been captured yet or the slot is out of range, so the caller
// can fall back to the raw (local) position.
func (r *Renderer) slotOrigin(slot int) (image.Point, bool) {
	cols := r.deck.deckCols
	rows := r.deck.deckRows
	if cols < 1 || rows < 1 || slot < 0 || slot >= rows*cols {
		return image.Point{}, false
	}
	gap := r.deck.deckGap
	deckW := r.deck.deckSize.X
	deckH := r.deck.deckSize.Y
	cellW := (deckW - (cols-1)*gap) / cols
	cellH := (deckH - (rows-1)*gap) / rows
	col := slot % cols
	row := slot / cols
	return image.Pt(col*(cellW+gap), row*(cellH+gap)), true
}

// layoutMenuPanel positions and draws the menu panel anchored to the clicked
// slot's tile. The panel is placed with an op.Offset clamped so it never runs
// off the right/bottom edge of the deck area. It draws a rounded filled+bordered
// panel with the three items stacked vertically.
//
// BUG FIX (menu always rendered over the top-left slot): Menu.Position holds the
// pointer position LOCAL to the clicked tile (see detectMenuGesture), which is a
// small 0..tileW / 0..tileH offset. Offsetting the panel by that local value in
// DECK space put every menu near the deck's top-left. Instead we anchor the
// panel at the clicked slot's deck-space origin (slotOrigin) and ADD the local
// pointer offset within the tile, so the panel appears near the actual click
// point inside the correct slot. If the geometry is unavailable we fall back to
// the raw position (previous behavior).
func (r *Renderer) layoutMenuPanel(gtx layout.Context) layout.Dimensions {
	panelW := gtx.Dp(unit.Dp(menuWidthDp))
	itemH := gtx.Dp(unit.Dp(menuItemHeightDp))
	pad := gtx.Dp(unit.Dp(menuPadDp))
	panelH := itemH*3 + pad*2

	// Anchor to the clicked slot's deck-space origin, plus the local pointer
	// offset within that tile, so the menu appears at/near the actual click.
	x := int(r.state.Menu.Position.X)
	y := int(r.state.Menu.Position.Y)
	if origin, ok := r.slotOrigin(r.state.Menu.Slot); ok {
		x += origin.X
		y += origin.Y
	}

	// Clamp the origin so the whole panel stays within the available area.
	max := gtx.Constraints.Max
	if x+panelW > max.X {
		x = max.X - panelW
	}
	if y+panelH > max.Y {
		y = max.Y - panelH
	}
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}

	defer op.Offset(image.Pt(x, y)).Push(gtx.Ops).Pop()

	// Constrain the panel to its fixed size for drawing + item layout.
	gtx.Constraints = layout.Exact(image.Pt(panelW, panelH))

	// Panel background + border, rounded.
	radius := gtx.Dp(unit.Dp(8))
	rrect := clip.UniformRRect(image.Rectangle{Max: image.Pt(panelW, panelH)}, radius)
	stack := rrect.Push(gtx.Ops)
	paint.Fill(gtx.Ops, menuPanelBg)
	strokeRRect(gtx, image.Pt(panelW, panelH), radius, gtx.Dp(unit.Dp(1)), menuPanelBorder)
	stack.Pop()

	pasteAvailable := r.state.Clipboard != nil

	return layout.UniformInset(unit.Dp(menuPadDp)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return r.layoutMenuItem(gtx, &r.deck.editMenuBtn, "Edit", true)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return r.layoutMenuItem(gtx, &r.deck.copyMenuBtn, "Copy", true)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return r.layoutMenuItem(gtx, &r.deck.pasteMenuBtn, "Paste", pasteAvailable)
			}),
		)
	})
}

// layoutMenuItem draws one clickable menu row. When enabled is false (Paste with
// an empty clipboard, Requirement 8.7) the row is greyed and its click is
// ignored by the drain in layoutContextMenu, so it is presented as unavailable.
func (r *Renderer) layoutMenuItem(gtx layout.Context, btn *widget.Clickable, label string, enabled bool) layout.Dimensions {
	itemH := gtx.Dp(unit.Dp(menuItemHeightDp))
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	gtx.Constraints.Min.Y = itemH
	gtx.Constraints.Max.Y = itemH

	fg := menuItemFg
	if !enabled {
		fg = menuItemDisFg
	}

	return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		size := image.Pt(gtx.Constraints.Max.X, itemH)
		return layout.Inset{Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				l := material.Label(r.th, unit.Sp(15), label)
				l.Color = fg
				l.Layout(gtx)
				return layout.Dimensions{Size: size}
			})
		})
	})
}

// menuEdit implements the Edit action (Requirement 8.4): switch to Config_View
// and select the target slot for editing, then close the menu. selectSlot
// (config.go) loads the slot's button into the Editor and syncs the widgets;
// it guards pagination slots, but Menu.Slot is never a pagination slot (the menu
// is suppressed on those). The View switch happens BEFORE selectSlot so the
// editor pane is showing when the slot loads.
func (r *Renderer) menuEdit() {
	slot := r.state.Menu.Slot
	r.state.View = ViewConfig
	// Clear any stale selection first (switchView normally does this on a header
	// switch; here we bypass switchView to keep the slot we are about to select).
	r.selectSlot(slot)
	r.closeMenu()
}

// menuCopy implements the Copy action (Requirement 8.5): store the target
// button's settings in the shared Button_Clipboard, then close. The clipboard is
// the SAME AppState.Clipboard the Config_View editor uses, so a deck copy can be
// pasted in the editor and vice versa. Copying an EMPTY slot copies the default
// settings (empty label/command, default colors) so a subsequent paste fills the
// slot with a blank-but-valid button; this mirrors the editor's copy of an
// unconfigured slot.
func (r *Renderer) menuCopy() {
	btn, ok := core.ButtonAtSlotOnPage(r.state.Config, r.state.CurrentPage, r.state.Menu.Slot)
	if !ok {
		// Empty slot: copy default settings.
		r.state.Clipboard = &ButtonClipboard{
			BgColor:   defaultEditorBgColor,
			FontColor: defaultEditorFontColor,
		}
	} else {
		r.state.Clipboard = &ButtonClipboard{
			Label:     btn.Label,
			Command:   btn.Command,
			BgImage:   btn.BgImage,
			BgColor:   btn.BgColor,
			FontColor: btn.FontColor,
			FontSize:  btn.FontSize,
		}
	}
	r.closeMenu()
}

// menuPaste implements the Paste action (Requirements 8.6, 8.7, 8.10): apply the
// shared clipboard to the target slot on the current page and save. The caller
// only invokes this when Clipboard != nil (8.7), re-checked defensively.
//
//   - It builds a core.ButtonConfig from the clipboard with Order = target slot.
//     If a button already occupies the slot its id is KEPT; otherwise a fresh id
//     is generated (design: "generating/keeping an id").
//   - It finds/creates the current page (currentPageIndex) and applies
//     core.WriteSlot (which sets Order = slot and replaces any prior occupant).
//   - Save-failure revert (8.10): the page's Buttons slice is CAPTURED before
//     WriteSlot. On SaveConfig failure the in-memory page is RESTORED to that
//     snapshot so the deck shows the PREVIOUS settings, and the error is logged.
//     (The deck has no error line; the visible "error indication" is that the
//     tile is unchanged and the failure is logged — the write is fully reverted
//     so on-disk and in-memory both retain the previous settings.)
//   - On success the menu closes and the deck repaints with the pasted button.
func (r *Renderer) menuPaste() {
	cb := r.state.Clipboard
	if cb == nil {
		return
	}
	slot := r.state.Menu.Slot

	// Keep the existing occupant's id if present, else generate one.
	id := ""
	if existing, ok := core.ButtonAtSlotOnPage(r.state.Config, r.state.CurrentPage, slot); ok {
		id = existing.ID
	}
	if id == "" {
		id = r.generateButtonID()
	}

	btn := core.ButtonConfig{
		ID:        id,
		Label:     cb.Label,
		Command:   cb.Command,
		BgImage:   cb.BgImage,
		BgColor:   cb.BgColor,
		FontColor: cb.FontColor,
		FontSize:  cb.FontSize,
		Order:     slot,
	}

	idx := r.currentPageIndex()
	// Snapshot the page's buttons for the save-failure revert (8.10).
	prevButtons := r.state.Config.Pages[idx].Buttons

	r.state.Config.Pages[idx] = core.WriteSlot(r.state.Config.Pages[idx], slot, btn)

	if err := r.state.Backend.SaveConfig(r.state.Config); err != nil {
		// Revert the in-memory mutation so the deck retains the previous
		// settings (Requirement 8.10), and log the failure as the error
		// indication (the runtime deck has no dedicated error UI).
		r.state.Config.Pages[idx].Buttons = prevButtons
		log.Printf("deck: paste to slot %d on page %d failed to save (reverted): %v", slot, r.state.CurrentPage, err)
	}

	r.closeMenu()
}
