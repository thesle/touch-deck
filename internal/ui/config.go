// This file implements the Config_View split-pane layout and live grid preview
// (task 14.1) per the design's "Detailed Component Design -> Config_View"
// (Split-pane layout 9.1; Grid preview + Pagination-slot previews 9.8) and the
// Svelte Configuration view in frontend/src/App.svelte.
//
// Scope of task 14.1 (DISPLAY-ONLY skeleton):
//   - A horizontal split-pane: LEFT = live grid preview, RIGHT = slot editor
//     area (a placeholder prompt for now; the real form fields are task 15.x).
//   - The LEFT pane renders the current page's slots as preview tiles reflecting
//     each button's label/bgColor and the current selection highlight, matching
//     the Svelte preview (Requirement 9.1).
//   - The two reserved Pagination_Slots render as NON-editable, dashed/muted
//     "[Prev Page]" / "[Next Page]" preview tiles (Requirement 9.8).
//
// Explicitly NOT implemented here (later tasks):
//   - rows/cols adjusters (14.2),
//   - page selector + slot-click selection loading the editor (14.3),
//   - editor form fields / save / clear (15.x),
//   - move / copy / paste / test-run (16.x).
//
// Because slot SELECTION is task 14.3, the preview here is display-only: no
// pointer/click handlers are wired. The selection HIGHLIGHT is still rendered
// from the current AppState.SelectedSlot value so that once 14.3 sets it, the
// highlight already works. No widget.Clickable scaffolding is added for the
// preview cells to keep this task minimal; 14.3 will add the click wiring.
//
// Config_View widget state: a dedicated per-view struct `configState` (below)
// holds any Config_View widget state, referenced from the Renderer via a single
// `cfg configState` field (added to render.go). This keeps AppState a pure data
// model and avoids growing the shared Renderer struct beyond one field. For
// 14.1 the struct is intentionally empty (no live widgets yet); adjusters, page
// buttons, and slot clickables are added to it by 14.2/14.3.
package ui

