package tree

import (
	"math"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"unicode/utf8"

	"github.com/momaek/streameraid/flow"
	"github.com/momaek/streameraid/geom"
	"github.com/momaek/streameraid/layout"
	"github.com/momaek/streameraid/measure"
	"github.com/momaek/streameraid/mindmap"
)

// corpus returns the mind maps of testdata/corpus, by file name.
func corpus(t testing.TB, dir string) map[string]string {
	t.Helper()
	files, _ := filepath.Glob("../../testdata/corpus/" + dir + "/mindmap-*.mmd")
	out := map[string]string{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		out[filepath.Base(f)] = string(b)
	}
	if len(out) == 0 {
		t.Fatal("no mind maps in testdata/corpus")
	}
	return out
}

func overlap(a, b geom.Rect, tol float64) bool {
	return a.X+tol < b.Right() && b.X+tol < a.Right() && a.Y+tol < b.Bottom() && b.Y+tol < a.Bottom()
}

// check verifies what makes a layout readable: every node and edge is
// there, nodes do not overlap and are inside the diagram, and edges end on
// the outlines of their nodes.
func check(t *testing.T, name string, g *flow.Graph, lo *layout.Flow) {
	t.Helper()
	if len(lo.Nodes) != len(g.NodeOrder) || len(lo.Edges) != len(g.Edges) {
		t.Errorf("%s: %d boxes for %d nodes, %d routes for %d edges", name, len(lo.Nodes), len(g.NodeOrder), len(lo.Edges), len(g.Edges))
	}
	for i, a := range lo.Nodes {
		if math.IsNaN(a.Rect.X) || math.IsNaN(a.Rect.Y) {
			t.Fatalf("%s: node %s at NaN", name, a.ID)
		}
		for _, b := range lo.Nodes[i+1:] {
			if overlap(a.Rect, b.Rect, 0.5) {
				t.Errorf("%s: with %d nodes, %s and %s overlap", name, len(lo.Nodes), a.ID, b.ID)
			}
		}
		if a.Rect.X < -0.5 || a.Rect.Y < -0.5 || a.Rect.Right() > lo.Width+0.5 || a.Rect.Bottom() > lo.Height+0.5 {
			t.Errorf("%s: node %s at %v is outside the %vx%v diagram", name, a.ID, a.Rect, lo.Width, lo.Height)
		}
	}
	for _, e := range lo.Edges {
		ge := g.Edges[e.Index]
		from, _ := lo.Node(ge.From)
		to, _ := lo.Node(ge.To)
		if len(e.Points) != 2 || len(e.Path.Ops) != 2 || e.Path.Ops[1].Kind != geom.CubicTo {
			t.Errorf("%s: edge %s is %d points, %d ops", name, e.ID, len(e.Points), len(e.Path.Ops))
			continue
		}
		if d := distToOutline(from.Outline, e.Points[0]); d > 1 {
			t.Errorf("%s: %s starts %.1f away from %s's outline", name, e.ID, d, ge.From)
		}
		if d := distToOutline(to.Outline, e.Points[1]); d > 1 {
			t.Errorf("%s: %s ends %.1f away from %s's outline", name, e.ID, d, ge.To)
		}
	}
}

func distToOutline(p geom.Path, q geom.Point) float64 {
	best := math.Inf(1)
	for _, poly := range p.Flatten() {
		for i := 1; i < len(poly); i++ {
			a, b := poly[i-1], poly[i]
			ab := b.Sub(a)
			t := 0.0
			if l := ab.X*ab.X + ab.Y*ab.Y; l > 0 {
				t = math.Max(0, math.Min(1, ((q.X-a.X)*ab.X+(q.Y-a.Y)*ab.Y)/l))
			}
			best = math.Min(best, q.Sub(a.Add(ab.Scale(t))).Len())
		}
	}
	return best
}

