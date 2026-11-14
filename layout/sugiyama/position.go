package sugiyama

import (
	"math"
	"slices"

	"github.com/momaek/streameraid/flow"
	"github.com/momaek/streameraid/geom"
	"github.com/momaek/streameraid/layout"
)

// gap returns the least distance between the centers of adjacent nodes a
// and b, a on the left.
func (s *scopeLayout) gap(a, b *lnode) float64 {
	o := s.l.opt
	base := (a.w+b.w)/2 + a.loopExtra
	pad := o.ClusterPad
	switch {
	case a.kind == kBorderL:
		return base + pad + s.crossTitle(a.cluster)
	case b.kind == kBorderR:
		return base + pad
	case a.kind == kBorderR && b.kind == kBorderL:
		return base + o.NodeSep/2
	case a.kind == kBorderR || b.kind == kBorderL:
		other := b
		if b.kind == kBorderL {
			other = a
		}
		if other.kind == kDummy {
			return base + o.EdgeSep
		}
		return base + o.NodeSep/2
	}
	ad, bd := a.kind == kDummy, b.kind == kDummy
	switch {
	case ad && bd:
		return base + o.EdgeSep
	case ad || bd:
		return base + math.Max(o.EdgeSep, o.NodeSep/2)
	}
	return base + o.NodeSep
}

// crossTitle is the room a subgraph's title takes across the ranks, when
// the title is on that side: the top of the screen when ranks go across.
func (s *scopeLayout) crossTitle(c string) float64 {
	if s.dir.Horizontal() {
		return s.l.titleSpace(c)
	}
	return 0
}

// rankTitle is the room a subgraph's title takes along the ranks at the
// start (top) and end (bottom) of its ranks.
func (s *scopeLayout) rankTitle(c string) (start, end float64) {
	switch s.dir {
	case flow.TB:
		return s.l.titleSpace(c), 0
	case flow.BT:
		return 0, s.l.titleSpace(c)
	}
	return 0, 0
}

// positionX places the nodes of each rank across the ranks. It minimizes
// the squared lengths of edges across, weighted to keep long edges
// straight, keeping each pair of neighbors at least their gap apart: each
// rank in turn is solved exactly for its neighbors' positions (an isotonic
// regression, by pooling adjacent violators), sweeping down and up. Border
// nodes are pulled to their subgraph's outermost border, so that its box
// is the same width on every rank.
func (s *scopeLayout) positionX() {
	// Anchored, the nodes of the earlier layout are held where they were,
	// and the rest start in the middle of them.
	anchored := s.l.anchor && s.anyPrev
	hold := func(v *lnode) float64 {
		if v.growing {
			return 1e5 // over the others, which make room for it
		}
		return 1e4
	}
	start := 0.0
	if anchored {
		sum, cnt := 0.0, 0.0
		for _, v := range s.nodes {
			if v.hasPrev {
				sum += v.prev
				cnt++
			}
		}
		start = sum / cnt
	}
	for _, layer := range s.layers {
		target := make([]float64, len(layer))
		weight := make([]float64, len(layer))
		for i, v := range layer {
			target[i], weight[i] = start, 1
			if anchored && v.hasPrev {
				target[i], weight[i] = v.prev, hold(v)
			}
		}
		s.solve(layer, target, weight)
	}
	n := len(s.layers)
	// The last rounds pin the borders where the boxes reach, pushing what is
	// outside a box out of it on every rank.
	const rounds, pinned = 22, 6
	for iter := 0; iter < rounds; iter++ {
		pull := 20.0
		if iter >= rounds-pinned {
			pull = 1e5
			if iter == rounds-pinned {
				s.straighten(anchored)
			}
		}
		lo, hi := s.borderExtents()
		for k := 0; k < n; k++ {
			l := k
			if iter%2 == 1 {
				l = n - 1 - k
			}
			layer := s.layers[l]
			target := make([]float64, len(layer))
			weight := make([]float64, len(layer))
			for i, v := range layer {
				sw, sx := 0.0, 0.0
				for _, a := range v.up {
					sw += a.w
					sx += a.w * a.n.x
				}
				for _, a := range v.down {
					sw += a.w
					sx += a.w * a.n.x
				}
				switch v.kind {
				case kBorderL:
					sw += pull
					sx += pull * lo[v.cluster]
				case kBorderR:
					sw += pull
					sx += pull * hi[v.cluster]
				}
				if anchored && v.hasPrev {
					sw += hold(v)
					sx += hold(v) * v.prev
				}
				if iter >= rounds-pinned && (v.kind == kItem || v.kind == kDummy || v.kind == kLabel) {
					// Hold what straighten aligned.
					sw += 30
					sx += 30 * v.x
				}
				if sw == 0 {
					target[i], weight[i] = v.x, 0.05
				} else {
					target[i], weight[i] = sx/sw, sw
				}
			}
			s.solve(layer, target, weight)
		}
	}
	s.separate()
}

