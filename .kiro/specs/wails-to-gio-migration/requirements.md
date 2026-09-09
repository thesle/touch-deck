# Requirements Document

## Introduction

TouchDeck is a Linux desktop appliance that presents a configurable grid of touch buttons, each of which runs a shell command, intended for use on a 7" touch screen. The application is currently built on the Wails framework, which renders its user interface as HTML/CSS inside an embedded WebView (WebKitGTK) driven by a Svelte + Tailwind frontend.

This feature migrates TouchDeck from Wails to Gio (gioui.org), a native, pure-Go, GPU-rendered immediate-mode UI toolkit. The migration eliminates the HTML/WebView rendering engine and renders the interface directly, which reduces overhead and improves responsiveness on the constrained touch-screen appliance.

All existing behavior — configuration persistence and legacy migration, command execution, image handling, deck rendering, pagination, and the full split-pane configuration editor — MUST be preserved with full parity. In addition, two new capabilities are introduced: a runtime full-screen toggle and a long-press / right-click per-button context menu with copy and paste. The configuration file format is frozen and existing configuration files MUST continue to work unchanged. Linux (Fedora, Ubuntu, Linux Mint) is the acceptance target; cross-platform support is desired later but is not part of the acceptance criteria for this feature.

## Glossary

- **TouchDeck**: The desktop application being migrated.
- **Gio_Renderer**: The Gio-based UI rendering subsystem that replaces the Wails WebView frontend and draws all views natively.
- **Backend**: The framework-agnostic Go logic (configuration load/save, command execution, image handling) currently in `app.go`, reused after removing Wails-specific runtime calls.
- **Config_Store**: The subsystem responsible for reading and writing the configuration file at `~/.config/touchdeck/config.json`.
- **Config**: The configuration model consisting of `rows` (int), `cols` (int), and `pages` (array of Page).
- **Page**: A configuration object containing `pageIndex` (int) and a `buttons` array.
- **Button**: A configured deck entry with fields `id`, `label`, `command`, `bgImage` (path), `bgColor` (hex), `fontColor` (hex), `fontSize` (int offset in the range -2 to +2), and `order` (slot index).
- **Legacy_Config**: A configuration file in the older flat format, containing a top-level `buttons` array where each entry carries a `page` field instead of the nested `pages` structure.
- **Deck_View**: The primary runtime view that renders the grid of buttons and executes commands.
- **Config_View**: The split-pane configuration editor view containing the live grid preview (left) and the slot editor form (right).
- **Slot**: A position in the grid, addressed by an index from 0 to `rows * cols - 1`.
- **Pagination_Slots**: The last two slots of the grid, at index `rows*cols-2` (Prev Page) and `rows*cols-1` (Next Page), reserved for page navigation and not editable.
- **Command_Runner**: The subsystem that executes shell commands via `bash -c`, offering asynchronous (fire-and-forget) and synchronous (10-second timeout) execution.
- **Image_Manager**: The subsystem that selects, copies, and lists background images under `~/.config/touchdeck/images/`.
- **File_Dialog**: A native operating-system file selection dialog provided by a Go-native dialog library.
- **Context_Menu**: The per-button menu opened by long-press or right-click on the Deck_View, offering edit, copy, and paste actions.
- **Button_Clipboard**: An in-application, in-memory holding area for a copied button's settings, used by paste actions.

## Requirements

### Requirement 1: Framework Migration and Native Rendering

**User Story:** As a TouchDeck maintainer, I want the application rendered natively with Gio instead of a WebView, so that the appliance runs without an embedded HTML engine and responds faster on the touch screen.

#### Acceptance Criteria

1. THE Gio_Renderer SHALL render all TouchDeck views using Gio (gioui.org) native drawing without any embedded WebView or HTML/CSS rendering engine.
2. THE TouchDeck SHALL remove all dependencies on the Wails framework from the Go module.
3. THE TouchDeck SHALL remove the Svelte and Tailwind frontend and its Wails-generated bindings from the build.
4. THE Backend SHALL retain the configuration, command-execution, and image-handling logic, with Wails runtime calls replaced by framework-agnostic equivalents.
5. THE TouchDeck SHALL provide a Deck_View and a Config_View that the user can switch between at runtime.
6. WHEN the application starts, THE Gio_Renderer SHALL display the Deck_View.