func TestCorpus(t *testing.T) {
	for _, dir := range []string{"ai", "mermaid-docs"} {
		for name, src := range corpus(t, dir) {
			g := mindmap.Parse(src, true)
			check(t, name, g, Layout(g, measure.Fixed{}, layout.Options{}))
		}
	}
}

func TestShape(t *testing.T) {
	g := mindmap.Parse("mindmap\n  root((Root))\n    A\n      A1\n      A2\n      A3\n    B\n    C\n      C1\n    D\n", true)
	lo := Layout(g, measure.Fixed{}, layout.Options{})
	box := func(id string) geom.Rect {
		b, ok := lo.Node(id)
		if !ok {
			t.Fatalf("no box for %s", id)
		}
		return b.Rect
	}
	root := box("n0")
	// Six leaves: A's three on the right, B, C's and D on the left.
	for id, right := range map[string]bool{"n0.0": true, "n0.1": false, "n0.2": false, "n0.3": false, "n0.0.1": true, "n0.2.0": false} {
		if r := box(id); (r.X > root.Right()) != right || (r.Right() < root.X) == right {
			t.Errorf("%s at %v, root at %v: want right %v", id, r, root, right)
		}
	}
	// Children are stacked in order, the parent centered on them, a level
	// gap away.
	a, a0, a1, a2 := box("n0.0"), box("n0.0.0"), box("n0.0.1"), box("n0.0.2")
	if !(a0.Bottom() < a1.Y && a1.Bottom() < a2.Y) {
		t.Errorf("A's children out of order: %v %v %v", a0, a1, a2)
	}
	if d := a.Center().Y - (a0.Center().Y+a2.Center().Y)/2; math.Abs(d) > 0.01 {
		t.Errorf("A is %.1f off the middle of its children", d)
	}
	if d := a0.X - a.Right(); math.Abs(d-3*14) > 0.01 {
		t.Errorf("A's children are %.1f from it", d)
	}
	// The left side grows leftward, its nodes' right sides toward the root.
	c, c0 := box("n0.2"), box("n0.2.0")
	if d := c.X - c0.Right(); math.Abs(d-3*14) > 0.01 {
		t.Errorf("C's child is %.1f from it", d)
	}
	// Each side is centered on the root.
	if d := root.Center().Y - a.Center().Y; math.Abs(d) > 0.01 {
		t.Errorf("the right side is %.1f off the root", d)
	}
	if d := root.Center().Y - (box("n0.1").Center().Y+box("n0.3").Center().Y)/2; math.Abs(d) > 0.01 {
		t.Errorf("the left side is %.1f off the root", d)
	}
	if b, c := box("n0.1"), box("n0.2"); b.Bottom() >= c.Y {
		t.Errorf("the left side is not in order top to bottom: B at %v, C at %v", b, c)
	}
	check(t, "shape", g, lo)
}

func TestNotATree(t *testing.T) {
	// Two roots, a node with two parents, a cycle: all drawn, nothing
	// overlapping.
	g := flow.Parse("flowchart LR\nA --> B & C\nD --> B\nC --> E --> C\nF\n", true)
	lo := Layout(g, measure.Fixed{}, layout.Options{})
	check(t, "not a tree", g, lo)
	if lo := Layout(flow.NewGraph(), measure.Fixed{}, layout.Options{}); len(lo.Nodes) != 0 || lo.Width <= 0 {
		t.Errorf("empty graph: %+v", lo)
	}
}