import (
	"image"
	"image/color"
	"log"
	"math/rand"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

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

// configState holds the Config_View's Gio widget state. It lives on the
// Renderer (one field: r.cfg) rather than on AppState so AppState stays a pure
// "what to draw" data model. Task 14.2 adds the four rows/cols adjuster
// buttons; the page-selector buttons and per-slot clickables (14.3),
// editor-field widgets (15.x), and action buttons (16.x) are added here by
// their respective tasks.
type configState struct {
	// Grid-size adjuster buttons (task 14.2, Requirements 9.2–9.4). Four
	// Clickables driving the "Rows: [-] N [+]" / "Cols: [-] N [+]" control bar
	// in the left pane. Their Clicked() events MUST be drained BEFORE they are
	// laid out each frame — see handleAdjusters — because in Gio v0.10.2
	// widget.Clickable.Layout begins by draining and discarding pending click
	// events, so a Clicked() check placed AFTER Layout swallows the click.
	rowsDec widget.Clickable
	rowsInc widget.Clickable
	colsDec widget.Clickable
	colsInc widget.Clickable

	// Page-selector buttons (task 14.3, Requirements 9.5, 9.6). One Clickable
	// per page ("Page 1".."Page 5"). There are exactly TotalPages of them and
	// they are stable across frames, so a fixed array is the right storage
	// (unlike the dynamic per-slot clickables below). Like the adjusters, their
	// Clicked() events MUST be drained BEFORE they are laid out each frame — see
	// handlePageSelector — because in Gio v0.10.2 widget.Clickable.Layout drains
	// and discards pending click events.
	pageBtns [TotalPages]widget.Clickable

	// Per-slot selection clickables (task 14.3, Requirements 9.7, 9.8). The
	// config grid is dynamic (rows/cols change), so these are allocated lazily
	// and keyed by slot INDEX rather than being a fixed array, mirroring
	// deck.go's tapClickable map pattern. Selection is by slot position — the
	// editor loads whatever button currently occupies that slot — so keying by
	// index is correct here. See slotClickable. Pagination slots get NO entry
	// (they are non-selectable, Requirement 9.8). Each cell's Clicked() is
	// drained BEFORE that cell's Layout in layoutConfigPreviewSlot.
	slotClicks map[int]*widget.Clickable

	// --- Editor form widgets (task 15.1, Requirements 10.1–10.5, 13.4–13.6) ---
	//
	// Construction strategy. configState is a value field on the Renderer
	// (r.cfg), so its widgets are zero values until first used. The two text
	// editors (labelEd/commandEd) are plain widget.Editor values that only need
	// their SingleLine flag configured, so they need no constructor. The two
	// ColorPickers and the FontSizeSelector DO have constructors, so they are
	// stored as POINTERS and lazily built on first use in ensureEditorWidgets
	// (nil-check). This keeps the "construct via the widget's own constructor"
	// contract while letting configState stay a value field. ensureEditorWidgets
	// is called at the top of both selectSlot's sync and layoutConfigRight so
	// the widgets always exist before they are synced or laid out.

	// labelEd is the single-line editor for Button.label (Requirement 10.1).
	labelEd widget.Editor
	// commandEd is the multi-line editor for Button.command (Requirement 10.2).
	commandEd widget.Editor

	// bgColor / fontColor are the tile-color and text-color pickers
	// (Requirements 10.3, 10.4). Pointers, lazily built in ensureEditorWidgets.
	bgColor   *ColorPicker
	fontColor *ColorPicker

	// fontSize is the -2..+2 segmented selector (Requirement 10.5). Pointer,
	// lazily built in ensureEditorWidgets.
	fontSize *FontSizeSelector

	// --- Background-image selection controls (Requirements 13.4–13.6) ---
	//
	// Gio v0.10.2 has no native dropdown/combo widget, so the Svelte <select> of
	// existing images is reimplemented as: a "Choose File" button (native
	// dialog, Req 13.1–13.3), a "Clear" button (Req 13.6), and a scrollable list
	// of the existing image filenames rendered as clickable rows (Req 13.4/13.5)
	// that set Editor.BgImage to that image's path. This is usable without a
	// popup and fits the immediate-mode model. All three sets of controls follow
	// drain-before-layout (their Clicked() are read before Layout).
	chooseFileBtn widget.Clickable
	clearImgBtn   widget.Clickable

	// imgRowClicks holds one Clickable per existing-image path (the selectable
	// rows). Keyed by path and allocated lazily (imgRowClickable), mirroring the
	// slotClicks pattern, because the set of images is dynamic (grows when the
	// user chooses a new file). Stale entries for images that no longer exist
	// are harmless (never laid out) so no pruning is needed.
	imgRowClicks map[string]*widget.Clickable

	// imgList is the scroll state for the existing-images list.
	imgList widget.List

	// existingImages caches the result of Backend.ListConfigImages so the images
	// directory is not re-read every frame (it would be a wasteful syscall per
	// frame). It is (re)loaded in refreshImages, which is called on slot select
	// (via syncEditorWidgets) and after a successful choose/clear so the list
	// stays current without per-frame directory reads.
	existingImages []string
	// imagesLoaded records whether existingImages has been populated at least
	// once, so the first layout triggers an initial load even if no slot-select
	// happened yet.
	imagesLoaded bool

	// --- Save / Clear action buttons (task 15.2, Requirements 10.6–10.8) ---
	//
	// saveBtn persists the editor's current values into the selected slot on the
	// current page (Requirement 10.6); clearBtn removes the button at the
	// selected slot (Requirement 10.7). Like every other Config_View control
	// their Clicked() events are drained BEFORE the buttons are laid out each
	// frame (drain-before-layout), at the TOP of layoutEditorForm — because in
	// Gio v0.10.2 widget.Clickable.Layout drains and discards pending click
	// events, so a Clicked() check placed after Layout would swallow the click.
	saveBtn  widget.Clickable
	clearBtn widget.Clickable

	// --- Move controls (task 16.1, Requirements 11.1–11.4) ---
	//
	// Four arrow Clickables (← ↑ ↓ →) that move the selected slot's button into
	// the adjacent slot in that direction (via core.MoveButton). Like every other
	// Config_View control their Clicked() events are drained BEFORE the buttons
	// are laid out each frame (drain-before-layout), at the TOP of
	// layoutEditorForm — because in Gio v0.10.2 widget.Clickable.Layout drains and
	// discards pending click events, so a Clicked() check placed after Layout
	// would swallow the click.
	moveLeft  widget.Clickable
	moveUp    widget.Clickable
	moveDown  widget.Clickable
	moveRight widget.Clickable

	// --- Copy / paste controls (task 16.2, Requirements 11.5–11.7) ---
	//
	// copyBtn stores the editor's current settings into the SHARED
	// r.state.Clipboard (*ButtonClipboard on AppState — also used by the Deck_View
	// context menu, task 11); pasteBtn applies that clipboard to the selected slot
	// and saves. Paste is presented as unavailable (greyed + click ignored) when
	// the shared clipboard is empty (Requirement 11.7). Drained at the TOP of
	// layoutEditorForm.
	copyBtn  widget.Clickable
	pasteBtn widget.Clickable

	// --- Test-run panel (task 16.3, Requirement 12) ---
	//
	// testRunBtn triggers a synchronous test run of the current editor command via
	// Backend.RunCommandSync, executed on a GOROUTINE so the (up to 10s) run never
	// blocks the UI thread (Requirement 12.5). The result of the goroutine is
	// handed back to the UI thread through the mutex-guarded testResult holder
	// below rather than by writing r.state.CommandTest directly, so all
	// AppState.CommandTest mutation stays on the single frame-loop thread.
	testRunBtn widget.Clickable

	// testMu guards testResult against the goroutine writing it and the UI thread
	// reading it. testResult is nil until a run finishes; the goroutine fills it
	// under the lock and calls r.w.Invalidate(), and the top of layoutEditorForm
	// (on the UI thread) drains it under the lock into r.state.CommandTest and
	// clears it. This keeps the CommandTest data model mutation entirely on the UI
	// thread while the blocking command runs off it.
	testMu     sync.Mutex
	testResult *testRunResult

	// cfgTheme is a Config_View-private material.Theme with a DARK-appropriate
	// palette (light Fg, dark Bg), created lazily by ensureCfgTheme. The shared
	// r.th theme is the default light-ish palette (dark Fg on light Bg), which is
	// why material widgets — especially material.Editor text and the ColorPicker
	// hex fields — render (near-)black text that is unreadable on the dark editor
	// pane. Rather than mutate the shared theme (which would affect the header,
	// deck, and every other view), the Config_View renders ALL of its material
	// widgets (editors, color pickers, font-size selector, buttons, captions,
	// labels) through this private clone so the whole editor pane is readable at
	// once — including the ColorPicker's INTERNAL editor, because its Layout takes
	// a *material.Theme parameter that we pass cfgTheme to. The left pane preview
	// keeps using the hand-picked cfg* colors (fine as-is). It is a pointer,
	// lazily built once, mirroring the other constructor-backed widgets.
	cfgTheme *material.Theme

	// saveErr holds the most recent save/clear persistence error message, or ""
	// when the last write succeeded (Requirement 10.8, refined). When non-empty
	// it is rendered as a red line below the action buttons so the user knows the
	// on-disk config was NOT updated and can retry. On a successful write it is
	// cleared. Note: a failed SaveConfig leaves the previously persisted (on-disk)
	// config unchanged — that failure itself satisfies "retain the previously
	// persisted config" — while the in-memory edits are kept so the user does not
	// lose their work and can retry.
	saveErr string
}

// testRunResult is the completed result of a background test run (task 16.3). The
// RunCommandSync goroutine fills one of these under configState.testMu and calls
// Invalidate; the next frame drains it into r.state.CommandTest on the UI thread
// and clears configState.testResult. Output is the combined stdout/stderr (plus
// any error text) and Success reports whether the command exited without error.
type testRunResult struct {
	Output  string
	Success bool
}

// Config_View palette. These mirror the Svelte Configuration view's Tailwind
// colors (frontend/src/App.svelte) so the native preview matches the WebView it
// replaces:
//   - cfgLeftBg        #0b0f19  left preview pane background
//   - cfgRightBg       #111827  right editor pane background
//   - cfgPaneBorder    #374151  vertical divider between panes
//   - cfgHeadingFg     #d1d5db  gray-300 pane headings
//   - cfgSelectedBg    #1e293b  selected preview tile fill
//   - cfgSelectedBorder#3b82f6  blue-500 selected tile border
//   - cfgTileBorder    #374151  configured tile border
//   - cfgEmptyBg       #111827  unconfigured tile fill (~30% alpha)
//   - cfgEmptyBorder   gray-700 @ ~50% unconfigured (dashed-approximation) border
//   - cfgEmptyFg       #9ca3af  gray-400 "+ Slot N" text on empty tiles
//   - cfgNavBg         #1f2937  pagination preview fill (~50% alpha)
//   - cfgNavBorder     #4b5563  pagination preview (dashed-approximation) border
//   - cfgNavFg         #60a5fa  blue-400 "[Prev/Next Page]" text
//   - cfgPromptFg      #6b7280  gray-500 "select a slot" prompt text
//   - cfgDefaultFontFg #ffffff  fallback label color when fontColor is unset
var (
	cfgLeftBg         = color.NRGBA{R: 0x0b, G: 0x0f, B: 0x19, A: 0xff}
	cfgRightBg        = color.NRGBA{R: 0x11, G: 0x18, B: 0x27, A: 0xff}
	cfgPaneBorder     = color.NRGBA{R: 0x37, G: 0x41, B: 0x51, A: 0xff}
	cfgHeadingFg      = color.NRGBA{R: 0xd1, G: 0xd5, B: 0xdb, A: 0xff}
	cfgSelectedBg     = color.NRGBA{R: 0x1e, G: 0x29, B: 0x3b, A: 0xff}
	cfgSelectedBorder = color.NRGBA{R: 0x3b, G: 0x82, B: 0xf6, A: 0xff}
	cfgTileBorder     = color.NRGBA{R: 0x37, G: 0x41, B: 0x51, A: 0xff}
	cfgEmptyBg        = color.NRGBA{R: 0x11, G: 0x18, B: 0x27, A: 0x4d}
	cfgEmptyBorder    = color.NRGBA{R: 0x37, G: 0x41, B: 0x51, A: 0xcc}
	cfgEmptyFg        = color.NRGBA{R: 0x9c, G: 0xa3, B: 0xaf, A: 0xff}
	cfgNavBg          = color.NRGBA{R: 0x1f, G: 0x29, B: 0x37, A: 0x80}
	cfgNavBorder      = color.NRGBA{R: 0x4b, G: 0x55, B: 0x63, A: 0xff}
	cfgNavFg          = color.NRGBA{R: 0x60, G: 0xa5, B: 0xfa, A: 0xff}
	cfgPromptFg       = color.NRGBA{R: 0x6b, G: 0x72, B: 0x80, A: 0xff}
	cfgDefaultFontFg  = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	// cfgClearBtnBg is the muted-red fill for the destructive "Clear Slot" button
	// (red-700 #b91c1c), and cfgErrorFg is the red-400 (#f87171) text used for the
	// save-failure line (task 15.2, Requirement 10.8).
	cfgClearBtnBg = color.NRGBA{R: 0xb9, G: 0x1c, B: 0x1c, A: 0xff}
	cfgErrorFg    = color.NRGBA{R: 0xf8, G: 0x71, B: 0x71, A: 0xff}

	// cfgTestBtnBg is the amber fill (yellow-600 #ca8a04) for the inline "Test
	// Run" button, matching the Svelte "⚡ Test Run Now" button. It gives the
	// button a distinct, visible color (with white cfgDefaultFontFg text) that
	// reads clearly as clickable and is distinguishable from the primary blue
	// "Save Changes" and the destructive red "Clear Slot".
	cfgTestBtnBg = color.NRGBA{R: 0xca, G: 0x8a, B: 0x04, A: 0xff}

	// Test-run panel colors (task 16.3, Requirement 12): green (#34d399) for
	// success output, red (#f87171 — reuses cfgErrorFg's tone) for failure
	// output, and a muted gray for the "executing" / "no command" hints. A
	// disabled-control tint (cfgDisabledBg / cfgDisabledFg) greys the Paste and
	// Test Run buttons when they are unavailable (empty clipboard / running).
	cfgTestSuccessFg = color.NRGBA{R: 0x34, G: 0xd3, B: 0x99, A: 0xff}
	cfgTestFailFg    = color.NRGBA{R: 0xf8, G: 0x71, B: 0x71, A: 0xff}
	cfgTestHintFg    = color.NRGBA{R: 0x9c, G: 0xa3, B: 0xaf, A: 0xff}
	cfgDisabledBg    = color.NRGBA{R: 0x37, G: 0x41, B: 0x51, A: 0xff}
	cfgDisabledFg    = color.NRGBA{R: 0x6b, G: 0x72, B: 0x80, A: 0xff}

	// Editor-pane palette for the Config_View-private cfgTheme (Issue 2 —
	// readable text fields on the dark right pane). material widgets rendered
	// through cfgTheme use these so their text is light on dark:
	//   - cfgThemeFg        #e5e7eb  gray-200 default foreground (editor text, labels)
	//   - cfgThemeBg        #111827  dark pane background (matches cfgRightBg)
	//   - cfgThemeContrastBg #3b82f6 blue-500 for active/important buttons
	//   - cfgThemeHint      #9ca3af  muted gray-400 for hint/placeholder text
	// Field-box colors draw a visible input box behind each text editor so it is
	// an obvious place to type:
	//   - cfgFieldBg        #1f2937  gray-800 field fill
	//   - cfgFieldBorder    #374151  gray-700 field border
	cfgThemeFg         = color.NRGBA{R: 0xe5, G: 0xe7, B: 0xeb, A: 0xff}
	cfgThemeBg         = color.NRGBA{R: 0x11, G: 0x18, B: 0x27, A: 0xff}
	cfgThemeContrastBg = color.NRGBA{R: 0x3b, G: 0x82, B: 0xf6, A: 0xff}
	cfgThemeHint       = color.NRGBA{R: 0x9c, G: 0xa3, B: 0xaf, A: 0xff}
	cfgFieldBg         = color.NRGBA{R: 0x1f, G: 0x29, B: 0x37, A: 0xff}
	cfgFieldBorder     = color.NRGBA{R: 0x37, G: 0x41, B: 0x51, A: 0xff}
)

// handleAdjusters drains the four grid-size adjuster Clickables and applies any
// resulting row/col change (task 14.2, Requirements 9.2–9.4). It MUST be called
// BEFORE the adjuster buttons are laid out this frame: in Gio v0.10.2
// widget.Clickable.Layout begins with an update loop that drains and discards
// pending click events, so a Clicked() check placed after Layout would swallow
// the click and never fire the handler. Because the click is drained here — at
// the top of layoutConfigLeft, before the button Layout calls — the mutation is
// visible on the SAME frame.
//
// Each Clicked() is drained in a for-loop so multiple queued presses in a single
// frame each apply (matching the header/deck ordering established elsewhere).
func (r *Renderer) handleAdjusters(gtx layout.Context) {
	for r.cfg.rowsDec.Clicked(gtx) {
		r.adjustRows(-1)
	}
	for r.cfg.rowsInc.Clicked(gtx) {
		r.adjustRows(+1)
	}
	for r.cfg.colsDec.Clicked(gtx) {
		r.adjustCols(-1)
	}
	for r.cfg.colsInc.Clicked(gtx) {
		r.adjustCols(+1)
	}
}

// handlePageSelector drains the five page-selector Clickables and applies any
// resulting page switch (task 14.3, Requirements 9.5, 9.6). Like
// handleAdjusters it MUST run BEFORE the page buttons are laid out this frame,
// because widget.Clickable.Layout (Gio v0.10.2) drains and discards pending
// click events, so a Clicked() check placed after Layout would swallow the
// click. Because the switch is applied here — at the top of layoutConfigLeft,
// before the page-button Layout calls — the page bar and preview drawn this
// frame reflect the newly selected page.
func (r *Renderer) handlePageSelector(gtx layout.Context) {
	for i := range r.cfg.pageBtns {
		for r.cfg.pageBtns[i].Clicked(gtx) {
			r.selectPage(i)
		}
	}
}

// selectPage switches the Config_View to page i and clears the current slot
// selection (Requirement 9.6): selecting a page shows that page's slots fresh,
// so a stale selection from the previous page must not carry over. A page that
// is already current still clears the selection (harmless and matches "shown
// fresh"). r.w.Invalidate() forces the next frame so the preview repaints for
// the new page even if no other event arrives (mirrors the pagination fix in
// deck.go).
func (r *Renderer) selectPage(i int) {
	r.state.CurrentPage = i
	r.state.SelectedSlot = nil
	// Clearing the selection also clears any stale test-run result (and pending
	// goroutine result) for consistency, so switching pages doesn't leave the
	// previous slot's test Output/Success showing.
	r.state.CommandTest = CommandTestState{}
	r.cfg.testMu.Lock()
	r.cfg.testResult = nil
	r.cfg.testMu.Unlock()
	if r.w != nil {
		r.w.Invalidate()
	}
}

// adjustRows changes the row count by delta (±1), clamped to a minimum of 1
// (Requirement 9.3; the spec sets no maximum). A no-op delta (already at the
// minimum) leaves the config untouched. On a real change it delegates to
// applyGridSize, which prunes out-of-bounds buttons and saves.
func (r *Renderer) adjustRows(delta int) {
	newRows := r.state.Config.Rows + delta
	if newRows < 1 {
		return
	}
	if newRows == r.state.Config.Rows {
		return
	}
	r.applyGridSize(newRows, r.state.Config.Cols)
}

// adjustCols changes the column count by delta (±1), clamped to a minimum of 1
// (Requirement 9.3; the spec sets no maximum). See adjustRows.
func (r *Renderer) adjustCols(delta int) {
	newCols := r.state.Config.Cols + delta
	if newCols < 1 {
		return
	}
	if newCols == r.state.Config.Cols {
		return
	}
	r.applyGridSize(r.state.Config.Rows, newCols)
}

// applyGridSize applies a new (rows, cols) grid size to the in-memory config,
// pruning any button whose order is out of the new bounds from EVERY page, then
// persisting the result (Requirement 9.4).
//
// core.PruneToGrid prunes based on the rows/cols passed to it but copies the
// input config's Rows/Cols onto the result unchanged, so we set the new Rows/Cols
// on the pruned config before assigning it back — the saved config therefore
// carries both the new dimensions AND the pruned button set.
//
// If the currently selected slot no longer exists in the new grid (its index is
// >= rows*cols), the selection is cleared so a stale out-of-bounds slot is not
// left selected for the editor. A SaveConfig failure is logged but the in-memory
// config is kept so the UI stays consistent with what the user sees.
func (r *Renderer) applyGridSize(newRows, newCols int) {
	newCfg := core.PruneToGrid(r.state.Config, newRows, newCols)
	newCfg.Rows = newRows
	newCfg.Cols = newCols
	r.state.Config = newCfg

	// Clear a now-out-of-bounds selection so the editor never targets a slot
	// that no longer exists in the resized grid.
	if r.state.SelectedSlot != nil && *r.state.SelectedSlot >= newRows*newCols {
		r.state.SelectedSlot = nil
	}

	if err := r.state.Backend.SaveConfig(r.state.Config); err != nil {
		log.Printf("config: save after grid resize to %dx%d failed: %v", newRows, newCols, err)
	}

	// Widget click events already schedule a frame, and because the mutation
	// above happens before this frame's layout the change is drawn immediately.
	// An explicit Invalidate is harmless and mirrors the pagination fix, keeping
	// the redraw guaranteed regardless of event batching.
	if r.w != nil {
		r.w.Invalidate()
	}
}

// layoutConfig renders the Config_View split-pane (task 14.1, Requirement 9.1).
// It lays out two equal panes with a horizontal layout.Flex: the LEFT pane is
// the live grid preview, the RIGHT pane is the slot editor area (a placeholder
// prompt until the form fields land in 15.x). A thin vertical divider separates
// them, mirroring the Svelte `border-r` between the two halves.
//
// This method REPLACES the skeleton placeholder that previously lived in
// render.go; render.go no longer defines layoutConfig (see the task report).
func (r *Renderer) layoutConfig(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return r.layoutConfigLeft(gtx)
		}),
		// Thin vertical divider between the two panes.
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			w := gtx.Dp(unit.Dp(1))
			paintRect(gtx, image.Point{X: w, Y: gtx.Constraints.Max.Y}, cfgPaneBorder)
			return layout.Dimensions{Size: image.Point{X: w, Y: gtx.Constraints.Max.Y}}
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return r.layoutConfigRight(gtx)
		}),
	)
}

