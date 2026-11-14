// Package sugiyama lays out flowcharts in ranks, in the manner of
// Sugiyama's method as dagre does it: break cycles, assign ranks, order
// the nodes of each rank to cut crossings, place them, and route edges
// through the ranks between their ends.
//
// Ranks are doubled, so that every edge spans at least one rank of its
// own: an edge's label sits there, on a dummy node of the label's size.
//
// Subgraphs are kept together in each rank (their nodes contiguous, with a
// border node at each side) and their boxes span the ranks they occupy. A
// subgraph with its own direction and no edge crossing its border is laid
// out on its own, in that direction, and placed as one box.
package sugiyama

import (
	"math"
	"slices"
	"sort"

	"github.com/momaek/streameraid/flow"
	"github.com/momaek/streameraid/geom"
	"github.com/momaek/streameraid/layout"
)

// Hints make a layout follow an earlier one of the same diagram, for a
// diagram that grows as its source streams in.
type Hints struct {
	// Prev is the layout of the diagram before its last change. Nodes it
	// holds keep their order in each rank unless another order has fewer
	// crossings; new nodes go where their neighbors are.
	Prev *layout.Flow
	// Anchor also keeps the nodes Prev holds where they were, as far as
	// the new ones leave room, and keeps the diagram's origin: what is
	// added then shows up beside and below what is there, rather than
	// moving it. Without it positions are balanced anew.
	Anchor bool
}

// Layout lays out a flowchart.
func Layout(g *flow.Graph, m layout.Measurer, opt layout.Options) *layout.Flow {
	return LayoutWith(g, m, opt, Hints{})
}

// LayoutWith lays out a flowchart following an earlier layout of it.
func LayoutWith(g *flow.Graph, m layout.Measurer, opt layout.Options, h Hints) *layout.Flow {
	opt = opt.Defaults()
	l := &layouter{
		g: g, m: m, opt: opt,
		nodeW: map[string]float64{}, nodeH: map[string]float64{},
		labels: map[string]layout.TextBox{}, edgeLabels: map[int]layout.TextBox{},
		titles: map[string]layout.TextBox{}, collapsed: map[string]bool{}, hasNodes: map[string]bool{},
		prev: map[string]geom.Rect{}, anchor: h.Anchor,
	}
	if h.Prev != nil {
		for _, n := range h.Prev.Nodes {
			l.prev[n.ID] = n.Rect
		}
		for _, c := range h.Prev.Clusters {
			if _, isNode := g.Nodes[c.ID]; !isNode {
				l.prev[c.ID] = c.Rect
			}
		}
	}
	l.measure()
	l.classify()
	root := l.scope("", g.Dir)
	out := l.assemble(root)
	out.Crossings = l.crossings
	return out
}

type layouter struct {
	g   *flow.Graph
	m   layout.Measurer
	opt layout.Options

	nodeW, nodeH map[string]float64
	labels       map[string]layout.TextBox
	edgeLabels   map[int]layout.TextBox
	titles       map[string]layout.TextBox
	// collapsed subgraphs are laid out on their own and placed as one box:
	// those with their own direction and no edge crossing their border, and
	// those with no nodes.
	collapsed map[string]bool
	hasNodes  map[string]bool

	// prev holds where the nodes and subgraph boxes of an earlier layout
	// were, on screen; anchor tells to keep them there.
	prev      map[string]geom.Rect
	anchor    bool
	crossings int
}

func (l *layouter) measure() {
	st := l.opt.TextStyle()
	for _, id := range l.g.NodeOrder {
		n := l.g.Nodes[id]
		s := st
		if w := l.g.NodeStyle(n).FontWeight; w == "bold" || w == "bolder" || w >= "600" && w <= "999" {
			s.Bold = true
		}
		tb := l.m.Measure(n.Label, s, l.opt.WrapWidth)
		l.labels[id] = tb
		l.nodeW[id], l.nodeH[id] = layout.NodeSize(n.Shape, tb, l.opt.FontSize)
	}
	for _, e := range l.g.Edges {
		if e.Label != "" {
			l.edgeLabels[e.Index] = l.m.Measure(e.Label, st, l.opt.WrapWidth)
		}
	}
	for _, id := range l.g.SubgraphOrder {
		if t := l.g.Subgraphs[id].Title; t != "" {
			l.titles[id] = l.m.Measure(t, st, 2*l.opt.WrapWidth)
		}
	}
}

