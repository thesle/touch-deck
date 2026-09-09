// Package ui is the Gio_Renderer layer for TouchDeck. It owns the window, the
// immediate-mode event/render loop, the application state, and the Deck_View,
// Config_View, Context_Menu, and reusable widgets. It calls into
// touchdeck/internal/core for all side effects.
//
// This file defines the in-memory UI models (task 6.1) per the design's
// "Data Models -> In-Memory UI Models (internal/ui)" section. Related pieces
// live in sibling files:
//   - task 6.2 implements the TextureCache (texture.go); the AppState.Textures
//     field references *TextureCache defined there.
//   - task 6.3 rewrites main.go and implements the Renderer event/render loop.
package ui

import (
	"time"

	"gioui.org/f32"

	"touchdeck/internal/core"
)

// View identifies which top-level view is active (Deck vs. Config). The Renderer
// dispatches to the matching layout function based on AppState.View.
type View int

const (
	// ViewDeck is the runtime deck grid view, shown on start (Requirement 1.6).
	ViewDeck View = iota
	// ViewConfig is the split-pane configuration editor view.
	ViewConfig
)

// TotalPages is the fixed number of deck pages (Requirement 6.1). Pagination
// wraps over this count.
const TotalPages = 5

// ButtonClipboard holds a copied button's settings for paste actions, shared by
// the Deck_View Context_Menu and the Config_View copy/paste controls. It carries
// only the visual/behavioral settings: the id and order belong to the target
// slot, not the copied settings (design: Data Models -> ButtonClipboard).
type ButtonClipboard struct {
	Label     string
	Command   string
	BgImage   string
	BgColor   string
	FontColor string
	FontSize  int
}

// EditorState is the Config_View form state. It mirrors the Svelte edit*
// variables (design: Data Models -> EditorState). When a slot is selected, the
// renderer loads the existing button here, or initializes an empty button with a
// generated id and the default colors below. Defaults are applied on slot
// selection in later tasks (6.3 / 14.x), not here.
type EditorState struct {
	ID        string
	Label     string
	Command   string
	BgImage   string
	BgColor   string // default "#1f2937" (applied on slot selection)
	FontColor string // default "#ffffff" (applied on slot selection)
	FontSize  int    // font-size offset in the range -2..+2
}

// CommandTestState tracks the Config_View test-run panel. While Running is true
// the panel shows an "executing" indicator; once complete, Output holds the
// combined stdout/stderr and Success reports whether the command succeeded
// (Requirement 12).
type CommandTestState struct {
	Running bool
	Output  string
	Success bool
}

// MenuState is the open/closed state of the Deck_View Context_Menu. When Open is
// true the menu is laid out as an overlay at Position for the target Slot
// (Requirement 8).
type MenuState struct {
	Open     bool
	Slot     int       // target slot index
	Position f32.Point // screen position where the menu opens
}

// LongPressState tracks an in-progress long-press on the Deck_View. On primary
// press the renderer records the target Slot, the press Origin, and the Start
// time; the menu opens once the press is held past the threshold without moving
// beyond the allowed radius (Requirement 8.1).
type LongPressState struct {
	Active bool
	Slot   int
	Origin f32.Point
	Start  time.Time
}

// AppState is the single source of UI truth held by the renderer between frames
// (design: Data Models -> AppState). It mirrors the on-disk Config in memory and
// carries all transient UI state.
type AppState struct {
	// Backend performs all side effects (config load/save, command execution,
	// image handling).
	Backend *core.Backend
	// View is the active top-level view (Deck vs. Config).
	View View
	// Config is the in-memory configuration, mirroring the on-disk file.
	Config core.Config
	// CurrentPage is the active page index, 0..TotalPages-1.
	CurrentPage int
	// SelectedSlot is the slot selected for editing in Config_View; nil when
	// none is selected.
	SelectedSlot *int
	// Fullscreen reports whether the window is currently borderless full-screen.
	Fullscreen bool
	// Clipboard holds copied button settings; nil when empty.
	Clipboard *ButtonClipboard
	// Editor holds the Config_View form fields.
	Editor EditorState
	// CommandTest holds the Config_View test-run panel state.
	CommandTest CommandTestState
	// Menu holds the Deck_View Context_Menu state.
	Menu MenuState
	// LongPress tracks an in-progress long-press on the Deck_View.
	LongPress LongPressState
	// Flash maps a button id to its tactile-flash deadline; the tile is drawn
	// highlighted while now is before the deadline (Requirement 5.2).
	Flash map[string]time.Time
	// Textures caches decoded images keyed by absolute image path. The
	// TextureCache type is defined by task 6.2 (texture.go).
	Textures *TextureCache
}

// NewAppState builds an AppState for startup: Deck_View active, first page
// selected, an initialized Flash map, and no selection/clipboard/menu. Later
// tasks (6.3) wire the real window/texture-cache startup; this keeps
// construction simple and centralizes zero-value initialization.
func NewAppState(b *core.Backend, cfg core.Config) *AppState {
	return &AppState{
		Backend:     b,
		View:        ViewDeck,
		Config:      cfg,
		CurrentPage: 0,
		Flash:       make(map[string]time.Time),
	}
}