// layoutConfigLeft renders the left pane: a "Live Grid Preview" heading above
// the live grid preview area (Requirement 9.1). The rows/cols adjusters and
// page selector controls that the Svelte view shows here are tasks 14.2/14.3 and
// are intentionally omitted; the heading + preview establish the pane so those
// controls can slot in above/around the preview later.
func (r *Renderer) layoutConfigLeft(gtx layout.Context) layout.Dimensions {
	fillBackground(gtx, cfgLeftBg)

	// Drain the adjuster click events BEFORE any of the four adjuster buttons
	// are laid out below (critical ordering — see handleAdjusters). This applies
	// a pending row/col change so the heading, adjuster bar, and preview drawn
	// this frame all reflect the new grid size.
	r.handleAdjusters(gtx)

	// Drain the page-selector click events BEFORE the page buttons are laid out
	// below (same critical drain-before-layout ordering as the adjusters — see
	// handlePageSelector). A pending page switch (and the selection clear it
	// implies) is therefore applied so the page bar and preview drawn this frame
	// reflect the newly selected page.
	r.handlePageSelector(gtx)

	// Issue 1 — breathing room: a 16dp outer inset around the whole left-pane
	// content (was 12dp) so the heading/controls/preview are not flush against
	// the pane edges, plus a small extra inset around the preview grid itself
	// (below) so the tiles are not edge-to-edge with the controls/divider.
	return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				h := material.Body1(r.th, "Live Grid Preview")
				h.Color = cfgHeadingFg
				return h.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
			// Rows/Cols adjuster control bar (task 14.2, Requirements 9.2–9.4).
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return r.layoutConfigAdjusters(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
			// Page selector bar (task 14.3, Requirements 9.5, 9.6). Sits between
			// the grid-size adjusters and the preview, matching the Svelte order
			// (grid-size controls, then the page bar, then the preview).
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return r.layoutConfigPageSelector(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				// Extra inset around the preview grid so tiles aren't flush
				// against the controls above or the pane/divider edges (Issue 1).
				return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return r.layoutConfigPreview(gtx)
				})
			}),
		)
	})
}

// layoutConfigAdjusters renders the grid-size control bar shown above the
// preview (Requirement 9.2): "Rows: [-] N [+]" then "Cols: [-] N [+]",
// displaying the current r.state.Config.Rows / Cols. The four `-`/`+` controls
// are the configState adjuster Clickables whose events were already drained in
// handleAdjusters (drain-before-layout ordering), so laying them out here only
// paints them — it never swallows an unhandled click. The `-`/`+` controls use
// small material.Button controls; the labels and numbers use material.Body1 in
// the dark-theme heading color.
func (r *Renderer) layoutConfigAdjusters(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return r.layoutAdjuster(gtx, "Rows:", r.state.Config.Rows, &r.cfg.rowsDec, &r.cfg.rowsInc)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(24)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return r.layoutAdjuster(gtx, "Cols:", r.state.Config.Cols, &r.cfg.colsDec, &r.cfg.colsInc)
		}),
	)
}

// layoutConfigPageSelector renders the five page buttons ("Page 1".."Page 5")
// shown between the adjuster bar and the preview (task 14.3, Requirements 9.5,
// 9.6). The button for the current r.state.CurrentPage is drawn in the theme's
// contrast color (active) while the others use the muted bg/fg pair, mirroring
// the header's active/inactive Deck/Config button styling in render.go. The
// buttons' Clicked() events were already drained in handlePageSelector
// (drain-before-layout), so laying them out here only paints them.
func (r *Renderer) layoutConfigPageSelector(gtx layout.Context) layout.Dimensions {
	children := make([]layout.FlexChild, 0, TotalPages*2-1)
	for i := 0; i < TotalPages; i++ {
		i := i
		if i > 0 {
			children = append(children, layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout))
		}
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			b := material.Button(r.th, &r.cfg.pageBtns[i], "Page "+strconv.Itoa(i+1))
			b.Inset = layout.UniformInset(unit.Dp(6))
			b.TextSize = unit.Sp(13)
			if r.state.CurrentPage != i {
				// Inactive page: muted styling, matching the header buttons.
				b.Background = r.th.Palette.Bg
				b.Color = r.th.Palette.Fg
			}
			return b.Layout(gtx)
		}))
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
}

// layoutAdjuster renders one labelled adjuster group: "<label> [-] <value> [+]".
// dec/inc are the two Clickables for this group (already drained this frame).
func (r *Renderer) layoutAdjuster(gtx layout.Context, label string, value int, dec, inc *widget.Clickable) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			l := material.Body1(r.th, label)
			l.Color = cfgHeadingFg
			return l.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return r.layoutAdjusterButton(gtx, dec, "-")
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			n := material.Body1(r.th, strconv.Itoa(value))
			n.Color = cfgHeadingFg
			return layout.Inset{Left: unit.Dp(2), Right: unit.Dp(2)}.Layout(gtx, n.Layout)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return r.layoutAdjusterButton(gtx, inc, "+")
		}),
	)
}

// layoutAdjusterButton lays out one small `-`/`+` adjuster control. Its
// Clicked() events are NOT read here — they were drained in handleAdjusters
// before this Layout call — so this only paints the control.
func (r *Renderer) layoutAdjusterButton(gtx layout.Context, btn *widget.Clickable, label string) layout.Dimensions {
	b := material.Button(r.th, btn, label)
	b.Inset = layout.UniformInset(unit.Dp(4))
	b.TextSize = unit.Sp(14)
	return b.Layout(gtx)
}

// layoutConfigPreview lays out the current page's slots as a rows x cols grid of
// preview tiles (Requirement 9.1). It reuses the same nested-Flex approach as
// the Deck_View (a vertical Flex of rows, each a horizontal Flex of equal
// cells) but with its own preview-specific cell renderer so deck.go is not
// touched. Slot index i = row*cols + col; the last two slots are the
// Pagination_Slots and render as non-editable previews (Requirement 9.8).
func (r *Renderer) layoutConfigPreview(gtx layout.Context) layout.Dimensions {
	rows := r.state.Config.Rows
	cols := r.state.Config.Cols

	// A malformed/empty grid has nothing to lay out; report the area so the
	// frame stays valid.
	if rows < 1 || cols < 1 {
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}

	prevSlot := core.PrevPageSlot(rows, cols)
	nextSlot := core.NextPageSlot(rows, cols)

	rowChildren := make([]layout.FlexChild, 0, rows*2-1)
	for rIdx := 0; rIdx < rows; rIdx++ {
		rIdx := rIdx
		if rIdx > 0 {
			rowChildren = append(rowChildren, layout.Rigid(layout.Spacer{Height: unit.Dp(tileGapDp)}.Layout))
		}
		rowChildren = append(rowChildren, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return r.layoutConfigPreviewRow(gtx, rIdx, cols, prevSlot, nextSlot)
		}))
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rowChildren...)
}

// layoutConfigPreviewRow lays out a single preview row: C equal cells separated
// by gaps.
func (r *Renderer) layoutConfigPreviewRow(gtx layout.Context, row, cols, prevSlot, nextSlot int) layout.Dimensions {
	cellChildren := make([]layout.FlexChild, 0, cols*2-1)
	for cIdx := 0; cIdx < cols; cIdx++ {
		i := core.RowColToSlot(row, cIdx, cols)
		if cIdx > 0 {
			cellChildren = append(cellChildren, layout.Rigid(layout.Spacer{Width: unit.Dp(tileGapDp)}.Layout))
		}
		cellChildren = append(cellChildren, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return r.layoutConfigPreviewSlot(gtx, i, prevSlot, nextSlot)
		}))
	}
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, cellChildren...)
}

// layoutConfigPreviewSlot renders a single preview cell. Pagination slots render
// as non-editable previews (Requirement 9.8); every other slot resolves the
// button occupying it (if any) and renders a selectable-looking tile whose
// appearance reflects the button's label/color and the current selection state
// (Requirement 9.1). Selection SETTING is task 14.3; here we only read
// AppState.SelectedSlot to draw the highlight.
func (r *Renderer) layoutConfigPreviewSlot(gtx layout.Context, slot, prevSlot, nextSlot int) layout.Dimensions {
	switch slot {
	case prevSlot:
		// Pagination slots are non-editable and non-selectable (Requirement
		// 9.8): they get NO clickable, so they can never be selected.
		return r.layoutConfigNavPreview(gtx, "[Prev Page]")
	case nextSlot:
		return r.layoutConfigNavPreview(gtx, "[Next Page]")
	default:
		btn, ok := core.ButtonAtSlotOnPage(r.state.Config, r.state.CurrentPage, slot)
		// Wrap the content tile in its per-slot Clickable so it is selectable
		// (task 14.3, Requirement 9.7). Drain Clicked() BEFORE Layout — the same
		// critical ordering as the deck tiles and adjusters — because
		// widget.Clickable.Layout (Gio v0.10.2) drains and discards pending
		// click events, so a Clicked() check after Layout would swallow the tap.
		c := r.slotClickable(slot)
		for c.Clicked(gtx) {
			r.selectSlot(slot)
		}
		return c.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return r.layoutConfigPreviewTile(gtx, slot, btn, ok)
		})
	}
}

// slotClickable returns the widget.Clickable owning the preview tile at the
// given slot index, allocating it (and the backing map) lazily on first use
// (task 14.3, mirroring deck.go's tapClickable). The config grid is dynamic, so
// the clickables are keyed by slot index rather than stored in a fixed array;
// selection is by slot position (the editor loads whatever button occupies the
// slot). Pagination slots never call this helper, so they get no clickable and
// stay non-selectable (Requirement 9.8).
func (r *Renderer) slotClickable(slot int) *widget.Clickable {
	if r.cfg.slotClicks == nil {
		r.cfg.slotClicks = make(map[int]*widget.Clickable)
	}
	c, ok := r.cfg.slotClicks[slot]
	if !ok {
		c = new(widget.Clickable)
		r.cfg.slotClicks[slot] = c
	}
	return c
}

