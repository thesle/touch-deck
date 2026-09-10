// This file implements the Gio_Renderer event/render loop (task 6.3) per the
// design's "Architecture -> Immediate-Mode Render Loop", "Application State
// Model", "Components and Interfaces -> Renderer", and "Gio_Renderer — Main
// Loop & Full-Screen" sections.
//
// Scope of task 6.3: stand up a runnable skeleton with a persistent header that
// switches between the Deck and Config views (Requirement 1.5). The Deck/Config
// CONTENT is stubbed with placeholders here; real rendering arrives in later
// tasks (8.x Deck_View, 14.x Config_View). Full-screen (F11 + control) is task
// 10 and is deliberately NOT implemented here — the loop is structured so that
// key.Filter handling and pointer routing can be added per frame without
// restructuring.
package ui

import (
	"gioui.org/app"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// Default windowed dimensions. main.go creates the window with
// app.Size(1024, 768); we restore to the same size when leaving full-screen
// (see toggleFullscreen / BUG 2 fix below). Kept here as the single source of
// truth for the windowed geometry the app returns to.
const (
	defaultWindowedW = unit.Dp(1024)
	defaultWindowedH = unit.Dp(768)
)

// Renderer owns the transient Gio-side state that does not belong on AppState:
// the material theme and the header's widget.Clickables. Keeping widget state
// here (rather than on AppState) keeps AppState a pure data model of "what to
// draw" while the Renderer holds "how we're drawing it and which widgets are
// live". Later view tasks (8.x/14.x) add their own widget state either here or
// in dedicated per-view structs.
type Renderer struct {
	state *AppState
	th    *material.Theme

	// w is the Gio window handle, threaded in from Run so the Renderer can
	// change the window mode for the full-screen toggle (task 10.1,
	// Requirements 7.2, 7.4). It is assigned once at the top of Run before the
	// event loop starts.
	w *app.Window

	// windowedW/windowedH are the dimensions the window is restored to when
	// leaving full-screen. In v0.10.2 app.Windowed.Option() only sets
	// cnf.Mode = Windowed and does NOT set a size, so on X11 window managers
	// switching back to Windowed without a size often fails to restore the
	// previous geometry and decorations. We therefore restore both the mode
	// AND an explicit size (see toggleFullscreen). They default to the
	// startup size main.go uses (1024x768).
	windowedW unit.Dp
	windowedH unit.Dp

	// Header controls that switch the active view (Requirement 1.5).
	deckBtn   widget.Clickable
	configBtn widget.Clickable

	// fsBtn is the on-screen full-screen toggle control in the header
	// (Requirements 7.4, 7.5). Enhancement 7: the header is now HIDDEN while
	// full-screen (see layout), so this control is not visible in full-screen.
	// The user returns to windowed mode via F11 (handleKeys) or the context
	// menu's Fullscreen / Exit Fullscreen item (deck.go layoutMenuPanel).
	fsBtn widget.Clickable

	// cfg holds the Config_View's Gio widget state (task 14.x, defined in
	// config.go). It lives on the Renderer so AppState stays a pure data model;
	// the Config_View layout code reads/writes it. For task 14.1 it is empty
	// (no live preview widgets yet).
	cfg configState

	// deck holds the Deck_View's Gio widget state (task 9.x, defined in
	// deck.go). It lives on the Renderer so AppState stays a pure data model;
	// the Deck_View layout code reads/writes it. Task 9.1 uses it for the
	// per-button tap Clickables.
	deck deckState
}

// NewRenderer builds a Renderer bound to state. material.NewTheme() in Gio
// v0.10.2 takes no arguments — it bundles the gofont collection and a default
// text.Shaper, so no separate font collection is required.
func NewRenderer(state *AppState) *Renderer {
	return &Renderer{
		state:     state,
		th:        material.NewTheme(),
		windowedW: defaultWindowedW,
		windowedH: defaultWindowedH,
	}
}

// Run drives the immediate-mode event/render loop for the window until it is
// destroyed. It is the single public entry point main.go calls, keeping main
// thin. The loop:
//
//  1. blocks on w.Event() (v0.10.2 pull-based event API),
//  2. on app.DestroyEvent returns e.Err (nil on a normal close),
//  3. on app.FrameEvent builds gtx from a persistent op.Ops, lays out the
//     header + active view, then flushes with e.Frame(gtx.Ops).
//
// A persistent op.Ops is reused across frames; app.NewContext calls ops.Reset
// each frame, so it must not be reallocated per frame.
func Run(w *app.Window, state *AppState) error {
	r := NewRenderer(state)
	// Thread the window handle to the Renderer so toggleFullscreen can change
	// the window mode (task 10.1). Run's signature is unchanged, so main.go
	// still calls ui.Run(w, state) as before.
	r.w = w
	var ops op.Ops

	for {
		switch e := w.Event().(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)

			// Process input and lay out the frame. Later tasks add pointer
			// routing and a key.Filter (F11) before/inside this call.
			r.layout(gtx)

			e.Frame(gtx.Ops)
		}
	}
}

