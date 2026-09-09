# Implementation Plan: Wails-to-Gio Migration

## Overview

This plan migrates TouchDeck from Wails (WebView + Svelte/Tailwind) to Gio (gioui.org),
following the design's two-package architecture: a framework-agnostic `internal/core`
Backend and an `internal/ui` Gio_Renderer wired together by `main.go`.

The work is ordered so the **core package and its property tests land first**, then a
**runnable (if incomplete) Gio app skeleton** appears early for incremental verification,
then the Deck_View, Config_View, context menu, and new full-screen capability are built on
top. Wails removal and frontend cleanup happen after the Gio app is functional so a working
build is preserved at each step.

Testing follows the design's Testing Strategy:
- Property-based tests use [`gopter`](https://github.com/leanovate/gopter), run a **minimum
  of 100 iterations** (`MinSuccessfulTests`), operate against a **temporary config root**
  (redirected `ConfigPath`/`ImagesDir`), and are tagged with a comment referencing the
  design property, e.g. `// Feature: wails-to-gio-migration, Property 1: Configuration round-trip`.
- The nine correctness properties target `internal/core` and the pure state helpers only.
- Gio rendering, gestures, full-screen, and the native dialog are **GUI-only** and are
  verified manually per the design; those tasks are marked **[Manual verification]**.

Tasks marked with `*` are optional test sub-tasks and can be skipped for a faster MVP.

## Tasks

- [x] 1. Migrate module dependencies and create package skeleton
  - [x] 1.1 Update `go.mod` for Gio and native dialog
    - Remove the `github.com/wailsapp/wails/v2` require and its transitive `// indirect` entries; drop the commented `replace` line
    - Add `gioui.org` and `github.com/sqweek/dialog`
    - Run `go mod tidy` so `go.sum` reflects the new dependency graph
    - _Requirements: 1.2, 13.1_

  - [x] 1.2 Create the `internal/core` and `internal/ui` package skeletons
    - Create `internal/core/` with a `Backend` struct and `New() *Backend` constructor (empty method stubs to be filled in later tasks)
    - Create `internal/ui/` with a package declaration and placeholder types
    - Keep the project compiling with `go build ./...` (stubs return zero values)
    - _Requirements: 1.4_

- [x] 2. Extract the Config_Store into `internal/core`
  - [x] 2.1 Move the persisted data models into `internal/core`
    - Copy `ButtonConfig`, `PageConfig`, `Config` verbatim from `app.go` with JSON tags frozen (`id`, `label`, `command`, `bgImage`, `bgColor`, `fontColor`, `fontSize`, `order`, `pageIndex`, `rows`, `cols`, `pages`)
    - _Requirements: 3.2_

  - [x] 2.2 Implement `ConfigPath`/`ImagesDir` with a testable config-root seam
    - Port `getConfigPath` → `ConfigPath()` and `getImagesDir` → `ImagesDir()`, `MkdirAll(0755)` before use
    - Resolve the base dir through a single indirection defaulting to `os.UserConfigDir` but overridable in tests (unexported `configRoot` field or env seam) so Linux behavior stays `~/.config/touchdeck/`
    - _Requirements: 3.1, 14.3_

  - [x] 2.3 Implement `LoadConfig` with default creation and legacy migration
    - Port `LoadConfig`: missing file → build the default Config (2 rows, 4 cols, one page, 3 sample buttons), save it, return it; legacy flat `buttons[]` with `page` → group into `pages`; modern config with `pages` → load as-is; read/decode failure → return error
    - Change the receiver from `*App` to `*Backend`
    - _Requirements: 3.3, 3.4, 3.5, 3.7_

  - [x] 2.4 Implement `SaveConfig`
    - Port `SaveConfig`: write indented JSON to the config path, return errors to the caller
    - _Requirements: 3.6_

  - [x]* 2.5 Write property test for configuration round-trip
    - **Property 1: Configuration round-trip** — save then load an arbitrary valid `Config` yields an equal `Config` (rows, cols, and pages/buttons up to page ordering)
    - Use a gopter generator for valid `Config` (bounded rows/cols, pages 0–4, in-range `order`, `fontSize` −2..+2), temp config root, `MinSuccessfulTests >= 100`, tag `// Feature: wails-to-gio-migration, Property 1`
    - **Validates: Requirements 3.2, 3.5, 1.4**

  - [x]* 2.6 Write property test for legacy migration membership
    - **Property 2: Legacy migration preserves membership** — every button lands on the page whose `pageIndex` equals its `page`; no button dropped or duplicated (total count preserved, per-page set correct)
    - gopter generator for Legacy_Config (flat buttons with `page` fields), temp config root, `MinSuccessfulTests >= 100`, tag `// Feature: wails-to-gio-migration, Property 2`
    - **Validates: Requirements 3.4**

  - [x]* 2.7 Write unit tests for Config_Store
    - Default-config creation on missing file, path resolution under a redirected config root, indented-JSON formatting, and malformed-config error path
    - _Requirements: 3.1, 3.3, 3.6, 3.7_

- [x] 3. Extract the Command_Runner and Image_Manager into `internal/core`
  - [x] 3.1 Port the Command_Runner
    - Port `RunCommandAsync` (`bash -c`, `Start()` then `Wait()` in a goroutine, return start errors) and `RunCommandSync` (`bash -c` under a 10s `context.WithTimeout`, return `CombinedOutput`, timeout error on deadline) with a `*Backend` receiver
    - _Requirements: 5.1, 5.4, 5.5, 14.2_

  - [x] 3.2 Port the Image_Manager copy/list and replace `SelectImage` with the native dialog
    - Port `CopyImageToConfig` (timestamp-prefixed copy into images dir, return dest path) and `ListConfigImages` (extension filter `.png/.jpg/.jpeg/.gif/.webp/.svg`) unchanged with a `*Backend` receiver
    - Reimplement `SelectImage` using `sqweek/dialog` (`dialog.File().Title(...).Filter("Images", ...).Load()`); map `dialog.ErrCancelled` to a "no selection" result so callers leave `bgImage` unchanged
    - Delete `GetImageBase64` (WebView-only workaround) — do not port it
    - _Requirements: 13.1, 13.2, 13.3, 13.7_

  - [x]* 3.3 Write property test for image copy content and naming
    - **Property 8: Image copy preserves content and naming** — copied file's bytes equal the source and its name is `<digits>_<basename>`; returns that dest path
    - Temp images dir, `MinSuccessfulTests >= 100`, tag `// Feature: wails-to-gio-migration, Property 8`
    - **Validates: Requirements 13.2**

  - [x]* 3.4 Write property test for image listing extension filter
    - **Property 9: Image listing extension filter** — returns exactly the files whose lower-cased extension is in the frozen set, excluding all others and subdirectories
    - Temp images dir seeded with mixed files, `MinSuccessfulTests >= 100`, tag `// Feature: wails-to-gio-migration, Property 9`
    - **Validates: Requirements 13.7**

  - [x]* 3.5 Write integration tests for Command_Runner and image copy
    - `RunCommandSync` against real `bash` (echo stdout, stderr capture, non-zero exit, and a bounded `sleep`-based 10s timeout check); one `CopyImageToConfig` round trip
    - _Requirements: 5.4, 5.5, 13.2_

- [x] 4. Extract pure state helpers into `internal/core`
  - [x] 4.1 Implement grid-math and content-resolution helpers
    - Add pure helpers operating on `Config`/`PageConfig` values (no Gio types): grid index ↔ row/col, `fontSize` offset → point-size mapping, `getButtonAtSlot(page, slot)`, `isRenderable(button)` predicate, and the pagination indicator string (`current / total`)
    - _Requirements: 4.6, 4.8, 6.5_

  - [x] 4.2 Implement pagination and slot-mutation helpers
    - Add pure helpers: `nextPage`/`prevPage` (wrap over `TotalPages`), `pruneToGrid(cfg, rows, cols)`, `moveButton(page, sel, dir)` (swap by rewriting `order`, no-op on out-of-bounds/pagination target), `writeSlot(page, slot, settings)` (write fields + `order`, replace any button at that slot)
    - _Requirements: 6.3, 6.4, 9.4, 10.6, 11.2, 11.3_

  - [x]* 4.3 Write property test for grid-size prune invariant
    - **Property 3: Grid-size prune invariant** — after `pruneToGrid`, no page has a button with `order >= rows*cols`
    - gopter, `MinSuccessfulTests >= 100`, tag `// Feature: wails-to-gio-migration, Property 3`
    - **Validates: Requirements 9.3, 9.4**

  - [x]* 4.4 Write property test for slot-swap self-inverse
    - **Property 4: Slot-swap is a self-inverse** — a valid move swaps exactly the two buttons' `order` and the opposite move restores the page; an invalid move (out of bounds or Pagination_Slot) leaves the page unchanged
    - gopter, `MinSuccessfulTests >= 100`, tag `// Feature: wails-to-gio-migration, Property 4`
    - **Validates: Requirements 11.2, 11.3**

  - [x]* 4.5 Write property test for pagination wrap
    - **Property 5: Pagination wrap** — advancing yields `(p+1) mod TotalPages`, going back yields `(p-1+TotalPages) mod TotalPages`; back-after-advance restores `p`; results stay in range
    - gopter (or `testing/quick`), `MinSuccessfulTests >= 100`, tag `// Feature: wails-to-gio-migration, Property 5`
    - **Validates: Requirements 6.3, 6.4**

  - [x]* 4.6 Write property test for button-order uniqueness per page
    - **Property 6: Button-order uniqueness per page** — after any sequence of write/paste, clear, move, and prune operations, no page has two buttons sharing an `order`
    - gopter sequence generator, `MinSuccessfulTests >= 100`, tag `// Feature: wails-to-gio-migration, Property 6`
    - **Validates: Requirements 4.8, 10.6, 11.2**

  - [x]* 4.7 Write property test for slot-write field/order preservation
    - **Property 7: Slot-write preserves fields and order** — after `writeSlot`, the page has a button at the target slot whose `label/command/bgImage/bgColor/fontColor/fontSize` equal the written settings and whose `order` equals the slot index, replacing any prior button there
    - gopter, `MinSuccessfulTests >= 100`, tag `// Feature: wails-to-gio-migration, Property 7`
    - **Validates: Requirements 8.6, 10.6, 11.6**

  - [x]* 4.8 Write unit tests for pure state helpers
    - Grid math edge cases (index↔row/col), fontSize offset mapping, `getButtonAtSlot`, `isRenderable`, and the pagination indicator string
    - _Requirements: 4.6, 4.8, 6.5_

- [x] 5. Checkpoint - core package complete and tested
  - Run `go build ./...` and `go test ./internal/core/...`; ensure all core unit and property tests pass. Ask the user if questions arise.

- [x] 6. Build the Gio app skeleton and wire it to `internal/core`
  - [x] 6.1 Define the in-memory UI models in `internal/ui`
    - Add `View` (`ViewDeck`/`ViewConfig`), `TotalPages = 5`, `ButtonClipboard`, `EditorState`, `CommandTestState`, `MenuState`, `LongPressState`, and `AppState` (holding `*core.Backend`, `Config`, `CurrentPage`, `SelectedSlot`, `Fullscreen`, `Clipboard`, `Editor`, `CommandTest`, `Menu`, `LongPress`, `Flash`, `Textures`) per the design's Data Models
    - _Requirements: 1.5_

  - [x] 6.2 Implement the TextureCache
    - Add `TextureCache` keyed by absolute image path: `Get(path)` decodes via `image.Decode` (`image/png`, `image/jpeg`, `image/gif` registered) on miss and caches the `paint.ImageOp`; failures recorded in a `bad` set to avoid retry storms; `Invalidate(path)`
    - _Requirements: 4.3_

  - [x] 6.3 Rewrite `main.go` and implement the Renderer event/render loop
    - Replace the Wails `main.go`: remove the `embed` import, the `//go:embed all:frontend/dist` directive, and the `wails.Run(...)` call
    - Create a Gio window windowed with title `TouchDeck` (`app.Title(...)`), load the initial `Config` via `core.LoadConfig`, initialize `AppState` with `View = ViewDeck`
    - Implement the loop: block on `window.Event()`, on `app.FrameEvent` build `gtx`, process input, dispatch to the active view (`layoutDeck`/`layoutConfig`) by `AppState.View`, overlay the menu when open, flush with `event.Frame`, exit on `app.DestroyEvent`
    - Add a persistent header/control that switches between Deck_View and Config_View (stub view layout funcs render placeholders for now)
    - _Requirements: 1.1, 1.4, 1.5, 1.6, 2.4_

  - [x] 6.4 [Manual verification] Confirm the skeleton launches
    - Manually verify the window opens windowed with a title bar, shows the Deck_View placeholder on start, and the header switches to the Config_View placeholder
    - _Requirements: 1.1, 1.5, 1.6, 7.1_

- [x] 7. Checkpoint - runnable Gio skeleton
  - Run `go build ./...`; confirm the app launches and view switching works. Ask the user if questions arise.

- [x] 8. Implement Deck_View rendering
  - [x] 8.1 Render the deck grid and resolve slot content
    - Lay out `rows × cols` equal cells with `layout.Flex`; slot index `i = row*C + col`; reserve `prevSlot = R*C-2`, `nextSlot = R*C-1`
    - For each non-pagination slot use `getButtonAtSlot` to find the button whose `order == i`; render precedence: decodable `bgImage` via TextureCache (scaled to cover, centered, translucent dark overlay) → `bgColor` fill → default `#1f2937`; draw label centered in `fontColor` at default size + offset; webp/svg or decode failure falls back to color + label (no crash)
    - Render unconfigured slots as dashed empty placeholders
    - _Requirements: 4.1, 4.2, 4.3, 4.4, 4.5, 4.6, 4.7, 4.8_

  - [x] 8.2 [Manual verification] Verify deck rendering fidelity
    - Manually verify image cover/center, color fill, default background, font color, all five font-size offsets, dashed placeholders, and graceful fallback on a `.webp`/`.svg` image
    - _Requirements: 4.3, 4.4, 4.5, 4.6, 4.7_

- [x] 9. Implement Deck_View interaction
  - [x] 9.1 Implement tap execution and tactile flash
    - Register pointer input per slot; a tap on a configured button with a non-empty command calls `RunCommandAsync` and sets `Flash[button.id] = now + 150ms`; render the highlight (`#3b82f6`) while `now < Flash[id]` and schedule an invalidate at the deadline; empty-command taps do nothing
    - _Requirements: 5.1, 5.2, 5.3_

  - [x] 9.2 Implement pagination controls
    - Render `prevSlot` as a Prev control (tap → `(CurrentPage-1+TotalPages) % TotalPages`) and `nextSlot` as a Next control with a `current+1 / TotalPages` indicator (tap → `(CurrentPage+1) % TotalPages`), using the core pagination helpers
    - _Requirements: 6.1, 6.2, 6.3, 6.4, 6.5_

  - [x] 9.3 [Manual verification] Verify tap flash and pagination
    - Manually verify the ~150ms highlight on tap, that empty-command taps are inert, and that Prev/Next wrap correctly with the `current / total` indicator
    - _Requirements: 5.2, 5.3, 6.3, 6.4, 6.5_

- [x] 10. Implement the full-screen toggle
  - [x] 10.1 Implement `toggleFullscreen` with F11 and an on-screen control
    - Register a `key.Filter` for `key.NameF11` on a focused root area each frame; F11 and the on-screen control both flip `AppState.Fullscreen` and issue `window.Option(app.Fullscreen)` / `app.Windowed`
    - Keep the on-screen control visible while full-screen so it returns to windowed mode; window starts windowed with a title bar
    - _Requirements: 7.1, 7.2, 7.3, 7.4, 7.5_

  - [x] 10.2 [Manual verification] Verify full-screen behavior
    - Manually verify F11 and the on-screen control both toggle, the title bar is hidden in full-screen, and the return-to-windowed control works while full-screen
    - _Requirements: 7.1, 7.2, 7.3, 7.4, 7.5_

- [x] 11. Implement the Context_Menu on the Deck_View
  - [x] 11.1 Implement long-press and right-click detection
    - On primary press start `LongPress{Slot, Origin, Start}`; each frame open the menu when the press is still held, movement stayed within 10px, and elapsed ≥ 500ms; a release or >10px drag before threshold cancels and is treated as a normal tap
    - Open the menu immediately on a secondary-button (`pointer.ButtonSecondary`) press; suppress the menu entirely on Pagination_Slots
    - _Requirements: 8.1, 8.2, 8.8_

  - [x] 11.2 Implement the Context_Menu overlay and actions
    - When `Menu.Open`, lay out an overlay at `Menu.Position` with Edit / Copy / Paste
    - Edit → `View = ViewConfig`, select target slot (load values into `Editor`), close; Copy → `Clipboard = &ButtonClipboard{...}`, close; Paste → apply clipboard to the target slot (preserve `order`, keep/generate `id`) via `writeSlot`, `SaveConfig`, close; Paste greyed/unavailable when `Clipboard == nil`; tap outside closes with no action
    - _Requirements: 8.3, 8.4, 8.5, 8.6, 8.7, 8.9_

  - [x] 11.3 [Manual verification] Verify context-menu gestures and actions
    - Manually verify long-press (touch) and right-click (mouse) open the menu, pagination slots suppress it, Edit bridges to Config_View, Copy/Paste round-trips, Paste is unavailable when clipboard empty, and outside-tap dismisses
    - _Requirements: 8.1, 8.2, 8.4, 8.5, 8.6, 8.7, 8.8, 8.9_

- [x] 12. Checkpoint - Deck_View fully functional
  - Run `go build ./...` and `go test ./...`; manually verify deck rendering, taps, pagination, full-screen, and the context menu. Ask the user if questions arise.

- [x] 13. Implement reusable Config_View widgets
  - [x] 13.1 Implement the ColorPicker widget
    - Hex text field + swatch preview + live color; parses hex to the fill color used by the preview and by the tile/text color fields
    - _Requirements: 10.3, 10.4_

  - [x] 13.2 Implement the FontSizeSelector widget
    - Segmented control offering -2 / -1 / Default (0) / +1 / +2, bound to `Editor.FontSize`
    - _Requirements: 10.5_

  - [x]* 13.3 Write unit tests for widget value parsing
    - ColorPicker hex parsing/validation and FontSizeSelector offset selection value mapping
    - _Requirements: 10.3, 10.4, 10.5_

- [x] 14. Implement Config_View grid, pages, and slot selection
  - [x] 14.1 Implement the split-pane layout and live grid preview
    - Horizontal `layout.Flex` with two equal panes: left = grid preview + controls, right = slot editor form (or a placeholder prompt when no slot is selected); preview renders the current page's slots as selectable tiles
    - Render Pagination_Slots as non-editable `[Prev Page]` / `[Next Page]` previews that cannot be selected
    - _Requirements: 9.1, 9.8_

  - [x] 14.2 Implement rows/cols adjusters with prune-and-save
    - `-`/`+` adjusters for rows and cols clamped to a minimum of 1; on a size change call `pruneToGrid` on every page then `SaveConfig`; clear the selection if the selected slot is now out of bounds
    - _Requirements: 9.2, 9.3, 9.4_

  - [x] 14.3 Implement the page selector and slot selection
    - Five page buttons (Page 1–5); selecting one sets `CurrentPage` and clears the selection
    - Selecting an editable slot loads the existing button into `Editor` or initializes an empty button with a generated id (`btn_<random>`) and default colors when unconfigured
    - _Requirements: 9.5, 9.6, 9.7_

  - [x] 14.4 [Manual verification] Verify grid/page/selection interactions
    - Manually verify the split-pane preview, min-1 adjuster bounds, prune-on-shrink, 5-page selector, editable-slot selection, and non-selectable pagination previews
    - _Requirements: 9.1, 9.2, 9.3, 9.5, 9.6, 9.7, 9.8_

- [x] 15. Implement Config_View editor fields, save, and clear
  - [x] 15.1 Implement the editor form fields
    - Single-line `label` editor, multi-line `command` editor, `bgColor` and `fontColor` ColorPickers, the FontSizeSelector, and the background-image dropdown (fed by `ListConfigImages`) + Choose File + Clear
    - Choose File flow: `SelectImage` → (if not cancelled) `CopyImageToConfig` → set `Editor.BgImage` → refresh the image list; dropdown selection sets `Editor.BgImage`; clear sets it to empty
    - _Requirements: 10.1, 10.2, 10.3, 10.4, 10.5, 13.4, 13.5, 13.6_

  - [x] 15.2 Implement save and clear
    - Save writes `label, command, bgImage, bgColor, fontColor, fontSize, order(=selected slot)` into the current page via `writeSlot` (replacing any button at that order) then `SaveConfig`; Clear removes the button at the selected slot and saves
    - _Requirements: 10.6, 10.7_

  - [x] 15.3 [Manual verification] Verify editor fields, save, clear, and image flow
    - Manually verify all form fields update the preview, the native dialog choose-file copies and sets the image, dropdown/clear work, and save/clear persist to `config.json`
    - _Requirements: 10.1, 10.2, 10.3, 10.4, 10.5, 10.6, 10.7, 13.4, 13.5, 13.6_

- [x] 16. Implement Config_View move, copy/paste, and test-run
  - [x] 16.1 Implement the move controls
    - Left/Up/Down/Right compute the adjacent slot via row/col arithmetic and call `moveButton` (swap by rewriting `order`), then `SaveConfig`, and follow the selection to the target; a target that is a Pagination_Slot or out of bounds leaves the Config unchanged
    - _Requirements: 11.1, 11.2, 11.3, 11.4_

  - [x] 16.2 Implement copy/paste of editor settings
    - Copy stores the editor's current settings in the shared `ButtonClipboard`; Paste applies the clipboard to the selected slot via `writeSlot` and saves; Paste is presented as unavailable when the clipboard is empty
    - _Requirements: 11.5, 11.6, 11.7_

  - [x] 16.3 Implement the test-run panel
    - Run `RunCommandSync` on the current editor command on a goroutine, applying the result to `CommandTest` on the next frame: "Executing…" while running, combined output on success, error output + failure on error, and a "no command to test" message when the command is empty
    - _Requirements: 12.1, 12.2, 12.3, 12.4, 12.5_

  - [x] 16.4 [Manual verification] Verify move, copy/paste, and test-run states
    - Manually verify move swaps and follows selection (and no-ops on pagination/out-of-bounds targets), copy/paste round-trips via the shared clipboard, paste unavailable when empty, and all four test-run states display
    - _Requirements: 11.2, 11.3, 11.4, 11.6, 11.7, 12.2, 12.3, 12.4, 12.5_

- [x] 17. Checkpoint - Config_View fully functional
  - Run `go build ./...` and `go test ./...`; manually verify the full configuration editor end to end. Ask the user if questions arise.

- [x] 18. Migrate the build system and remove Wails config
  - [x] 18.1 Update the Makefile and remove `wails.json`
    - Rewrite `build`/`clean` (and drop/replace `dev`) to use the Go/Gio toolchain (`go build` to `build/bin/touchdeck`, `rm -rf` the output) with no `wails` command
    - Delete `wails.json`
    - _Requirements: 2.1, 2.2, 2.3, 2.4_

  - [x] 18.2 Verify no Wails/WebView imports remain and the Linux build succeeds
    - Confirm `go list -m all` is free of `wailsapp/wails`, `go build ./...` and `make build` produce a runnable binary, and the `//go:embed frontend/dist` directive is gone
    - _Requirements: 1.2, 2.1, 2.2, 2.4, 14.1_

- [x] 19. Remove the Svelte/Tailwind frontend and Node tooling
  - [x] 19.1 Delete the frontend, generated bindings, and Node tooling
    - Remove `frontend/src/`, `frontend/dist/`, the generated `frontend/wailsjs/` bindings, and the Node build tooling (`package.json`, `vite.config.js`, `tailwind.config.cjs`, `postcss.config.cjs`, `node_modules/`, etc.)
    - Confirm the Gio app still builds and runs after removal (`go build ./...`, `make build`)
    - _Requirements: 1.3_

- [x] 20. Final checkpoint - migration complete
  - Run `go build ./...`, `make build`, and `go test ./...`; manually smoke-test the app on the Linux target (view switching, deck taps, pagination, full-screen, context menu, full editor, native file dialog). Ask the user if questions arise.

## Notes

- Tasks marked with `*` are optional test sub-tasks and can be skipped for a faster MVP; core implementation tasks are never optional.
- Tasks marked **[Manual verification]** cover the Gio rendering, gesture, full-screen, and native-dialog behavior that cannot be meaningfully unit-tested; verify them manually on the Linux target per the design's GUI / Manual Verification notes.
- Property-based tests use `gopter`, run `MinSuccessfulTests >= 100`, operate against a temporary config root, and carry a `// Feature: wails-to-gio-migration, Property N` comment tag.
- The nine correctness properties target `internal/core` and its pure state helpers only; command execution and the native file dialog are covered by integration tests (task 3.5).
- Each task references specific requirement sub-clauses for traceability; property test tasks additionally reference their design property number.
- Wails removal (18) and frontend cleanup (19) happen only after the Gio app is functional so a working build is preserved throughout the migration.

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1.1"] },
    { "id": 1, "tasks": ["1.2"] },
    { "id": 2, "tasks": ["2.1", "2.2", "3.1"] },
    { "id": 3, "tasks": ["2.3", "2.4", "3.2", "4.1"] },
    { "id": 4, "tasks": ["2.5", "2.6", "2.7", "3.3", "3.4", "3.5", "4.2"] },
    { "id": 5, "tasks": ["4.3", "4.4", "4.5", "4.6", "4.7", "4.8", "6.1", "6.2"] },
    { "id": 6, "tasks": ["6.3", "13.1", "13.2"] },
    { "id": 7, "tasks": ["6.4", "8.1", "10.1", "13.3", "14.1"] },
    { "id": 8, "tasks": ["8.2", "9.1", "9.2", "10.2", "14.2", "14.3"] },
    { "id": 9, "tasks": ["9.3", "11.1", "14.4", "15.1"] },
    { "id": 10, "tasks": ["11.2", "15.2", "16.1", "16.2", "16.3"] },
    { "id": 11, "tasks": ["11.3", "15.3", "16.4"] },
    { "id": 12, "tasks": ["18.1"] },
    { "id": 13, "tasks": ["18.2"] },
    { "id": 14, "tasks": ["19.1"] }
  ]
}
```
