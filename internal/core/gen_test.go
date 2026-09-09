package core

// Shared gopter generators and comparison/normalization helpers for the
// Config_Store property tests (Property 1 and Property 2). Kept in package
// core (white-box) so tests can construct &Backend{configRoot: t.TempDir()}
// and redirect all path resolution away from the real ~/.config.

import (
	"reflect"
	"sort"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
)

// --- Bounds shared by the generators ---
//
// Rows/Cols are bounded to 1..8 so the grid stays small and Order values have
// a meaningful in-range space (0 .. Rows*Cols-1). FontSize is generated in the
// documented -2..+2 offset range from Requirement 10.5.
const (
	minDim      = 1
	maxDim      = 8
	minFontSize = -2
	maxFontSize = 2
	maxPages    = 5 // page indices live in 0..4 (TotalPages == 5)
)

// genString produces arbitrary (possibly empty) strings for the free-text
// button fields (Label/Command/BgImage/BgColor/FontColor). gen.AnyString can
// emit any unicode; that is fine because SaveConfig/LoadConfig round-trip
// through JSON which preserves arbitrary UTF-8.
func genString() gopter.Gen {
	return gen.AnyString()
}

// genNonEmptyID produces a non-empty-ish ID. IDs identify buttons, so we avoid
// the empty string to keep generated buttons distinguishable, though the
// round-trip does not actually depend on it.
func genNonEmptyID() gopter.Gen {
	return gen.Identifier().SuchThat(func(s string) bool { return s != "" })
}

// genButtonWithOrder builds a ButtonConfig generator whose Order is drawn from
// 0 .. maxOrder (inclusive). maxOrder is Rows*Cols-1 for the owning config.
func genButtonWithOrder(maxOrder int) gopter.Gen {
	if maxOrder < 0 {
		maxOrder = 0
	}
	return gopter.CombineGens(
		genNonEmptyID(),                        // ID
		genString(),                            // Label
		genString(),                            // Command
		genString(),                            // BgImage
		genString(),                            // BgColor
		genString(),                            // FontColor
		gen.IntRange(minFontSize, maxFontSize), // FontSize
		gen.IntRange(0, maxOrder),              // Order
	).Map(func(vals []interface{}) ButtonConfig {
		return ButtonConfig{
			ID:        vals[0].(string),
			Label:     vals[1].(string),
			Command:   vals[2].(string),
			BgImage:   vals[3].(string),
			BgColor:   vals[4].(string),
			FontColor: vals[5].(string),
			FontSize:  vals[6].(int),
			Order:     vals[7].(int),
		}
	})
}

// genPage builds a PageConfig generator for a fixed pageIndex, with a bounded
// list of buttons whose Order is in 0..maxOrder. Orders are NOT forced unique
// here (round-trip preserves whatever is written); Property 1 compares buttons
// order-insensitively so duplicates are still round-tripped faithfully.
func genPage(pageIndex, maxOrder int) gopter.Gen {
	return gen.SliceOf(genButtonWithOrder(maxOrder)).Map(func(btns []ButtonConfig) PageConfig {
		return PageConfig{PageIndex: pageIndex, Buttons: btns}
	})
}

// genValidConfig builds a generator for a valid MODERN Config: bounded
// Rows/Cols, a non-empty set of pages with unique PageIndex values drawn from
// 0..4, and buttons whose Order is in 0..Rows*Cols-1 and FontSize in -2..+2.
//
// Non-empty Pages guarantees LoadConfig takes the modern (load-as-is) path so
// the round-trip should preserve pages exactly.
func genValidConfig() gopter.Gen {
	return gopter.CombineGens(
		gen.IntRange(minDim, maxDim), // Rows
		gen.IntRange(minDim, maxDim), // Cols
	).FlatMap(func(v interface{}) gopter.Gen {
		vals := v.([]interface{})
		rows := vals[0].(int)
		cols := vals[1].(int)
		maxOrder := rows*cols - 1

		// Choose a non-empty subset of page indices from 0..maxPages-1 with
		// unique PageIndex values. We generate a count 1..maxPages and take the
		// first N indices (0..N-1); uniqueness is guaranteed by construction.
		return gen.IntRange(1, maxPages).FlatMap(func(nv interface{}) gopter.Gen {
			n := nv.(int)
			pageGens := make([]gopter.Gen, n)
			for i := 0; i < n; i++ {
				pageGens[i] = genPage(i, maxOrder)
			}
			return gopter.CombineGens(pageGens...).Map(func(pv []interface{}) Config {
				pages := make([]PageConfig, len(pv))
				for i := range pv {
					pages[i] = pv[i].(PageConfig)
				}
				return Config{Rows: rows, Cols: cols, Pages: pages}
			})
		}, reflectTypeConfig)
	}, reflectTypeConfig)
}

