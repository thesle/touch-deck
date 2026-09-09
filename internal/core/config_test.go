package core

// Config_Store tests (tasks 2.5, 2.6, 2.7). These are white-box tests in
// package core so they can construct &Backend{configRoot: t.TempDir()} and
// redirect every path resolution away from the real ~/.config. No test here
// ever touches the user's real config directory.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/prop"
)

// newTestBackend returns a Backend whose configRoot is a fresh temp directory,
// so ConfigPath/ImagesDir resolve under t.TempDir() instead of ~/.config.
func newTestBackend(t *testing.T) *Backend {
	t.Helper()
	return &Backend{configRoot: t.TempDir()}
}

// ---------------------------------------------------------------------------
// Task 2.5 — Property 1: Configuration round-trip
// ---------------------------------------------------------------------------

// Feature: wails-to-gio-migration, Property 1: Configuration round-trip
//
// For any valid Config, SaveConfig then LoadConfig yields an equal Config:
// equal Rows, equal Cols, and — up to page ordering and within-page button
// ordering — equal pages/buttons.
//
// Normalization choices (documented):
//   - Pages are compared after sorting by PageIndex. The generated config uses
//     unique PageIndex values, and LoadConfig on a modern (non-empty pages)
//     config loads pages as-is, so this is order-insensitive but robust.
//   - Each page's Buttons are compared after sorting by Order. This makes the
//     comparison insensitive to slice order.
//   - FontSize IS compared here: the modern load path preserves it (only the
//     legacy migration drops it, which Property 2 covers).
//   - A single temp config root is reused across iterations; each SaveConfig
//     overwrites the same config.json, which is sufficient because LoadConfig
//     reads the whole file back.
func TestProperty1_ConfigRoundTrip(t *testing.T) {
	params := gopter.DefaultTestParameters()
	params.MinSuccessfulTests = 200

	b := &Backend{configRoot: t.TempDir()}

	properties := gopter.NewProperties(params)
	properties.Property("save then load yields an equal Config (up to ordering)", prop.ForAll(
		func(cfg Config) bool {
			if err := b.SaveConfig(cfg); err != nil {
				t.Logf("SaveConfig error: %v", err)
				return false
			}
			loaded, err := b.LoadConfig()
			if err != nil {
				t.Logf("LoadConfig error: %v", err)
				return false
			}
			return configsEqualUpToOrder(cfg, loaded)
		},
		genValidConfig(),
	))

	properties.TestingRun(t)
}