// selectSlot selects the given slot for editing and loads the editor state from
// the button occupying it, mirroring the Svelte selectSlot (task 14.3,
// Requirement 9.7).
//
//   - Pagination slots are non-selectable (Requirement 9.8). Content tiles are
//     the only slots wrapped in a clickable, so this guard is defensive; it
//     ensures a pagination slot can never end up selected regardless of caller.
//   - A copy of the slot index is stored so AppState.SelectedSlot points at a
//     stable value (never at a loop variable that would later change).
//   - If a button occupies the slot on the current page, its fields are copied
//     into Editor; empty bgColor/fontColor fall back to the Svelte defaults
//     (#1f2937 / #ffffff).
//   - If the slot is empty, Editor is initialized to a NEW button template with
//     a freshly generated id and the default colors, ready for the 15.x form to
//     render and eventually save via writeSlot.
//
// Task 15.x will render the form fields from Editor and, when the ColorPicker /
// FontSizeSelector widget instances exist, push these values into them on
// selection (SetText / value). 14.3 only populates the data model.
//
// r.w.Invalidate() forces the next frame so the preview highlight and the right
// pane repaint immediately (mirrors the pagination fix in deck.go).
func (r *Renderer) selectSlot(slot int) {
	if core.IsPaginationSlot(slot, r.state.Config.Rows, r.state.Config.Cols) {
		return
	}

	s := slot
	r.state.SelectedSlot = &s

	// Clear any stale test-run result from the previously selected slot so the
	// newly selected slot does NOT show the old slot's Output/Success. Reset the
	// data-model state to its zero value, and also discard any pending goroutine
	// result under testMu so a late-arriving result from the old slot's in-flight
	// run does not repopulate the panel on the new slot. (The running command
	// itself can't be cancelled — RunCommandSync has its own 10s timeout — but
	// discarding its result on slot change is the correct UX.)
	r.state.CommandTest = CommandTestState{}
	r.cfg.testMu.Lock()
	r.cfg.testResult = nil
	r.cfg.testMu.Unlock()

	if btn, ok := core.ButtonAtSlotOnPage(r.state.Config, r.state.CurrentPage, slot); ok {
		bgColor := btn.BgColor
		if bgColor == "" {
			bgColor = defaultEditorBgColor
		}
		fontColor := btn.FontColor
		if fontColor == "" {
			fontColor = defaultEditorFontColor
		}
		r.state.Editor = EditorState{
			ID:        btn.ID,
			Label:     btn.Label,
			Command:   btn.Command,
			BgImage:   btn.BgImage,
			BgColor:   bgColor,
			FontColor: fontColor,
			FontSize:  btn.FontSize,
		}
	} else {
		// Empty slot: start a fresh button template with a generated id and the
		// default colors (Requirement 9.7).
		r.state.Editor = EditorState{
			ID:        r.generateButtonID(),
			BgColor:   defaultEditorBgColor,
			FontColor: defaultEditorFontColor,
			FontSize:  0,
		}
	}

	// Push the freshly loaded Editor data into the live editor WIDGETS so the
	// form renders the selected slot's values (task 15.1). Without this the
	// widgets would keep whatever the previous slot left in them.
	r.syncEditorWidgets()

	if r.w != nil {
		r.w.Invalidate()
	}
}

// ensureEditorWidgets lazily constructs the editor widgets that have
// constructors (the two ColorPickers and the FontSizeSelector) and configures
// the plain widget.Editor fields (task 15.1). It is idempotent and cheap, so it
// is safe to call at the top of both syncEditorWidgets and layoutConfigRight;
// the nil-checks make the first call build the widgets and every later call a
// no-op. The initial values seeded here are the current Editor values so a
// widget built for the first time already reflects the selected slot.
func (r *Renderer) ensureEditorWidgets() {
	// The label editor is single-line (Requirement 10.1); the command editor is
	// multi-line (Requirement 10.2). SingleLine is set every call because it is
	// a plain field with no constructor and setting it is idempotent.
	r.cfg.labelEd.SingleLine = true
	r.cfg.commandEd.SingleLine = false

	if r.cfg.bgColor == nil {
		r.cfg.bgColor = NewColorPicker(r.state.Editor.BgColor)
	}
	if r.cfg.fontColor == nil {
		r.cfg.fontColor = NewColorPicker(r.state.Editor.FontColor)
	}
	if r.cfg.fontSize == nil {
		r.cfg.fontSize = NewFontSizeSelector(r.state.Editor.FontSize)
	}
}

// ensureCfgTheme lazily builds the Config_View-private material.Theme with a
// dark-appropriate palette and returns it (Issue 2). The shared r.th is the
// default light-ish theme (dark Fg), so its material widgets render near-black
// text that is unreadable on the dark editor pane. We clone a fresh theme (so we
// inherit its Shaper/Icons/TextSize) and override only the Palette so ALL
// Config_View material widgets rendered through this theme — including the
// ColorPicker's internal editor, since ColorPicker.Layout takes a *material.Theme
// — become light-on-dark. The shared r.th is left untouched so the header, deck,
// and other views keep their normal appearance.
func (r *Renderer) ensureCfgTheme() *material.Theme {
	if r.cfg.cfgTheme == nil {
		th := material.NewTheme()
		th.Palette.Fg = cfgThemeFg
		th.Palette.Bg = cfgThemeBg
		th.Palette.ContrastBg = cfgThemeContrastBg
		th.Palette.ContrastFg = cfgDefaultFontFg
		r.cfg.cfgTheme = th
	}
	return r.cfg.cfgTheme
}

// layoutFieldBox draws a visible, rounded input-box background (a cfgFieldBg fill
// with a cfgFieldBorder outline) and lays out w inset inside it (Issue 2). This
// gives each text editor an obvious affordance on the dark pane — without it the
// material.Editor has no visible box and the user cannot tell where to type. The
// box sizes itself to the content it wraps.
func (r *Renderer) layoutFieldBox(gtx layout.Context, w layout.Widget) layout.Dimensions {
	radius := gtx.Dp(unit.Dp(4))
	return widget.Border{
		Color:        cfgFieldBorder,
		Width:        unit.Dp(1),
		CornerRadius: unit.Dp(4),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		// Record the content size by laying it out inside a macro so we can paint
		// the fill behind it at the right dimensions.
		macro := op.Record(gtx.Ops)
		dims := layout.UniformInset(unit.Dp(8)).Layout(gtx, w)
		call := macro.Stop()

		rr := clip.UniformRRect(image.Rectangle{Max: dims.Size}, radius)
		defer rr.Push(gtx.Ops).Pop()
		paint.Fill(gtx.Ops, cfgFieldBg)
		call.Add(gtx.Ops)
		return dims
	})
}

// syncEditorWidgets pushes the current AppState.Editor DATA into the live editor
// WIDGET instances (task 15.1). It is called at the end of selectSlot so that
// selecting a slot loads that slot's values into the form. It first ensures the
// widgets exist, then sets each one's value from Editor. The existing-images
// list is also refreshed here so the choose-from-existing list is current for
// the newly selected slot.
func (r *Renderer) syncEditorWidgets() {
	r.ensureEditorWidgets()
	r.cfg.labelEd.SetText(r.state.Editor.Label)
	r.cfg.commandEd.SetText(r.state.Editor.Command)
	r.cfg.bgColor.SetHex(r.state.Editor.BgColor)
	r.cfg.fontColor.SetHex(r.state.Editor.FontColor)
	r.cfg.fontSize.SetValue(r.state.Editor.FontSize)
	r.refreshImages()
}

// readEditorWidgets reads the live widget values back into AppState.Editor
// (task 15.1). This runs every frame the form is laid out so the Editor data
// model stays authoritative and captures the user's in-progress edits — the
// save step (task 15.2) reads Editor, so it must reflect the latest widget
// state. BgImage is NOT read from a widget here: it is owned directly by Editor
// and mutated by the choose/clear/existing-image controls, so it is left as-is.
func (r *Renderer) readEditorWidgets() {
	r.ensureEditorWidgets()
	r.state.Editor.Label = r.cfg.labelEd.Text()
	r.state.Editor.Command = r.cfg.commandEd.Text()
	r.state.Editor.BgColor = r.cfg.bgColor.Hex()
	r.state.Editor.FontColor = r.cfg.fontColor.Hex()
	r.state.Editor.FontSize = r.cfg.fontSize.Value()
}

// refreshImages reloads the cached list of existing images from the backend
// (Requirement 13.4). Caching avoids a directory read every frame; refreshImages
// is called on slot select (via syncEditorWidgets), on the first layout (when
// imagesLoaded is false), and after a successful choose/clear so the list stays
// current. A listing error is logged and leaves the previous cache in place so a
// transient failure does not blank the list.
func (r *Renderer) refreshImages() {
	imgs, err := r.state.Backend.ListConfigImages()
	if err != nil {
		log.Printf("config: list existing images failed: %v", err)
		r.cfg.imagesLoaded = true
		return
	}
	r.cfg.existingImages = imgs
	r.cfg.imagesLoaded = true
}

// imgRowClickable returns the widget.Clickable for the existing-image row at the
// given path, allocating it (and the backing map) lazily on first use, mirroring
// slotClickable. Keying by path keeps a row's Clickable stable across frames as
// the image set grows.
func (r *Renderer) imgRowClickable(path string) *widget.Clickable {
	if r.cfg.imgRowClicks == nil {
		r.cfg.imgRowClicks = make(map[string]*widget.Clickable)
	}
	c, ok := r.cfg.imgRowClicks[path]
	if !ok {
		c = new(widget.Clickable)
		r.cfg.imgRowClicks[path] = c
	}
	return c
}

// chooseImage runs the native file-dialog choose-file flow (Requirements
// 13.1–13.3). It opens the dialog via Backend.SelectImage; on cancel (SelectImage
// returns "", nil) it does nothing so Editor.BgImage is unchanged (13.3). On a
// selection it copies the file into the config images dir via CopyImageToConfig
// and sets Editor.BgImage to the returned copied path (13.2), then refreshes the
// existing-images list so the new image appears immediately, and invalidates the
// texture cache for the copied path so any preview re-decodes it.
//
// Blocking-vs-goroutine decision: SelectImage (native modal dialog) and
// CopyImageToConfig (file copy) are called SYNCHRONOUSLY here. The trade-off is
// that the UI thread blocks while the modal dialog is open and during the copy —
// but that is exactly the behavior the user expects from a modal file dialog
// (the app is "waiting for you to pick a file"), and running them synchronously
// keeps all AppState.Editor mutation on the single frame-loop thread, avoiding
// the data race a goroutine writing Editor.BgImage would create with the layout
// pass. Because chooseImage is invoked from the drain-before-layout click check
// at the top of layoutConfigRight, the resulting BgImage change is applied before
// this frame lays out the form. r.w.Invalidate() schedules a repaint.
func (r *Renderer) chooseImage() {
	src, err := r.state.Backend.SelectImage()
	if err != nil {
		log.Printf("config: select image failed: %v", err)
		return
	}
	if src == "" {
		// Dialog cancelled: leave BgImage unchanged (Requirement 13.3).
		return
	}
	copied, err := r.state.Backend.CopyImageToConfig(src)
	if err != nil {
		log.Printf("config: copy image to config failed: %v", err)
		return
	}
	if copied == "" {
		return
	}
	r.state.Editor.BgImage = copied
	r.refreshImages()
	if r.state.Textures != nil {
		r.state.Textures.Invalidate(copied)
	}
	if r.w != nil {
		r.w.Invalidate()
	}
}

// friendlyImageName strips the timestamp prefix from a copied image path for
// display, mirroring the Svelte getFriendlyImageName: take the base name, and if
// it contains an underscore return the part after the FIRST underscore (the
// CopyImageToConfig naming is "<unixnano>_<basename>"), otherwise the base name
// as-is. An empty path yields an empty string.
func friendlyImageName(path string) string {
	if path == "" {
		return ""
	}
	base := filepath.Base(path)
	if i := strings.IndexByte(base, '_'); i != -1 {
		return base[i+1:]
	}
	return base
}