### Requirement 2: Build System Migration

**User Story:** As a TouchDeck maintainer, I want the build tooling to produce a Gio binary instead of a Wails binary, so that I can build and run the app without Wails installed.

#### Acceptance Criteria

1. THE Makefile SHALL build TouchDeck using the Gio/Go toolchain without invoking the `wails` command.
2. THE Makefile SHALL provide a target that produces a runnable TouchDeck binary.
3. THE Makefile SHALL provide a clean target that removes build output.
4. THE TouchDeck SHALL remove the `wails.json` build configuration and the WebView asset-embedding directives from the application entry point.

### Requirement 3: Configuration File Compatibility

**User Story:** As a TouchDeck user, I want my existing configuration to keep working after the migration, so that I do not lose my buttons or have to reconfigure the deck.

#### Acceptance Criteria

1. THE Config_Store SHALL read and write the configuration file at the path `~/.config/touchdeck/config.json`.
2. THE Config_Store SHALL preserve the existing JSON configuration file format, including the `rows`, `cols`, and nested `pages` structure, without schema changes.
3. WHEN the configuration file does not exist, THE Config_Store SHALL create a default Config with 2 rows, 4 columns, and one page of sample buttons, and save it to the configuration path.
4. WHEN the configuration file is a Legacy_Config containing a top-level `buttons` array with `page` fields, THE Config_Store SHALL migrate it into the nested `pages` structure, grouping buttons by their `page` value.
5. WHEN the configuration file already contains a `pages` array, THE Config_Store SHALL load the pages without migration.
6. WHEN the user saves the configuration, THE Config_Store SHALL write the Config to the configuration path as indented JSON.
7. IF reading the configuration file fails, THEN THE Config_Store SHALL return an error to the caller.

### Requirement 4: Deck Rendering

**User Story:** As a TouchDeck user, I want the deck to show my configured buttons in a grid, so that I can see and use my shortcuts on the touch screen.

#### Acceptance Criteria

1. THE Deck_View SHALL render a grid of `rows` × `cols` Slots using the current Config.
2. WHERE a Slot on the current page has a configured Button with a label, background image, or command, THE Deck_View SHALL render that Button.
3. WHERE a Button has a `bgImage`, THE Deck_View SHALL render the image scaled to cover the Slot and centered.
4. WHERE a Button has a `bgColor`, THE Deck_View SHALL render the Slot background using that color.
5. THE Deck_View SHALL render each Button's label using the Button's `fontColor`.
6. THE Deck_View SHALL apply the Button's `fontSize` offset, selected from the set {-2, -1, 0, +1, +2}, relative to the default label size.
7. WHERE a Slot on the current page has no configured Button, THE Deck_View SHALL render the Slot as a dashed empty placeholder.
8. THE Deck_View SHALL map each rendered Button to the Slot whose index equals the Button's `order` value on the current page.

### Requirement 5: Command Execution

**User Story:** As a TouchDeck user, I want tapping a button to run its command with visible feedback, so that I know my action registered.

#### Acceptance Criteria

1. WHEN the user taps a configured Button on the Deck_View, THE Command_Runner SHALL execute the Button's `command` asynchronously via `bash -c` without blocking the interface.
2. WHEN the user taps a configured Button on the Deck_View, THE Deck_View SHALL display a tactile highlight on that Button for approximately 150 milliseconds.
3. IF a Button has an empty `command`, THEN THE Deck_View SHALL take no execution action when the Button is tapped.
4. WHEN the user activates the test-run action in the Config_View, THE Command_Runner SHALL execute the entered command synchronously via `bash -c` and return the combined standard output and standard error.
5. IF a synchronously executed command does not complete within 10 seconds, THEN THE Command_Runner SHALL cancel the command and return a timeout error.

### Requirement 6: Pagination

**User Story:** As a TouchDeck user, I want to page through multiple screens of buttons, so that I can store more shortcuts than fit on one grid.

#### Acceptance Criteria

