package ui

import (
	"image"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"touchdeck/internal/core"
)

// configScrollHarness drives the real Renderer config view headlessly through
// a Gio input.Router, so wheel events take the same routing path as in the app.
type configScrollHarness struct {
	r      *Renderer
	router input.Router
	ops    op.Ops
	now    time.Time
}

func newConfigScrollHarness(t *testing.T) *configScrollHarness {
	t.Helper()
	// refreshImages -> ListConfigImages -> os.UserConfigDir must not touch the
	// real config directory.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("AppData", t.TempDir())

	cfg := core.Config{
		Rows: 2,
		Cols: 4,
		Pages: []core.PageConfig{{
			PageIndex: 0,
			Buttons:   []core.ButtonConfig{{ID: "a", Label: "A", Command: "echo hi"}},
		}},
	}
	st := NewAppState(core.New(), cfg)
	st.Textures = NewTextureCache()
	st.View = ViewConfig
	h := &configScrollHarness{r: NewRenderer(st), now: time.Unix(0, 0)}
	h.r.selectSlot(0)
	h.frame()
	h.frame()
	return h
}

func (h *configScrollHarness) frame() {
	h.ops.Reset()
	gtx := layout.Context{
		Ops:         &h.ops,
		Constraints: layout.Exact(image.Pt(1024, 500)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         h.now,
		Source:      h.router.Source(),
	}
	h.r.layout(gtx)
	h.router.Frame(&h.ops)
	h.now = h.now.Add(16 * time.Millisecond)
}

func (h *configScrollHarness) wheel(pos f32.Point, dy float32) {
	h.router.Queue(pointer.Event{
		Kind:     pointer.Scroll,
		Source:   pointer.Mouse,
		Position: pos,
		Scroll:   f32.Pt(0, dy),
	})
	h.frame()
}

func (h *configScrollHarness) moved() bool {
	p := h.r.cfg.formList.Position
	return p.First > 0 || p.Offset > 0
}

// TestConfigFormWheelScroll covers a Windows-sized wheel notch (120 px) over
// the form itself.
func TestConfigFormWheelScroll(t *testing.T) {
	h := newConfigScrollHarness(t)
	h.wheel(f32.Pt(768, 300), 120)
	if !h.moved() {
		t.Fatalf("form did not scroll: %+v", h.r.cfg.formList.Position)
	}
}

// TestConfigFormWheelScrollInPaneMargin covers a wheel event inside the right
// pane's 16 dp inset, outside the form list's own rect.
func TestConfigFormWheelScrollInPaneMargin(t *testing.T) {
	h := newConfigScrollHarness(t)
	h.wheel(f32.Pt(1016, 300), 120)
	if !h.moved() {
		t.Fatalf("form did not scroll from pane margin: %+v", h.r.cfg.formList.Position)
	}
}

// TestConfigFormWheelScrollSubPixel covers high-resolution wheel/touchpad
// deltas smaller than one pixel, which must accumulate.
func TestConfigFormWheelScrollSubPixel(t *testing.T) {
	h := newConfigScrollHarness(t)
	for i := 0; i < 3; i++ {
		h.wheel(f32.Pt(768, 300), 0.5)
	}
	if p := h.r.cfg.formList.Position; p.First != 0 || p.Offset != 1 {
		t.Fatalf("want First=0 Offset=1 after 3x0.5px, got %+v", p)
	}
}
