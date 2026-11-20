// Package tree lays out mind maps: a tree drawn across the page, its root
// in the middle and its first-level branches split between a right side
// and a left side, each growing outward.
//
// On each side a branch is a tidy tree: children stacked top to bottom,
// a fixed gap from their parent, the parent centered on them, and the
// subtrees of siblings packed as close as their nodes (and the curves to
// their children) allow. Edges are curves from the side of a parent to
// the near side of its child.
//
// The layout takes any graph: edges that make no tree (a second parent, a
// cycle) are drawn but do not place nodes, and a graph of several trees
// has them one below another.
package tree

import (
	"math"

	"github.com/momaek/streameraid/flow"
	"github.com/momaek/streameraid/geom"
	"github.com/momaek/streameraid/layout"
)

// Hints make a layout follow an earlier one of the same diagram, for a
// diagram that grows as its source streams in.
type Hints struct {
	// Prev is the layout of the diagram before its last change. The
	// first-level branches it holds stay on the side they were on; a new
	// branch goes to the side with fewer leaves.
	Prev *layout.Flow
	// Anchor also keeps the nodes Prev holds where they were, and the root
	// where it was on the page: a new node goes below its siblings,
	// pushing down what is below it, and parents are not centered anew on
	// their children. Without it positions are balanced anew.
	Anchor bool
}

// Layout lays out a mind map.
func Layout(g *flow.Graph, m layout.Measurer, opt layout.Options) *layout.Flow {
	return LayoutWith(g, m, opt, Hints{})
}

// The gaps of the layout, in ems.
const (
	levelGap   = 3.0 // between a parent and its children, across
	siblingGap = 0.6 // between sibling subtrees
	branchGap  = 1.0 // between the first-level branches of a side
	treeGap    = 2.0 // between the trees of a graph of several
	// shiftStep is the step the diagram moves in when an anchored layout
	// grows past the top or left edge: moving it by more than is needed
	// leaves room for what comes next.
	shiftStep  = 8.0
	shiftStepY = 2.0
)

type node struct {
	id     string
	shape  flow.Shape
	label  layout.TextBox
	w, h   float64
	parent *node
	kids   []*node
	leaves int
	// side is +1 for a node right of its tree's root, -1 left of it.
	side int
	// dy is the offset of the node's center below its parent's; c its
	// center once placed.
	dy float64
	c  geom.Point
}

type layouter struct {
	g      *flow.Graph
	opt    layout.Options
	nodes  map[string]*node
	prev   map[string]geom.Point // centers of the nodes of Prev
	anchor bool
}

