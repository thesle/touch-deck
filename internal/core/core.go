// Package core is the framework-agnostic Backend for TouchDeck. It has no
// dependency on Gio or any UI library. It owns configuration persistence
// (Config_Store), command execution (Command_Runner), and image handling
// (Image_Manager, including the native File_Dialog call).
//
// This file is a skeleton created by task 1.2. The method bodies here are
// stubs that return zero values; later tasks fill them in:
//   - task 2.1 (done) moved the persisted data models with frozen JSON tags into config.go
//   - tasks 2.2-2.4 implement the Config_Store (ConfigPath, ImagesDir, LoadConfig, SaveConfig)
//   - task 3.1 implements the Command_Runner (RunCommandAsync, RunCommandSync)
//   - task 3.2 implements the Image_Manager (SelectImage, CopyImageToConfig, ListConfigImages)
//
// The persisted data models (ButtonConfig, PageConfig, Config) live in config.go.
package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/sqweek/dialog"
)

// Backend is the framework-agnostic TouchDeck backend. The Wails *App receiver
// becomes a *Backend receiver and the Wails startup/context are dropped.
type Backend struct {
	// configRoot is the base directory under which the "touchdeck" config
	// directory is created. It is the test seam described in the design: when
	// empty (the default from New), baseDir falls back to os.UserConfigDir so
	// the public Linux behavior stays ~/.config/touchdeck/. Package-internal
	// tests set this to a t.TempDir() to redirect all path resolution away from
	// the real ~/.config.
	configRoot string
}

// New constructs a Backend with default (empty) configRoot, so path resolution
// uses os.UserConfigDir and the public Linux behavior is unchanged.
func New() *Backend { return &Backend{} }

// baseDir resolves the base configuration directory through a single
// indirection: the configRoot seam if set, otherwise os.UserConfigDir. This is
// the only place ConfigPath/ImagesDir learn where "~/.config" lives, which lets
// tests redirect it to a temporary directory.
func (b *Backend) baseDir() (string, error) {
	if b.configRoot != "" {
		return b.configRoot, nil
	}
	return os.UserConfigDir()
}

// --- Config_Store (implemented by tasks 2.2-2.4) ---

// LoadConfig loads the application configuration. Ported verbatim from the
// Wails *App.LoadConfig, with the receiver changed to *Backend and path/save
// resolved through b.ConfigPath()/b.SaveConfig().
//
// Behavior (Requirements 3.3, 3.4, 3.5, 3.7):
//   - Missing file → build the default Config (2 rows, 4 cols, one page with 3
//     sample buttons), attempt to save it (ignoring the save error), return it (3.3).
//   - Modern config with a non-empty pages[] → load as-is (3.5).
//   - Legacy flat buttons[] with a page field → group into pages (3.4).
//   - Neither → cfg.Pages = []PageConfig{}.
//   - Open/decode failure → return the error (3.7).
//
// Note on behavior parity: the legacy migration groups buttons via a
// map[int][]ButtonConfig, so migrated page ordering is nondeterministic (the
// design preserves this; correctness is stated over page membership, not slice
// order). The migration also does NOT copy FontSize into the reconstructed
// button — this is preserved exactly as in app.go.
func (b *Backend) LoadConfig() (Config, error) {
	configPath, err := b.ConfigPath()
	if err != nil {
		return Config{}, err
	}

	// If config doesn't exist, return a default one
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		defaultConfig := Config{
			Rows: 2,
			Cols: 4,
			Pages: []PageConfig{
				{
					PageIndex: 0,
					Buttons: []ButtonConfig{
						{
							ID:      "btn_default_1",
							Label:   "Say Hello 👋",
							Command: "notify-send 'TouchDeck' 'Hello! Your TouchDeck is working beautifully.'",
							Order:   0,
						},
						{
							ID:      "btn_default_2",
							Label:   "Open GitHub 🌐",
							Command: "xdg-open 'https://github.com'",
							Order:   1,
						},
						{
							ID:      "btn_default_3",
							Label:   "System Date 🕒",
							Command: "notify-send 'Current Date' \"$(date)\"",
							Order:   2,
						},
					},
				},
			},
		}
		// Try to save the default config
		_ = b.SaveConfig(defaultConfig)
		return defaultConfig, nil
	}

	file, err := os.Open(configPath)
	if err != nil {
		return Config{}, err
	}
	defer file.Close()

	// Temporary structure to allow decoding older formats
	var raw struct {
		Rows    int `json:"rows"`
		Cols    int `json:"cols"`
		Buttons []struct {
			ButtonConfig
			Page int `json:"page"`
		} `json:"buttons"`
		Pages []PageConfig `json:"pages"`
	}

	decoder := json.NewDecoder(file)
	err = decoder.Decode(&raw)
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	cfg.Rows = raw.Rows
	cfg.Cols = raw.Cols

	if len(raw.Pages) > 0 {
		cfg.Pages = raw.Pages
	} else if len(raw.Buttons) > 0 {
		// Migrate old flat structure to nested Pages structure
		pagesMap := make(map[int][]ButtonConfig)
		for _, btnRaw := range raw.Buttons {
			btn := ButtonConfig{
				ID:        btnRaw.ID,
				Label:     btnRaw.Label,
				Command:   btnRaw.Command,
				BgImage:   btnRaw.BgImage,
				BgColor:   btnRaw.BgColor,
				FontColor: btnRaw.FontColor,
				Order:     btnRaw.Order,
			}
			pagesMap[btnRaw.Page] = append(pagesMap[btnRaw.Page], btn)
		}

		for pageIdx, btns := range pagesMap {
			cfg.Pages = append(cfg.Pages, PageConfig{
				PageIndex: pageIdx,
				Buttons:   btns,
			})
		}
	} else {
		cfg.Pages = []PageConfig{}
	}

	return cfg, nil
}