// separate makes the subgraphs' boxes hold: a box spans all its ranks at
// its widest, so on every rank what is next to a box must clear that
// width. Where something does not, the box (with all it holds) or the node
// is pushed aside, or the box grows to hold what is inside it, until
// nothing overlaps. Last, every border is set where its box reaches.
func (s *scopeLayout) separate() {
	if len(s.clusters) == 0 {
		return
	}
	shiftCluster := func(c string, d float64) {
		for _, layer := range s.layers {
			for _, v := range layer {
				if slices.Contains(v.path, c) {
					v.x += d
				}
			}
		}
	}
	shiftSide := func(c string, kind nodeKind, d float64) {
		for _, layer := range s.layers {
			for _, v := range layer {
				if v.kind == kind && v.cluster == c {
					v.x += d
				}
			}
		}
	}
	unit := func(v *lnode, d float64) {
		if v.kind == kBorderL || v.kind == kBorderR {
			shiftCluster(v.cluster, d)
		} else {
			v.x += d
		}
	}
	for iter := 0; iter < 400; iter++ {
		lo, hi := s.borderExtents()
		at := func(v *lnode) float64 {
			switch v.kind {
			case kBorderL:
				return lo[v.cluster]
			case kBorderR:
				return hi[v.cluster]
			}
			return v.x
		}
		fixed := false
		for _, layer := range s.layers {
			for i := 0; i+1 < len(layer) && !fixed; i++ {
				a, b := layer[i], layer[i+1]
				v := s.gap(a, b) - (at(b) - at(a))
				if v <= 0.01 {
					continue
				}
				fixed = true
				switch {
				case a.kind == kBorderL: // b is inside a's box: widen it
					shiftSide(a.cluster, kBorderL, -v)
				case b.kind == kBorderR:
					shiftSide(b.cluster, kBorderR, v)
				default: // side by side: push both aside
					unit(a, -v/2)
					unit(b, v/2)
				}
			}
			if fixed {
				break
			}
		}
		if !fixed {
			break
		}
	}
	lo, hi := s.borderExtents()
	for _, layer := range s.layers {
		for _, v := range layer {
			switch v.kind {
			case kBorderL:
				v.x = lo[v.cluster]
			case kBorderR:
				v.x = hi[v.cluster]
			}
		}
	}
}

// straighten lines up chains: a node whose only neighbor above has no
// other below moves under it, as far as its neighbors in the rank let it;
// then a node with no such parent moves over its only child.
func (s *scopeLayout) straighten(anchored bool) {
	single := func(v *lnode, up bool) *lnode {
		adj, back := v.down, func(n *lnode) []ladj { return n.up }
		if up {
			adj, back = v.up, func(n *lnode) []ladj { return n.down }
		}
		if len(adj) == 1 && len(back(adj[0].n)) == 1 {
			return adj[0].n
		}
		return nil
	}
	move := func(layer []*lnode, i int, want float64) {
		v := layer[i]
		lo, hi := math.Inf(-1), math.Inf(1)
		if i > 0 {
			lo = layer[i-1].x + s.gap(layer[i-1], v)
		}
		if i+1 < len(layer) {
			hi = layer[i+1].x - s.gap(v, layer[i+1])
		}
		if lo <= hi {
			v.x = math.Max(lo, math.Min(hi, want))
		}
	}
	movable := func(v *lnode) bool {
		if anchored && v.hasPrev {
			return false // held where it was
		}
		return v.kind == kItem || v.kind == kDummy || v.kind == kLabel
	}
	for _, layer := range s.layers {
		for i, v := range layer {
			if p := single(v, true); p != nil && movable(v) {
				move(layer, i, p.x)
			}
		}
	}
	for l := len(s.layers) - 1; l >= 0; l-- {
		layer := s.layers[l]
		for i, v := range layer {
			if single(v, true) != nil || !movable(v) {
				continue
			}
			if c := single(v, false); c != nil {
				move(layer, i, c.x)
			}
		}
	}
}

