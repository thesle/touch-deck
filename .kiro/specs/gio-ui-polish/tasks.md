# Implementation Plan: Gio UI Polish

## Overview

A batch of 10 UI polish enhancements to the already-migrated Gio TouchDeck app
(module `touchdeck`, Gio v0.10.2, Go 1.27, Linux Mint). This is a **lightweight
spec**: design decisions are baked directly into the task descriptions below —
there is no separate `requirements.md` or `design.md`. Tasks reference the
concrete files/functions to change (`internal/ui/deck.go`, `config.go`,
`render.go`, `colorpicker.go`, `internal/core/*`, `main.go`, `Makefile`).

Tasks are ordered so the low-risk, independent "CSS-like" tweaks come first, the
medium full-screen work next, and the largest/riskiest self-contained items
(graphical color picker, app icon, color emoji) come last. Enhancement numbers
(e.g. _Enhancement 1_) are used in place of requirement references.

**Recurring convention — drain-before-layout:** wherever a task touches widgets
that produce click/drag events (buttons, menu items, pointer targets), drain
their `Clicked()` / event queue at the TOP of the enclosing layout function
(matching the existing pattern in `layoutEditorForm`, `render.go layout()`),
then only paint during layout.

**GUI verification:** most of these are visual and cannot be unit-tested.
Checkpoint tasks call for the user to build and eyeball the running app
(`go build ./... && ./touchdeck` or the Makefile target). Only pure logic
(HSV↔RGB conversion) gets automated tests.

## Tasks

- [ ] 1. Deck padding and tile gap (Enhancement 1)
  - In `internal/ui/deck.go`, increase `tileGapDp` from `8` to `14`
    (round(8 × 1.7) = 14). This widens the gap between adjacent tiles; it is
    already consumed by the row/cell `Spacer`s and the `gap` calc in
    `layoutDeck`, so no other change is needed for the inter-tile gap.
  - Add an outer inset (~`16dp`) around the whole deck grid area so tiles are
    not flush to the window edge. Wrap the grid body inside `layoutDeck` in a
    `layout.UniformInset(unit.Dp(16)).Layout(gtx, ...)` (or `layout.Inset`),
    keeping the captured grid geometry / Context_Menu positioning correct
    (the geometry capture must reflect the inset origin so tap-to-slot mapping
    stays accurate).
  - Keep this consistent with the config preview, which already has padding.
  - _Enhancement 1_

- [ ] 2. Tile Color + Text Color side by side (Enhancement 2)
  - In `internal/ui/config.go`, combine the two stacked editor rows
    `fieldTileColor` and `fieldTextColor` into ONE horizontal row with two
    columns to save vertical space.
  - Replace the two separate entries in the editor form row list
    (`{r.fieldTileColor}`, `{spacerRow(12)}`, `{r.fieldTextColor}`) with a
    single combined field (e.g. `fieldColorsRow`) that lays out
    `fieldTileColor` and `fieldTextColor` side by side using
    `layout.Flex{Axis: layout.Horizontal}` with two `Flexed(1, ...)` columns
    and a small horizontal spacer between them.
  - Preserve the existing per-field vertical structure (caption label above the
    `ColorPicker.Layout`) inside each column, and keep rendering through
    `cfgTheme`.
  - _Enhancement 2_

- [ ] 3. Next-tile page count separated at bottom (Enhancement 6)
  - In `internal/ui/deck.go` `drawNavTile`, give the "current/total" page-count
    indicator its own reserved band at the very bottom of the Next tile, clearly
    separated (spacing) from the label + arrow group above it.
  - To preserve the label alignment previously achieved with a bottom overlay:
    reserve an equal small bottom band on BOTH the Prev and Next tiles so the
    label/arrow group stays vertically aligned between them; the Next tile fills
    its bottom band with the count, the Prev tile leaves its band empty.
  - Implement via a vertical `layout.Flex` inside the nav tile:
    `Flexed(1, label+arrow group)` then a `Rigid` bottom band of fixed height
    that holds the count (centered) with clear top spacing. Replace the current
    Stack overlay approach for the indicator while keeping the tile's rounded
    background.
  - _Enhancement 6_

