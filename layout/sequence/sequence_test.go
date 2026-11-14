package sequence

import (
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"unicode/utf8"

	"github.com/momaek/streameraid/geom"
	"github.com/momaek/streameraid/layout"
	"github.com/momaek/streameraid/measure"
	"github.com/momaek/streameraid/seq"
)

func corpus(t testing.TB) map[string]string {
	t.Helper()
	files, _ := filepath.Glob("../../testdata/corpus/*/*.mmd")
	out := map[string]string{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if d := seq.Parse(string(b), true); d.HasHeader {
			out[filepath.Base(f)] = string(b)
		}
	}
	if len(out) == 0 {
		t.Fatal("no sequence diagrams in testdata/corpus")
	}
	return out
}

func finite(vs ...float64) bool {
	for _, v := range vs {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return true
}

// check verifies what makes a layout sound: everything is inside the
// diagram, lifelines go left to right without their heads overlapping,
// and messages go down the page in order.
func check(t *testing.T, name string, d *seq.Diagram, s *layout.Sequence) {
	t.Helper()
	all := geom.Rect{X: -0.5, Y: -0.5, W: s.Width + 1, H: s.Height + 1}
	within := func(what string, r geom.Rect) {
		if !finite(r.X, r.Y, r.W, r.H) {
			t.Errorf("%s: %s is not finite: %v", name, what, r)
			return
		}
		if r.X < all.X || r.Y < all.Y || r.Right() > all.Right() || r.Bottom() > all.Bottom() {
			t.Errorf("%s: %s %v is outside the %vx%v diagram", name, what, r, s.Width, s.Height)
		}
	}
	shown := 0
	for _, p := range d.Participants {
		if p.Created != seq.NotYetCreated {
			shown++
		}
	}
	if len(s.Participants) != shown {
		t.Errorf("%s: %d of %d participants placed", name, len(s.Participants), shown)
	}
	for i, p := range s.Participants {
		within("participant "+p.ID, p.Rect)
		if p.LifeBottom.Y < p.LifeTop.Y {
			t.Errorf("%s: %s's lifeline ends above its start", name, p.ID)
		}
		if i > 0 {
			if prev := s.Participants[i-1]; prev.Rect.Right() > p.Rect.X+0.5 && prev.Rect.Bottom() > p.Rect.Y && p.Rect.Bottom() > prev.Rect.Y {
				t.Errorf("%s: the heads of %s and %s overlap", name, prev.ID, p.ID)
			}
		}
	}
	messages := 0
	for _, e := range d.Events {
		if e.Kind == seq.Message {
			messages++
		}
	}
	if len(s.Messages) != messages {
		t.Errorf("%s: %d of %d messages placed", name, len(s.Messages), messages)
	}
	last := math.Inf(-1)
	for _, m := range s.Messages {
		within("message "+m.ID+"'s text", geom.RectCentered(m.LabelAt, m.Label.W, m.Label.H))
		if m.LabelAt.Y < last {
			t.Errorf("%s: message %s is above the one before it", name, m.ID)
		}
		last = m.LabelAt.Y
	}
	for _, n := range s.Notes {
		within("note "+n.ID, n.Rect)
	}
	for _, b := range s.Blocks {
		if b.ID != "" {
			within("block "+b.ID, b.Rect)
		}
	}
	for _, a := range s.Activations {
		within("activation "+a.ID, a.Rect)
	}
	for _, g := range s.Groups {
		within("group "+g.ID, g.Rect)
	}
}

func TestCorpusInvariants(t *testing.T) {
	for name, src := range corpus(t) {
		d := seq.Parse(src, true)
		check(t, name, d, Layout(d, measure.Fixed{}, layout.Options{}))
	}
}

func TestDeterministic(t *testing.T) {
	for name, src := range corpus(t) {
		a := Layout(seq.Parse(src, true), measure.Fixed{}, layout.Options{})
		b := Layout(seq.Parse(src, true), measure.Fixed{}, layout.Options{})
		if !reflect.DeepEqual(a, b) {
			t.Errorf("%s: two layouts differ", name)
		}
	}
}

// TestStreaming streams every sample a character at a time. Every layout
// on the way must be sound, and what is there must hold still: a message
// never moves up or down once it shows, and a lifeline only ever moves to
// the right, to make room.
func TestStreaming(t *testing.T) {
	steps, shifted := 0, 0
	for name, src := range corpus(t) {
		tops := map[string]float64{}
		xs := map[string]float64{}
		var prev *layout.Sequence
		for end := 1; end <= len(src); end++ {
			if end < len(src) && !utf8.RuneStart(src[end]) {
				continue
			}
			d := seq.Parse(src[:end], false)
			if !d.HasHeader {
				continue
			}
			s := LayoutWith(d, measure.Fixed{}, layout.Options{}, Hints{Prev: prev, Anchor: true})
			prev = s
			check(t, name, d, s)
			for _, m := range s.Messages {
				top := m.LabelAt.Y - m.Label.H/2
				if was, ok := tops[m.ID]; ok && math.Abs(was-top) > 0.01 {
					t.Errorf("%s: message %s moved from y=%.1f to %.1f", name, m.ID, was, top)
				}
				tops[m.ID] = top
			}
			moved := false
			for _, p := range s.Participants {
				x := p.LifeTop.X
				if was, ok := xs[p.ID]; ok {
					if x < was-0.01 {
						t.Errorf("%s: %s's lifeline moved left, from x=%.1f to %.1f", name, p.ID, was, x)
					}
					moved = moved || x > was+0.01
				}
				xs[p.ID] = x
			}
			steps++
			if moved {
				shifted++
			}
		}
	}
	t.Logf("%d of %d steps (%.1f%%) moved a lifeline", shifted, steps, 100*float64(shifted)/float64(steps))
	if float64(shifted) > 0.2*float64(steps) {
		t.Errorf("%d of %d steps moved a lifeline: too many", shifted, steps)
	}
}

func TestActivationsAndSelfMessages(t *testing.T) {
	d := seq.Parse("sequenceDiagram\n    A->>+B: go\n    B->>B: think\n    B-->>-A: done\n", true)
	s := Layout(d, measure.Fixed{}, layout.Options{})
	if len(s.Activations) != 1 {
		t.Fatalf("%d activations", len(s.Activations))
	}
	act := s.Activations[0]
	b := s.Participants[1]
	if math.Abs(act.Rect.Center().X-b.LifeTop.X) > 0.01 {
		t.Errorf("the activation at x=%.1f is not on B's lifeline at %.1f", act.Rect.Center().X, b.LifeTop.X)
	}
	first, last := s.Messages[0], s.Messages[2]
	if act.Rect.Y > first.LabelAt.Y+first.Label.H || act.Rect.Bottom() < last.LabelAt.Y {
		t.Errorf("the activation %v does not span from the first message to the last", act.Rect)
	}
	self := s.Messages[1]
	if self.LabelAt.X < b.LifeTop.X {
		t.Errorf("a message to oneself should show to the right of the lifeline")
	}
}

func BenchmarkLayout(b *testing.B) {
	src, err := os.ReadFile("../../testdata/corpus/ai/seq-009-microservices.mmd")
	if err != nil {
		b.Fatal(err)
	}
	for b.Loop() {
		Layout(seq.Parse(string(src), true), measure.Fixed{}, layout.Options{})
	}
}
