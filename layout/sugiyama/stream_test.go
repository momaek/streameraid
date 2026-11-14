package sugiyama

import (
	"math"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/momaek/streameraid/flow"
	"github.com/momaek/streameraid/geom"
	"github.com/momaek/streameraid/layout"
	"github.com/momaek/streameraid/measure"
)

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
		g := flow.Parse(src[:end], final)
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

func aiCorpus(t testing.TB) map[string]string {
	out := map[string]string{}
	for f, src := range corpus(t) {
		if strings.Contains(f, "/ai/") {
			out[filepath.Base(f)] = src
		}
	}
	return out
}

// TestStreamingStability streams the AI-style samples and compares laying
// out afresh at every step with following the layout before. Following
// must move far fewer of the nodes that are there, and stay within the
// limits below, which guard against regressions.
func TestStreamingStability(t *testing.T) {
	var fresh, ordered, anchored motion
	names := []string{}
	per := map[string][3]motion{}
	for name, src := range aiCorpus(t) {
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
		t.Logf("%-28s fresh %5.1fpx %3.0f%%  ordered %5.1fpx %3.0f%%  anchored %5.1fpx %3.0f%% (worst %4.0f, shaken %d/%d)",
			n, p[0].mean(), 100*p[0].share(), p[1].mean(), 100*p[1].share(), p[2].mean(), 100*p[2].share(), p[2].worst, p[2].shaken, p[2].steps)
	}
	report := func(name string, m motion) {
		t.Logf("%-9s mean %.2fpx per node per step, %.1f%% of nodes moved, %d of %d steps moved most nodes",
			name, m.mean(), 100*m.share(), m.shaken, m.steps)
	}
	report("fresh", fresh)
	report("ordered", ordered)
	report("anchored", anchored)
	if a, f := anchored.share(), fresh.share(); a > 0.6*f {
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
// achieves on the corpus (2.45px, 5.9% and 2.5% when they were set, against
// 4.47px, 14.4% and 10.5% laying out afresh): tighten them when it
// improves. What still moves is mostly unavoidable in a layout by ranks: a
// new edge that puts a node on a lower rank, a label that makes a rank
// taller, a node that belongs before the others.
const (
	maxMeanMotion  = 3.0
	maxMovedShare  = 0.075
	maxShakenShare = 0.035
)

// TestStreamingInvariants checks that every layout on the way holds: no
// overlaps, nodes inside their subgraphs.
func TestStreamingInvariants(t *testing.T) {
	for name, src := range aiCorpus(t) {
		stream(src, func(p *layout.Flow) Hints { return Hints{Prev: p, Anchor: true} }, func(g *flow.Graph, lo *layout.Flow) {
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
			}
		})
	}
}

// TestOpenSubgraphWaits checks that a subgraph still open and empty is not
// drawn while its source streams in, and is once the source is complete.
func TestOpenSubgraphWaits(t *testing.T) {
	src := "flowchart TD\n    A --> B\n    subgraph Storage\n"
	if lo := Layout(flow.Parse(src, false), measure.Fixed{}, layout.Options{}); len(lo.Clusters) != 0 {
		t.Errorf("streaming: %d boxes for an empty open subgraph", len(lo.Clusters))
	}
	if lo := Layout(flow.Parse(src+"        C\n", false), measure.Fixed{}, layout.Options{}); len(lo.Clusters) != 1 {
		t.Errorf("streaming, with a node: %d boxes", len(lo.Clusters))
	}
	if lo := Layout(flow.Parse(src+"    end\n", true), measure.Fixed{}, layout.Options{}); len(lo.Clusters) != 1 {
		t.Errorf("complete: %d boxes for an empty subgraph", len(lo.Clusters))
	}
}