func TestHintsKeepSides(t *testing.T) {
	src := "mindmap\n  root\n    A\n    B\n    C\n"
	g := mindmap.Parse(src, true)
	prev := Layout(g, measure.Fixed{}, layout.Options{})
	side := func(lo *layout.Flow, id string) int {
		r, _ := lo.Node("n0")
		b, _ := lo.Node(id)
		if b.Rect.Center().X > r.Rect.Center().X {
			return 1
		}
		return -1
	}
	// A grows a lot: laid out afresh, B would go to the left.
	more := mindmap.Parse(src[:len(src)-len("    B\n    C\n")]+"      A1\n      A2\n      A3\n    B\n    C\n    D\n", true)
	fresh := Layout(more, measure.Fixed{}, layout.Options{})
	if side(fresh, "n0.1") != -1 {
		t.Fatal("B is not on the left laid out afresh")
	}
	for _, anchor := range []bool{false, true} {
		lo := LayoutWith(more, measure.Fixed{}, layout.Options{}, Hints{Prev: prev, Anchor: anchor})
		for _, id := range []string{"n0.0", "n0.1", "n0.2"} {
			if side(lo, id) != side(prev, id) {
				t.Errorf("anchor %v: %s changed sides", anchor, id)
			}
		}
		// D is new: it goes to the side with fewer leaves, the left.
		if side(lo, "n0.3") != -1 {
			t.Errorf("anchor %v: D is not on the left", anchor)
		}
		check(t, "hints", more, lo)
	}
}

// motion measures how much the nodes that are already there move as a
// diagram streams in.
type motion struct {
	steps  int     // changes of the graph
	pairs  int     // (node, step) pairs: a node there before and after a step
	moved  int     // pairs that moved more than a pixel
	total  float64 // distance moved, summed over pairs
	worst  float64
	shaken int // steps in which more than half of the nodes moved
}

func (m motion) mean() float64  { return m.total / math.Max(1, float64(m.pairs)) }
func (m motion) share() float64 { return float64(m.moved) / math.Max(1, float64(m.pairs)) }

func (m *motion) add(o motion) {
	m.steps += o.steps
	m.pairs += o.pairs
	m.moved += o.moved
	m.total += o.total
	m.worst = math.Max(m.worst, o.worst)
	m.shaken += o.shaken
}

// stream feeds src a character at a time, laying out after each change of
// the graph with hints from the layout before, and measures the motion of
// the nodes that kept their size: a node whose label is being written
// grows, which is not the motion this guards against. It calls check with
// each layout.
func stream(src string, hints func(prev *layout.Flow) Hints, check func(g *flow.Graph, lo *layout.Flow)) motion {
	var m motion
	var prevG *flow.Graph
	var prev *layout.Flow
	for end := 1; end <= len(src); end++ {
		if end < len(src) && !utf8.RuneStart(src[end]) {
			continue // streams deliver whole characters
		}
		final := end == len(src)
		g := mindmap.Parse(src[:end], final)
		if !final && flow.Compare(prevG, g).Empty() {
			continue
		}
		h := Hints{}
		if prev != nil {
			h = hints(prev)
		}
		if final {
			h.Anchor = false
		}
		lo := LayoutWith(g, measure.Fixed{}, layout.Options{}, h)
		if check != nil {
			check(g, lo)
		}
		if prev != nil && !final {
			was := map[string]geom.Rect{}
			for _, n := range prev.Nodes {
				was[n.ID] = n.Rect
			}
			count, moved := 0, 0
			for _, n := range lo.Nodes {
				r, ok := was[n.ID]
				if !ok || math.Abs(r.W-n.Rect.W) > 0.01 || math.Abs(r.H-n.Rect.H) > 0.01 {
					continue
				}
				d := n.Rect.Center().Sub(r.Center()).Len()
				count++
				m.pairs++
				m.total += d
				m.worst = math.Max(m.worst, d)
				if d > 1 {
					moved++
					m.moved++
				}
			}
			m.steps++
			if count > 1 && moved*2 > count {
				m.shaken++
			}
		}
		prevG, prev = g, lo
	}
	return m
}