// LayoutWith lays out a mind map following an earlier layout of it.
func LayoutWith(g *flow.Graph, m layout.Measurer, opt layout.Options, h Hints) *layout.Flow {
	opt = opt.Defaults()
	l := &layouter{g: g, opt: opt, nodes: map[string]*node{}, prev: map[string]geom.Point{}}
	if h.Prev != nil {
		for _, n := range h.Prev.Nodes {
			l.prev[n.ID] = n.Rect.Center()
		}
		l.anchor = h.Anchor
	}
	st := opt.TextStyle()
	var order []*node
	for _, id := range g.NodeOrder {
		gn := g.Nodes[id]
		s := st
		if w := g.NodeStyle(gn).FontWeight; w == "bold" || w == "bolder" || w >= "600" && w <= "999" {
			s.Bold = true
		}
		n := &node{id: id, shape: gn.Shape, label: m.Measure(gn.Label, s, opt.WrapWidth)}
		n.w, n.h = layout.NodeSize(gn.Shape, n.label, opt.FontSize)
		l.nodes[id] = n
		order = append(order, n)
	}
	l.link(order)
	roots := []*node{}
	for _, n := range order {
		if n.parent == nil {
			roots = append(roots, n)
		}
	}
	out := &layout.Flow{}
	if len(roots) == 0 {
		out.Width, out.Height = 2*opt.Margin, 2*opt.Margin
		return out
	}
	// Lay out each tree around its root at the origin, then stack them.
	var all geom.Rect
	for i, r := range roots {
		countLeaves(r)
		l.splitSides(r)
		for _, side := range []int{1, -1} {
			var kids []*node
			for _, k := range r.kids {
				if k.side == side {
					kids = append(kids, k)
				}
			}
			if len(kids) > 0 {
				l.place(r, kids, branchGap)
			}
		}
		r.c = geom.Point{}
		bounds := l.position(r)
		if i > 0 {
			d := geom.Point{Y: all.Bottom() + treeGap*opt.FontSize - bounds.Y}
			move(r, d)
			bounds = bounds.Translate(d)
			all = all.Union(bounds)
		} else {
			all = bounds
		}
	}
	// Place the whole on the page.
	em := opt.FontSize
	var d geom.Point
	if p, ok := l.prev[roots[0].id]; ok && l.anchor {
		d = p
		// What grows past the left or top edge moves everything. It moves
		// by more than is needed, to leave room for what comes next: on the
		// left as much room as the right side takes, which a balanced left
		// side grows to.
		if lack := opt.Margin - (all.X + d.X); lack > 0.01 {
			step := shiftStep * em
			d.X += math.Max(math.Ceil(lack/step)*step, lack+all.Right()+all.X)
		}
		if lack := opt.Margin - (all.Y + d.Y); lack > 0.01 {
			step := shiftStepY * em
			d.Y += math.Ceil(lack/step) * step
		}
	} else {
		d = geom.Point{X: opt.Margin - all.X, Y: opt.Margin - all.Y}
	}
	out.Width = all.Right() + d.X + opt.Margin
	out.Height = all.Bottom() + d.Y + opt.Margin
	for _, n := range order {
		c := n.c.Add(d)
		r := geom.RectCentered(c, n.w, n.h)
		out.Nodes = append(out.Nodes, layout.NodeBox{
			ID: n.id, Rect: r, Shape: n.shape, Label: n.label,
			Outline: layout.Outline(n.shape, r),
			Lines:   layout.Decorations(n.shape, r, opt.FontSize),
		})
	}
	boxes := map[string]layout.NodeBox{}
	for _, b := range out.Nodes {
		boxes[b.ID] = b
	}
	for _, e := range g.Edges {
		from, ok1 := boxes[e.From]
		to, ok2 := boxes[e.To]
		if !ok1 || !ok2 || e.From == e.To || e.Line == flow.Invisible {
			continue
		}
		out.Edges = append(out.Edges, route(e, from, to))
	}
	return out
}

// link makes the tree: each node's parent is the source of the first edge
// to it that makes no cycle. Children keep the order of the nodes.
func (l *layouter) link(order []*node) {
	for _, e := range l.g.Edges {
		from, to := l.nodes[e.From], l.nodes[e.To]
		if from == nil || to == nil || from == to || to.parent != nil {
			continue
		}
		cycle := false
		for a := from; a != nil; a = a.parent {
			if a == to {
				cycle = true
				break
			}
		}
		if !cycle {
			to.parent = from
		}
	}
	for _, n := range order {
		if n.parent != nil {
			n.parent.kids = append(n.parent.kids, n)
		}
	}
}

func countLeaves(n *node) int {
	n.leaves = 0
	for _, k := range n.kids {
		n.leaves += countLeaves(k)
	}
	n.leaves = max(n.leaves, 1)
	return n.leaves
}

// splitSides puts each first-level branch of root on a side: where it was
// in the layout before, else the first branches on the right and the rest
// on the left, the sides as even in leaves as can be.
func (l *layouter) splitSides(root *node) {
	rc, hasRoot := l.prev[root.id]
	var right, left int
	known := false
	if hasRoot {
		for _, k := range root.kids {
			if p, ok := l.prev[k.id]; ok {
				known = true
				k.side = 1
				if p.X < rc.X {
					k.side = -1
					left += k.leaves
				} else {
					right += k.leaves
				}
			}
		}
	}
	if known {
		for _, k := range root.kids {
			if _, ok := l.prev[k.id]; ok {
				continue
			}
			k.side = 1
			if left < right {
				k.side = -1
				left += k.leaves
			} else {
				right += k.leaves
			}
		}
	} else {
		total := root.leaves
		if len(root.kids) == 0 {
			total = 0
		}
		best, split, sum := math.Inf(1), 0, 0
		for i, k := range root.kids {
			sum += k.leaves
			// Ties go to the split with more on the right.
			if d := math.Abs(float64(2*sum - total)); d <= best {
				best, split = d, i+1
			}
		}
		for i, k := range root.kids {
			k.side = 1
			if i >= split {
				k.side = -1
			}
		}
	}
	var mark func(n *node, side int)
	mark = func(n *node, side int) {
		n.side = side
		for _, k := range n.kids {
			mark(k, side)
		}
	}
	for _, k := range root.kids {
		mark(k, k.side)
	}
}