// defaultEditorBgColor / defaultEditorFontColor are the editor defaults applied
// on slot selection when a button has no bgColor / fontColor (and for a new
// empty-slot button), matching the Svelte defaults.
const (
	defaultEditorBgColor   = "#1f2937"
	defaultEditorFontColor = "#ffffff"
)

// generateButtonID returns a new button id of the form "btn_<random>", matching
// the Svelte `'btn_' + Math.random().toString(36)...` pattern (Requirement 9.7,
// "a generated id that is unique among all existing Button ids").
//
// It draws a 9-character base36 suffix from math/rand. A 9-char base36 suffix is
// 36^9 ≈ 1e14 possibilities, so a collision with the handful of ids in a config
// is astronomically unlikely; to make uniqueness guaranteed rather than merely
// overwhelmingly likely, it regenerates on the (practically impossible) chance
// the suffix collides with an existing id on any page. math/rand (not
// crypto/rand) is used deliberately: ids only need to be unique, not
// unpredictable, and the default source is adequate here.
func (r *Renderer) generateButtonID() string {
	for {
		id := "btn_" + randBase36(9)
		if !r.buttonIDExists(id) {
			return id
		}
	}
}

// buttonIDExists reports whether id already occurs on any page of the current
// config. Used by generateButtonID to guarantee uniqueness.
func (r *Renderer) buttonIDExists(id string) bool {
	for _, p := range r.state.Config.Pages {
		for _, b := range p.Buttons {
			if b.ID == id {
				return true
			}
		}
	}
	return false
}

// randBase36 returns an n-character string drawn from [0-9a-z] using math/rand.
const base36Alphabet = "0123456789abcdefghijklmnopqrstuvwxyz"

func randBase36(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = base36Alphabet[rand.Intn(len(base36Alphabet))]
	}
	return string(b)
}

// selectedSlotIs reports whether the given slot is the currently selected slot
// (AppState.SelectedSlot). Nil selection is never a match.
func (r *Renderer) selectedSlotIs(slot int) bool {
	return r.state.SelectedSlot != nil && *r.state.SelectedSlot == slot
}

// layoutConfigPreviewTile renders one editable preview tile, matching the Svelte
// preview cell (frontend/src/App.svelte):
//
//   - Selected slot: fill #1e293b, solid blue-500 border, label in fontColor.
//   - Configured (has a button): fill bgColor (or #1f2937 default), solid tile
//     border, label = button.Label or "(No Label)" in fontColor.
//   - Unconfigured: faint fill, muted border (dashed-approximation), and the
//     placeholder text "+ Slot N" (1-based) in gray.
//
// The label size follows the button's fontSize offset via FontSizeForOffset so
// the preview reflects the chosen size, matching getConfigFontSizeClass.
//
// Issue 3 (live preview): when this tile IS the currently selected slot, its
// label/colors/fontSize/background-image are read from the LIVE r.state.Editor
// instead of the saved button, so the preview updates as the user edits fields
// (matching the Svelte reactive preview). readEditorWidgets keeps Editor current
// each frame, so the selected tile reflects the latest typed values (at most one
// frame behind, which is fine). Non-selected tiles keep rendering from the saved
// button.
//
// Issue 4 (background image): when the resolved tile has a BgImage that decodes,
// the image is drawn with a CONTAIN fit (whole image visible, aspect preserved,
// centered) over the bgColor backdrop so leftover margins show the tile color,
// mirroring the deck. If the image fails to decode (e.g. webp/svg) the tile
// gracefully falls back to bgColor + label.
func (r *Renderer) layoutConfigPreviewTile(gtx layout.Context, slot int, btn core.ButtonConfig, hasButton bool) layout.Dimensions {
	size := gtx.Constraints.Max
	radius := gtx.Dp(unit.Dp(tileRadiusDp))

	rrect := clip.UniformRRect(image.Rectangle{Max: size}, radius)
	defer rrect.Push(gtx.Ops).Pop()

	selected := r.selectedSlotIs(slot)

	// Resolve the source values for this tile. For the selected slot the LIVE
	// Editor values win (Issue 3); a selected slot always has "content" (the
	// Editor holds either the loaded button or a fresh template), so it is
	// treated like a configured tile for label/color/image purposes.
	var (
		fill       color.NRGBA
		border     color.NRGBA
		labelText  string
		labelFg    color.NRGBA
		fontSize   int
		bgImage    string
		hasContent bool
	)
	switch {
	case selected:
		// Selection is indicated ONLY by a thicker blue border (Enhancement 4a);
		// the fill/backdrop keeps the REAL live-edited tile color (resolved just
		// below via the `selected` backdrop path) instead of the old cfgSelectedBg
		// tint, so the preview shows the actual color the user is editing.
		fill = cfgTileFillColor(r.state.Editor.BgColor)
		border = cfgSelectedBorder
		labelText = previewLabel(r.state.Editor.Label)
		labelFg = cfgTileFontColor(r.state.Editor.FontColor)
		fontSize = r.state.Editor.FontSize
		bgImage = r.state.Editor.BgImage
		hasContent = true
	case hasButton:
		fill = cfgTileFillColor(btn.BgColor)
		border = cfgTileBorder
		labelText = previewLabel(btn.Label)
		labelFg = cfgTileFontColor(btn.FontColor)
		fontSize = btn.FontSize
		bgImage = btn.BgImage
		hasContent = true
	default:
		fill = cfgEmptyBg
		border = cfgEmptyBorder
		labelText = "+ Slot " + strconv.Itoa(slot+1)
		labelFg = cfgEmptyFg
	}

	// For the selected slot, the bgColor backdrop should reflect the live edited
	// tile color rather than the selection tint, so a contained image's margins
	// show the color the user is editing. Non-selected tiles keep their resolved
	// fill (selection tint never applies to them).
	backdrop := fill
	if selected {
		backdrop = cfgTileFillColor(r.state.Editor.BgColor)
	}

	// Draw the background image with a Contain fit over the bgColor backdrop when
	// the tile has content and its image decodes (Issue 4). Otherwise just fill.
	drewImage := false
	if hasContent && bgImage != "" && r.state.Textures != nil {
		if op, ok := r.state.Textures.Get(bgImage); ok {
			paint.Fill(gtx.Ops, backdrop)
			r.drawConfigPreviewImage(gtx, op)
			drewImage = true
		}
	}
	if !drewImage {
		paint.Fill(gtx.Ops, fill)
	}

	// Selected tile: a thicker (8dp) blue border is the ONLY selection indicator
	// now (Enhancement 4a). Non-selected tiles keep the thin tileBorderDp border
	// in their normal border color.
	borderWidth := gtx.Dp(unit.Dp(tileBorderDp))
	if selected {
		borderWidth = gtx.Dp(unit.Dp(8))
	}
	strokeRRect(gtx, size, radius, borderWidth, border)

	r.layoutConfigTileLabel(gtx, labelText, labelFg, core.FontSizeForOffset(fontSize))

	return layout.Dimensions{Size: size}
}

// drawConfigPreviewImage paints op scaled to CONTAIN the current tile
// constraints (whole image visible, aspect ratio preserved, no crop) and
// centered, filling leftover space with whatever the caller painted behind it
// (Issue 4). This mirrors the deck's contain behavior but is defined locally in
// config.go so the preview does not depend on deck.go's drawContainImage (which
// a concurrent task is editing). The caller has already pushed the rounded-rect
// clip. Scale is 1/PxPerDp so one image pixel maps to one output pixel before
// the Contain fit rescales it to fit the tile.
func (r *Renderer) drawConfigPreviewImage(gtx layout.Context, op paint.ImageOp) {
	img := widget.Image{
		Src:      op,
		Fit:      widget.Contain,
		Position: layout.Center,
		Scale:    1.0 / gtx.Metric.PxPerDp,
	}
	img.Layout(gtx)
}

// layoutConfigNavPreview renders a Pagination_Slot preview (Requirement 9.8):
// a muted, dashed-style tile (approximated with a thin muted border over a faint
// fill, since Gio has no dashed-stroke primitive) carrying non-editable text
// "[Prev Page]" / "[Next Page]" in blue. These tiles are display-only and are
// never selectable (the selection guard is enforced in 14.3).
func (r *Renderer) layoutConfigNavPreview(gtx layout.Context, label string) layout.Dimensions {
	size := gtx.Constraints.Max
	radius := gtx.Dp(unit.Dp(tileRadiusDp))

	rrect := clip.UniformRRect(image.Rectangle{Max: size}, radius)
	defer rrect.Push(gtx.Ops).Pop()

	paint.Fill(gtx.Ops, cfgNavBg)
	strokeRRect(gtx, size, radius, gtx.Dp(unit.Dp(tileBorderDp)), cfgNavBorder)

	r.layoutConfigTileLabel(gtx, label, cfgNavFg, 13)

	return layout.Dimensions{Size: size}
}

// layoutConfigTileLabel draws label centered within the current preview tile at
// the given color and point size. Long labels wrap and are centered (matching
// the Svelte break-all + text-center), inset from the tile edge. An empty label
// draws nothing.
func (r *Renderer) layoutConfigTileLabel(gtx layout.Context, label string, fg color.NRGBA, sizeSp float32) layout.Dimensions {
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

// layoutConfigRight renders the right pane: the slot editor area. Since the
// editor FORM FIELDS are task 15.x, this shows a centered prompt when no slot is
// selected (matching the Svelte "Select any grid slot..." empty state,
// Requirement 9.1) and a minimal "Editing Slot N" header when a slot IS
// selected. The real form fields, save/clear, and action controls arrive in
// 15.x/16.x.
func (r *Renderer) layoutConfigRight(gtx layout.Context) layout.Dimensions {
	fillBackground(gtx, cfgRightBg)

	return layout.Inset{
		Top:    unit.Dp(16),
		Bottom: unit.Dp(16),
		Left:   unit.Dp(16),
		Right:  unit.Dp(16),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if r.state.SelectedSlot == nil {
			// No selection: centered prompt.
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				p := material.Body1(r.ensureCfgTheme(), "Select a grid slot to edit")
				p.Color = cfgPromptFg
				p.Alignment = text.Middle
				return p.Layout(gtx)
			})
		}
		// A slot is selected: render the editor form (task 15.1).
		return r.layoutEditorForm(gtx)
	})
}