// classify finds the subgraphs laid out on their own.
func (l *layouter) classify() {
	g := l.g
	for _, id := range g.NodeOrder {
		for s := g.Nodes[id].Subgraph; s != ""; s = g.Subgraphs[s].Parent {
			l.hasNodes[s] = true
		}
	}
	for _, id := range g.SubgraphOrder {
		sg := g.Subgraphs[id]
		if !l.hasNodes[id] {
			l.collapsed[id] = true
			continue
		}
		if !sg.HasDir || sg.Dir == l.parentDir(sg) {
			continue
		}
		crossing := false
		for _, e := range g.Edges {
			if e.From == id || e.To == id {
				continue
			}
			if l.inside(id, e.From) != l.inside(id, e.To) {
				crossing = true
				break
			}
		}
		l.collapsed[id] = !crossing
	}
}

// parentDir is the direction the subgraph's parent lays out in.
func (l *layouter) parentDir(sg *flow.Subgraph) flow.Direction {
	for p := sg.Parent; p != ""; p = l.g.Subgraphs[p].Parent {
		if ps := l.g.Subgraphs[p]; l.collapsed[p] && ps.HasDir {
			return ps.Dir
		}
	}
	return l.g.Dir
}

// inside reports whether node or subgraph id is subgraph sg or inside it.
func (l *layouter) inside(sg, id string) bool {
	return id == sg || l.g.Contains(sg, id)
}

// scopeOf returns the collapsed subgraph that lays out the subgraph s, or
// the node inside s: s itself when collapsed, else its nearest collapsed
// ancestor, or "" for the whole diagram.
func (l *layouter) scopeOf(s string) string {
	for ; s != ""; s = l.g.Subgraphs[s].Parent {
		if l.collapsed[s] {
			return s
		}
	}
	return ""
}

// clusterScope returns the scope a subgraph's box is placed in.
func (l *layouter) clusterScope(s string) string {
	return l.scopeOf(l.g.Subgraphs[s].Parent)
}

// result is a scope laid out, in screen coordinates local to it.
type result struct {
	w, h     float64
	nodes    map[string]geom.Rect
	clusters map[string]geom.Rect
	titleAt  map[string]geom.Point
	edges    map[int]*route
}

type route struct {
	pts      []geom.Point
	path     geom.Path
	labelAt  geom.Point
	hasLabel bool
}

func newResult() *result {
	return &result{nodes: map[string]geom.Rect{}, clusters: map[string]geom.Rect{},
		titleAt: map[string]geom.Point{}, edges: map[int]*route{}}
}

func (r *result) translate(d geom.Point) {
	for k, v := range r.nodes {
		r.nodes[k] = v.Translate(d)
	}
	for k, v := range r.clusters {
		r.clusters[k] = v.Translate(d)
	}
	for k, v := range r.titleAt {
		r.titleAt[k] = v.Add(d)
	}
	for _, e := range r.edges {
		for i := range e.pts {
			e.pts[i] = e.pts[i].Add(d)
		}
		e.path = e.path.Transform(func(p geom.Point) geom.Point { return p.Add(d) })
		e.labelAt = e.labelAt.Add(d)
	}
}

func (r *result) merge(o *result) {
	for k, v := range o.nodes {
		r.nodes[k] = v
	}
	for k, v := range o.clusters {
		r.clusters[k] = v
	}
	for k, v := range o.titleAt {
		r.titleAt[k] = v
	}
	for k, v := range o.edges {
		r.edges[k] = v
	}
}