// configsEqualUpToOrder compares two configs for Rows/Cols equality and
// page/button equality up to page ordering and within-page button ordering.
func configsEqualUpToOrder(a, b Config) bool {
	if a.Rows != b.Rows || a.Cols != b.Cols {
		return false
	}
	pa := sortPagesByIndex(a.Pages)
	pb := sortPagesByIndex(b.Pages)
	if len(pa) != len(pb) {
		return false
	}
	for i := range pa {
		if pa[i].PageIndex != pb[i].PageIndex {
			return false
		}
		ba := sortButtonsByOrder(pa[i].Buttons)
		bb := sortButtonsByOrder(pb[i].Buttons)
		if len(ba) != len(bb) {
			return false
		}
		for j := range ba {
			if ba[j] != bb[j] {
				return false
			}
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// Task 2.6 — Property 2: Legacy migration preserves membership
// ---------------------------------------------------------------------------

// Feature: wails-to-gio-migration, Property 2: Legacy migration preserves membership
//
// For any Legacy_Config (flat buttons[] each carrying a page value, no pages[]
// key), LoadConfig migrates it so that:
//   - every button lands on the page whose PageIndex equals that button's page,
//   - the total button count across all resulting pages equals the input count
//     (none dropped or duplicated),
//   - for each page, the multiset of buttons equals the input buttons with that
//     page value.
//
// Comparison choices (documented):
//   - Buttons are compared IGNORING FontSize, because the migration
//     intentionally does not copy FontSize (a preserved app.go quirk). All
//     other fields (ID/Label/Command/BgImage/BgColor/FontColor/Order) are
//     compared.
//   - The comparison is order-insensitive at both levels (page order and
//     within-page button order) because the map-based migration is
//     nondeterministic. A multiset (map of key -> count) is used so duplicates
//     are handled correctly.
//   - The empty-buttons edge case is covered: LoadConfig then produces
//     cfg.Pages == []PageConfig{} (len 0), which matches an empty expected map.
func TestProperty2_LegacyMigrationMembership(t *testing.T) {
	params := gopter.DefaultTestParameters()
	params.MinSuccessfulTests = 200

	properties := gopter.NewProperties(params)
	properties.Property("legacy migration places every button on page==pageIndex, none lost/dup", prop.ForAll(
		func(legacy legacyConfig) bool {
			// Fresh temp root per iteration so a previous legacy file cannot
			// leak into this one (and so the missing-file default path is never
			// hit — we always write the file first).
			root := t.TempDir()
			b := &Backend{configRoot: root}

			// Write the legacy JSON directly to the ConfigPath.
			cfgPath, err := b.ConfigPath()
			if err != nil {
				t.Logf("ConfigPath error: %v", err)
				return false
			}
			data, err := json.MarshalIndent(legacy, "", "  ")
			if err != nil {
				t.Logf("marshal legacy error: %v", err)
				return false
			}
			if err := os.WriteFile(cfgPath, data, 0644); err != nil {
				t.Logf("write legacy file error: %v", err)
				return false
			}

			loaded, err := b.LoadConfig()
			if err != nil {
				t.Logf("LoadConfig error: %v", err)
				return false
			}

			// Build expected per-page multiset from the input.
			expected := map[int]map[buttonKeyNoFontSize]int{}
			expectedTotal := 0
			for _, lb := range legacy.Buttons {
				expectedTotal++
				if expected[lb.Page] == nil {
					expected[lb.Page] = map[buttonKeyNoFontSize]int{}
				}
				expected[lb.Page][keyNoFontSize(lb.ButtonConfig)]++
			}

			// Build actual per-page multiset from the loaded config, asserting
			// each button sits on the page whose PageIndex it was assigned to.
			actual := map[int]map[buttonKeyNoFontSize]int{}
			actualTotal := 0
			seenPageIndex := map[int]bool{}
			for _, pg := range loaded.Pages {
				if seenPageIndex[pg.PageIndex] {
					// The migration groups by page value into a map, so each
					// page index should appear exactly once.
					t.Logf("duplicate page index %d in migrated config", pg.PageIndex)
					return false
				}
				seenPageIndex[pg.PageIndex] = true
				if actual[pg.PageIndex] == nil {
					actual[pg.PageIndex] = map[buttonKeyNoFontSize]int{}
				}
				for _, btn := range pg.Buttons {
					actualTotal++
					actual[pg.PageIndex][keyNoFontSize(btn)]++
				}
			}

			// Total count preserved (none dropped or duplicated).
			if actualTotal != expectedTotal {
				t.Logf("count mismatch: expected %d, got %d", expectedTotal, actualTotal)
				return false
			}

			// Per-page multiset equality: every button landed on page==pageIndex.
			if !reflect.DeepEqual(expected, actual) {
				t.Logf("membership mismatch:\nexpected=%v\nactual=%v", expected, actual)
				return false
			}
			return true
		},
		genLegacyConfig(),
	))

	properties.TestingRun(t)
}

// ---------------------------------------------------------------------------
// Task 2.7 — Unit tests for Config_Store
// ---------------------------------------------------------------------------

// Default-config creation on a missing file: LoadConfig on an empty temp root
// returns the default (Rows=2, Cols=4, one page, 3 sample buttons) AND writes
// config.json to disk (Requirement 3.3).
func TestLoadConfig_DefaultCreatedOnMissingFile(t *testing.T) {
	root := t.TempDir()
	b := &Backend{configRoot: root}

	cfg, err := b.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}

	if cfg.Rows != 2 || cfg.Cols != 4 {
		t.Errorf("default grid = %dx%d, want 2x4", cfg.Rows, cfg.Cols)
	}
	if len(cfg.Pages) != 1 {
		t.Fatalf("default pages = %d, want 1", len(cfg.Pages))
	}
	if cfg.Pages[0].PageIndex != 0 {
		t.Errorf("default page index = %d, want 0", cfg.Pages[0].PageIndex)
	}
	if len(cfg.Pages[0].Buttons) != 3 {
		t.Fatalf("default buttons = %d, want 3", len(cfg.Pages[0].Buttons))
	}
	for i, btn := range cfg.Pages[0].Buttons {
		if btn.ID == "" {
			t.Errorf("default button %d has empty ID", i)
		}
		if btn.Command == "" {
			t.Errorf("default button %d has empty command", i)
		}
	}

	// The default config must have been saved to disk.
	cfgPath := filepath.Join(root, "touchdeck", "config.json")
	if _, err := os.Stat(cfgPath); err != nil {
		t.Errorf("expected default config written to %s, stat error: %v", cfgPath, err)
	}
}

// Path resolution under a redirected config root: ConfigPath resolves to
// <root>/touchdeck/config.json and ImagesDir to <root>/touchdeck/images, and
// both directories are created (Requirements 3.1, 14.3).
func TestPathResolution_UnderRedirectedRoot(t *testing.T) {
	root := t.TempDir()
	b := &Backend{configRoot: root}

	cfgPath, err := b.ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath error: %v", err)
	}
	wantCfg := filepath.Join(root, "touchdeck", "config.json")
	if cfgPath != wantCfg {
		t.Errorf("ConfigPath = %q, want %q", cfgPath, wantCfg)
	}
	// The touchdeck directory should have been created.
	if info, err := os.Stat(filepath.Join(root, "touchdeck")); err != nil || !info.IsDir() {
		t.Errorf("expected touchdeck dir created under %s: err=%v", root, err)
	}

	imgDir, err := b.ImagesDir()
	if err != nil {
		t.Fatalf("ImagesDir error: %v", err)
	}
	wantImg := filepath.Join(root, "touchdeck", "images")
	if imgDir != wantImg {
		t.Errorf("ImagesDir = %q, want %q", imgDir, wantImg)
	}
	if info, err := os.Stat(wantImg); err != nil || !info.IsDir() {
		t.Errorf("expected images dir created at %s: err=%v", wantImg, err)
	}
}

// Indented-JSON formatting: after SaveConfig, the raw file uses two-space
// indentation (from encoder.SetIndent("", "  ")) (Requirement 3.6).
func TestSaveConfig_WritesIndentedJSON(t *testing.T) {
	root := t.TempDir()
	b := &Backend{configRoot: root}

	cfg := Config{
		Rows: 2,
		Cols: 4,
		Pages: []PageConfig{
			{PageIndex: 0, Buttons: []ButtonConfig{{ID: "a", Label: "L", Order: 0}}},
		},
	}
	if err := b.SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig error: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(root, "touchdeck", "config.json"))
	if err != nil {
		t.Fatalf("read config file error: %v", err)
	}
	content := string(raw)

	// Two-space indentation: top-level fields are indented by exactly two spaces.
	if !strings.Contains(content, "\n  \"rows\": 2") {
		t.Errorf("expected two-space indented \"rows\" field, got:\n%s", content)
	}
	// A line beginning with exactly two spaces should exist.
	hasTwoSpaceLine := false
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "   ") {
			hasTwoSpaceLine = true
			break
		}
	}
	if !hasTwoSpaceLine {
		t.Errorf("expected at least one line beginning with exactly two spaces, got:\n%s", content)
	}

	// Sanity: it should be valid JSON that round-trips back to the same config.
	var back Config
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Errorf("saved file is not valid JSON: %v", err)
	}
}

// Malformed-config error path: invalid JSON at config.json causes LoadConfig to
// return a non-nil error (Requirement 3.7).
func TestLoadConfig_MalformedReturnsError(t *testing.T) {
	root := t.TempDir()
	b := &Backend{configRoot: root}

	cfgPath, err := b.ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath error: %v", err)
	}
	if err := os.WriteFile(cfgPath, []byte("{ not json"), 0644); err != nil {
		t.Fatalf("write malformed file error: %v", err)
	}

	if _, err := b.LoadConfig(); err == nil {
		t.Errorf("expected LoadConfig to return an error on malformed JSON, got nil")
	}
}