- [ ] 4. Move Copy/Paste inline with move arrows (Enhancement 8)
  - In `internal/ui/config.go`, merge the separate `fieldMoveControls`
    (← ↑ ↓ → arrows) and `fieldCopyPaste` (Copy Settings / Paste Settings) rows
    into a single horizontal row: `[← ↑ ↓ →]  [Copy Settings] [Paste Settings]`.
  - In the editor form row list, replace the `{r.fieldMoveControls}`,
    `{spacerRow(12)}`, `{r.fieldCopyPaste}` entries with one combined field
    (e.g. `fieldMoveCopyPaste`) laying the arrows group and the copy/paste group
    on one `layout.Flex{Axis: layout.Horizontal}` with a spacer between the two
    groups.
  - Preserve drain-before-layout: the arrows' and copy/paste buttons'
    `Clicked()` events are already drained at the TOP of `layoutEditorForm`;
    keep that drain and only paint here.
  - Preserve the paste-unavailable greying (Paste Settings disabled/greyed when
    `r.state.Clipboard == nil`, matching current `fieldCopyPaste` behavior).
  - Render through `cfgTheme`.
  - _Enhancement 8_

- [ ] 5. Selected-tile border + real colors in config preview (Enhancement 4a)
  - In `internal/ui/config.go` `layoutConfigPreviewTile`, change the selected
    slot so it keeps its REAL `bgColor` / `bgImage` fill (do NOT overwrite with
    `cfgSelectedBg`) and indicates selection ONLY via a thicker colored border.
  - Use a `2–3dp` border in the blue `cfgSelectedBorder` (#3b82f6). Non-selected
    tiles keep their existing thin/no border.
  - The selected tile already renders from the live `r.state.Editor`, so the
    live-edited tile/text color continues to show through — verify this still
    holds after removing the `cfgSelectedBg` override.
  - _Enhancement 4a_

- [ ] 6. Text shadow + semi-bold on tile labels (Enhancement 5)
  - [ ] 6.1 Add a shadowed/semi-bold text helper
    - In `internal/ui/deck.go` (or a small new helper in the `ui` package), add
      a helper `drawShadowedLabel(gtx, th, sizeSp, weight, textColor, text)`
      that draws the label text several times offset in a dark color behind the
      main text (e.g. 1px offsets in 4 or 8 directions, or a single drop shadow)
      to mimic the Svelte text-shadow, then draws the main text on top.
    - Set semi-bold weight via `LabelStyle.Font.Weight = font.SemiBold`. Verify
      the gofont collection exposes a semibold face; if it does not, fall back to
      `font.Bold`. Note the chosen fallback in a code comment.
    - _Enhancement 5_
  - [ ] 6.2 Use the helper for deck tile labels
    - Replace the plain `material.Label(th, unit.Sp(size), text)` label draws for
      deck tiles in `deck.go` with `drawShadowedLabel(...)`, using the tile's
      text color.
    - _Enhancement 5_
  - [ ] 6.3 Use the helper for config preview tile labels
    - Apply the same shadowed/semi-bold helper to the config LEFT preview tile
      labels in `layoutConfigPreviewTile` for visual consistency.
    - _Enhancement 5_

- [ ] 7. Checkpoint — visual verification of tweaks 1–6
  - Build (`go build ./...`) and run the app. Manually verify: deck outer
    padding + wider tile gap; Tile/Text color fields side by side; Next-tile page
    count separated at bottom with Prev/Next labels still aligned; Copy/Paste
    inline right of the move arrows with paste greying intact; selected preview
    tile shows real colors with a thick blue border; tile labels are semi-bold
    with a readable shadow.
  - Ensure all tests pass, ask the user if questions arise.

- [ ] 8. True full-screen + context-menu toggle (Enhancement 7)
  - [ ] 8.1 Hide header in full-screen
    - In `internal/ui/render.go` `layout()`, only lay out the header
      (`layoutHeader`, Deck/Config/Exit-Fullscreen buttons) when
      `!state.Fullscreen`. When `state.Fullscreen` is true, the active view
      (deck/config) uses the whole window.
    - Keep draining the header click widgets (`deckBtn`/`configBtn`/`fsBtn`)
      before layout regardless of full-screen, so state stays consistent even
      when the header is not painted.
    - Keep `handleKeys(gtx)` so F11 still calls `toggleFullscreen()`.
    - _Enhancement 7_
  - [ ] 8.2 Add Fullscreen toggle to the deck context menu
    - In `internal/ui/deck.go`, add a menu button `fsMenuBtn` (`widget.Clickable`)
      to `deckState`, and render a corresponding menu item in `layoutMenuPanel`
      (alongside menuEdit/menuCopy/menuPaste) available on ALL context menus
      including empty/pagination slots.
    - The item label is "Exit Fullscreen" when `state.Fullscreen` else
      "Fullscreen". On click it calls `toggleFullscreen()` then `closeMenu()`.
    - Drain `fsMenuBtn.Clicked()` in the same drain pass as the other menu
      buttons (drain-before-layout), before painting the menu panel.
    - _Enhancement 7_
  - [ ] 8.3 Verify context menu works in full-screen and note Config caveat
    - Confirm the context menu (drawn over the deck) still opens and the
      Fullscreen toggle is reachable while full-screen (this is the escape hatch
      since the header/Deck-Config switch is hidden).
    - Add a code comment noting full-screen primarily targets the deck; in Config
      view the header is hidden, so the context-menu toggle (or F11) is the way
      back. Keep the toggle robust.
    - _Enhancement 7_

- [ ] 9. Checkpoint — verify full-screen behavior
  - Run the app. Verify: F11 toggles full-screen and hides the header so the deck
    fills the screen; right-click/long-press shows a "Fullscreen"/"Exit
    Fullscreen" menu item (including on empty slots) that toggles correctly and
    closes the menu; labels/toggle state update between "Fullscreen" and "Exit
    Fullscreen".
  - Ensure all tests pass, ask the user if questions arise.

- [ ] 10. Graphical color picker (Enhancements 3 and 4b)
  - [ ] 10.1 Add HSV↔RGB conversion helpers
    - In `internal/ui/colorpicker.go` (or a small `internal/core` helper if you
      prefer framework-agnostic pure logic), add `HSVtoRGB(h, s, v)` and
      `RGBtoHSV(r, g, b)` conversion functions. Keep the existing
      `ParseHexColor` / `NormalizeHex` / `Hex` / `SetHex` API intact.
    - _Enhancement 3_
  - [ ]* 10.2 Unit tests for HSV↔RGB conversion
    - Test round-trip consistency (RGB→HSV→RGB within tolerance), known anchors
      (pure red/green/blue, black, white, gray), and hue wrap-around at 0/360.
    - _Enhancement 3_
  - [ ] 10.3 Saturation/Value square widget
    - In `colorpicker.go`, add an SV square that draws a 2D gradient (x =
      saturation 0→1, y = value 1→0) tinted by the current hue, with a draggable
      selector dot at the current (s, v).
    - Handle pointer drag inside the square (register a `pointer.InputOp` / use a
      `gesture` drag or `widget` pointer handling), clamping to the square
      bounds. Drain drag events at the top of the picker layout
      (drain-before-layout) and convert the drag position to (s, v).
    - _Enhancement 3_
  - [ ] 10.4 Hue strip widget
    - Add a vertical (or horizontal) hue strip drawn as a rainbow gradient with a
      draggable selector, mapping the selector position to hue 0→360. Reuse the
      same pointer-drag/drain pattern as the SV square.
    - _Enhancement 3_
  - [ ] 10.5 Wire drag input to color + hex, and hex to picker
    - Dragging in the SV square or hue strip updates the live color, recomputes
      hex, and updates the hex text field. Typing a valid hex in the field
      updates the picker's hue/SV selectors (parse via `ParseHexColor`, then
      `RGBtoHSV`). Keep the current color as the single source of truth to avoid
      feedback loops.
    - _Enhancement 3_
  - [ ] 10.6 Integrate into ColorPicker.Layout with a live swatch (Enhancement 4b)
    - Update `ColorPicker.Layout(gtx, th, label)` to render, together: the SV
      square, the hue strip, the existing hex text field, and a swatch that shows
      the LIVE currently-picked color next to the picker.
    - Because `fieldTileColor` / `fieldTextColor` feed `r.state.Editor` and the
      selected config preview tile renders from live `Editor`, confirm dragging
      the picker updates BOTH the beside-picker swatch AND the selected preview
      tile's live tile/text color (border-only selection from Task 5 must remain).
    - Keep the combined side-by-side layout from Task 2 working with the richer
      picker (it may need more vertical room per column — adjust the form row
      height/scroll as needed).
    - _Enhancements 3, 4b_