// item is a box laid out in a scope: a node, or a collapsed subgraph.
type item struct {
	id        string
	isCluster bool
	w, h      float64 // screen size
	sub       *result // a collapsed subgraph's content
	cluster   string  // innermost subgraph of the scope holding it
	key       int     // the order items first appeared in
	selfLoops []int   // edge indexes
	loopExtra float64 // room the self loops take at the side
}

// end is an end of an edge resolved in a scope: an item, or the box of a
// subgraph laid out in the scope.
type end struct {
	item    int // -1 for a subgraph
	cluster string
}

type scopeEdge struct {
	e        *flow.Edge
	from, to end
	rep      [2]int // the items the edge is routed between
	reversed bool
	flat     bool
}

const titleGapEm = 0.35

// titleSpace is the room a subgraph's title takes across the side it is
// on.
func (l *layouter) titleSpace(id string) float64 {
	t, ok := l.titles[id]
	if !ok {
		return 0
	}
	return t.H + titleGapEm*l.opt.FontSize
}

// scope lays out the nodes and subgraphs that subgraph id (or the whole
// diagram for "") holds directly or through subgraphs that are not
// collapsed.
func (l *layouter) scope(id string, dir flow.Direction) *result {
	g := l.g
	s := &scopeLayout{l: l, id: id, dir: dir, itemIndex: map[string]int{}, paths: map[string][]string{}}

	// Items.
	for _, nid := range g.NodeOrder {
		n := g.Nodes[nid]
		if l.scopeOf(n.Subgraph) != id {
			continue
		}
		cl := n.Subgraph
		if cl == id {
			cl = ""
		}
		s.add(&item{id: nid, w: l.nodeW[nid], h: l.nodeH[nid], cluster: cl, key: n.Order})
	}
	for _, sid := range g.SubgraphOrder {
		sg := g.Subgraphs[sid]
		if !l.collapsed[sid] || l.clusterScope(sid) != id {
			continue
		}
		if !l.hasNodes[sid] && sg.Open && !g.Final {
			// A subgraph just opened, as its source streams in: it shows
			// with its first node, rather than as an empty box somewhere
			// that then moves around the node.
			continue
		}
		sub := l.scope(sid, l.dirOf(sg))
		pad := l.opt.ClusterPad
		title := l.titles[sid]
		w := math.Max(sub.w+2*pad, title.W+2*pad)
		h := sub.h + 2*pad + l.titleSpace(sid)
		if sub.w == 0 {
			h = math.Max(h, 2*l.opt.FontSize)
		}
		cl := sg.Parent
		if cl == id {
			cl = ""
		}
		key := 1 << 30
		for _, nid := range g.NodeOrder {
			if l.inside(sid, nid) {
				key = g.Nodes[nid].Order
				break
			}
		}
		if key == 1<<30 {
			key += sg.Order
		}
		s.add(&item{id: sid, isCluster: true, w: w, h: h, sub: sub, cluster: cl, key: key})
	}
	sort.SliceStable(s.items, func(i, j int) bool { return s.items[i].key < s.items[j].key })
	for i, it := range s.items {
		s.itemIndex[it.id] = i
	}
	// The subgraphs of this scope that keep their nodes together.
	for _, sid := range g.SubgraphOrder {
		if !l.collapsed[sid] && l.clusterScope(sid) == id {
			s.clusters = append(s.clusters, sid)
		}
	}
	for _, sid := range s.clusters {
		var path []string
		for c := sid; c != id && c != ""; c = g.Subgraphs[c].Parent {
			path = append([]string{c}, path...)
		}
		s.paths[sid] = path
	}

	// Edges.
	for _, e := range g.Edges {
		from, ok1 := s.resolve(e.From)
		to, ok2 := s.resolve(e.To)
		if !ok1 || !ok2 {
			continue
		}
		if from.item >= 0 && from.item == to.item {
			it := s.items[from.item]
			if !it.isCluster && e.From == e.To {
				it.selfLoops = append(it.selfLoops, e.Index)
			}
			continue
		}
		if from.item < 0 && to.item < 0 && from.cluster == to.cluster {
			continue
		}
		// An edge between a subgraph and what is inside it has no sensible
		// route.
		if from.item < 0 && s.endInside(to, from.cluster) || to.item < 0 && s.endInside(from, to.cluster) {
			continue
		}
		s.edges = append(s.edges, &scopeEdge{e: e, from: from, to: to})
	}

	res := s.run()
	// Place the collapsed subgraphs' content in their boxes.
	for _, it := range s.items {
		if !it.isCluster {
			continue
		}
		box := res.clusters[it.id]
		pad := l.opt.ClusterPad
		it.sub.translate(geom.Point{X: box.X + (box.W-it.sub.w)/2, Y: box.Y + pad + l.titleSpace(it.id)})
		res.merge(it.sub)
		res.titleAt[it.id] = geom.Point{X: box.X + pad + l.titles[it.id].W/2, Y: box.Y + pad*0.6 + l.titles[it.id].H/2}
	}
	return res
}