// layout processes header input and dispatches to the active view. This is the
// stable seam later tasks fill in: header stays constant, the view body is
// swapped by AppState.View.
func (r *Renderer) layout(gtx layout.Context) layout.Dimensions {
	// Process the F11 key toggle before laying out the frame (Requirement 7.2).
	r.handleKeys(gtx)

	// Process header control clicks first so the view dispatched below reflects
	// the switch on the same frame.
	if r.deckBtn.Clicked(gtx) {
		r.switchView(ViewDeck)
	}
	if r.configBtn.Clicked(gtx) {
		r.switchView(ViewConfig)
	}
	// The on-screen full-screen control toggles the same way F11 does
	// (Requirements 7.4, 7.5).
	if r.fsBtn.Clicked(gtx) {
		r.toggleFullscreen()
	}

	// Enhancement 7: hide the header while full-screen so the active view fills
	// the whole window. The header (Deck/Config/Exit-Fullscreen buttons) is only
	// laid out when NOT full-screen. The header click widgets are still drained
	// above regardless of full-screen, so state stays consistent even when the
	// header is not painted (the clickables simply receive no events). The user
	// returns to windowed via F11 (handleKeys) or the context menu's Fullscreen
	// item (see deck.go layoutMenuPanel).
	children := make([]layout.FlexChild, 0, 2)
	if !r.state.Fullscreen {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return r.layoutHeader(gtx)
		}))
	}
	children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		switch r.state.View {
		case ViewConfig:
			return r.layoutConfig(gtx)
		default:
			return r.layoutDeck(gtx)
		}
	}))
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

// switchView changes the active top-level view (Requirement 1.5). Switching
// clears any Config_View slot selection so a stale selection from a prior visit
// does not leak across views; later Config tasks (14.x) own richer selection
// semantics.
func (r *Renderer) switchView(v View) {
	if r.state.View == v {
		return
	}
	r.state.View = v
	r.state.SelectedSlot = nil
}

// handleKeys processes F11 presses to toggle full-screen (Requirement 7.2).
//
// v0.10.2 global-shortcut idiom (verified against io/input):
//
// A key.Filter whose Focus field is nil is a GLOBAL filter: it matches key
// events regardless of which tag (if any) currently holds keyboard focus.
// This is enforced in io/input/key.go keyFilterMatch, which short-circuits the
// focus check with `if f.Focus != nil && f.Focus != focus { return false }` —
// when Focus is nil the focus comparison is skipped entirely. The router test
// io/input/key_test.go:TestKeyRouting confirms this: a bare key.Filter{Name:
// "B"} is delivered even though nothing is focused, whereas key.Filter{Focus:
// h, Name: "A"} is only delivered after h receives focus.
//
// The previous implementation used key.Filter{Focus: tag, ...} together with
// an event.Op + key.FocusCmd focus dance. On X11 that focus never reliably
// stuck to the bare root tag (nothing grants/holds focus for it the way a
// clip-backed widget does), so F11 was filtered out and never toggled — the
// root cause of BUG 1. Because a full-screen shortcut is inherently
// window-global, a focus-independent filter is both simpler and correct, and
// it needs no event.Op/FocusCmd.
//
// We drain all matching events for the frame and toggle on the Press edge only
// (ignoring Release) so one physical key press yields one toggle.
func (r *Renderer) handleKeys(gtx layout.Context) {
	for {
		ev, ok := gtx.Event(key.Filter{Name: key.NameF11})
		if !ok {
			break
		}
		if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
			r.toggleFullscreen()
		}
	}
}