// SaveConfig persists the application configuration. It resolves the config
// path via ConfigPath, creates (truncates) the file, and writes the Config as
// indented JSON (two-space indent). Any error from resolving the path, creating
// the file, or encoding is returned to the caller (Requirement 3.6).
func (b *Backend) SaveConfig(cfg Config) error {
	configPath, err := b.ConfigPath()
	if err != nil {
		return err
	}

	file, err := os.Create(configPath)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(cfg)
}

// ConfigPath returns the path to the config file (was getConfigPath). It
// resolves to <baseDir>/touchdeck/config.json, creating the touchdeck directory
// with MkdirAll(0755) before returning.
func (b *Backend) ConfigPath() (string, error) {
	base, err := b.baseDir()
	if err != nil {
		return "", err
	}
	appConfigDir := filepath.Join(base, "touchdeck")
	if err := os.MkdirAll(appConfigDir, 0755); err != nil {
		return "", err
	}
	return filepath.Join(appConfigDir, "config.json"), nil
}

// ImagesDir returns the path to the custom images directory (was getImagesDir).
// It resolves to <baseDir>/touchdeck/images, creating the directory with
// MkdirAll(0755) before returning.
func (b *Backend) ImagesDir() (string, error) {
	base, err := b.baseDir()
	if err != nil {
		return "", err
	}
	imagesDir := filepath.Join(base, "touchdeck", "images")
	if err := os.MkdirAll(imagesDir, 0755); err != nil {
		return "", err
	}
	return imagesDir, nil
}

// --- Command_Runner (implemented by task 3.1) ---

// RunCommandAsync executes a command in the background (fire-and-forget). It
// runs the command through `bash -c`, starts it, and reaps it in a goroutine so
// the UI never blocks; only the start error is returned to the caller.
func (b *Backend) RunCommandAsync(command string) error {
	cmd := exec.Command("bash", "-c", command)
	err := cmd.Start()
	if err != nil {
		return err
	}
	go func() {
		_ = cmd.Wait()
	}()
	return nil
}

// RunCommandSync executes a command and waits for its combined output (useful
// for testing scripts in Config). It runs through `bash -c` under a 10 second
// context timeout; on deadline it returns a timeout error, otherwise it returns
// the combined stdout/stderr and any run error.
func (b *Backend) RunCommandSync(command string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("command timed out after 10 seconds")
		}
		return string(output), err
	}
	return string(output), nil
}

// --- Image_Manager (implemented by task 3.2) ---

// SelectImage opens a native file dialog to select a background image, filtered
// to the supported image extensions. It replaces the Wails
// runtime.OpenFileDialog with the framework-independent sqweek/dialog native
// chooser (Requirement 13.1). On Linux this uses GTK via cgo.
//
// dialog.File().Load() returns dialog.ErrCancelled when the user dismisses the
// dialog; that is mapped to a "no selection" result ("", nil) so callers leave
// bgImage unchanged (Requirement 13.3). Any other error is returned as-is.
func (b *Backend) SelectImage() (string, error) {
	path, err := dialog.File().
		Title("Select Background Image").
		Filter("Images", "png", "jpg", "jpeg", "gif", "webp", "svg").
		Load()
	if err != nil {
		if err == dialog.ErrCancelled {
			return "", nil
		}
		return "", err
	}
	return path, nil
}

// CopyImageToConfig copies a selected image file into the touchdeck config
// images directory, giving it a timestamp-prefixed name so repeated copies of
// the same source never collide, and returns the destination path (Requirement
// 13.2). An empty srcPath is a no-op returning ("", nil). Ported verbatim from
// the Wails *App.CopyImageToConfig with the receiver changed to *Backend.
func (b *Backend) CopyImageToConfig(srcPath string) (string, error) {
	if srcPath == "" {
		return "", nil
	}

	imagesDir, err := b.ImagesDir()
	if err != nil {
		return "", err
	}

	fileName := filepath.Base(srcPath)
	destFileName := fmt.Sprintf("%d_%s", time.Now().UnixNano(), fileName)
	destPath := filepath.Join(imagesDir, destFileName)

	srcFile, err := os.Open(srcPath)
	if err != nil {
		return "", err
	}
	defer srcFile.Close()

	destFile, err := os.Create(destPath)
	if err != nil {
		return "", err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, srcFile)
	if err != nil {
		return "", err
	}

	return destPath, nil
}

// ListConfigImages returns absolute paths of all images stored in the touchdeck
// config images directory, including only files whose lower-cased extension is
// in the frozen set (.png/.jpg/.jpeg/.gif/.webp/.svg) and skipping
// subdirectories (Requirement 13.7). Ported verbatim from the Wails
// *App.ListConfigImages with the receiver changed to *Backend.
func (b *Backend) ListConfigImages() ([]string, error) {
	imagesDir, err := b.ImagesDir()
	if err != nil {
		return nil, err
	}

	files, err := os.ReadDir(imagesDir)
	if err != nil {
		return nil, err
	}

	var imagePaths []string
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		// Match typical image extensions
		ext := strings.ToLower(filepath.Ext(file.Name()))
		if ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".gif" || ext == ".webp" || ext == ".svg" {
			imagePaths = append(imagePaths, filepath.Join(imagesDir, file.Name()))
		}
	}

	return imagePaths, nil
}