func (s *scopeLayout) borderExtents() (lo, hi map[string]float64) {
	lo, hi = map[string]float64{}, map[string]float64{}
	for _, layer := range s.layers {
		for _, v := range layer {
			switch v.kind {
			case kBorderL:
				if x, ok := lo[v.cluster]; !ok || v.x < x {
					lo[v.cluster] = v.x
				}
			case kBorderR:
				if x, ok := hi[v.cluster]; !ok || v.x > x {
					hi[v.cluster] = v.x
				}
			}
		}
	}
	return lo, hi
}

// solve places a layer's nodes as near their targets as their gaps allow.
func (s *scopeLayout) solve(layer []*lnode, target, weight []float64) {
	n := len(layer)
	if n == 0 {
		return
	}
	offset := make([]float64, n)
	for i := 1; i < n; i++ {
		offset[i] = offset[i-1] + s.gap(layer[i-1], layer[i])
	}
	type block struct {
		w, wt      float64
		start, end int
	}
	stack := make([]block, 0, n)
	for i := 0; i < n; i++ {
		stack = append(stack, block{weight[i], weight[i] * (target[i] - offset[i]), i, i})
		for len(stack) > 1 {
			a, b := stack[len(stack)-2], stack[len(stack)-1]
			if a.wt/a.w <= b.wt/b.w {
				break
			}
			stack = append(stack[:len(stack)-2], block{a.w + b.w, a.wt + b.wt, a.start, b.end})
		}
	}
	for _, b := range stack {
		m := b.wt / b.w
		for i := b.start; i <= b.end; i++ {
			layer[i].x = m + offset[i]
		}
	}
}

// positionY places the ranks along the page, leaving room between them for
// the subgraphs' padding and titles. It returns each rank's center and
// height, and the total length.
func (s *scopeLayout) positionY() (ys, hs []float64, total float64) {
	o := s.l.opt
	n := len(s.layers)
	hs = make([]float64, n)
	for l, layer := range s.layers {
		for _, v := range layer {
			hs[l] = math.Max(hs[l], v.h)
		}
	}
	top, bottom := s.clusterExtents()
	startExt := make([]float64, n+1)
	endExt := make([]float64, n+1)
	for c, sp := range s.span {
		startExt[sp[0]] = math.Max(startExt[sp[0]], top[c])
		endExt[sp[1]] = math.Max(endExt[sp[1]], bottom[c])
	}
	ys = make([]float64, n)
	if n == 0 {
		return ys, hs, 0
	}
	y := startExt[0] + hs[0]/2
	ys[0] = y
	for l := 0; l+1 < n; l++ {
		need := o.RankSep / 2
		if ext := endExt[l] + startExt[l+1]; ext > 0 {
			need = math.Max(need, ext+o.RankSep/4)
		}
		y += hs[l]/2 + need + hs[l+1]/2
		ys[l+1] = y
	}
	total = y + hs[n-1]/2 + endExt[n-1]
	return ys, hs, total
}

// clusterExtents returns how far each subgraph's box reaches beyond its
// first and last ranks: its padding and title, and those of the
// subgraphs inside it that start or end on the same rank.
func (s *scopeLayout) clusterExtents() (top, bottom map[string]float64) {
	top, bottom = map[string]float64{}, map[string]float64{}
	pad := s.l.opt.ClusterPad
	// Innermost first: deeper paths before shallower ones.
	order := append([]string(nil), s.clusters...)
	slices.SortStableFunc(order, func(a, b string) int { return len(s.paths[b]) - len(s.paths[a]) })
	for _, c := range order {
		sp, ok := s.span[c]
		if !ok {
			continue
		}
		ts, te := s.rankTitle(c)
		t, b := pad+ts, pad+te
		for _, d := range s.clusters {
			p := s.paths[d]
			if len(p) >= 2 && p[len(p)-2] == c {
				if s.span[d][0] == sp[0] {
					t = math.Max(t, pad+ts+top[d])
				}
				if s.span[d][1] == sp[1] {
					b = math.Max(b, pad+te+bottom[d])
				}
			}
		}
		top[c], bottom[c] = t, b
	}
	return top, bottom
}

