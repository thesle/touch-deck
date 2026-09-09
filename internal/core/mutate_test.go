package core

// Property and unit tests for the pure state helpers in grid.go and mutate.go
// (tasks 4.3–4.8). White-box tests in package core so they can reuse the shared
// gopter generators/helpers already defined in gen_test.go (genValidConfig,
// genButtonWithOrder, sortButtonsByOrder, ...).
//
// New identifiers in this file are prefixed with "mut" or "grid" to avoid
// colliding with the shared identifiers declared in gen_test.go / config_test.go
// / image_test.go (newTestBackend, genValidConfig, genLegacyConfig,
// legacyButton, legacyConfig, keyNoFontSize, sortButtonsByOrder,
// sortPagesByIndex, buttonKeyNoFontSize, reflectTypeConfig, isAllDigits, img*).

import (
	"reflect"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// mutMinIters is the shared MinSuccessfulTests floor required by the design
// (>= 100). We use 200 for parity with the existing property tests.
const mutMinIters = 200

// ---------------------------------------------------------------------------
// Shared generators for the mutate/grid property tests
// ---------------------------------------------------------------------------

// mutGenSmallDims produces a (rows, cols) pair in a small range so grids stay
// tractable but still exercise multi-row/multi-col geometry. We use 1..8 to
// match the existing genValidConfig bounds.
func mutGenSmallDims() gopter.Gen {
	return gopter.CombineGens(
		gen.IntRange(minDim, maxDim),
		gen.IntRange(minDim, maxDim),
	)
}

// mutGenDirection draws one of the four move directions.
func mutGenDirection() gopter.Gen {
	return gen.OneConstOf(Left, Up, Down, Right).WithShrinker(gopter.NoShrinker)
}

// mutGenUniquePage builds a PageConfig generator whose buttons have UNIQUE Order
// values drawn from 0..maxOrder. Each candidate slot is independently included
// (with a button) or not, so pages range from empty to full. Order uniqueness is
// guaranteed by construction (one button per included slot index), which makes
// Property 4 / Property 6 well-defined starting states.
//
// pageIndex is fixed on the produced page. maxOrder is rows*cols-1.
func mutGenUniquePage(pageIndex, maxOrder int) gopter.Gen {
	if maxOrder < 0 {
		maxOrder = 0
	}
	nSlots := maxOrder + 1
	// One boolean per slot deciding whether that slot is occupied, plus one
	// button generator per slot to fill it when occupied.
	slotGens := make([]gopter.Gen, 0, nSlots*2)
	for i := 0; i < nSlots; i++ {
		slotGens = append(slotGens, gen.Bool())
		slotGens = append(slotGens, genButtonWithOrder(maxOrder))
	}
	return gopter.CombineGens(slotGens...).Map(func(vals []interface{}) PageConfig {
		btns := make([]ButtonConfig, 0, nSlots)
		for slot := 0; slot < nSlots; slot++ {
			occupied := vals[slot*2].(bool)
			if !occupied {
				continue
			}
			b := vals[slot*2+1].(ButtonConfig)
			b.Order = slot // force a unique Order == its slot index
			btns = append(btns, b)
		}
		return PageConfig{PageIndex: pageIndex, Buttons: btns}
	})
}

// mutOpposite returns the opposite move direction: Left<->Right, Up<->Down.
func mutOpposite(dir Direction) Direction {
	switch dir {
	case Left:
		return Right
	case Right:
		return Left
	case Up:
		return Down
	case Down:
		return Up
	}
	return dir
}

// mutOccupancy returns the set of occupied Order values on a page.
func mutOccupancy(page PageConfig) map[int]ButtonConfig {
	m := make(map[int]ButtonConfig, len(page.Buttons))
	for _, b := range page.Buttons {
		m[b.Order] = b
	}
	return m
}

// mutPagesEqualUpToOrder reports whether two pages hold the same set of buttons
// keyed by Order (order-insensitive on slice order). Uses sortButtonsByOrder
// from gen_test.go.
func mutPagesEqualUpToOrder(a, b PageConfig) bool {
	sa := sortButtonsByOrder(a.Buttons)
	sb := sortButtonsByOrder(b.Buttons)
	if len(sa) != len(sb) {
		return false
	}
	for i := range sa {
		if sa[i] != sb[i] {
			return false
		}
	}
	return true
}

// mutHasUniqueOrders reports whether every button on the page has a distinct
// Order value.
func mutHasUniqueOrders(page PageConfig) bool {
	seen := make(map[int]bool, len(page.Buttons))
	for _, b := range page.Buttons {
		if seen[b.Order] {
			return false
		}
		seen[b.Order] = true
	}
	return true
}

// ---------------------------------------------------------------------------
// Task 4.3 — Property 3: Grid-size prune invariant
// ---------------------------------------------------------------------------

// Feature: wails-to-gio-migration, Property 3: Grid-size prune invariant
//
// For any valid Config and any new grid dimensions (rows,cols in 1..8), after
// PruneToGrid(cfg, rows, cols) no page contains a button whose Order is >=
// rows*cols. Additionally the input Config is NOT mutated (verified by
// deep-comparing a pre-copy taken before the call).
//
// Choices (documented):
//   - The config comes from the shared genValidConfig(); its buttons have Orders
//     in 0..oldRows*oldCols-1, which may be larger OR smaller than the new
//     rows*cols, so both "prunes something" and "prunes nothing" cases occur.
//   - A deep copy of the input is taken with mutDeepCopyConfig before the call
//     and reflect.DeepEqual'd against the input afterward to assert purity.
func TestProperty3_GridPruneInvariant(t *testing.T) {
	params := gopter.DefaultTestParameters()
	params.MinSuccessfulTests = mutMinIters

	properties := gopter.NewProperties(params)
	properties.Property("after PruneToGrid no button has Order >= rows*cols; input unchanged", prop.ForAll(
		func(cfg Config, dims []interface{}) bool {
			rows := dims[0].(int)
			cols := dims[1].(int)
			maxSlots := rows * cols

			before := mutDeepCopyConfig(cfg)
			out := PruneToGrid(cfg, rows, cols)

			// Invariant: no page has a button with Order >= rows*cols.
			for _, p := range out.Pages {
				for _, b := range p.Buttons {
					if b.Order >= maxSlots {
						t.Logf("page %d retained button Order=%d >= maxSlots=%d", p.PageIndex, b.Order, maxSlots)
						return false
					}
				}
			}

			// Purity: input config must be unchanged.
			if !reflect.DeepEqual(before, cfg) {
				t.Logf("PruneToGrid mutated its input config")
				return false
			}
			return true
		},
		genValidConfig(),
		mutGenSmallDims(),
	))

	properties.TestingRun(t)
}

// mutDeepCopyConfig returns a structurally independent copy of cfg (fresh
// Pages/Buttons slices) so a mutation of the original would be detectable.
func mutDeepCopyConfig(cfg Config) Config {
	out := cfg
	out.Pages = make([]PageConfig, len(cfg.Pages))
	for i, p := range cfg.Pages {
		np := p
		np.Buttons = make([]ButtonConfig, len(p.Buttons))
		copy(np.Buttons, p.Buttons)
		out.Pages[i] = np
	}
	return out
}

// ---------------------------------------------------------------------------
// Task 4.4 — Property 4: Slot-swap is a self-inverse
// ---------------------------------------------------------------------------

// Feature: wails-to-gio-migration, Property 4: Slot-swap is a self-inverse
//
// Generate a page with unique Orders, a rows x cols grid, a selected slot, and a
// direction. Let (p2, newSel, ok) = MoveButton(page, sel, rows, cols, dir).
//   - If ok: applying the OPPOSITE direction from newSel restores the original
//     page (compared order-insensitively). Exactly the two involved slots change
//     occupancy; every other slot is unchanged.
//   - If !ok: p2 equals the input page and newSel == sel. This covers the
//     invalid cases (sel is a pagination slot, target off-grid, target is a
//     pagination slot).
//
// Choices (documented):
//   - The selected slot is drawn from the full 0..rows*cols-1 range so both
//     valid (interior/edge non-pagination) and invalid (edge/pagination) moves
//     are exercised. When sel lands on a pagination slot or the move hits an
//     edge, ok is false and the "unchanged" branch is checked.
//   - opposite(): Left<->Right, Up<->Down (mutOpposite).
//   - The page's Orders are unique by construction (mutGenUniquePage), so
//     occupancy comparison by Order is well-defined.
func TestProperty4_SlotSwapSelfInverse(t *testing.T) {
	params := gopter.DefaultTestParameters()
	params.MinSuccessfulTests = mutMinIters

	// A FlatMap threads rows/cols through the page generator and the
	// selected-slot range so all dependent values share the same grid geometry.
	properties := gopter.NewProperties(params)
	properties.Property("valid move swaps two slots and is self-inverse; invalid move is a no-op", prop.ForAll(
		func(scenario mutMoveScenario) bool {
			page := scenario.Page
			sel := scenario.Sel
			rows := scenario.Rows
			cols := scenario.Cols
			dir := scenario.Dir

			beforeOcc := mutOccupancy(page)
			p2, newSel, ok := MoveButton(page, sel, rows, cols, dir)

			if !ok {
				// Invalid move: page unchanged, selection unchanged.
				if newSel != sel {
					t.Logf("invalid move changed sel: %d -> %d", sel, newSel)
					return false
				}
				if !mutPagesEqualUpToOrder(p2, page) {
					t.Logf("invalid move changed the page")
					return false
				}
				return true
			}

			// Valid move: figure out the target it swapped with.
			target := newSel
			if target == sel {
				t.Logf("valid move returned newSel == sel (%d)", sel)
				return false
			}
			if IsPaginationSlot(sel, rows, cols) || IsPaginationSlot(target, rows, cols) {
				t.Logf("valid move touched a pagination slot (sel=%d target=%d)", sel, target)
				return false
			}

			afterOcc := mutOccupancy(p2)

			// Only slots {sel, target} may change occupancy; all others equal.
			for slot, b := range beforeOcc {
				if slot == sel || slot == target {
					continue
				}
				ab, ok := afterOcc[slot]
				if !ok || ab != b {
					t.Logf("unrelated slot %d changed", slot)
					return false
				}
			}
			for slot := range afterOcc {
				if slot == sel || slot == target {
					continue
				}
				if _, ok := beforeOcc[slot]; !ok {
					t.Logf("unrelated slot %d appeared", slot)
					return false
				}
			}

			// The two involved slots must have swapped their (Order-adjusted)
			// buttons. A button that was at sel must now be at target (with
			// Order==target), and vice versa.
			if bSel, had := beforeOcc[sel]; had {
				got, ok := afterOcc[target]
				bSel.Order = target
				if !ok || got != bSel {
					t.Logf("button from sel did not land at target correctly")
					return false
				}
			} else {
				// sel was empty -> target must now be empty.
				if _, ok := afterOcc[target]; ok {
					t.Logf("target should be empty after moving an empty sel")
					return false
				}
			}
			if bTarget, had := beforeOcc[target]; had {
				got, ok := afterOcc[sel]
				bTarget.Order = sel
				if !ok || got != bTarget {
					t.Logf("button from target did not land at sel correctly")
					return false
				}
			} else {
				if _, ok := afterOcc[sel]; ok {
					t.Logf("sel should be empty after moving into an empty target")
					return false
				}
			}

			// Self-inverse: opposite move from newSel restores the page.
			p3, backSel, ok2 := MoveButton(p2, newSel, rows, cols, mutOpposite(dir))
			if !ok2 {
				t.Logf("opposite move unexpectedly invalid")
				return false
			}
			if backSel != sel {
				t.Logf("opposite move selection %d != original sel %d", backSel, sel)
				return false
			}
			if !mutPagesEqualUpToOrder(p3, page) {
				t.Logf("opposite move did not restore the original page")
				return false
			}
			return true
		},
		mutGenMoveScenario(),
	))

	properties.TestingRun(t)
}

// mutMoveScenario bundles a page with the grid dims, a selected slot, and a
// direction so all dependent values share the same rows/cols.
type mutMoveScenario struct {
	Rows int
	Cols int
	Page PageConfig
	Sel  int
	Dir  Direction
}

var mutReflectMoveScenario = reflect.TypeOf(mutMoveScenario{})

// mutGenMoveScenario threads rows/cols through the page generator and the
// selected-slot range so they are always consistent.
func mutGenMoveScenario() gopter.Gen {
	return mutGenSmallDims().FlatMap(func(v interface{}) gopter.Gen {
		dims := v.([]interface{})
		rows := dims[0].(int)
		cols := dims[1].(int)
		maxOrder := rows*cols - 1
		if maxOrder < 0 {
			maxOrder = 0
		}
		return gopter.CombineGens(
			mutGenUniquePage(0, maxOrder),
			gen.IntRange(0, maxOrder),
			mutGenDirection(),
		).Map(func(vals []interface{}) mutMoveScenario {
			return mutMoveScenario{
				Rows: rows,
				Cols: cols,
				Page: vals[0].(PageConfig),
				Sel:  vals[1].(int),
				Dir:  vals[2].(Direction),
			}
		})
	}, mutReflectMoveScenario)
}

// ---------------------------------------------------------------------------
// Task 4.5 — Property 5: Pagination wrap
// ---------------------------------------------------------------------------

// Feature: wails-to-gio-migration, Property 5: Pagination wrap
//
// For any total >= 1 and any p in 0..total-1:
//   - NextPage(p,total) == (p+1) % total
//   - PrevPage(p,total) == (p-1+total) % total
//   - PrevPage(NextPage(p,total),total) == p  (back-after-advance restores)
//   - NextPage(PrevPage(p,total),total) == p  (advance-after-back restores)
//   - both results stay in 0..total-1
//
// The app's real TotalPages is 5, so a dedicated subtest pins total==5.
func TestProperty5_PaginationWrap(t *testing.T) {
	params := gopter.DefaultTestParameters()
	params.MinSuccessfulTests = mutMinIters

	properties := gopter.NewProperties(params)
	properties.Property("next/prev wrap correctly and are mutual inverses", prop.ForAll(
		func(scenario mutPageScenario) bool {
			p := scenario.P
			total := scenario.Total

			next := NextPage(p, total)
			prev := PrevPage(p, total)

			if next != (p+1)%total {
				t.Logf("NextPage(%d,%d)=%d, want %d", p, total, next, (p+1)%total)
				return false
			}
			if prev != (p-1+total)%total {
				t.Logf("PrevPage(%d,%d)=%d, want %d", p, total, prev, (p-1+total)%total)
				return false
			}
			if next < 0 || next >= total || prev < 0 || prev >= total {
				t.Logf("result out of range: next=%d prev=%d total=%d", next, prev, total)
				return false
			}
			if got := PrevPage(NextPage(p, total), total); got != p {
				t.Logf("PrevPage(NextPage(%d))=%d, want %d", p, got, p)
				return false
			}
			if got := NextPage(PrevPage(p, total), total); got != p {
				t.Logf("NextPage(PrevPage(%d))=%d, want %d", p, got, p)
				return false
			}
			return true
		},
		mutGenPageScenario(),
	))

	properties.TestingRun(t)

	// Dedicated coverage for the app's actual TotalPages (5): every page index
	// wraps as specified.
	const total = 5
	for p := 0; p < total; p++ {
		if got := NextPage(p, total); got != (p+1)%total {
			t.Errorf("NextPage(%d,5)=%d, want %d", p, got, (p+1)%total)
		}
		if got := PrevPage(p, total); got != (p-1+total)%total {
			t.Errorf("PrevPage(%d,5)=%d, want %d", p, got, (p-1+total)%total)
		}
	}
	if NextPage(4, total) != 0 {
		t.Errorf("NextPage(4,5) should wrap to 0")
	}
	if PrevPage(0, total) != 4 {
		t.Errorf("PrevPage(0,5) should wrap to 4")
	}
}

// mutPageScenario bundles a total page count with an in-range page index.
type mutPageScenario struct {
	Total int
	P     int
}

var mutReflectPageScenario = reflect.TypeOf(mutPageScenario{})

// mutGenPageScenario generates total in 1..12 and p in 0..total-1.
func mutGenPageScenario() gopter.Gen {
	return gen.IntRange(1, 12).FlatMap(func(v interface{}) gopter.Gen {
		total := v.(int)
		return gen.IntRange(0, total-1).Map(func(p int) mutPageScenario {
			return mutPageScenario{Total: total, P: p}
		})
	}, mutReflectPageScenario)
}

// ---------------------------------------------------------------------------
// Task 4.6 — Property 6: Button-order uniqueness per page
// ---------------------------------------------------------------------------

// mutOpKind enumerates the operations applied in a Property 6 sequence.
type mutOpKind int

const (
	mutOpWrite mutOpKind = iota
	mutOpClear
	mutOpMove
	mutOpPrune
)

// mutOp is a single generated operation against a modeled single page (fixed
// rows/cols). Fields are interpreted per Kind.
type mutOp struct {
	Kind      mutOpKind
	Slot      int          // WriteSlot / ClearSlot / MoveButton sel
	Btn       ButtonConfig // WriteSlot payload
	Dir       Direction    // MoveButton direction
	PruneRows int          // PruneToGrid rows
	PruneCols int          // PruneToGrid cols
}

// mutOpSequence bundles a starting page with rows/cols and a list of operations.
type mutOpSequence struct {
	Rows int
	Cols int
	Page PageConfig
	Ops  []mutOp
}

var mutReflectOpSequence = reflect.TypeOf(mutOpSequence{})

// Feature: wails-to-gio-migration, Property 6: Button-order uniqueness per page
//
// Starting from a page with UNIQUE Orders, applying any sequence of operations
// drawn from {WriteSlot, ClearSlot, MoveButton, PruneToGrid} preserves the
// invariant that no two buttons on the page share an Order. The invariant is
// checked after EVERY operation.
//
// Choices (documented):
//   - The model is a single page with fixed rows/cols. PruneToGrid is a
//     Config-level helper, so the op wraps the page in a one-page Config, prunes
//     with a (possibly smaller) grid, and unwraps page 0. WriteSlot/ClearSlot/
//     MoveButton operate on the page directly.
//   - The starting page is generated with mutGenUniquePage so the invariant is
//     well-defined initially; the property is that the helpers MAINTAIN it.
//   - Op slots are drawn from the full 0..rows*cols-1 range; invalid MoveButton
//     targets (edges/pagination) are no-ops, which trivially preserve the
//     invariant.
func TestProperty6_OrderUniquenessPerPage(t *testing.T) {
	params := gopter.DefaultTestParameters()
	params.MinSuccessfulTests = mutMinIters

	properties := gopter.NewProperties(params)
	properties.Property("no page ever holds two buttons sharing an Order across an op sequence", prop.ForAll(
		func(seq mutOpSequence) bool {
			page := seq.Page
			rows := seq.Rows
			cols := seq.Cols

			// Sanity: starting page must have unique Orders (generator guarantee).
			if !mutHasUniqueOrders(page) {
				t.Logf("generated starting page violated uniqueness precondition")
				return false
			}

			for i, op := range seq.Ops {
				switch op.Kind {
				case mutOpWrite:
					page = WriteSlot(page, op.Slot, op.Btn)
				case mutOpClear:
					page = ClearSlot(page, op.Slot)
				case mutOpMove:
					page, _, _ = MoveButton(page, op.Slot, rows, cols, op.Dir)
				case mutOpPrune:
					pr := op.PruneRows
					pc := op.PruneCols
					wrapped := Config{Rows: pr, Cols: pc, Pages: []PageConfig{page}}
					pruned := PruneToGrid(wrapped, pr, pc)
					page = pruned.Pages[0]
				}
				if !mutHasUniqueOrders(page) {
					t.Logf("uniqueness violated after op %d (kind=%d)", i, op.Kind)
					return false
				}
			}
			return true
		},
		mutGenOpSequence(),
	))

	properties.TestingRun(t)
}

// mutGenOpSequence threads rows/cols through the starting page and each op so
// slots stay in range for the modeled grid.
func mutGenOpSequence() gopter.Gen {
	return mutGenSmallDims().FlatMap(func(v interface{}) gopter.Gen {
		dims := v.([]interface{})
		rows := dims[0].(int)
		cols := dims[1].(int)
		maxOrder := rows*cols - 1
		if maxOrder < 0 {
			maxOrder = 0
		}
		return gopter.CombineGens(
			mutGenUniquePage(0, maxOrder),
			gen.SliceOf(mutGenOp(rows, cols, maxOrder)),
		).Map(func(vals []interface{}) mutOpSequence {
			return mutOpSequence{
				Rows: rows,
				Cols: cols,
				Page: vals[0].(PageConfig),
				Ops:  vals[1].([]mutOp),
			}
		})
	}, mutReflectOpSequence)
}

var mutReflectOp = reflect.TypeOf(mutOp{})

// mutGenOp builds a single operation generator for a rows x cols grid.
func mutGenOp(rows, cols, maxOrder int) gopter.Gen {
	return gopter.CombineGens(
		gen.IntRange(0, 3),           // op kind
		gen.IntRange(0, maxOrder),    // slot
		genButtonWithOrder(maxOrder), // write payload
		mutGenDirection(),            // move direction
		gen.IntRange(1, maxDim),      // prune rows (>=1)
		gen.IntRange(1, maxDim),      // prune cols (>=1)
	).Map(func(vals []interface{}) mutOp {
		return mutOp{
			Kind:      mutOpKind(vals[0].(int)),
			Slot:      vals[1].(int),
			Btn:       vals[2].(ButtonConfig),
			Dir:       vals[3].(Direction),
			PruneRows: vals[4].(int),
			PruneCols: vals[5].(int),
		}
	})
}

// ---------------------------------------------------------------------------
// Task 4.7 — Property 7: Slot-write preserves fields and order
// ---------------------------------------------------------------------------

// Feature: wails-to-gio-migration, Property 7: Slot-write preserves fields and order
//
// For any page, any editable target slot, and any button settings, after
// WriteSlot(page, slot, btn):
//   - ButtonAtSlot(result, slot) returns a button whose Label/Command/BgImage/
//     BgColor/FontColor/FontSize equal btn's, and whose Order == slot.
//   - Exactly one button occupies that Order (any prior occupant is replaced).
//
// The generated btn may carry any Order; WriteSlot must overwrite it with slot.
func TestProperty7_SlotWritePreservesFieldsAndOrder(t *testing.T) {
	params := gopter.DefaultTestParameters()
	params.MinSuccessfulTests = mutMinIters

	properties := gopter.NewProperties(params)
	properties.Property("WriteSlot places btn's fields at target slot with Order==slot, replacing prior", prop.ForAll(
		func(scenario mutWriteScenario) bool {
			page := scenario.Page
			slot := scenario.Slot
			btn := scenario.Btn

			result := WriteSlot(page, slot, btn)

			// Exactly one button at Order == slot.
			count := 0
			for _, b := range result.Buttons {
				if b.Order == slot {
					count++
				}
			}
			if count != 1 {
				t.Logf("expected exactly 1 button at slot %d, got %d", slot, count)
				return false
			}

			got, ok := ButtonAtSlot(result, slot)
			if !ok {
				t.Logf("no button at slot %d after WriteSlot", slot)
				return false
			}
			if got.Order != slot {
				t.Logf("written button Order = %d, want %d", got.Order, slot)
				return false
			}
			if got.Label != btn.Label ||
				got.Command != btn.Command ||
				got.BgImage != btn.BgImage ||
				got.BgColor != btn.BgColor ||
				got.FontColor != btn.FontColor ||
				got.FontSize != btn.FontSize {
				t.Logf("written button fields differ from input settings")
				return false
			}
			return true
		},
		mutGenWriteScenario(),
	))

	properties.TestingRun(t)
}

// mutWriteScenario bundles a page, a target slot, and a button payload sharing
// consistent grid bounds.
type mutWriteScenario struct {
	Page PageConfig
	Slot int
	Btn  ButtonConfig
}

var mutReflectWriteScenario = reflect.TypeOf(mutWriteScenario{})

// mutGenWriteScenario threads maxOrder through the page, slot, and payload.
func mutGenWriteScenario() gopter.Gen {
	return mutGenSmallDims().FlatMap(func(v interface{}) gopter.Gen {
		dims := v.([]interface{})
		rows := dims[0].(int)
		cols := dims[1].(int)
		maxOrder := rows*cols - 1
		if maxOrder < 0 {
			maxOrder = 0
		}
		return gopter.CombineGens(
			mutGenUniquePage(0, maxOrder),
			gen.IntRange(0, maxOrder),
			genButtonWithOrder(maxOrder),
		).Map(func(vals []interface{}) mutWriteScenario {
			return mutWriteScenario{
				Page: vals[0].(PageConfig),
				Slot: vals[1].(int),
				Btn:  vals[2].(ButtonConfig),
			}
		})
	}, mutReflectWriteScenario)
}

// ---------------------------------------------------------------------------
// Task 4.8 — Unit tests for the pure state helpers
// ---------------------------------------------------------------------------

// Grid math: SlotToRowCol / RowColToSlot round-trip and edge cases.
func TestGridMath_RoundTripAndEdges(t *testing.T) {
	cases := []struct {
		index, cols int
	}{
		{0, 4},
		{3, 4},
		{4, 4},
		{7, 4},
		{5, 3},
		{10, 1},
	}
	for _, c := range cases {
		row, col := SlotToRowCol(c.index, c.cols)
		back := RowColToSlot(row, col, c.cols)
		if back != c.index {
			t.Errorf("round trip: SlotToRowCol(%d,%d)=(%d,%d) -> RowColToSlot=%d, want %d",
				c.index, c.cols, row, col, back, c.index)
		}
	}

	// Concrete coordinate checks for a 4-column grid.
	if r, c := SlotToRowCol(5, 4); r != 1 || c != 1 {
		t.Errorf("SlotToRowCol(5,4) = (%d,%d), want (1,1)", r, c)
	}
	if s := RowColToSlot(1, 1, 4); s != 5 {
		t.Errorf("RowColToSlot(1,1,4) = %d, want 5", s)
	}

	// index 0 -> (0,0).
	if r, c := SlotToRowCol(0, 4); r != 0 || c != 0 {
		t.Errorf("SlotToRowCol(0,4) = (%d,%d), want (0,0)", r, c)
	}

	// cols <= 0 guards: SlotToRowCol -> (0,0), RowColToSlot -> 0.
	if r, c := SlotToRowCol(7, 0); r != 0 || c != 0 {
		t.Errorf("SlotToRowCol(7,0) = (%d,%d), want (0,0)", r, c)
	}
	if r, c := SlotToRowCol(7, -3); r != 0 || c != 0 {
		t.Errorf("SlotToRowCol(7,-3) = (%d,%d), want (0,0)", r, c)
	}
	if s := RowColToSlot(2, 1, 0); s != 0 {
		t.Errorf("RowColToSlot(2,1,0) = %d, want 0", s)
	}
}

// FontSizeForOffset: the five documented offsets plus out-of-range fallbacks.
func TestFontSizeForOffset(t *testing.T) {
	cases := []struct {
		offset int
		want   float32
	}{
		{-2, 12},
		{-1, 14},
		{0, 18},
		{1, 24},
		{2, 30},
		{3, 18},  // out of range -> default
		{-5, 18}, // out of range -> default
		{100, 18},
	}
	for _, c := range cases {
		if got := FontSizeForOffset(c.offset); got != c.want {
			t.Errorf("FontSizeForOffset(%d) = %v, want %v", c.offset, got, c.want)
		}
	}
	if DefaultFontSizePt != 18 {
		t.Errorf("DefaultFontSizePt = %v, want 18", DefaultFontSizePt)
	}
}

// ButtonAtSlot: found and not-found cases.
func TestButtonAtSlot_FoundAndMissing(t *testing.T) {
	page := PageConfig{
		PageIndex: 0,
		Buttons: []ButtonConfig{
			{ID: "a", Label: "A", Order: 0},
			{ID: "b", Label: "B", Order: 2},
		},
	}
	if b, ok := ButtonAtSlot(page, 2); !ok || b.ID != "b" {
		t.Errorf("ButtonAtSlot(page,2) = (%+v,%v), want button b", b, ok)
	}
	if _, ok := ButtonAtSlot(page, 1); ok {
		t.Errorf("ButtonAtSlot(page,1) found a button, want none")
	}
}

// ButtonAtSlotOnPage: matching and non-matching pageIndex.
func TestButtonAtSlotOnPage(t *testing.T) {
	cfg := Config{
		Rows: 2, Cols: 4,
		Pages: []PageConfig{
			{PageIndex: 0, Buttons: []ButtonConfig{{ID: "p0", Order: 1}}},
			{PageIndex: 3, Buttons: []ButtonConfig{{ID: "p3", Order: 5}}},
		},
	}
	if b, ok := ButtonAtSlotOnPage(cfg, 3, 5); !ok || b.ID != "p3" {
		t.Errorf("ButtonAtSlotOnPage(cfg,3,5) = (%+v,%v), want p3", b, ok)
	}
	if _, ok := ButtonAtSlotOnPage(cfg, 0, 5); ok {
		t.Errorf("ButtonAtSlotOnPage(cfg,0,5) found a button, want none (no button at slot 5 on page 0)")
	}
	if _, ok := ButtonAtSlotOnPage(cfg, 2, 1); ok {
		t.Errorf("ButtonAtSlotOnPage(cfg,2,1) found a button, want none (no page 2)")
	}
}

// IsRenderable: true when any of label/bgImage/command set; false for zero.
func TestIsRenderable(t *testing.T) {
	if !IsRenderable(ButtonConfig{Label: "hi"}) {
		t.Error("button with label should be renderable")
	}
	if !IsRenderable(ButtonConfig{BgImage: "/x.png"}) {
		t.Error("button with bgImage should be renderable")
	}
	if !IsRenderable(ButtonConfig{Command: "ls"}) {
		t.Error("button with command should be renderable")
	}
	if IsRenderable(ButtonConfig{}) {
		t.Error("zero button should not be renderable")
	}
	// A button with only cosmetic fields (no label/image/command) is not
	// renderable.
	if IsRenderable(ButtonConfig{BgColor: "#fff", FontColor: "#000", FontSize: 2, ID: "x", Order: 3}) {
		t.Error("button with only colors/id/order should not be renderable")
	}
}

// PageIndicator: 1-based "current / total".
func TestPageIndicator(t *testing.T) {
	if got := PageIndicator(0, 5); got != "1 / 5" {
		t.Errorf("PageIndicator(0,5) = %q, want %q", got, "1 / 5")
	}
	if got := PageIndicator(4, 5); got != "5 / 5" {
		t.Errorf("PageIndicator(4,5) = %q, want %q", got, "5 / 5")
	}
}

// IsPaginationSlot / PrevPageSlot / NextPageSlot for a 2x4 grid (8 slots):
// prev=6, next=7; pagination true for 6,7 and false for 0..5.
func TestPaginationSlots_2x4(t *testing.T) {
	const rows, cols = 2, 4
	if got := PrevPageSlot(rows, cols); got != 6 {
		t.Errorf("PrevPageSlot(2,4) = %d, want 6", got)
	}
	if got := NextPageSlot(rows, cols); got != 7 {
		t.Errorf("NextPageSlot(2,4) = %d, want 7", got)
	}
	for slot := 0; slot <= 5; slot++ {
		if IsPaginationSlot(slot, rows, cols) {
			t.Errorf("IsPaginationSlot(%d,2,4) = true, want false", slot)
		}
	}
	if !IsPaginationSlot(6, rows, cols) {
		t.Errorf("IsPaginationSlot(6,2,4) = false, want true")
	}
	if !IsPaginationSlot(7, rows, cols) {
		t.Errorf("IsPaginationSlot(7,2,4) = false, want true")
	}
}
