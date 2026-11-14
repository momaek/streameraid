package mermaid

import (
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"github.com/momaek/streameraid/geom"
)

const reply = "Here is the flow:\n\n```mermaid\nflowchart TD\n    A[Start] --> B{Valid?}\n    B -->|yes| C[Save record]\n    B -->|no| D[Show error]\n    D --> A\n    subgraph Storage\n        C --> E[(Database)]\n    end\n```\n\nThat is all.\n"

func hooked() *markdown.Options {
	return &markdown.Options{Code: map[string]func(*ui.Context, *markdown.CodeBlockView){"mermaid": CodeBlock}}
}

// snapshot saves the tester's last frame when STREAMERAID_SNAPSHOTS names
// a directory, to look at.
func snapshot(t *testing.T, tt *ui.Tester, name string) {
	dir := os.Getenv("STREAMERAID_SNAPSHOTS")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, tt.Image()); err != nil {
		t.Fatal(err)
	}
}

// TestMarkdownHook checks that a mermaid block shows as a diagram whose
// labels are text, not as code.
func TestMarkdownHook(t *testing.T) {
	for _, dark := range []bool{false, true} {
		md := hooked()
		tt := ui.NewTester(func(c *ui.Context) {
			ui.Scroll(c).Fill().Padding(16).Children(func() { markdown.View(c, reply, md) })
		}, 560, 720)
		tt.SetDark(dark)
		tt.Frame()
		for _, s := range []string{"Start", "Valid?", "Save record", "Show error", "Database", "yes", "no", "Storage"} {
			if !tt.HasText(s) {
				t.Errorf("dark=%v: label %q missing; texts %q", dark, s, tt.Texts())
			}
		}
		if tt.HasText("flowchart TD") {
			t.Errorf("the block shows as code")
		}
		name := "hook-light"
		if dark {
			name = "hook-dark"
		}
		snapshot(t, tt, name)
	}
}

// TestStreaming shows the reply as it streams in, a few bytes per frame,
// and checks the diagram is there at the end.
func TestStreaming(t *testing.T) {
	src := ""
	md := hooked()
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Scroll(c).Fill().Padding(16).Children(func() { markdown.View(c, src, md) })
	}, 560, 720)
	// Three bytes every 30 ms, so that each change comes while the
	// transition of the one before is still on its way.
	shots := 0
	for i := 0; i <= len(reply); i += 3 {
		src = reply[:i]
		tt.Frame()
		tt.Advance(30 * time.Millisecond)
		if i*5/len(reply) > shots {
			shots++
			snapshot(t, tt, "streaming-"+string(rune('0'+shots)))
		}
	}
	src = reply
	tt.Frame()
	tt.Advance(time.Second)
	snapshot(t, tt, "streaming-6-end")
	if !tt.HasText("Database") || tt.HasText("flowchart TD") {
		t.Errorf("texts at the end: %q", tt.Texts())
	}
}

// TestSelectLabel selects a node's label with the pointer and copies it.
func TestSelectLabel(t *testing.T) {
	md := hooked()
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Scroll(c).Fill().Padding(16).Children(func() { markdown.View(c, reply, md) })
	}, 560, 720)
	tt.Frame()
	r, ok := tt.Find("Save record")
	if !ok {
		t.Fatal("label not found")
	}
	y := r.Y + r.H/2
	tt.Press(r.X+1, y)
	tt.Move(r.X+r.W/2, y)
	tt.Move(r.X+r.W-1, y)
	tt.Release(r.X+r.W-1, y)
	tt.Key(ui.Cmd, ui.KeyC)
	if got := tt.Clipboard(); !strings.Contains(got, "Save record") && !strings.Contains(got, "ave recor") {
		t.Errorf("clipboard %q after selecting the label", got)
	}
}

func TestUnsupportedShowsCode(t *testing.T) {
	src := "```mermaid\npie\n    \"Dogs\" : 3\n```\n"
	md := hooked()
	tt := ui.NewTester(func(c *ui.Context) { markdown.View(c, src, md) }, 400, 300)
	tt.Frame()
	found := false
	for _, s := range tt.Texts() {
		if strings.Contains(s, "Dogs") {
			found = true
		}
	}
	if !found {
		t.Errorf("an unsupported diagram should show as code; texts %q", tt.Texts())
	}
}