// legacyButton mirrors a single entry in the legacy flat buttons[] array: the
// full ButtonConfig fields plus a top-level "page" field. This is what a
// Legacy_Config file carries.
type legacyButton struct {
	ButtonConfig
	Page int `json:"page"`
}

// legacyConfig is the on-disk shape of a Legacy_Config: rows/cols and a flat
// buttons[] with no pages[] key. Marshaling this produces exactly the legacy
// JSON that LoadConfig migrates.
type legacyConfig struct {
	Rows    int            `json:"rows"`
	Cols    int            `json:"cols"`
	Buttons []legacyButton `json:"buttons"`
}

// genLegacyButton builds a generator for a legacy button carrying a page value
// in 0..maxPages-1 and varied fields (including FontSize, so we can confirm the
// migration intentionally drops it).
func genLegacyButton() gopter.Gen {
	return gopter.CombineGens(
		genNonEmptyID(),                        // ID
		genString(),                            // Label
		genString(),                            // Command
		genString(),                            // BgImage
		genString(),                            // BgColor
		genString(),                            // FontColor
		gen.IntRange(minFontSize, maxFontSize), // FontSize (should be dropped by migration)
		gen.IntRange(0, 64),                    // Order (legacy files are unconstrained)
		gen.IntRange(0, maxPages-1),            // Page
	).Map(func(vals []interface{}) legacyButton {
		return legacyButton{
			ButtonConfig: ButtonConfig{
				ID:        vals[0].(string),
				Label:     vals[1].(string),
				Command:   vals[2].(string),
				BgImage:   vals[3].(string),
				BgColor:   vals[4].(string),
				FontColor: vals[5].(string),
				FontSize:  vals[6].(int),
				Order:     vals[7].(int),
			},
			Page: vals[8].(int),
		}
	})
}

// genLegacyConfig builds a generator for a Legacy_Config. The buttons slice may
// be empty (edge case) — gen.SliceOf can produce an empty slice — so both the
// empty and populated cases are exercised.
func genLegacyConfig() gopter.Gen {
	return gopter.CombineGens(
		gen.IntRange(minDim, maxDim),   // Rows
		gen.IntRange(minDim, maxDim),   // Cols
		gen.SliceOf(genLegacyButton()), // Buttons
	).Map(func(vals []interface{}) legacyConfig {
		btnsAny := vals[2].([]legacyButton)
		return legacyConfig{
			Rows:    vals[0].(int),
			Cols:    vals[1].(int),
			Buttons: btnsAny,
		}
	})
}

// reflectTypeConfig is the reflect.Type of the value FlatMap produces, so
// gopter knows the generated value's type.
var reflectTypeConfig = reflect.TypeOf(Config{})

// --- Comparison / normalization helpers ---

// buttonKeyNoFontSize compares buttons IGNORING FontSize. The legacy migration
// intentionally does not copy FontSize (a preserved app.go quirk), so
// Property 2 must compare on ID/Label/Command/BgImage/BgColor/FontColor/Order
// only.
type buttonKeyNoFontSize struct {
	ID        string
	Label     string
	Command   string
	BgImage   string
	BgColor   string
	FontColor string
	Order     int
}

func keyNoFontSize(b ButtonConfig) buttonKeyNoFontSize {
	return buttonKeyNoFontSize{
		ID:        b.ID,
		Label:     b.Label,
		Command:   b.Command,
		BgImage:   b.BgImage,
		BgColor:   b.BgColor,
		FontColor: b.FontColor,
		Order:     b.Order,
	}
}

// sortButtonsByOrder returns a copy of the buttons sorted by Order (then by a
// full field comparison to make the order-insensitive comparison total and
// stable even when Orders collide). Used by Property 1 to compare a page's
// buttons ignoring slice order.
func sortButtonsByOrder(btns []ButtonConfig) []ButtonConfig {
	out := make([]ButtonConfig, len(btns))
	copy(out, btns)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Order != out[j].Order {
			return out[i].Order < out[j].Order
		}
		if out[i].ID != out[j].ID {
			return out[i].ID < out[j].ID
		}
		return out[i].Label < out[j].Label
	})
	return out
}

// sortPagesByIndex returns a copy of the pages sorted by PageIndex. Used by
// Property 1 to compare pages ignoring slice order (robust against the modern
// path, and required in general because the design does not guarantee page
// slice order).
func sortPagesByIndex(pages []PageConfig) []PageConfig {
	out := make([]PageConfig, len(pages))
	copy(out, pages)
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].PageIndex < out[j].PageIndex
	})
	return out
}