// layoutEditorForm renders the slot editor form in the right pane when a slot is
// selected (task 15.1, Requirements 10.1–10.5, 13.4–13.6). Scope is FIELDS +
// image selection + widget sync ONLY: the "Editing Slot N" header stays and the
// fields are laid out below it; Save/Clear (15.2) and move/copy/paste/test-run
// (16.x) add their controls in later tasks.
//
// Ordering: the widgets are ensured to exist, the choose/clear button clicks are
// DRAINED before anything is laid out (drain-before-layout — widget.Clickable in
// Gio v0.10.2 discards pending clicks in Layout), the existing-images list is
// loaded on first show, the live widget values are read back into Editor so the
// data model stays authoritative for the save step, and finally the form is laid
// out inside a scrollable list so it never clips on short windows.
func (r *Renderer) layoutEditorForm(gtx layout.Context) layout.Dimensions {
	r.ensureEditorWidgets()

	// Drain the Choose File / Clear button clicks BEFORE laying them out. Because
	// this runs at the top of the form layout, a click's mutation (BgImage set or
	// cleared) is reflected on the SAME frame.
	for r.cfg.chooseFileBtn.Clicked(gtx) {
		r.chooseImage()
	}
	for r.cfg.clearImgBtn.Clicked(gtx) {
		// Clear the background image (Requirement 13.6).
		r.state.Editor.BgImage = ""
	}

	// Drain the Save / Clear button clicks BEFORE the form (and their buttons)
	// are laid out below (drain-before-layout — see the saveBtn/clearBtn field
	// docs). saveSelectedSlot re-reads the live widgets first so it captures this
	// frame's edits rather than last frame's; both handlers persist via
	// SaveConfig and record any failure in cfg.saveErr (Requirements 10.6–10.8).
	for r.cfg.saveBtn.Clicked(gtx) {
		r.saveSelectedSlot()
	}
	for r.cfg.clearBtn.Clicked(gtx) {
		r.clearSelectedSlot()
	}

	// Drain the Move arrow clicks BEFORE their buttons are laid out below
	// (drain-before-layout). Each swaps the selected slot's button with the
	// adjacent slot in that direction via moveSelected (task 16.1).
	for r.cfg.moveLeft.Clicked(gtx) {
		r.moveSelected(core.Left)
	}
	for r.cfg.moveUp.Clicked(gtx) {
		r.moveSelected(core.Up)
	}
	for r.cfg.moveDown.Clicked(gtx) {
		r.moveSelected(core.Down)
	}
	for r.cfg.moveRight.Clicked(gtx) {
		r.moveSelected(core.Right)
	}

	// Drain the Copy / Paste clicks BEFORE their buttons are laid out (task 16.2).
	// Copy captures the editor's live settings into the shared clipboard; Paste
	// applies the shared clipboard to the selected slot and saves. Paste is
	// ignored when the shared clipboard is empty (Requirement 11.7).
	for r.cfg.copyBtn.Clicked(gtx) {
		r.copyEditorSettings()
	}
	for r.cfg.pasteBtn.Clicked(gtx) {
		if r.state.Clipboard != nil {
			r.pasteEditorSettings()
		}
	}

	// Drain the Test Run click BEFORE its button is laid out (task 16.3). Ignored
	// while a run is already in progress (Requirement 12.5 — presented
	// unavailable while executing).
	for r.cfg.testRunBtn.Clicked(gtx) {
		if !r.state.CommandTest.Running {
			r.runCommandTest()
		}
	}

	// Apply any completed background test-run result on the UI thread (task
	// 16.3). The goroutine fills cfg.testResult under cfg.testMu and Invalidates;
	// here — on the frame-loop thread — we move it into r.state.CommandTest so all
	// CommandTest mutation stays single-threaded.
	r.applyPendingTestResult()

	// First-time load of the existing-images list even if no slot-select has
	// happened this session (defensive; selectSlot already refreshes).
	if !r.cfg.imagesLoaded {
		r.refreshImages()
	}

	// Read the live widget values back into Editor every frame so 15.2 can save
	// the latest edits (label/command/colors/fontSize). BgImage is owned by
	// Editor directly (set by choose/clear/existing-image), so it is not read
	// from a widget.
	r.readEditorWidgets()

	// The form is a vertical stack of labelled fields, wrapped in a scrollable
	// material.List so a tall form (image list + all fields) does not clip on a
	// short window.
	r.cfg.imgList.Axis = layout.Vertical

	type formRow struct {
		w layout.Widget
	}
	rows := []formRow{
		{r.editorHeader},
		{spacerRow(12)},
		{r.fieldLabelEditor},
		{spacerRow(12)},
		{r.fieldCommandEditor},
		{spacerRow(12)},
		{r.fieldBackgroundImage},
		{spacerRow(12)},
		// Tile Color + Text Color side by side in one row (Enhancement 2) to
		// save vertical space; each column keeps its caption-above-picker layout.
		{r.fieldColorsRow},
		{spacerRow(12)},
		{r.fieldFontSize},
		{spacerRow(16)},
		// Move controls (task 16.1) and copy/paste (task 16.2) sit between the
		// fields and the Save/Clear actions, matching the Svelte editor order
		// (arrange controls, then persist). The test-run panel (task 16.3) sits
		// last so its output area can grow at the bottom of the scrollable form.
		// Move arrows + Copy/Paste merged into one inline row (Enhancement 8).
		{r.fieldMoveCopyPaste},
		{spacerRow(16)},
		{r.editorActions},
		{spacerRow(16)},
		// Test-run OUTPUT area only. The Test Run BUTTON now lives inline in
		// editorActions (between Save and Clear); this row shows just the
		// result/running/success/failure/empty states below the actions. The
		// former "Test Run" caption + standalone button (fieldTestRun) were
		// removed so there is no duplicate button and no separate heading.
		{r.fieldTestRunOutput},
	}

	return material.List(r.ensureCfgTheme(), &r.cfg.imgList).Layout(gtx, len(rows), func(gtx layout.Context, i int) layout.Dimensions {
		return rows[i].w(gtx)
	})
}

// spacerRow returns a widget that draws a vertical gap of the given dp height,
// used to separate form fields in the scrollable list.
func spacerRow(dp float32) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Spacer{Height: unit.Dp(dp)}.Layout(gtx)
	}
}

// editorHeader draws the "Editing Slot N" header at the top of the form.
func (r *Renderer) editorHeader(gtx layout.Context) layout.Dimensions {
	slot := 0
	if r.state.SelectedSlot != nil {
		slot = *r.state.SelectedSlot
	}
	h := material.H6(r.ensureCfgTheme(), "Editing Slot "+strconv.Itoa(slot+1))
	h.Color = cfgHeadingFg
	return h.Layout(gtx)
}

// fieldCaption draws a small field caption above a control.
func (r *Renderer) fieldCaption(gtx layout.Context, text string) layout.Dimensions {
	c := material.Body2(r.ensureCfgTheme(), text)
	c.Color = cfgHeadingFg
	return c.Layout(gtx)
}

// fieldLabelEditor renders the single-line Label editor (Requirement 10.1). The
// editor is drawn through cfgTheme (light text on dark) inside a visible field
// box so it is readable and its input area is obvious (Issue 2).
func (r *Renderer) fieldLabelEditor(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return r.fieldCaption(gtx, "Label")
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return r.layoutFieldBox(gtx, func(gtx layout.Context) layout.Dimensions {
				ed := material.Editor(r.ensureCfgTheme(), &r.cfg.labelEd, "Button label")
				ed.Color = cfgThemeFg
				ed.HintColor = cfgThemeHint
				return ed.Layout(gtx)
			})
		}),
	)
}

// fieldCommandEditor renders the multi-line Command editor (Requirement 10.2).
func (r *Renderer) fieldCommandEditor(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return r.fieldCaption(gtx, "Command")
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return r.layoutFieldBox(gtx, func(gtx layout.Context) layout.Dimensions {
				ed := material.Editor(r.ensureCfgTheme(), &r.cfg.commandEd, "Command to run")
				ed.Color = cfgThemeFg
				ed.HintColor = cfgThemeHint
				// Reserve a few lines of height for the multi-line command so it is
				// comfortably tall without a fixed pixel size.
				gtx.Constraints.Min.Y = gtx.Dp(unit.Dp(72))
				return ed.Layout(gtx)
			})
		}),
	)
}

// fieldColorsRow lays the Tile Color and Text Color pickers SIDE BY SIDE in a
// single horizontal row to save vertical space (Enhancement 2). Each column is a
// Flexed(1) share of the width and keeps its existing caption-above-picker
// vertical structure (fieldTileColor / fieldTextColor), with a small horizontal
// spacer between the two columns. Rendering still goes through cfgTheme via those
// helpers. The pickers' internal widgets carry no click-drain concerns here.
func (r *Renderer) fieldColorsRow(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return r.fieldTileColor(gtx)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return r.fieldTextColor(gtx)
		}),
	)
}

// fieldTileColor renders the tile-color (bgColor) ColorPicker (Requirement 10.3).
func (r *Renderer) fieldTileColor(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return r.fieldCaption(gtx, "Tile Color")
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			// The ColorPicker's internal hex editor uses the theme's Fg, so
			// passing cfgTheme makes its text readable on the dark pane (Issue 2).
			// The box gives the hex field a visible input affordance too.
			return r.layoutFieldBox(gtx, func(gtx layout.Context) layout.Dimensions {
				return r.cfg.bgColor.Layout(gtx, r.ensureCfgTheme(), "#1f2937")
			})
		}),
	)
}

// fieldTextColor renders the text-color (fontColor) ColorPicker (Requirement 10.4).
func (r *Renderer) fieldTextColor(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return r.fieldCaption(gtx, "Text Color")
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return r.layoutFieldBox(gtx, func(gtx layout.Context) layout.Dimensions {
				return r.cfg.fontColor.Layout(gtx, r.ensureCfgTheme(), "#ffffff")
			})
		}),
	)
}

// fieldFontSize renders the FontSizeSelector segmented control (Requirement 10.5).
func (r *Renderer) fieldFontSize(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return r.fieldCaption(gtx, "Font Size")
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return r.cfg.fontSize.Layout(gtx, r.ensureCfgTheme())
		}),
	)
}

// editorActions renders the Save / Clear action row plus, when a persistence
// error occurred, a red error line below the buttons (task 15.2, Requirements
// 10.6–10.8). "Save Changes" is the primary (themed) button; "Clear Slot" is
// styled muted-red to signal its destructive nature. The buttons' Clicked()
// events were already drained at the TOP of layoutEditorForm
// (drain-before-layout), so this only paints them.
func (r *Renderer) editorActions(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					b := material.Button(r.ensureCfgTheme(), &r.cfg.saveBtn, "Save Changes")
					b.Inset = layout.UniformInset(unit.Dp(8))
					b.TextSize = unit.Sp(14)
					return b.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				// Test Run sits inline between Save and Clear (relocated from the
				// former separate test-run section). This is the ONLY place the
				// r.cfg.testRunBtn Clickable is laid out this frame; its Clicked()
				// events were already drained at the TOP of layoutEditorForm
				// (drain-before-layout). It is a neutral/secondary button (the
				// theme's muted bg/fg) between the primary Save and destructive
				// Clear, and is presented unavailable while a run is in progress
				// (Requirement 12.5).
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					th := r.ensureCfgTheme()
					b := material.Button(th, &r.cfg.testRunBtn, "Test Run")
					b.Inset = layout.UniformInset(unit.Dp(8))
					b.TextSize = unit.Sp(14)
					if r.state.CommandTest.Running {
						// Presented unavailable while executing (Requirement 12.5).
						b.Background = cfgDisabledBg
						b.Color = cfgDisabledFg
					} else {
						// Amber accent fill with white text so the button is
						// clearly clickable and distinct from the primary blue
						// Save and destructive red Clear.
						b.Background = cfgTestBtnBg
						b.Color = cfgDefaultFontFg
					}
					return b.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					b := material.Button(r.ensureCfgTheme(), &r.cfg.clearBtn, "Clear Slot")
					b.Inset = layout.UniformInset(unit.Dp(8))
					b.TextSize = unit.Sp(14)
					// Muted destructive styling: red fill, white text.
					b.Background = cfgClearBtnBg
					b.Color = cfgDefaultFontFg
					return b.Layout(gtx)
				}),
			)
		}),
		// Persistence-error line (Requirement 10.8). Only shown when the last
		// save/clear write failed; cleared on the next successful write.
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if r.cfg.saveErr == "" {
				return layout.Dimensions{}
			}
			return layout.Inset{Top: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				e := material.Body2(r.ensureCfgTheme(), r.cfg.saveErr)
				e.Color = cfgErrorFg
				return e.Layout(gtx)
			})
		}),
	)
}

// currentPageIndex returns the index into r.state.Config.Pages of the page whose
// PageIndex equals the current page (task 15.2). When no such page exists yet a
// fresh empty PageConfig{PageIndex: CurrentPage} is appended and its index
// returned, so the caller can always write a slot into a valid page. This is the
// find-or-create seam the save/clear paths use before applying WriteSlot /
// ClearSlot and writing the updated page back.
func (r *Renderer) currentPageIndex() int {
	for i := range r.state.Config.Pages {
		if r.state.Config.Pages[i].PageIndex == r.state.CurrentPage {
			return i
		}
	}
	r.state.Config.Pages = append(r.state.Config.Pages, core.PageConfig{PageIndex: r.state.CurrentPage})
	return len(r.state.Config.Pages) - 1
}