func TestProblemsReported(t *testing.T) {
	src := "```mermaid\nflowchart TD\n  A --> B\n  A -->> ???\n```\n"
	md := &markdown.Options{Code: map[string]func(*ui.Context, *markdown.CodeBlockView){"mermaid": CodeBlockWith(Options{Labels: ChineseLabels})}}
	tt := ui.NewTester(func(c *ui.Context) { markdown.View(c, src, md) }, 400, 400)
	tt.Frame()
	if !tt.HasText("有 1 条语句没能画出来") {
		t.Fatalf("texts %q", tt.Texts())
	}
	if err := tt.Click("查看源码"); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range tt.Texts() {
		if strings.Contains(s, "A -->> ???") {
			found = true
		}
	}
	if !found {
		t.Errorf("source not shown: %q", tt.Texts())
	}
}

// below returns the top of the text under the diagram, which follows the
// diagram's height.
func below(t *testing.T, tt *ui.Tester) float32 {
	t.Helper()
	r, ok := tt.Find("below the diagram")
	if !ok {
		t.Fatalf("texts %q", tt.Texts())
	}
	return r.Y
}

// inkBeside returns the color just left of a label, inside its node's
// box: the node's fill, or what is behind it while the node fades in.
func inkBeside(t *testing.T, tt *ui.Tester, label string) color.Color {
	t.Helper()
	r, ok := tt.Find(label)
	if !ok {
		t.Fatalf("label %q not found: %q", label, tt.Texts())
	}
	return tt.Image().At(int(r.X-6), int(r.Y+r.H/2))
}

func sameColor(a, b color.Color) bool {
	r1, g1, b1, _ := a.RGBA()
	r2, g2, b2, _ := b.RGBA()
	return r1 == r2 && g1 == g2 && b1 == b2
}

// TestMotion checks that a change of the diagram is shown as a transition:
// the diagram makes room at once, the new node is there from the start but
// fades in over the transition's duration, what was there stays where it
// was, and nothing changes once the transition is over.
func TestMotion(t *testing.T) {
	src := "flowchart TD\n    A[Start] --> B[Next step]\n"
	streaming := true
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Padding(16).Children(func() {
			View(c, src, Options{Streaming: streaming})
			ui.Text(c, "below the diagram")
		})
	}, 480, 480)
	tt.Frame()
	tt.Advance(time.Second)
	before := below(t, tt)
	held, _ := tt.Find("Next step")
	settled := inkBeside(t, tt, "Next step")
	snapshot(t, tt, "motion-0-before")

	src += "    B --> C[Another step]\n    C --> D{Done?}\n"
	tt.Frame()
	if y := below(t, tt); y <= before {
		t.Errorf("the diagram should make room for what is new at once: the text below is at %v, was at %v", y, before)
	}
	if r, _ := tt.Find("Next step"); r != held {
		t.Errorf("a node that was there moved from %v to %v", held, r)
	}
	start := inkBeside(t, tt, "Another step")
	if sameColor(start, settled) {
		t.Errorf("the new node shows in full at once: it should fade in")
	}
	snapshot(t, tt, "motion-1-start")
	tt.Advance(motionDuration / 3)
	snapshot(t, tt, "motion-2-third")
	tt.Advance(motionDuration / 3)
	mid := inkBeside(t, tt, "Another step")
	if sameColor(mid, start) || sameColor(mid, settled) {
		t.Errorf("two thirds through, the new node should be partly in: %v (from %v to %v)", mid, start, settled)
	}
	snapshot(t, tt, "motion-3-two-thirds")
	tt.Advance(time.Second)
	if end := inkBeside(t, tt, "Another step"); !sameColor(end, settled) {
		t.Errorf("after the transition the new node is %v, the others %v", end, settled)
	}
	after := below(t, tt)
	snapshot(t, tt, "motion-4-after")
	tt.Advance(time.Second)
	if y := below(t, tt); y != after {
		t.Errorf("still moving after the transition: %v then %v", after, y)
	}

	// Complete, the diagram is balanced anew, and everything is still there.
	streaming = false
	tt.Frame()
	tt.Advance(time.Second)
	for _, s := range []string{"Start", "Next step", "Another step", "Done?"} {
		if !tt.HasText(s) {
			t.Errorf("label %q lost: %q", s, tt.Texts())
		}
	}
	snapshot(t, tt, "motion-5-final")
}

// TestReduceMotion checks that changes show at once when the desktop asks
// for less motion.
func TestReduceMotion(t *testing.T) {
	src := "flowchart TD\n    A[Start] --> B[Next step]\n"
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Padding(16).Children(func() {
			View(c, src, Options{Streaming: true})
			ui.Text(c, "below the diagram")
		})
	}, 480, 480)
	// The tester drew a first frame before the preference was set: let
	// that transition end.
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tt.Advance(time.Second)
	settled := inkBeside(t, tt, "Next step")
	src += "    B --> C[Another step]\n"
	tt.Frame()
	if got := inkBeside(t, tt, "Another step"); !sameColor(got, settled) {
		t.Errorf("with reduced motion the new node should show in full at once: %v, the others %v", got, settled)
	}
}