// TestStreamingStability streams the AI-style samples and compares laying
// out afresh at every step with following the layout before. Following
// with Anchor must move far fewer of the nodes that are there, and stay
// within the limits below, which guard against regressions.
func TestStreamingStability(t *testing.T) {
	var fresh, ordered, anchored motion
	names := []string{}
	per := map[string][3]motion{}
	for name, src := range corpus(t, "ai") {
		f := stream(src, func(*layout.Flow) Hints { return Hints{} }, nil)
		o := stream(src, func(p *layout.Flow) Hints { return Hints{Prev: p} }, nil)
		a := stream(src, func(p *layout.Flow) Hints { return Hints{Prev: p, Anchor: true} }, nil)
		fresh.add(f)
		ordered.add(o)
		anchored.add(a)
		per[name] = [3]motion{f, o, a}
		names = append(names, name)
	}
	sort.Strings(names)
	for _, n := range names {
		p := per[n]
		t.Logf("%-32s fresh %5.1fpx %3.0f%%  ordered %5.1fpx %3.0f%%  anchored %5.1fpx %3.0f%% (worst %4.0f, shaken %d/%d)",
			n, p[0].mean(), 100*p[0].share(), p[1].mean(), 100*p[1].share(), p[2].mean(), 100*p[2].share(), p[2].worst, p[2].shaken, p[2].steps)
	}
	report := func(name string, m motion) {
		t.Logf("%-9s mean %.2fpx per node per step, %.1f%% of nodes moved, %d of %d steps moved most nodes",
			name, m.mean(), 100*m.share(), m.shaken, m.steps)
	}
	report("fresh", fresh)
	report("ordered", ordered)
	report("anchored", anchored)
	if a, f := anchored.share(), fresh.share(); a > 0.3*f {
		t.Errorf("anchoring moves %.1f%% of nodes a step, laying out afresh %.1f%%: it should move far fewer", 100*a, 100*f)
	}
	if got := anchored.mean(); got > maxMeanMotion {
		t.Errorf("anchored layouts move nodes %.2fpx per step on average, over the limit of %.2f", got, maxMeanMotion)
	}
	if got := anchored.share(); got > maxMovedShare {
		t.Errorf("%.1f%% of nodes move per step, over the limit of %.1f%%", 100*got, 100*maxMovedShare)
	}
	if got := float64(anchored.shaken) / float64(anchored.steps); got > maxShakenShare {
		t.Errorf("%.1f%% of steps move most nodes, over the limit of %.1f%%", 100*got, 100*maxShakenShare)
	}
}

// Limits for TestStreamingStability, set a little above what the layout
// achieves on the corpus (4.12px, 1.7% and 1.7% when they were set,
// against 8.82px, 25.3% and 19.5% laying out afresh): tighten them when it
// improves. What still moves is almost all the whole diagram moving right
// when its left side grows past the edge of the page, once or twice a
// sample.
const (
	maxMeanMotion  = 5.0
	maxMovedShare  = 0.025
	maxShakenShare = 0.025
)

// TestStreamingInvariants checks that every layout on the way holds: no
// overlaps, edges on outlines, everything on the page.
func TestStreamingInvariants(t *testing.T) {
	for name, src := range corpus(t, "ai") {
		for _, anchor := range []bool{false, true} {
			stream(src, func(p *layout.Flow) Hints { return Hints{Prev: p, Anchor: anchor} }, func(g *flow.Graph, lo *layout.Flow) {
				check(t, name, g, lo)
			})
		}
	}
}

func BenchmarkLayout(b *testing.B) {
	src := corpus(b, "ai")["mindmap-001-ml.mmd"]
	g := mindmap.Parse(src, true)
	for b.Loop() {
		Layout(g, measure.Fixed{}, layout.Options{})
	}
}

func FuzzLayout(f *testing.F) {
	f.Add("mindmap\n  root((a))\n    b[c]\n      d)e(\n    f\n  g\n")
	f.Fuzz(func(t *testing.T, src string) {
		g := mindmap.Parse(src, false)
		lo := Layout(g, measure.Fixed{}, layout.Options{})
		LayoutWith(mindmap.Parse(src, true), measure.Fixed{}, layout.Options{}, Hints{Prev: lo, Anchor: true})
	})
}
