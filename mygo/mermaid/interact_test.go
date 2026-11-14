package mermaid

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

const wide = "flowchart LR\n    A[Collect requirements] --> B[Write the design doc] --> C[Review with the team] --> D[Implement the feature] --> E[Ship to production]\n"

const small = "flowchart TD\n    A[Start] --> B{Valid?}\n    B -->|yes| C[Save record]\n    B -->|no| D[Show error]\n"

func view(src *string, o *Options) func(c *ui.Context) {
	return func(c *ui.Context) {
		ui.Column(c).Padding(16).AlignItems(ui.Stretch).Children(func() { View(c, *src, *o) })
	}
}

// settle runs the frames of the transitions under way.
func settle(tt *ui.Tester) {
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	tt.Advance(time.Second)
}

// TestFitWidth checks that a complete diagram wider than the view is
// scaled down to fit, and that one still streaming in is not.
func TestFitWidth(t *testing.T) {
	src, o := wide, Options{Streaming: true}
	tt := ui.NewTester(view(&src, &o), 520, 300)
	settle(tt)
	last, ok := tt.Find("Ship to production")
	first, _ := tt.Find("Collect requirements")
	if !ok {
		t.Fatalf("texts %q", tt.Texts())
	}
	if last.X < 520 {
		t.Errorf("streaming, the diagram was scaled: its last node is at x=%v in a view 520 wide", last.X)
	}
	snapshot(t, tt, "fit-0-streaming")
	o.Streaming = false
	settle(tt)
	fit, ok := tt.Find("Ship to production")
	if !ok || fit.X+fit.W > 520 {
		t.Errorf("complete, the last node's label is at %v: it should fit in the view", fit)
	}
	if now, _ := tt.Find("Collect requirements"); now.H >= first.H {
		t.Errorf("the labels should be smaller once the diagram is scaled down: %v, was %v", now.H, first.H)
	}
	snapshot(t, tt, "fit-1-complete")

	o.NoFit = true
	src += "\n"
	settle(tt)
	if r, _ := tt.Find("Ship to production"); r.X < 520 {
		t.Errorf("with NoFit the diagram should keep its size: last node at x=%v", r.X)
	}
}

// overDiagram moves the pointer onto the diagram, which shows its
// controls.
func overDiagram(t *testing.T, tt *ui.Tester, label string) ui.Rect {
	t.Helper()
	r, ok := tt.Find(label)
	if !ok {
		t.Fatalf("label %q not found: %q", label, tt.Texts())
	}
	tt.Move(r.X+r.W/2, r.Y+r.H/2)
	tt.Frame()
	return r
}

func TestToolbar(t *testing.T) {
	var exported string
	src, o := small, Options{OnExport: func(svg string) { exported = svg }}
	tt := ui.NewTester(view(&src, &o), 520, 420)
	settle(tt)
	if tt.HasText("100%") {
		t.Errorf("the controls show without the pointer on the diagram")
	}
	before := overDiagram(t, tt, "Save record")
	if !tt.HasText("100%") || !tt.HasText("Copy source") {
		t.Fatalf("no controls under the pointer: %q", tt.Texts())
	}
	snapshot(t, tt, "toolbar")

	if err := tt.Click("Copy source"); err != nil {
		t.Fatal(err)
	}
	if got := tt.Clipboard(); got != small {
		t.Errorf("clipboard %q, want the source", got)
	}
	if !tt.HasText("Copied") {
		t.Errorf("no word that the source was copied: %q", tt.Texts())
	}
	tt.Advance(2 * time.Second)
	if tt.HasText("Copied") {
		t.Errorf("the word stays")
	}

	overDiagram(t, tt, "Save record")
	if err := tt.Click("SVG"); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(exported, "<svg") || !strings.Contains(exported, "Save record") {
		t.Errorf("exported %.60q", exported)
	}

	overDiagram(t, tt, "Save record")
	if err := tt.Click("Zoom in"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	tt.Frame()
	after, _ := tt.Find("Save record")
	if after.H <= before.H || !tt.HasText("125%") {
		t.Errorf("after zooming in the label is %v, was %v; texts %q", after, before, tt.Texts())
	}
	snapshot(t, tt, "zoomed")
	if err := tt.Click("Reset zoom"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	tt.Frame()
	if reset, _ := tt.Find("Save record"); reset.H != before.H {
		t.Errorf("after resetting the label is %v, was %v", reset, before)
	}
}

func TestNoToolbar(t *testing.T) {
	src, o := small, Options{NoToolbar: true}
	tt := ui.NewTester(view(&src, &o), 520, 420)
	settle(tt)
	overDiagram(t, tt, "Save record")
	if tt.HasText("100%") {
		t.Errorf("controls show with NoToolbar")
	}
}

// TestNodeClick checks that a click on a node is reported with its ID,
// whether on its label or beside it.
func TestNodeClick(t *testing.T) {
	var clicked []string
	src, o := small, Options{OnNodeClick: func(id string) { clicked = append(clicked, id) }, NoToolbar: true}
	tt := ui.NewTester(view(&src, &o), 520, 420)
	settle(tt)
	r := overDiagram(t, tt, "Save record")
	tt.ClickAt(r.X-6, r.Y+r.H/2) // inside the node, beside its label
	tt.Frame()
	tt.ClickAt(r.X+r.W/2, r.Y+r.H/2) // on its label
	tt.Frame()
	if len(clicked) != 2 || clicked[0] != "C" || clicked[1] != "C" {
		t.Errorf("clicks reported: %v, want [C C]", clicked)
	}
	if tt.Cursor() != ui.CursorPointer {
		t.Errorf("the pointer over a node that takes clicks should show a hand")
	}
	clicked = nil
	tt.ClickAt(500, 400) // nowhere
	tt.Frame()
	if len(clicked) != 0 {
		t.Errorf("a click outside nodes reported %v", clicked)
	}
}

// TestHoverHighlight checks that the edges of the node under the pointer
// change color.
func TestHoverHighlight(t *testing.T) {
	t.Run("plain", func(t *testing.T) { hoverHighlight(t, Options{NoToolbar: true}) })
	// Nodes that take clicks have boxes over them: the node under the
	// pointer must still be found.
	t.Run("clickable", func(t *testing.T) { hoverHighlight(t, Options{NoToolbar: true, OnNodeClick: func(string) {}}) })
}

func hoverHighlight(t *testing.T, o Options) {
	src := small
	tt := ui.NewTester(view(&src, &o), 520, 420)
	settle(tt)
	// A point on the edge from Start to Valid?: under Start's box, on its
	// axis.
	start, _ := tt.Find("Start")
	valid, _ := tt.Find("Valid?")
	x, y := int(start.X+start.W/2), int((start.Y+start.H+valid.Y)/2)-4
	plain := tt.Image().At(x, y)
	overDiagram(t, tt, "Start")
	lit := tt.Image().At(x, y)
	if sameColor(plain, lit) {
		t.Errorf("the edge under the hovered node did not change: %v", lit)
	}
	snapshot(t, tt, "hover")
	overDiagram(t, tt, "Show error")
	if dim := tt.Image().At(x, y); sameColor(dim, lit) || sameColor(dim, plain) {
		t.Errorf("an edge away from the hovered node should fade: %v (plain %v, lit %v)", dim, plain, lit)
	}
}