// TestTransitionFrames checks the frames between two layouts.
func TestTransitionFrames(t *testing.T) {
	a := &frame{w: 100, h: 100, nodes: map[string]nodeFrame{"A": {rect: geom.Rect{X: 0, Y: 0, W: 10, H: 10}, alpha: 1}}}
	b := &frame{w: 200, h: 100, nodes: map[string]nodeFrame{
		"A": {rect: geom.Rect{X: 100, Y: 0, W: 10, H: 10}, alpha: 1},
		"B": {rect: geom.Rect{X: 50, Y: 50, W: 10, H: 10}, alpha: 1},
	}}
	start := time.Unix(0, 0)
	tr := &transition{from: a, to: b, start: start, dur: time.Second}
	if f := tr.at(start); f.nodes["A"].rect.X != 0 || f.nodes["B"].alpha != 0 || f.w != 200 {
		t.Errorf("at the start: %+v", f)
	}
	mid := tr.at(start.Add(500 * time.Millisecond))
	if x := mid.nodes["A"].rect.X; x <= 50 || x >= 100 {
		t.Errorf("half way A is at %v: it should be past the middle, easing out", x)
	}
	if al := mid.nodes["B"].alpha; al <= 0 || al >= 1 {
		t.Errorf("half way B's alpha is %v", al)
	}
	if !tr.done(start.Add(time.Second)) || tr.at(start.Add(time.Second)) != b {
		t.Errorf("the end should be the target frame")
	}
}

const seqReply = "The login flow:\n\n```mermaid\nsequenceDiagram\n    autonumber\n    actor U as User\n    participant F as Frontend\n    participant B as Backend\n    U->>F: Enter password\n    F->>+B: POST /login\n    Note right of B: Check the hash\n    alt valid\n        B-->>F: 200 OK\n    else invalid\n        B-->>-F: 401 Unauthorized\n    end\n    F-->>U: Show result\n```\n\nDone.\n"

// TestSequenceDiagram checks that a sequence diagram shows natively, with
// its texts as text, in both themes.
func TestSequenceDiagram(t *testing.T) {
	for _, dark := range []bool{false, true} {
		md := hooked()
		tt := ui.NewTester(func(c *ui.Context) {
			ui.Scroll(c).Fill().Padding(16).Children(func() { markdown.View(c, seqReply, md) })
		}, 560, 640)
		tt.SetDark(dark)
		tt.Frame()
		tt.Advance(time.Second)
		for _, s := range []string{"User", "Frontend", "Backend", "Enter password", "POST /login", "Check the hash", "alt", "[valid]", "[invalid]", "401 Unauthorized", "Show result", "1", "5"} {
			if !tt.HasText(s) {
				t.Errorf("dark=%v: text %q missing; texts %q", dark, s, tt.Texts())
			}
		}
		if tt.HasText("sequenceDiagram") {
			t.Errorf("the block shows as code")
		}
		name := "seq-light"
		if dark {
			name = "seq-dark"
		}
		snapshot(t, tt, name)
	}
}

// TestSequenceStreaming streams a sequence diagram against a moving clock:
// what shows holds its place while more comes in under it.
func TestSequenceStreaming(t *testing.T) {
	src := ""
	md := hooked()
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Scroll(c).Fill().Padding(16).Children(func() { markdown.View(c, src, md) })
	}, 560, 640)
	var first ui.Rect
	seen := false
	shots := 0
	for i := 0; i <= len(seqReply); i += 3 {
		src = seqReply[:i]
		tt.Frame()
		tt.Advance(30 * time.Millisecond)
		if r, ok := tt.Find("Enter password"); ok {
			if d := r.Y - first.Y; seen && (d > 0.5 || d < -0.5) {
				t.Fatalf("the first message moved from y=%v to %v at byte %d", first.Y, r.Y, i)
			}
			if !seen {
				// Its line may still be growing: take its place once the next
				// line has started.
				if _, next := tt.Find("POST /login"); next {
					first, seen = r, true
				}
			}
		}
		if i*5/len(seqReply) > shots {
			shots++
			snapshot(t, tt, "seq-streaming-"+string(rune('0'+shots)))
		}
	}
	src = seqReply
	tt.Frame()
	tt.Advance(time.Second)
	if !seen || !tt.HasText("Show result") {
		t.Errorf("texts at the end: %q", tt.Texts())
	}
	snapshot(t, tt, "seq-streaming-6-end")
}