1. THE Deck_View SHALL support 5 pages of buttons.
2. THE Deck_View SHALL render the Slot at index `rows*cols-2` as a Prev Page navigation control and the Slot at index `rows*cols-1` as a Next Page navigation control.
3. WHEN the user taps the Next Page control, THE Deck_View SHALL advance to the next page, wrapping from the last page to the first page.
4. WHEN the user taps the Prev Page control, THE Deck_View SHALL move to the previous page, wrapping from the first page to the last page.
5. THE Deck_View SHALL display a page indicator on the Next Page control showing the current page number and the total page count in the form `current / total`.

### Requirement 7: Full-Screen Toggle

**User Story:** As a TouchDeck user, I want to toggle full-screen at runtime, so that I can configure the app in a normal window on my monitor and then run it borderless on the 7" touch screen.

#### Acceptance Criteria

1. WHEN the application starts, THE Gio_Renderer SHALL display TouchDeck in windowed mode with a title bar.
2. WHEN the user presses the F11 key, THE Gio_Renderer SHALL toggle between windowed mode and full-screen mode.
3. WHERE TouchDeck is in full-screen mode, THE Gio_Renderer SHALL hide the window title bar.
4. WHEN the user activates the on-screen full-screen control, THE Gio_Renderer SHALL toggle between windowed mode and full-screen mode.
5. WHILE TouchDeck is in full-screen mode, THE Gio_Renderer SHALL provide a control that returns TouchDeck to windowed mode.

### Requirement 8: Long-Press and Right-Click Context Menu

**User Story:** As a TouchDeck user, I want a long-press or right-click on a deck button to open a menu, so that I can edit, copy, and paste buttons directly from the deck using either touch or a mouse.

#### Acceptance Criteria

1. WHEN the user presses and holds a Deck_View Button beyond the long-press threshold, THE Deck_View SHALL open the Context_Menu for that Button.
2. WHEN the user right-clicks a Deck_View Button with a mouse, THE Deck_View SHALL open the same Context_Menu for that Button.
3. THE Context_Menu SHALL present edit, copy, and paste actions for the target Button.
4. WHEN the user selects the edit action from the Context_Menu, THE Gio_Renderer SHALL open the Config_View with the target Button's Slot selected for editing.
5. WHEN the user selects the copy action from the Context_Menu, THE Deck_View SHALL store the target Button's settings in the Button_Clipboard.
6. WHEN the user selects the paste action from the Context_Menu, THE Deck_View SHALL apply the Button_Clipboard settings to the target Button's Slot and save the Config.
7. IF the Button_Clipboard is empty, THEN THE Context_Menu SHALL present the paste action as unavailable.
8. WHERE the long-press or right-click targets a Pagination_Slot, THE Deck_View SHALL suppress the Context_Menu.
9. WHEN the user dismisses the Context_Menu without selecting an action, THE Deck_View SHALL close the Context_Menu and take no further action.

### Requirement 9: Configuration Editor — Grid, Pages, and Slot Selection

**User Story:** As a TouchDeck user, I want to adjust the grid, choose a page, and select a slot to edit, so that I can arrange my deck layout.

#### Acceptance Criteria

1. THE Config_View SHALL present a live grid preview showing the current page's Slots in a `rows` × `cols` layout.
2. THE Config_View SHALL provide increase and decrease adjusters for the row count and the column count.
3. THE Config_View SHALL enforce a minimum of 1 for both the row count and the column count.
4. WHEN the user reduces the grid size, THE Config_View SHALL prune Buttons whose `order` is greater than or equal to `rows * cols` from all pages and save the Config.
5. THE Config_View SHALL provide a page selector for the 5 pages.
6. WHEN the user selects a page in the Config_View, THE Config_View SHALL display that page's Slots for editing.
7. WHEN the user selects an editable Slot in the live grid preview, THE Config_View SHALL load that Slot's Button values into the editor form, or initialize an empty Button with a generated `id` when the Slot is unconfigured.
8. WHERE a Slot is a Pagination_Slot, THE Config_View SHALL render it as a non-editable pagination preview and SHALL NOT allow it to be selected for editing.

### Requirement 10: Configuration Editor — Slot Editor Fields

**User Story:** As a TouchDeck user, I want to edit all properties of a button, so that I can fully customize each shortcut.

#### Acceptance Criteria

