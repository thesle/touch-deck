// Command touchdeck is the Gio entry point for TouchDeck.
//
// This replaces the previous Wails entry point: the //go:embed of
// frontend/dist, the embed import, and the wails.Run(...) call are all gone
// (Requirements 2.4, 1.1). main() now builds the framework-agnostic core
// Backend, loads the configuration, constructs the in-memory UI AppState, and
// hands control to the Gio_Renderer event loop in internal/ui (Requirement
// 1.4). main.go is intentionally thin — the render loop lives in
// internal/ui/render.go.
package main

import (
	"log"
	"os"

	"gioui.org/app"

	"touchdeck/internal/core"
	"touchdeck/internal/ui"
)

func main() {
	// Backend performs all side effects (config load/save, command execution,
	// image handling). It has no Gio dependency.
	backend := core.New()

	// LoadConfig returns a usable default when no file exists; a genuine
	// read/decode error is non-fatal here — log it and continue with whatever
	// Config was returned so the app stays usable (design: Error Handling).
	cfg, err := backend.LoadConfig()
	if err != nil {
		log.Printf("touchdeck: load config: %v (continuing with returned config)", err)
	}

	// AppState is the single source of UI truth held between frames. The Deck
	// view is active on start (Requirement 1.6).
	state := ui.NewAppState(backend, cfg)
	state.Textures = ui.NewTextureCache()

	go func() {
		// v0.10.2 window construction: new(app.Window) then w.Option(...).
		// There is no app.NewWindow in this version. The window starts
		// windowed with a title bar (Requirements 1.6, 7.1); full-screen is a
		// later task.
		w := new(app.Window)
		w.Option(
			app.Title("TouchDeck"),
			app.Size(1024, 768),
		)
		if err := ui.Run(w, state); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()

	app.Main()
}