// saveSelectedSlot persists the editor's current values into the selected slot
// on the current page (task 15.2, Requirement 10.6), following the Svelte
// saveButton flow.
//
//   - Guard: no slot selected → nothing to save.
//   - It calls readEditorWidgets() FIRST so Editor reflects THIS frame's live
//     widget text/colors/fontSize. The frame-level readEditorWidgets runs AFTER
//     the drain in layoutEditorForm, so when saveBtn.Clicked fires during the
//     drain, Editor would otherwise still hold last frame's values — re-reading
//     here makes the saved button carry the newest edits.
//   - It builds a core.ButtonConfig from Editor (generating an id if none),
//     finds/creates the current page, applies core.WriteSlot (which sets
//     Order = slot and replaces any button already at that slot), and writes the
//     updated page back into Config.Pages.
//   - It then persists via SaveConfig. On failure (Requirement 10.8) it logs and
//     records the error in cfg.saveErr; the in-memory edits are kept (so the user
//     can retry) and the on-disk config is left unchanged by the failed write. On
//     success cfg.saveErr is cleared.
func (r *Renderer) saveSelectedSlot() {
	if r.state.SelectedSlot == nil {
		return
	}
	slot := *r.state.SelectedSlot

	// Capture the latest widget values before building the button (see doc).
	r.readEditorWidgets()

	id := r.state.Editor.ID
	if id == "" {
		id = r.generateButtonID()
		r.state.Editor.ID = id
	}

	btn := core.ButtonConfig{
		ID:        id,
		Label:     r.state.Editor.Label,
		Command:   r.state.Editor.Command,
		BgImage:   r.state.Editor.BgImage,
		BgColor:   r.state.Editor.BgColor,
		FontColor: r.state.Editor.FontColor,
		FontSize:  r.state.Editor.FontSize,
		Order:     slot,
	}

	idx := r.currentPageIndex()
	r.state.Config.Pages[idx] = core.WriteSlot(r.state.Config.Pages[idx], slot, btn)

	if err := r.state.Backend.SaveConfig(r.state.Config); err != nil {
		log.Printf("config: save slot %d on page %d failed: %v", slot, r.state.CurrentPage, err)
		r.cfg.saveErr = "Failed to save configuration: " + err.Error()
	} else {
		r.cfg.saveErr = ""
	}

	if r.w != nil {
		r.w.Invalidate()
	}
}

// clearSelectedSlot removes the button at the selected slot on the current page
// and saves (task 15.2, Requirement 10.7), mirroring the Svelte deleteButton.
//
//   - Guard: no slot selected → nothing to clear.
//   - It finds/creates the current page, applies core.ClearSlot to drop any
//     button at the slot, and writes the page back.
//   - It persists via SaveConfig with the same error handling as save
//     (Requirement 10.8): failure is logged and surfaced in cfg.saveErr while the
//     in-memory config is kept; success clears saveErr.
//   - The slot stays selected but the editor fields are reset to a fresh empty
//     template (new id + default colors) and pushed into the widgets, matching
//     the Svelte deleteButton which clears the form after deleting.
func (r *Renderer) clearSelectedSlot() {
	if r.state.SelectedSlot == nil {
		return
	}
	slot := *r.state.SelectedSlot

	idx := r.currentPageIndex()
	r.state.Config.Pages[idx] = core.ClearSlot(r.state.Config.Pages[idx], slot)

	if err := r.state.Backend.SaveConfig(r.state.Config); err != nil {
		log.Printf("config: clear slot %d on page %d failed: %v", slot, r.state.CurrentPage, err)
		r.cfg.saveErr = "Failed to save configuration: " + err.Error()
	} else {
		r.cfg.saveErr = ""
	}

	// Reset the editor to a fresh empty template for the (still-selected) slot
	// and sync the widgets so the form visibly clears.
	r.state.Editor = EditorState{
		ID:        r.generateButtonID(),
		BgColor:   defaultEditorBgColor,
		FontColor: defaultEditorFontColor,
		FontSize:  0,
	}
	r.syncEditorWidgets()

	if r.w != nil {
		r.w.Invalidate()
	}
}

// fieldBackgroundImage renders the background-image controls (Requirements
// 13.4–13.6): a Choose File button + Clear button row, the current image's
// friendly name, and a scrollable list of existing images as clickable rows. The
// Choose File / Clear clicks were already drained at the top of layoutEditorForm
// (drain-before-layout); the per-image rows drain their own Clicked() before
// their Layout below.
func (r *Renderer) fieldBackgroundImage(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return r.fieldCaption(gtx, "Background Image")
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
		// Choose File + Clear buttons.
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					b := material.Button(r.ensureCfgTheme(), &r.cfg.chooseFileBtn, "Choose File")
					b.Inset = layout.UniformInset(unit.Dp(6))
					b.TextSize = unit.Sp(13)
					return b.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					th := r.ensureCfgTheme()
					b := material.Button(th, &r.cfg.clearImgBtn, "Clear")
					b.Inset = layout.UniformInset(unit.Dp(6))
					b.TextSize = unit.Sp(13)
					// Muted (secondary) styling against the dark pane.
					b.Background = cfgFieldBg
					b.Color = th.Palette.Fg
					return b.Layout(gtx)
				}),
			)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
		// Current image friendly name (or a "none" hint).
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			name := friendlyImageName(r.state.Editor.BgImage)
			if name == "" {
				name = "No background image chosen"
			}
			c := material.Body2(r.ensureCfgTheme(), name)
			c.Color = cfgPromptFg
			return c.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
		// Existing images list.
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return r.layoutExistingImages(gtx)
		}),
	)
}

// layoutExistingImages renders the cached existing images as clickable rows
// (Requirements 13.4, 13.5). Selecting a row sets Editor.BgImage to that image's
// path. Each row's Clicked() is drained BEFORE its Layout (drain-before-layout).
// When there are no images a muted hint is shown instead.
func (r *Renderer) layoutExistingImages(gtx layout.Context) layout.Dimensions {
	if len(r.cfg.existingImages) == 0 {
		c := material.Caption(r.ensureCfgTheme(), "No existing images")
		c.Color = cfgPromptFg
		return c.Layout(gtx)
	}

	children := make([]layout.FlexChild, 0, len(r.cfg.existingImages)*2+2)
	// A small caption above the list makes the click affordance explicit — the
	// boxed rows below are selectable items, not static text.
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		c := material.Caption(r.ensureCfgTheme(), "Existing images (click to use)")
		c.Color = cfgHeadingFg
		return c.Layout(gtx)
	}))
	children = append(children, layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout))
	for _, path := range r.cfg.existingImages {
		path := path
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			c := r.imgRowClickable(path)
			for c.Clicked(gtx) {
				// Select an existing image (Requirement 13.5).
				r.state.Editor.BgImage = path
				if r.w != nil {
					r.w.Invalidate()
				}
			}
			selected := r.state.Editor.BgImage == path
			return c.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return r.layoutExistingImageRow(gtx, friendlyImageName(path), selected)
			})
		}))
		// A little vertical breathing room between the boxed rows (bumped from
		// 2dp to 5dp so the boxes are visually distinct list items).
		children = append(children, layout.Rigid(layout.Spacer{Height: unit.Dp(5)}.Layout))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

// layoutExistingImageRow draws a single existing-image row as a visibly
// clickable list item: a rounded-rect box (fill + border) with the friendly
// name inside, so the row reads as a tappable selectable item rather than plain
// text. The SELECTED image (Editor.BgImage == this path) uses a highlighted
// style — the blue selected fill/border from the preview palette (cfgSelectedBg
// / cfgSelectedBorder) with blue text — while unselected rows use the muted
// field-box fill/border (cfgFieldBg / cfgFieldBorder) with light text. Both use
// the Config_View's dark-readable theme (cfgTheme, via ensureCfgTheme) so the
// text is readable, matching how the rest of the editor form renders its labels.
//
// The box is drawn by laying the label out inside padding inside a widget.Border
// (rounded corners), recording it into a macro so the fill can be painted behind
// it at the exact content dimensions with a clip.UniformRRect — the same
// technique as layoutFieldBox, but with selected-aware colors that layoutFieldBox
// hardcodes.
func (r *Renderer) layoutExistingImageRow(gtx layout.Context, name string, selected bool) layout.Dimensions {
	radius := gtx.Dp(unit.Dp(4))

	fill := cfgFieldBg
	borderCol := cfgFieldBorder
	textCol := cfgHeadingFg
	if selected {
		fill = cfgSelectedBg
		borderCol = cfgSelectedBorder
		textCol = cfgNavFg
	}

	return widget.Border{
		Color:        borderCol,
		Width:        unit.Dp(1),
		CornerRadius: unit.Dp(4),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		// Record the padded label so the rounded fill can be painted behind it at
		// the label's exact size.
		macro := op.Record(gtx.Ops)
		dims := layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				lbl := material.Body2(r.ensureCfgTheme(), name)
				lbl.Color = textCol
				return lbl.Layout(gtx)
			})
		call := macro.Stop()

		rr := clip.UniformRRect(image.Rectangle{Max: dims.Size}, radius)
		defer rr.Push(gtx.Ops).Pop()
		paint.Fill(gtx.Ops, fill)
		call.Add(gtx.Ops)
		return dims
	})
}

// --- Small helpers ---

// previewLabel returns the button label to show in a preview tile: the label
// itself, or "(No Label)" when the button has an empty label (matching the
// Svelte `button.label || '(No Label)'`).
func previewLabel(label string) string {
	if label == "" {
		return "(No Label)"
	}
	return label
}

// cfgTileFillColor picks a preview tile's fill for a configured slot: the
// button's bgColor when it is a valid hex, otherwise the default tile color
// (#1f2937), matching the Svelte `button.bgColor || '#1f2937'`.
func cfgTileFillColor(bgColor string) color.NRGBA {
	if col, ok := ParseHexColor(bgColor); ok {
		return col
	}
	return defaultTileBg
}

// cfgTileFontColor picks a preview label color: the button's fontColor when
// valid, otherwise the default white, matching `button.fontColor || '#ffffff'`.
func cfgTileFontColor(fontColor string) color.NRGBA {
	if col, ok := ParseHexColor(fontColor); ok {
		return col
	}
	return cfgDefaultFontFg
}

// --- Task 16.1: Move controls (Requirements 11.1–11.4) ---