- [ ] 11. Checkpoint — verify color picker + live preview
  - Run the app. Verify: SV square + hue strip drag update color live; hex field
    and picker stay in sync both directions; a swatch beside each picker shows the
    live color; the selected preview tile shows the live-edited color with only a
    thick blue border indicating selection.
  - Ensure all tests pass, ask the user if questions arise.

- [ ] 12. App icon + .desktop + install targets (Enhancement 9)
  - [ ] 12.1 Create the app icon asset
    - Add a simple TouchDeck PNG icon at ~256×256 under a new dir (e.g.
      `build/appicon/touchdeck.png` or `assets/touchdeck.png`). Generate it
      programmatically (a small Go generator using `image`/`image/png`, or the
      standard library) or commit it as an asset. Keep it simple (solid rounded
      background + a glyph/monogram).
    - _Enhancement 9_
  - [ ] 12.2 Create the .desktop file
    - Add `touchdeck.desktop` (e.g. under `build/` or repo root) with:
      `[Desktop Entry]`, `Type=Application`, `Name=TouchDeck`,
      `Exec=` pointing at the installed binary, `Icon=touchdeck`,
      `Categories=Utility;`, `Terminal=false`.
    - _Enhancement 9_
  - [ ] 12.3 Add Makefile install / uninstall targets
    - Add an `install` target that installs: the binary (to `~/.local/bin` or
      `/usr/local/bin`), the icon (to
      `~/.local/share/icons/hicolor/256x256/apps/touchdeck.png`), and the
      `.desktop` file (to `~/.local/share/applications/touchdeck.desktop`,
      with `Exec` matching the installed binary path). Add a matching
      `uninstall` target that removes all three.
    - Add a code/Makefile comment documenting WHY: Gio v0.10.2 has no window-icon
      API, so the panel/dock icon comes from the installed `.desktop` + hicolor
      PNG (the reliable Linux mechanism). If, on verification, the vendored Gio
      version does expose a window-icon option, also set it in `main.go` — but do
      not assume it exists; the `.desktop` path is authoritative.
    - _Enhancement 9_