// place turns positions into the scope's geometry: items' boxes, subgraphs'
// boxes and edges' routes, in screen coordinates.
func (s *scopeLayout) place(ys, hs []float64, total float64) *result {
	res := newResult()
	o := s.l.opt
	// Extent across the ranks.
	minX, maxX := math.Inf(1), math.Inf(-1)
	for _, layer := range s.layers {
		for _, v := range layer {
			minX = math.Min(minX, v.x-v.w/2)
			maxX = math.Max(maxX, v.x+v.w/2+v.loopExtra)
		}
	}
	if math.IsInf(minX, 1) {
		return res
	}
	lo, hi := s.borderExtents()
	top, bottom := s.clusterExtents()
	// Subgraph boxes in the frame, widened for their titles.
	type frameBox struct{ x0, x1, y0, y1 float64 }
	boxes := map[string]frameBox{}
	for _, c := range s.clusters {
		sp, ok := s.span[c]
		if !ok {
			continue
		}
		b := frameBox{lo[c], hi[c], ys[sp[0]] - hs[sp[0]]/2 - top[c], ys[sp[1]] + hs[sp[1]]/2 + bottom[c]}
		need := s.l.titles[c].W + 2*o.ClusterPad
		// The title starts at the box's left, so a box too narrow for it
		// grows to the right: a title being written then moves nothing that
		// is before the box.
		switch d := need - (b.x1 - b.x0); {
		case s.dir == flow.LR:
			b.y1 = math.Max(b.y1, b.y0+need)
		case s.dir == flow.RL:
			b.y0 = math.Min(b.y0, b.y1-need)
		case d > 0:
			b.x1 += d
		}
		boxes[c] = b
		minX, maxX = math.Min(minX, b.x0), math.Max(maxX, b.x1)
		total = math.Max(total, b.y1)
	}
	// Anchored, the diagram keeps its origin, so that what was there stays
	// where it was on screen: it only moves over when something new needs
	// room before it.
	if s.l.anchor && s.anyPrev && s.id == "" && minX > 0 {
		minX = 0
	}
	minY := 0.0
	for _, b := range boxes {
		minY = math.Min(minY, b.y0)
	}
	total -= minY
	crossW := maxX - minX
	// frame (across, along) to screen.
	toScreen := func(x, y float64) geom.Point {
		x -= minX
		y -= minY
		switch s.dir {
		case flow.BT:
			return geom.Point{X: x, Y: total - y}
		case flow.LR:
			return geom.Point{X: y, Y: x}
		case flow.RL:
			return geom.Point{X: total - y, Y: x}
		}
		return geom.Point{X: x, Y: y}
	}
	if s.dir.Horizontal() {
		res.w, res.h = total, crossW
	} else {
		res.w, res.h = crossW, total
	}
	rectOf := func(x0, x1, y0, y1 float64) geom.Rect {
		a, b := toScreen(x0, y0), toScreen(x1, y1)
		return geom.Rect{X: math.Min(a.X, b.X), Y: math.Min(a.Y, b.Y), W: math.Abs(a.X - b.X), H: math.Abs(a.Y - b.Y)}
	}
	for c, b := range boxes {
		r := rectOf(b.x0, b.x1, b.y0, b.y1)
		res.clusters[c] = r
		t := s.l.titles[c]
		res.titleAt[c] = geom.Point{X: r.X + o.ClusterPad + t.W/2, Y: r.Y + o.ClusterPad*0.6 + t.H/2}
	}
	centers := make([]geom.Point, len(s.items))
	for i, it := range s.items {
		v := s.nodes[i]
		c := toScreen(v.x, ys[v.rank])
		centers[i] = c
		r := geom.RectCentered(c, it.w, it.h)
		if it.isCluster {
			res.clusters[it.id] = r
		} else {
			res.nodes[it.id] = r
		}
	}
	// Routes.
	for _, se := range s.edges {
		pts := []geom.Point{centers[se.rep[0]]}
		chain := s.chains[se]
		if se.reversed {
			for i := len(chain) - 1; i >= 0; i-- {
				pts = append(pts, toScreen(chain[i].x, ys[chain[i].rank]))
			}
		} else {
			for _, d := range chain {
				pts = append(pts, toScreen(d.x, ys[d.rank]))
			}
		}
		pts = append(pts, centers[se.rep[1]])
		rt := &route{}
		for _, d := range chain {
			if d.kind == kLabel {
				rt.labelAt, rt.hasLabel = toScreen(d.x, ys[d.rank]), true
			}
		}
		if !rt.hasLabel {
			rt.labelAt = pts[0].Lerp(pts[len(pts)-1], 0.5)
		}
		pts = s.clipEnd(pts, se.from, res, true)
		pts = s.clipEnd(pts, se.to, res, false)
		rt.pts = pts
		rt.path = geom.BasisPath(pts)
		res.edges[se.e.Index] = rt
	}
	for _, it := range s.items {
		for _, ei := range it.selfLoops {
			res.edges[ei] = s.selfLoop(res.nodes[it.id], s.l.g.Nodes[it.id].Shape, ei)
		}
	}
	return res
}