// place lays out the subtree of n with the given children, on their
// side, setting the children's offsets (dy) below n. It returns what the
// subtree takes up, in a frame where n's near side is at x = 0 and its
// middle at y = 0, x growing away from the root: n's box, those of the
// nodes below it, and the room of the curves to its children.
func (l *layouter) place(n *node, kids []*node, gapEm float64) []geom.Rect {
	em := l.opt.FontSize
	own := geom.Rect{X: 0, Y: -n.h / 2, W: n.w, H: n.h}
	if len(kids) == 0 {
		return []geom.Rect{own}
	}
	cx := n.w + levelGap*em
	gap := gapEm * em
	var acc []geom.Rect
	kept := false
	for i, k := range kids {
		sub := l.place(k, k.kids, siblingGap)
		for j := range sub {
			sub[j].X += cx
		}
		low := math.Inf(-1)
		for _, a := range acc {
			for _, b := range sub {
				// Anchored, a subtree goes below the whole of the one
				// before, not into the room beside its deeper nodes: the
				// children still to come of a node would push it down.
				if l.anchor || a.X < b.Right() && b.X < a.Right() {
					low = math.Max(low, a.Bottom()+gap-b.Y)
				}
			}
		}
		y := 0.0
		if d, ok := l.desired(n, k); ok {
			y, kept = d, true
			if i > 0 && y < low {
				y = low
			}
		} else if i > 0 {
			y = low
		}
		k.dy = y
		for _, b := range sub {
			acc = append(acc, b.Translate(geom.Point{Y: y}))
		}
	}
	first, last := kids[0], kids[len(kids)-1]
	if !kept {
		// Center n on its children.
		mid := (first.dy + last.dy) / 2
		for _, k := range kids {
			k.dy -= mid
		}
		for j := range acc {
			acc[j].Y -= mid
		}
	}
	top, bottom := math.Min(0, first.dy), math.Max(0, last.dy)
	return append(acc, own, geom.Rect{X: n.w, Y: top, W: cx - n.w, H: bottom - top})
}

// desired returns where an anchored layout keeps child k: as far below n
// as it was before.
func (l *layouter) desired(n, k *node) (float64, bool) {
	if !l.anchor {
		return 0, false
	}
	pn, ok1 := l.prev[n.id]
	pk, ok2 := l.prev[k.id]
	if !ok1 || !ok2 {
		return 0, false
	}
	return pk.Y - pn.Y, true
}

// position sets the centers of the nodes below n from n's and their
// offsets, and returns the bounds of n's tree.
func (l *layouter) position(n *node) geom.Rect {
	b := geom.RectCentered(n.c, n.w, n.h)
	gap := levelGap * l.opt.FontSize
	for _, k := range n.kids {
		k.c = geom.Point{X: n.c.X + float64(k.side)*(n.w/2+gap+k.w/2), Y: n.c.Y + k.dy}
		b = b.Union(l.position(k))
	}
	return b
}

func move(n *node, d geom.Point) {
	n.c = n.c.Add(d)
	for _, k := range n.kids {
		move(k, d)
	}
}

// route draws an edge as a curve from the side of its source facing its
// target to the near side of the target.
func route(e *flow.Edge, from, to layout.NodeBox) layout.EdgeRoute {
	fc, tc := from.Rect.Center(), to.Rect.Center()
	dir := 1.0
	if tc.X < fc.X {
		dir = -1
	}
	far := 1e4
	start := geom.ClipToOutline(from.Outline, fc, geom.Point{X: fc.X + dir*far, Y: fc.Y})
	end := geom.ClipToOutline(to.Outline, tc, geom.Point{X: tc.X - dir*far, Y: tc.Y})
	dx := (end.X - start.X) / 2
	var p geom.Path
	p.MoveTo(start).CubicTo(geom.Point{X: start.X + dx, Y: start.Y}, geom.Point{X: end.X - dx, Y: end.Y}, end)
	return layout.EdgeRoute{ID: e.ID, Index: e.Index, Path: p, Points: []geom.Point{start, end}}
}