1. THE Config_View SHALL provide a single-line text input for the Button `label`.
2. THE Config_View SHALL provide a multi-line text input for the Button `command`.
3. THE Config_View SHALL provide a color picker for the Button `bgColor` (tile color).
4. THE Config_View SHALL provide a color picker for the Button `fontColor` (text color).
5. THE Config_View SHALL provide a font-size offset selector offering the choices -2, -1, default (0), +1, and +2.
6. WHEN the user saves the edited Slot, THE Config_Store SHALL persist the Button's `label`, `command`, `bgImage`, `bgColor`, `fontColor`, `fontSize`, and `order` to the current page and write the Config.
7. WHEN the user clears the selected Slot, THE Config_View SHALL remove the Button at that Slot from the current page and save the Config.

### Requirement 11: Configuration Editor — Move, Copy, and Paste

**User Story:** As a TouchDeck user, I want to rearrange buttons and reuse button settings, so that I can organize my deck efficiently.

#### Acceptance Criteria

1. THE Config_View SHALL provide move controls for the selected Slot in the left, up, down, and right directions.
2. WHEN the user activates a move control, THE Config_View SHALL swap the selected Slot's Button with the Button in the adjacent Slot in that direction and save the Config.
3. IF a move would target a Pagination_Slot or a Slot outside the grid bounds, THEN THE Config_View SHALL leave the Config unchanged.
4. WHEN a move completes, THE Config_View SHALL keep the moved Button's new Slot selected.
5. WHEN the user activates the copy-settings action, THE Config_View SHALL store the editor form's current Button settings in the Button_Clipboard.
6. WHEN the user activates the paste-settings action, THE Config_View SHALL apply the Button_Clipboard settings to the selected Slot and save the Config.
7. IF the Button_Clipboard is empty, THEN THE Config_View SHALL present the paste-settings action as unavailable.

### Requirement 12: Configuration Editor — Test-Run Panel

**User Story:** As a TouchDeck user, I want to test-run a button's command in the editor, so that I can verify it works before saving.

#### Acceptance Criteria

1. THE Config_View SHALL provide a test-run panel that executes the currently entered command synchronously.
2. WHEN the user activates the test-run action, THE Config_View SHALL display the command's combined output on success.
3. IF the command entered for test-run is empty, THEN THE Config_View SHALL display a message indicating there is no command to test.
4. IF the test-run command returns an error, THEN THE Config_View SHALL display the error output and indicate failure.
5. WHILE a test-run command is executing, THE Config_View SHALL indicate that execution is in progress.

### Requirement 13: Native File Dialog Image Selection

**User Story:** As a TouchDeck user configuring on my monitor with a mouse, I want to pick a background image with a native file dialog and reuse previously chosen images, so that I can style buttons easily.

#### Acceptance Criteria

1. WHEN the user activates the choose-file action for a Button background, THE Image_Manager SHALL open a native File_Dialog filtered to image file types.
2. WHEN the user selects an image file in the File_Dialog, THE Image_Manager SHALL copy the selected file into `~/.config/touchdeck/images/` with a timestamp-prefixed filename and set the Button's `bgImage` to the copied file path.
3. IF the user cancels the File_Dialog, THEN THE Image_Manager SHALL leave the Button's `bgImage` unchanged.
4. THE Config_View SHALL provide a dropdown listing the existing images stored in `~/.config/touchdeck/images/` for selection as the Button background.
5. WHEN the user selects an existing image from the dropdown, THE Config_View SHALL set the Button's `bgImage` to that image's path.
6. WHEN the user activates the clear-image action, THE Config_View SHALL set the Button's `bgImage` to empty.
7. THE Image_Manager SHALL list only files with image extensions (`.png`, `.jpg`, `.jpeg`, `.gif`, `.webp`, `.svg`) from the images directory.

### Requirement 14: Linux-First Platform Support

**User Story:** As a TouchDeck maintainer, I want the migrated app to build and run on mainstream Linux distributions, so that the appliance target is fully supported.

#### Acceptance Criteria

1. THE TouchDeck SHALL build and run on Fedora, Ubuntu, and Linux Mint.
2. THE Command_Runner SHALL execute commands through `bash` on the Linux target.
3. THE TouchDeck SHALL resolve the configuration and images directories from the operating system user configuration directory, producing `~/.config/touchdeck/` on the Linux target.
4. THE TouchDeck SHALL use only cross-platform Go and Gio constructs where practical so that later Windows and macOS support is not precluded, while treating Linux as the acceptance target.