func (l *layouter) dirOf(sg *flow.Subgraph) flow.Direction {
	if sg.HasDir {
		return sg.Dir
	}
	return l.parentDir(sg)
}

// assemble turns the root result into the layout's geometry.
func (l *layouter) assemble(r *result) *layout.Flow {
	g := l.g
	m := l.opt.Margin
	r.translate(geom.Point{X: m, Y: m})
	out := &layout.Flow{Width: math.Ceil(r.w + 2*m), Height: math.Ceil(r.h + 2*m)}
	if len(r.nodes) == 0 && len(r.clusters) == 0 {
		out.Width, out.Height = 0, 0
	}
	for _, id := range g.NodeOrder {
		rect, ok := r.nodes[id]
		if !ok {
			continue
		}
		n := g.Nodes[id]
		out.Nodes = append(out.Nodes, layout.NodeBox{
			ID: id, Rect: rect, Shape: n.Shape, Label: l.labels[id],
			Outline: layout.Outline(n.Shape, rect),
			Lines:   layout.Decorations(n.Shape, rect, l.opt.FontSize),
		})
	}
	for _, id := range g.SubgraphOrder {
		rect, ok := r.clusters[id]
		if !ok {
			continue
		}
		depth := 0
		for p := g.Subgraphs[id].Parent; p != ""; p = g.Subgraphs[p].Parent {
			depth++
		}
		out.Clusters = append(out.Clusters, layout.ClusterBox{ID: id, Rect: rect, Title: l.titles[id], TitleAt: r.titleAt[id], Depth: depth})
	}
	for _, e := range g.Edges {
		rt, ok := r.edges[e.Index]
		if !ok {
			continue
		}
		er := layout.EdgeRoute{ID: e.ID, Index: e.Index, Path: rt.path, Points: rt.pts}
		if tb, ok := l.edgeLabels[e.Index]; ok {
			er.HasLabel, er.Label, er.LabelAt = true, tb, rt.labelAt
		}
		out.Edges = append(out.Edges, er)
	}
	return out
}

// scopeLayout lays out one scope.
type scopeLayout struct {
	l         *layouter
	id        string
	dir       flow.Direction
	items     []*item
	itemIndex map[string]int
	clusters  []string            // subgraphs kept together, in order
	paths     map[string][]string // a subgraph's path from the scope down to it
	edges     []*scopeEdge

	rank   []int
	layers [][]*lnode
	nodes  []*lnode          // one per item
	span   map[string][2]int // the first and last rank of each subgraph
	chains map[*scopeEdge][]*lnode
	// anyPrev tells that some node was in an earlier layout.
	anyPrev bool
}

func (s *scopeLayout) add(it *item) { s.items = append(s.items, it) }