// clipEnd cuts an edge's points where they cross the outline of the end
// they leave (first) or reach.
func (s *scopeLayout) clipEnd(pts []geom.Point, e end, res *result, first bool) []geom.Point {
	if len(pts) < 2 {
		return pts
	}
	if !first {
		rev := slices.Clone(pts)
		slices.Reverse(rev)
		rev = s.clipEnd(rev, e, res, true)
		slices.Reverse(rev)
		return rev
	}
	if e.item >= 0 {
		it := s.items[e.item]
		var outline geom.Path
		if it.isCluster {
			outline = layout.Outline(flow.Rect, res.clusters[it.id])
		} else {
			outline = layout.Outline(s.l.g.Nodes[it.id].Shape, res.nodes[it.id])
		}
		pts[0] = geom.ClipToOutline(outline, pts[0], pts[1])
		return pts
	}
	// A subgraph: start where the route leaves its box.
	box := res.clusters[e.cluster]
	for i := 1; i < len(pts); i++ {
		if box.Contains(pts[i]) {
			continue
		}
		p := geom.ClipToOutline(layout.Outline(flow.Rect, box), pts[i-1], pts[i])
		return append([]geom.Point{p}, pts[i:]...)
	}
	return pts
}

// selfLoop routes an edge from a node to itself as a loop at the node's
// side: the right, or the bottom when ranks go across.
func (s *scopeLayout) selfLoop(r geom.Rect, shape flow.Shape, ei int) *route {
	fs := s.l.opt.FontSize
	outline := layout.Outline(shape, r)
	c := r.Center()
	d := 1.4 * fs
	var a, b, c1, c2, label geom.Point
	tb, hasLabel := s.l.edgeLabels[ei]
	if s.dir.Horizontal() {
		a = geom.ClipToOutline(outline, c, geom.Point{X: c.X - r.W/4, Y: r.Bottom() + 1})
		b = geom.ClipToOutline(outline, c, geom.Point{X: c.X + r.W/4, Y: r.Bottom() + 1})
		c1 = geom.Point{X: a.X - d*0.4, Y: a.Y + d}
		c2 = geom.Point{X: b.X + d*0.4, Y: b.Y + d}
		label = geom.Point{X: c.X, Y: r.Bottom() + d*0.75 + tb.H/2 + 0.2*fs}
	} else {
		a = geom.ClipToOutline(outline, c, geom.Point{X: r.Right() + 1, Y: c.Y - r.H/4})
		b = geom.ClipToOutline(outline, c, geom.Point{X: r.Right() + 1, Y: c.Y + r.H/4})
		c1 = geom.Point{X: a.X + d, Y: a.Y - d*0.4}
		c2 = geom.Point{X: b.X + d, Y: b.Y + d*0.4}
		label = geom.Point{X: r.Right() + d*0.75 + tb.W/2 + 0.3*fs, Y: c.Y}
	}
	var p geom.Path
	p.MoveTo(a).CubicTo(c1, c2, b)
	return &route{pts: []geom.Point{a, b}, path: p, labelAt: label, hasLabel: hasLabel}
}