// toggleFullscreen flips AppState.Fullscreen and switches the window between
// full-screen and windowed mode (Requirements 7.2, 7.4). Gio's full-screen
// window mode is borderless, so the title bar is hidden while full-screen
// (Requirement 7.3); returning to Windowed restores it. Both F11 and the
// on-screen control call this. Enhancement 7: the header (with the on-screen
// control) is HIDDEN while full-screen (see layout), so the user returns to
// windowed mode via F11 or the context menu's Fullscreen / Exit Fullscreen item
// (deck.go layoutMenuPanel), which is available on every context menu
// (Requirement 7.5).
func (r *Renderer) toggleFullscreen() {
	r.state.Fullscreen = !r.state.Fullscreen
	if r.state.Fullscreen {
		r.w.Option(app.Fullscreen.Option())
	} else {
		// BUG 2 fix: restore BOTH the windowed mode AND an explicit size.
		//
		// In v0.10.2, app.Windowed.Option() only sets cnf.Mode = Windowed
		// (see io/os.go WindowMode.Option). It does not touch cnf.Size, so on
		// X11 the window manager was left to re-derive geometry and often did
		// not restore the previous windowed size or re-add the title bar.
		// app.Size(w, h) sets both cnf.Mode = Windowed and cnf.Size (see
		// app/window.go Size). Passing it alongside Windowed.Option() makes the
		// intent explicit and guarantees a concrete size is applied. The title
		// bar returns because Gio's fallback decorations are drawn whenever
		// cnf.Mode != Fullscreen (app/window.go fallbackDecorate/decorate), and
		// leaving Fullscreen for Windowed re-enables them.
		r.w.Option(app.Windowed.Option(), app.Size(r.windowedW, r.windowedH))
	}
}

// layoutHeader draws the top bar with the "Deck"/"Config" buttons and the
// full-screen control. The button for the active view is drawn in the theme's
// contrast color so the current view is obvious. Enhancement 7: the header is
// only laid out when NOT full-screen (see layout); while full-screen it is
// hidden so the active view fills the whole window, and the user returns to
// windowed mode via F11 or the context menu's Fullscreen / Exit Fullscreen item.
func (r *Renderer) layoutHeader(gtx layout.Context) layout.Dimensions {
	return layout.Inset{
		Top:    unit.Dp(8),
		Bottom: unit.Dp(8),
		Left:   unit.Dp(12),
		Right:  unit.Dp(12),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				btn := material.Button(r.th, &r.deckBtn, "Deck")
				if r.state.View != ViewDeck {
					btn.Background = r.th.Palette.Bg
					btn.Color = r.th.Palette.Fg
				}
				return btn.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				btn := material.Button(r.th, &r.configBtn, "Config")
				if r.state.View != ViewConfig {
					btn.Background = r.th.Palette.Bg
					btn.Color = r.th.Palette.Fg
				}
				return btn.Layout(gtx)
			}),
			// Spacer pushes the full-screen control to the right edge.
			layout.Flexed(1, layout.Spacer{}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				// Label reflects the current mode so the control doubles as the
				// return-to-windowed control while full-screen (Req 7.5).
				label := "Fullscreen"
				if r.state.Fullscreen {
					label = "Exit Fullscreen"
				}
				return material.Button(r.th, &r.fsBtn, label).Layout(gtx)
			}),
		)
	})
}

// layoutDeck is implemented in deck.go (task 8.1): it renders the real Deck_View
// grid, resolving each slot's content and painting the tiles. The render loop's
// dispatch in layout() calls it via r.layoutDeck. Tap handling/flash (9.1),
// pagination behavior (9.2), and the context menu (11) arrive in later tasks.

// layoutConfig is implemented in config.go (task 14.1): it renders the real
// Config_View split-pane (live grid preview on the left, slot editor area on the
// right). The render loop's dispatch in layout() calls it via r.layoutConfig.
// Adjusters (14.2), page selector + slot selection (14.3), editor form fields
// (15.x), and move/copy/paste/test-run (16.x) arrive in later tasks.