// resolve finds what an edge's end is in this scope.
func (s *scopeLayout) resolve(id string) (end, bool) {
	g := s.l.g
	var up string
	if n := g.Nodes[id]; n != nil {
		if i, ok := s.itemIndex[id]; ok {
			return end{item: i}, true
		}
		up = n.Subgraph
	} else if sg := g.Subgraphs[id]; sg != nil {
		if i, ok := s.itemIndex[id]; ok {
			return end{item: i}, true
		}
		if !s.l.collapsed[id] && s.l.clusterScope(id) == s.id {
			return end{item: -1, cluster: id}, true
		}
		up = sg.Parent
	} else {
		return end{}, false
	}
	for ; up != "" && up != s.id; up = g.Subgraphs[up].Parent {
		if i, ok := s.itemIndex[up]; ok {
			return end{item: i}, true
		}
	}
	return end{}, false
}

// path returns the subgraphs of the scope holding an item, outermost first.
func (s *scopeLayout) itemPath(it *item) []string {
	if it.cluster == "" {
		return nil
	}
	return s.paths[it.cluster]
}

// members returns the items inside subgraph c.
func (s *scopeLayout) members(c string) []int {
	var out []int
	for i, it := range s.items {
		if slices.Contains(s.itemPath(it), c) {
			out = append(out, i)
		}
	}
	return out
}

func (s *scopeLayout) endInside(e end, cluster string) bool {
	if e.item >= 0 {
		return slices.Contains(s.itemPath(s.items[e.item]), cluster)
	}
	return slices.Contains(s.paths[e.cluster], cluster) && e.cluster != cluster
}

// endItems returns the items an end stands for when ranking.
func (s *scopeLayout) endItems(e end) []int {
	if e.item >= 0 {
		return []int{e.item}
	}
	return s.members(e.cluster)
}

func (s *scopeLayout) run() *result {
	s.assignRanks()
	s.buildLayers()
	s.localizePrev()
	s.order()
	s.l.crossings += s.crossings()
	s.positionX()
	ys, hs, total := s.positionY()
	return s.place(ys, hs, total)
}

// localizePrev turns the earlier positions, which are on screen, into this
// scope's own coordinates across the ranks. The whole diagram's start at
// its margin; a subgraph laid out on its own has them start at its
// leftmost node, since its box may have moved.
func (s *scopeLayout) localizePrev() {
	off := s.l.opt.Margin
	if s.id != "" {
		off = math.Inf(1)
		for _, n := range s.nodes {
			if n.hasPrev {
				off = math.Min(off, n.prev-n.w/2)
			}
		}
	}
	for _, n := range s.nodes {
		if n.hasPrev {
			n.prev -= off
			s.anyPrev = true
		}
	}
}

// --- Ranks -----------------------------------------------------------------

type redge struct {
	u, v   int
	minlen int
}

func (s *scopeLayout) assignRanks() {
	var edges []redge
	for _, se := range s.edges {
		minlen := 2 * se.e.Length
		for _, u := range s.endItems(se.from) {
			for _, v := range s.endItems(se.to) {
				if u != v {
					edges = append(edges, redge{u, v, minlen})
				}
			}
		}
	}
	rank, _ := rankItems(len(s.items), edges)
	s.rank = rank
	// An edge is reversed when the ranks put its target above its source.
	for _, se := range s.edges {
		se.rep = [2]int{s.repItem(se.from, true), s.repItem(se.to, false)}
		ru, rv := rank[se.rep[0]], rank[se.rep[1]]
		switch {
		case ru > rv:
			se.reversed = true
		case ru == rv:
			se.flat = true
		}
	}
}

// repItem picks the item an end's route starts or stops at: for a
// subgraph, its lowest member as a source and its highest as a target.
func (s *scopeLayout) repItem(e end, source bool) int {
	if e.item >= 0 {
		return e.item
	}
	best := -1
	for _, i := range s.members(e.cluster) {
		if best < 0 || source && s.rank[i] > s.rank[best] || !source && s.rank[i] < s.rank[best] {
			best = i
		}
	}
	return best
}