// moveSelected swaps the selected slot's button with the button in the adjacent
// slot in the given direction on the current page, then saves and follows the
// selection to the target slot (task 16.1, Requirements 11.1, 11.2, 11.4). It
// mirrors the Svelte moveButton.
//
//   - Guard: no slot selected → nothing to move.
//   - It resolves the current page (find-or-create) and calls core.MoveButton,
//     which computes the target via row/col arithmetic and rejects invalid moves
//     (edge of grid, target/selection is a Pagination_Slot). When core.MoveButton
//     returns ok == false the move is invalid (Requirement 11.3): the config and
//     selection are left completely unchanged and no save happens.
//   - On a valid move it writes the swapped page back, persists via SaveConfig
//     (logging any failure — the in-memory swap is kept so the UI stays
//     consistent), then re-selects the TARGET slot via selectSlot so the editor
//     form and preview follow the moved button (Requirement 11.4). selectSlot
//     stores a stable copy of the new slot index in AppState.SelectedSlot and
//     reloads the editor widgets from the button now at that slot; it does NOT
//     itself save, so the SaveConfig above is the single persistence point.
//
// Parity note: the swap operates on the SAVED buttons at the two slots, exactly
// like the Svelte moveButton — any UNSAVED edits in the form for the current slot
// are not part of the move. After the move, selectSlot reloads the editor from
// the (saved) button now at the target slot, so those unsaved edits are
// discarded, matching the Svelte behavior where move re-reads stored buttons.
func (r *Renderer) moveSelected(dir core.Direction) {
	if r.state.SelectedSlot == nil {
		return
	}
	sel := *r.state.SelectedSlot

	idx := r.currentPageIndex()
	newPage, newSel, ok := core.MoveButton(
		r.state.Config.Pages[idx],
		sel,
		r.state.Config.Rows,
		r.state.Config.Cols,
		dir,
	)
	if !ok {
		// Invalid move (edge or Pagination_Slot target): leave everything
		// unchanged (Requirement 11.3).
		return
	}

	r.state.Config.Pages[idx] = newPage

	if err := r.state.Backend.SaveConfig(r.state.Config); err != nil {
		log.Printf("config: save after move slot %d -> %d on page %d failed: %v", sel, newSel, r.state.CurrentPage, err)
		r.cfg.saveErr = "Failed to save configuration: " + err.Error()
	} else {
		r.cfg.saveErr = ""
	}

	// Follow the selection to the target slot and reload the editor from the
	// button now there (Requirement 11.4). selectSlot re-syncs the widgets and
	// invalidates, so no extra Invalidate is needed here.
	r.selectSlot(newSel)
}

// fieldMoveCopyPaste merges the move arrows and the copy/paste controls into a
// SINGLE horizontal row (Enhancement 8): [← ↑ ↓ →]  [Copy Settings] [Paste
// Settings]. It reuses the same rendering (arrows through cfgTheme, copy/paste
// with the paste-unavailable greying) as the former fieldMoveControls /
// fieldCopyPaste, laying the arrows group and the copy/paste group on one
// layout.Flex{Axis: Horizontal} separated by a 24dp spacer. The buttons'
// Clicked() events are still drained at the TOP of layoutEditorForm
// (drain-before-layout), so this only paints them. The "Move" caption is dropped
// to keep the single row compact; the arrows remain self-explanatory.
func (r *Renderer) fieldMoveCopyPaste(gtx layout.Context) layout.Dimensions {
	th := r.ensureCfgTheme()
	arrow := func(gtx layout.Context, btn *widget.Clickable, label string) layout.Dimensions {
		b := material.Button(th, btn, label)
		b.Inset = layout.UniformInset(unit.Dp(8))
		b.TextSize = unit.Sp(16)
		return b.Layout(gtx)
	}
	pasteAvailable := r.state.Clipboard != nil
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		// Arrows group: ← ↑ ↓ →
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return arrow(gtx, &r.cfg.moveLeft, "←")
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return arrow(gtx, &r.cfg.moveUp, "↑")
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return arrow(gtx, &r.cfg.moveDown, "↓")
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return arrow(gtx, &r.cfg.moveRight, "→")
		}),
		// Gap between the two groups.
		layout.Rigid(layout.Spacer{Width: unit.Dp(24)}.Layout),
		// Copy/paste group: [Copy Settings] [Paste Settings]
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			b := material.Button(th, &r.cfg.copyBtn, "Copy Settings")
			b.Inset = layout.UniformInset(unit.Dp(8))
			b.TextSize = unit.Sp(14)
			b.Background = cfgFieldBg
			b.Color = th.Palette.Fg
			return b.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			b := material.Button(th, &r.cfg.pasteBtn, "Paste Settings")
			b.Inset = layout.UniformInset(unit.Dp(8))
			b.TextSize = unit.Sp(14)
			if pasteAvailable {
				b.Background = cfgFieldBg
				b.Color = th.Palette.Fg
			} else {
				// Empty clipboard: greyed to signal unavailable (Requirement 11.7).
				b.Background = cfgDisabledBg
				b.Color = cfgDisabledFg
			}
			return b.Layout(gtx)
		}),
	)
}

// --- Task 16.2: Copy / paste of editor settings (Requirements 11.5–11.7) ---

// copyEditorSettings stores the editor's CURRENT settings in the SHARED
// r.state.Clipboard (task 16.2, Requirement 11.5), mirroring the Svelte
// copyButton (which copies the edit* values). It calls readEditorWidgets first
// so the clipboard captures the live in-progress edits (label/command/colors/
// fontSize) rather than a stale Editor snapshot, then writes a fresh
// ButtonClipboard. The id and order are intentionally NOT copied — they belong to
// the target slot, not the copied settings (design: Data Models -> ButtonClipboard).
// This is the SAME AppState.Clipboard the Deck_View context menu uses, so a copy
// here can be pasted on the deck and vice versa.
func (r *Renderer) copyEditorSettings() {
	r.readEditorWidgets()
	r.state.Clipboard = &ButtonClipboard{
		Label:     r.state.Editor.Label,
		Command:   r.state.Editor.Command,
		BgImage:   r.state.Editor.BgImage,
		BgColor:   r.state.Editor.BgColor,
		FontColor: r.state.Editor.FontColor,
		FontSize:  r.state.Editor.FontSize,
	}
	if r.w != nil {
		r.w.Invalidate()
	}
}

// pasteEditorSettings applies the SHARED clipboard to the editor and persists it
// to the selected slot (task 16.2, Requirement 11.6), mirroring the Svelte
// pasteButton (which pastes the settings then calls saveButton). The caller only
// invokes this when r.state.Clipboard != nil (the drain guard), but it re-checks
// defensively. It copies the clipboard's visual/behavioral settings into
// r.state.Editor — keeping the current Editor.ID and the selected slot — then
// syncs the widgets so the form reflects the pasted values, and finally calls
// saveSelectedSlot so the pasted settings persist (which re-reads the widgets and
// writes the slot). Invalidate happens inside saveSelectedSlot/syncEditorWidgets.
func (r *Renderer) pasteEditorSettings() {
	cb := r.state.Clipboard
	if cb == nil {
		return
	}
	r.state.Editor.Label = cb.Label
	r.state.Editor.Command = cb.Command
	r.state.Editor.BgImage = cb.BgImage
	r.state.Editor.BgColor = cb.BgColor
	r.state.Editor.FontColor = cb.FontColor
	r.state.Editor.FontSize = cb.FontSize

	// Push pasted values into the widgets so the form updates, then save.
	r.syncEditorWidgets()
	r.saveSelectedSlot()
}

// --- Task 16.3: Test-run panel (Requirement 12) ---

// runCommandTest starts a test run of the current editor command (task 16.3,
// Requirements 12.1–12.5), mirroring the Svelte testCommand. It first reads the
// live widgets so it tests the command the user currently sees. An empty or
// whitespace-only command shows a "no command to test" message and does NOT
// invoke the backend (Requirement 12.3). Otherwise it flips CommandTest.Running
// on (so the panel shows an "Executing…" indicator and the Test Run button is
// presented unavailable — Requirement 12.5) and runs Backend.RunCommandSync on a
// GOROUTINE so the (up to 10s) run never blocks the UI thread. The goroutine
// hands its result back through the mutex-guarded cfg.testResult holder and
// Invalidates; applyPendingTestResult (on the UI thread) moves it into
// CommandTest next frame, so all CommandTest mutation stays single-threaded.
func (r *Renderer) runCommandTest() {
	r.readEditorWidgets()
	cmd := r.state.Editor.Command

	if strings.TrimSpace(cmd) == "" {
		// Empty command: show a message, do not run anything (Requirement 12.3).
		r.state.CommandTest = CommandTestState{
			Running: false,
			Output:  "No command to test",
			Success: false,
		}
		if r.w != nil {
			r.w.Invalidate()
		}
		return
	}

	// Enter the running state (Requirement 12.5).
	r.state.CommandTest = CommandTestState{Running: true, Output: "", Success: false}
	if r.w != nil {
		r.w.Invalidate()
	}

	backend := r.state.Backend
	win := r.w
	go func() {
		out, err := backend.RunCommandSync(cmd)

		res := &testRunResult{}
		if err != nil {
			// Failure: show combined output plus the error text (Requirements
			// 12.4, 12.5 — the timeout error from RunCommandSync mentions
			// "timed out"). Keep the command's output when present so the user
			// sees whatever ran before it failed.
			res.Success = false
			if out != "" {
				res.Output = out + "\n" + err.Error()
			} else {
				res.Output = err.Error()
			}
		} else {
			// Success: show the combined output (Requirement 12.2).
			res.Success = true
			if out == "" {
				res.Output = "Command executed successfully with no output."
			} else {
				res.Output = out
			}
		}

		// Hand the result to the UI thread under the lock and wake the loop.
		r.cfg.testMu.Lock()
		r.cfg.testResult = res
		r.cfg.testMu.Unlock()
		if win != nil {
			win.Invalidate()
		}
	}()
}

// applyPendingTestResult moves a completed background test-run result (if any)
// into r.state.CommandTest on the UI thread (task 16.3). It is called at the top
// of layoutEditorForm, on the frame-loop thread, so CommandTest is never written
// from the goroutine directly. It takes the pending result under cfg.testMu,
// clears the holder, then applies it (Running=false) outside the lock.
func (r *Renderer) applyPendingTestResult() {
	r.cfg.testMu.Lock()
	res := r.cfg.testResult
	r.cfg.testResult = nil
	r.cfg.testMu.Unlock()

	if res == nil {
		return
	}
	r.state.CommandTest = CommandTestState{
		Running: false,
		Output:  res.Output,
		Success: res.Success,
	}
}

// fieldTestRunOutput renders the test-run OUTPUT area as its own form row (task
// 16.3, Requirement 12). The Test Run BUTTON now lives inline in editorActions
// (between Save and Clear), so this row no longer draws a "Test Run" caption or
// a standalone button — it only delegates to layoutTestRunOutput, which shows
// the "Executing…" indicator while a run is in progress (Requirement 12.5), the
// combined output in green on success (Requirement 12.2), error output in red on
// failure (Requirement 12.4 — also covers the "No command to test" message), or
// an empty-state hint when there is no result yet.
func (r *Renderer) fieldTestRunOutput(gtx layout.Context) layout.Dimensions {
	return r.layoutTestRunOutput(gtx)
}

// layoutTestRunOutput renders the test-run output area below the Test Run button
// (task 16.3). It shows, in priority order: an "Executing…" hint while a run is
// in progress (Requirement 12.5); nothing when there is no result yet and no run
// in progress; otherwise the combined output text colored green on success
// (Requirement 12.2) or red on failure (Requirement 12.4 — also covers the
// "No command to test" message, which is a non-success state). The text is drawn
// in a boxed area (reusing the field-box affordance) so it reads as an output
// panel, and wraps for multi-line output.
func (r *Renderer) layoutTestRunOutput(gtx layout.Context) layout.Dimensions {
	th := r.ensureCfgTheme()
	ct := r.state.CommandTest

	if ct.Running {
		return r.layoutFieldBox(gtx, func(gtx layout.Context) layout.Dimensions {
			l := material.Body2(th, "Executing…")
			l.Color = cfgTestHintFg
			return l.Layout(gtx)
		})
	}

	if ct.Output == "" {
		// No result yet: show a subtle hint so the panel isn't empty/confusing.
		l := material.Caption(th, "Run to see the command's output here")
		l.Color = cfgTestHintFg
		return l.Layout(gtx)
	}

	return r.layoutFieldBox(gtx, func(gtx layout.Context) layout.Dimensions {
		l := material.Body2(th, ct.Output)
		if ct.Success {
			l.Color = cfgTestSuccessFg
		} else {
			l.Color = cfgTestFailFg
		}
		return l.Layout(gtx)
	})
}
