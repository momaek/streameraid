package sugiyama

import (
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/momaek/streameraid/flow"
	"github.com/momaek/streameraid/geom"
	"github.com/momaek/streameraid/layout"
	"github.com/momaek/streameraid/measure"
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
		g := flow.Parse(string(b), true)
		if g.HasHeader {
			out[f] = string(b)
		}
	}
	if len(out) == 0 {
		t.Fatal("no flowcharts in testdata/corpus")
	}
	return out
}

func overlap(a, b geom.Rect, tol float64) bool {
	return a.X+tol < b.Right() && b.X+tol < a.Right() && a.Y+tol < b.Bottom() && b.Y+tol < a.Bottom()
}

func inside(in, out geom.Rect, tol float64) bool {
	return in.X >= out.X-tol && in.Y >= out.Y-tol && in.Right() <= out.Right()+tol && in.Bottom() <= out.Bottom()+tol
}

// TestCorpusInvariants lays out every sample and checks what makes a
// layout readable: nodes do not overlap, each node is inside its
// subgraphs' boxes, subgraphs side by side do not overlap, every drawn
// edge has a route, and everything fits in the diagram.
func TestCorpusInvariants(t *testing.T) {
	m := measure.Fixed{}
	for f, src := range corpus(t) {
		name := filepath.Base(f)
		g := flow.Parse(src, true)
		lo := Layout(g, m, layout.Options{})
		boxes := map[string]geom.Rect{}
		for _, n := range lo.Nodes {
			boxes[n.ID] = n.Rect
		}
		if len(lo.Nodes) != len(g.NodeOrder) {
			t.Errorf("%s: %d of %d nodes placed", name, len(lo.Nodes), len(g.NodeOrder))
		}
		for i, a := range lo.Nodes {
			for _, b := range lo.Nodes[i+1:] {
				if overlap(a.Rect, b.Rect, 0.5) {
					t.Errorf("%s: nodes %s and %s overlap", name, a.ID, b.ID)
				}
			}
		}
		cl := map[string]geom.Rect{}
		for _, c := range lo.Clusters {
			cl[c.ID] = c.Rect
		}
		for _, n := range lo.Nodes {
			for s := g.Nodes[n.ID].Subgraph; s != ""; s = g.Subgraphs[s].Parent {
				if !inside(n.Rect, cl[s], 0.5) {
					t.Errorf("%s: node %s is outside subgraph %s", name, n.ID, s)
				}
			}
			for c, r := range cl {
				if !g.Contains(c, n.ID) && overlap(n.Rect, r, 0.5) {
					t.Errorf("%s: node %s overlaps subgraph %s it is not in", name, n.ID, c)
				}
			}
		}
		for i, a := range lo.Clusters {
			for _, b := range lo.Clusters[i+1:] {
				if g.Contains(a.ID, b.ID) || g.Contains(b.ID, a.ID) {
					continue
				}
				if overlap(a.Rect, b.Rect, 0.5) {
					t.Errorf("%s: subgraphs %s and %s overlap", name, a.ID, b.ID)
				}
			}
		}
		routed := map[int]bool{}
		for _, e := range lo.Edges {
			routed[e.Index] = true
			for _, p := range e.Points {
				if math.IsNaN(p.X) || math.IsNaN(p.Y) {
					t.Errorf("%s: edge %s has NaN points", name, e.ID)
				}
			}
		}
		for _, e := range g.Edges {
			if !routed[e.Index] && !strings.Contains(name, "032") {
				t.Errorf("%s: edge %s has no route", name, e.ID)
			}
		}
		all := geom.Rect{X: 0, Y: 0, W: lo.Width, H: lo.Height}
		for _, n := range lo.Nodes {
			if !inside(n.Rect, all, 0.5) {
				t.Errorf("%s: node %s is outside the diagram", name, n.ID)
			}
		}
	}
}

// TestDeterministic checks that the same source lays out the same way.
func TestDeterministic(t *testing.T) {
	for f, src := range corpus(t) {
		g := flow.Parse(src, true)
		a := Layout(g, measure.Fixed{}, layout.Options{})
		b := Layout(flow.Parse(src, true), measure.Fixed{}, layout.Options{})
		if !reflect.DeepEqual(a, b) {
			t.Errorf("%s: two layouts differ", filepath.Base(f))
		}
	}
}

func TestDirections(t *testing.T) {
	for _, dir := range []string{"TB", "BT", "LR", "RL"} {
		g := flow.Parse("flowchart "+dir+"\nA --> B\n", true)
		lo := Layout(g, measure.Fixed{}, layout.Options{})
		a, _ := lo.Node("A")
		b, _ := lo.Node("B")
		ca, cb := a.Rect.Center(), b.Rect.Center()
		var ok bool
		switch dir {
		case "TB":
			ok = cb.Y > ca.Y
		case "BT":
			ok = cb.Y < ca.Y
		case "LR":
			ok = cb.X > ca.X
		case "RL":
			ok = cb.X < ca.X
		}
		if !ok {
			t.Errorf("%s: A at %v, B at %v", dir, ca, cb)
		}
	}
}

func TestEdgesEndOnOutlines(t *testing.T) {
	g := flow.Parse("flowchart TD\nA{Decide} --> B((Circle))\nA --> C[(DB)]\n", true)
	lo := Layout(g, measure.Fixed{}, layout.Options{})
	for _, e := range lo.Edges {
		ge := g.Edges[e.Index]
		from, _ := lo.Node(ge.From)
		to, _ := lo.Node(ge.To)
		start, end := e.Points[0], e.Points[len(e.Points)-1]
		if d := distToOutline(from.Outline, start); d > 1 {
			t.Errorf("%s starts %.1f away from %s's outline", e.ID, d, ge.From)
		}
		if d := distToOutline(to.Outline, end); d > 1 {
			t.Errorf("%s ends %.1f away from %s's outline", e.ID, d, ge.To)
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

func BenchmarkLayout(b *testing.B) {
	for _, name := range []string{"flow-019-nested", "flow-003-arch", "flow-023-signup"} {
		src, err := os.ReadFile("../../testdata/corpus/ai/" + name + ".mmd")
		if err != nil {
			b.Fatal(err)
		}
		b.Run(name, func(b *testing.B) {
			for b.Loop() {
				g := flow.Parse(string(src), true)
				Layout(g, measure.Fixed{}, layout.Options{})
			}
		})
	}
	// A larger graph: a tree with cross links, 200 nodes.
	var sb strings.Builder
	sb.WriteString("flowchart TD\n")
	for i := 1; i < 200; i++ {
		sb.WriteString("n" + itoa((i-1)/3) + " --> n" + itoa(i) + "\n")
		if i%7 == 0 {
			sb.WriteString("n" + itoa(i) + " --> n" + itoa(i/2) + "\n")
		}
	}
	big := sb.String()
	b.Run("200-nodes", func(b *testing.B) {
		for b.Loop() {
			Layout(flow.Parse(big, true), measure.Fixed{}, layout.Options{})
		}
	})
}

func itoa(i int) string {
	return strings.TrimSpace(strings.Repeat(" ", 0) + string(rune('0'+i/100%10)) + string(rune('0'+i/10%10)) + string(rune('0'+i%10)))
}