- [ ] 13. Checkpoint — verify install and panel icon
  - Run `make install`, then confirm TouchDeck appears in the application menu
    with the icon, launches via the `.desktop` entry, and shows the icon in the
    panel/dock. Run `make uninstall` and confirm cleanup. (Manual verification.)
  - Ask the user if questions arise.

- [ ] 14. Color emoji rendering (Enhancement 10 — highest risk, self-contained)
  - [ ] 14.1 Include an emoji font in the theme's shaper
    - Build the `material.Theme` with a `text.Shaper` / font collection that
      includes Noto Color Emoji as a fallback face, so glyphs missing from gofont
      (monochrome) fall back to the emoji font. Prefer embedding a bundled copy
      via `go:embed` for portability (accept the ~10MB+ binary size increase, per
      user); alternatively load from `/usr/share/fonts/truetype/noto/NotoColorEmoji.ttf`.
    - Wire this where the theme is created (the `Renderer.th` /
      `material.NewTheme()` path). Register the emoji face as a fallback in the
      shaper's font collection.
    - _Enhancement 10_
  - [ ] 14.2 Investigate + implement the color-glyph render path
    - Determine whether `material.Label` / `widget.Label` render the color-bitmap
      glyphs automatically in Gio v0.10.2, or whether they show monochrome/tofu.
      Gio's shaper has a `Bitmaps()` path (`font.GlyphBitmap` → image) and go-text
      supports COLR/sbix/CBDT.
    - If the standard label path does NOT render color glyphs, implement the
      minimal custom text draw that invokes the bitmap path for emoji glyphs (draw
      the `font.GlyphBitmap` images at the glyph positions), used for tile labels.
    - If full-color proves infeasible in v0.10.2's label path, fall back to
      documented monochrome symbol rendering. Add a clear code comment recording
      what was found and which path was taken.
    - _Enhancement 10_
  - [ ] 14.3 Apply emoji rendering to tile labels
    - Ensure deck tile labels (and config preview labels, via the shadowed-label
      helper from Task 6 if compatible) display unicode emoji such as 📱 in color
      (or documented monochrome fallback). Verify no regression to normal text
      rendering.
    - _Enhancement 10_

- [ ] 15. Final checkpoint — full visual pass
  - Run the app and re-verify all 10 enhancements together: padding/gap,
    side-by-side colors, graphical color picker with live swatch, selected-tile
    border + real colors, semi-bold shadowed labels, separated page count,
    full-screen + menu toggle, inline copy/paste, installed icon/.desktop, and
    color (or documented monochrome) emoji on tiles.
  - Ensure all tests pass, ask the user if questions arise.

## Notes

- Tasks marked with `*` (10.2) are optional and can be skipped for a faster MVP.
- This is a lightweight spec: design decisions are inline in the tasks; there is
  no `requirements.md` / `design.md`. References use enhancement numbers.
- Most enhancements are visual and validated via the manual checkpoints (Tasks 7,
  9, 11, 13, 15) in the running app; only HSV↔RGB conversion has automated tests.
- Ordering: low-risk independent tweaks (1–6) first, medium full-screen work (8)
  next, then the largest/riskiest self-contained items — color picker (10), app
  icon (12), and color emoji (14) — last.
- Remember drain-before-layout wherever widgets/clicks/drags are involved.

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1", "2", "3", "4", "5", "6.1"] },
    { "id": 1, "tasks": ["6.2", "6.3", "8.1", "8.2", "10.1"] },
    { "id": 2, "tasks": ["8.3", "10.2", "10.3", "10.4", "12.1"] },
    { "id": 3, "tasks": ["10.5", "12.2", "14.1"] },
    { "id": 4, "tasks": ["10.6", "12.3", "14.2"] },
    { "id": 5, "tasks": ["14.3"] }
  ]
}
```