// rankItems assigns ranks: cycles are broken by reversing the edges that a
// depth-first search in item order finds going back, ranks are the longest
// paths from the sources, and sources are then pulled down next to what
// they point at. It returns the ranks and which edges it reversed.
func rankItems(n int, edges []redge) ([]int, []bool) {
	adj := make([][]int, n)
	for i, e := range edges {
		adj[e.u] = append(adj[e.u], i)
	}
	state := make([]int8, n)
	reversed := make([]bool, len(edges))
	var dfs func(u int)
	dfs = func(u int) {
		state[u] = 1
		for _, ei := range adj[u] {
			switch v := edges[ei].v; state[v] {
			case 1:
				reversed[ei] = true
			case 0:
				dfs(v)
			}
		}
		state[u] = 2
	}
	for u := range n {
		if state[u] == 0 {
			dfs(u)
		}
	}
	type fe struct{ u, v, minlen int }
	in := make([][]fe, n)
	out := make([][]fe, n)
	indeg := make([]int, n)
	for i, e := range edges {
		u, v := e.u, e.v
		if reversed[i] {
			u, v = v, u
		}
		f := fe{u, v, e.minlen}
		out[u] = append(out[u], f)
		in[v] = append(in[v], f)
		indeg[v]++
	}
	var ready []int
	for u := range n {
		if indeg[u] == 0 {
			ready = append(ready, u)
		}
	}
	rank := make([]int, n)
	var topo []int
	for len(ready) > 0 {
		u := ready[0]
		ready = ready[1:]
		topo = append(topo, u)
		for _, f := range out[u] {
			rank[f.v] = max(rank[f.v], rank[u]+f.minlen)
			indeg[f.v]--
			if indeg[f.v] == 0 {
				k := sort.SearchInts(ready, f.v)
				ready = slices.Insert(ready, k, f.v)
			}
		}
	}
	for pass := 0; pass < 2; pass++ {
		for i := len(topo) - 1; i >= 0; i-- {
			v := topo[i]
			if len(in[v]) > 0 || len(out[v]) == 0 {
				continue
			}
			m := math.MaxInt
			for _, f := range out[v] {
				m = min(m, rank[f.v]-f.minlen)
			}
			rank[v] = m
		}
	}
	lo := math.MaxInt
	for _, r := range rank {
		lo = min(lo, r)
	}
	for i := range rank {
		rank[i] -= lo
	}
	return rank, reversed
}

// --- Layers ----------------------------------------------------------------

type nodeKind uint8

const (
	kItem nodeKind = iota
	kDummy
	kLabel
	kBorderL
	kBorderR
)

type lnode struct {
	kind      nodeKind
	item      int
	cluster   string   // a border's subgraph
	path      []string // the subgraphs holding it, outermost first
	rank      int
	order     int
	w, h      float64 // across the ranks, along them
	x         float64
	loopExtra float64
	up, down  []ladj
	seq       int // creation order, for stable ties
	// prev is where the node was across the ranks in the earlier layout,
	// when hasPrev.
	prev    float64
	hasPrev bool
	growing bool
}

type ladj struct {
	n *lnode
	w float64
}

func link(a, b *lnode) {
	w := 1.0
	switch {
	case a.kind != kItem && b.kind != kItem:
		w = 8
	case a.kind != kItem || b.kind != kItem:
		w = 2
	}
	a.down = append(a.down, ladj{b, w})
	b.up = append(b.up, ladj{a, w})
}

// frameSize returns an item's size across and along the ranks.
func (s *scopeLayout) frameSize(w, h float64) (cross, along float64) {
	if s.dir.Horizontal() {
		return h, w
	}
	return w, h
}

func (s *scopeLayout) buildLayers() {
	maxRank := 0
	for _, r := range s.rank {
		maxRank = max(maxRank, r)
	}
	s.layers = make([][]*lnode, maxRank+1)
	seq := 0
	newNode := func(n *lnode) *lnode {
		n.seq = seq
		seq++
		s.layers[n.rank] = append(s.layers[n.rank], n)
		return n
	}
	fs := s.l.opt.FontSize
	for i, it := range s.items {
		cw, ch := s.frameSize(it.w, it.h)
		n := newNode(&lnode{kind: kItem, item: i, path: s.itemPath(it), rank: s.rank[i], w: cw, h: ch})
		if r, ok := s.l.prev[it.id]; ok {
			c := r.Center()
			n.prev, n.hasPrev = c.X, true
			was := r.W
			if s.dir.Horizontal() {
				n.prev, was = c.Y, r.H
			}
			// A node whose size changed is still being written. It grows
			// from its near edge, which stays where it was, and what is
			// after it makes room: so typing a label moves only what is
			// beside it, not everything.
			if math.Abs(was-cw) > 0.01 {
				n.growing = true
				n.prev += (cw - was) / 2
			}
		}
		if len(it.selfLoops) > 0 {
			extra := 1.6 * fs
			for _, ei := range it.selfLoops {
				if tb, ok := s.l.edgeLabels[ei]; ok {
					lw, _ := s.frameSize(tb.W, tb.H)
					if s.dir.Horizontal() {
						lw = tb.H
					}
					extra = math.Max(extra, 1.4*fs+lw+0.4*fs)
				}
			}
			it.loopExtra = extra
			n.loopExtra = extra
		}
		s.nodes = append(s.nodes, n)
	}
	for _, se := range s.edges {
		if se.flat {
			continue
		}
		u, v := s.nodes[se.rep[0]], s.nodes[se.rep[1]]
		if se.reversed {
			u, v = v, u
		}
		pu, pv := s.endPath(se.from), s.endPath(se.to)
		lca := commonPrefix(pu, pv)
		mid := -1
		if _, ok := s.l.edgeLabels[se.e.Index]; ok {
			mid = u.rank + (v.rank-u.rank)/2
			if (mid-u.rank)%2 == 0 && mid-1 > u.rank {
				mid--
			}
		}
		prev := u
		for r := u.rank + 1; r < v.rank; r++ {
			d := &lnode{kind: kDummy, rank: r, path: lca, item: -1}
			if r == mid {
				tb := s.l.edgeLabels[se.e.Index]
				d.kind = kLabel
				d.w, d.h = s.frameSize(tb.W+0.5*fs, tb.H+0.3*fs)
			}
			newNode(d)
			link(prev, d)
			s.chain(se, d)
			prev = d
		}
		link(prev, v)
	}
	// Borders.
	s.span = map[string][2]int{}
	for _, c := range s.clusters {
		lo, hi := math.MaxInt, -1
		for _, i := range s.members(c) {
			lo, hi = min(lo, s.rank[i]), max(hi, s.rank[i])
		}
		if hi < 0 {
			continue
		}
		s.span[c] = [2]int{lo, hi}
		for r := lo; r <= hi; r++ {
			newNode(&lnode{kind: kBorderL, cluster: c, path: s.paths[c], rank: r, item: -1})
			newNode(&lnode{kind: kBorderR, cluster: c, path: s.paths[c], rank: r, item: -1})
		}
	}
}

// chains holds each scope edge's dummy nodes, in rank order.
func (s *scopeLayout) chain(se *scopeEdge, d *lnode) {
	if s.chains == nil {
		s.chains = map[*scopeEdge][]*lnode{}
	}
	s.chains[se] = append(s.chains[se], d)
}

// endPath is the path of the subgraphs an edge's dummies may sit in at an
// end: an item's subgraphs, or a subgraph's parents.
func (s *scopeLayout) endPath(e end) []string {
	if e.item >= 0 {
		return s.itemPath(s.items[e.item])
	}
	p := s.paths[e.cluster]
	return p[:len(p)-1]
}

func commonPrefix(a, b []string) []string {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return a[:n:n]
}
