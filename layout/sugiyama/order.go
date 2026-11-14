package sugiyama

import (
	"math"
	"sort"
)

// order orders the nodes of each rank to cut crossings: a depth-first
// first guess, then sweeps down and up placing each node at the mean
// position of its neighbors in the rank before, keeping the best order.
// Subgraphs stay contiguous throughout, their borders at their ends.
func (s *scopeLayout) order() {
	s.initOrder()
	best := s.snapshot()
	bestCross := s.crossings()
	stall := 0
	for iter := 0; iter < 24 && stall < 4 && bestCross > 0; iter++ {
		if iter%2 == 0 {
			for l := 1; l < len(s.layers); l++ {
				s.sortLayer(l, true)
			}
		} else {
			for l := len(s.layers) - 2; l >= 0; l-- {
				s.sortLayer(l, false)
			}
		}
		if c := s.crossings(); c < bestCross {
			best, bestCross, stall = s.snapshot(), c, 0
		} else {
			stall++
		}
	}
	s.restore(best)
	s.alignSiblings()
}

// alignSiblings gives subgraphs side by side the same order on every rank,
// so that their boxes, which span all their ranks, cannot overlap: each
// subgraph's place is the mean of its positions over its ranks, and on
// each rank the subgraphs trade the slots they hold to follow it.
func (s *scopeLayout) alignSiblings() {
	if len(s.clusters) < 2 {
		return
	}
	sum, count := map[string]float64{}, map[string]float64{}
	for _, layer := range s.layers {
		for _, v := range layer {
			if v.kind == kBorderL {
				sum[v.cluster] += float64(v.order) / float64(len(layer))
				count[v.cluster]++
			}
		}
	}
	key := func(c string) float64 { return sum[c] / count[c] }
	for l, layer := range s.layers {
		s.layers[l] = s.alignBlock(layer, 0, key)
		s.renumber(l)
	}
}

// alignBlock reorders, at one depth, the subgraph blocks among the slots
// they hold, recursing into each block.
func (s *scopeLayout) alignBlock(nodes []*lnode, depth int, key func(string) float64) []*lnode {
	type unit struct {
		cluster string
		nodes   []*lnode
	}
	var units []*unit
	byCluster := map[string]*unit{}
	for _, n := range nodes {
		if len(n.path) > depth {
			c := n.path[depth]
			if u := byCluster[c]; u != nil {
				u.nodes = append(u.nodes, n)
				continue
			}
			u := &unit{cluster: c, nodes: []*lnode{n}}
			byCluster[c] = u
			units = append(units, u)
			continue
		}
		units = append(units, &unit{nodes: []*lnode{n}})
	}
	var blocks []*unit
	for _, u := range units {
		if u.cluster != "" {
			blocks = append(blocks, u)
		}
	}
	sorted := append([]*unit(nil), blocks...)
	sort.SliceStable(sorted, func(i, j int) bool { return key(sorted[i].cluster) < key(sorted[j].cluster) })
	k := 0
	var out []*lnode
	for _, u := range units {
		if u.cluster == "" {
			out = append(out, u.nodes...)
			continue
		}
		b := sorted[k]
		k++
		// Keep the block's borders at its ends while ordering inside it.
		var left, right *lnode
		var inner []*lnode
		for _, n := range b.nodes {
			switch {
			case n.kind == kBorderL && n.cluster == b.cluster:
				left = n
			case n.kind == kBorderR && n.cluster == b.cluster:
				right = n
			default:
				inner = append(inner, n)
			}
		}
		if left != nil {
			out = append(out, left)
		}
		out = append(out, s.alignBlock(inner, depth+1, key)...)
		if right != nil {
			out = append(out, right)
		}
	}
	return out
}

func (s *scopeLayout) initOrder() {
	if s.anyPrev {
		s.orderFromPrev()
		return
	}
	visited := map[*lnode]bool{}
	fresh := make([][]*lnode, len(s.layers))
	var dfs func(n *lnode)
	dfs = func(n *lnode) {
		if visited[n] {
			return
		}
		visited[n] = true
		fresh[n.rank] = append(fresh[n.rank], n)
		for _, a := range n.down {
			dfs(a.n)
		}
	}
	starts := append([]*lnode(nil), s.nodes...)
	sort.SliceStable(starts, func(i, j int) bool { return starts[i].rank < starts[j].rank })
	for _, n := range starts {
		dfs(n)
	}
	for l, layer := range s.layers {
		for _, n := range layer {
			if !visited[n] {
				visited[n] = true
				fresh[l] = append(fresh[l], n)
			}
		}
	}
	for l := range s.layers {
		bary := map[*lnode]float64{}
		for i, n := range fresh[l] {
			bary[n] = float64(i)
		}
		s.layers[l] = s.hsort(fresh[l], 0, bary)
		s.renumber(l)
	}
}

// orderFromPrev starts from the order of an earlier layout: nodes it held
// go where they were across the ranks, and the others (new nodes, and the
// dummies of edges) at the mean of their neighbors, or after everything
// when they have none.
func (s *scopeLayout) orderFromPrev() {
	key := map[*lnode]float64{}
	hi := math.Inf(-1)
	for _, n := range s.nodes {
		if n.hasPrev {
			key[n] = n.prev
			hi = math.Max(hi, n.prev)
		}
	}
	place := func(n *lnode) {
		if _, ok := key[n]; ok || n.kind == kBorderL || n.kind == kBorderR {
			return
		}
		sum, cnt := 0.0, 0.0
		for _, adj := range [][]ladj{n.up, n.down} {
			for _, a := range adj {
				if k, ok := key[a.n]; ok {
					sum += k
					cnt++
				}
			}
		}
		if cnt > 0 {
			// A little after the mean, so that a new node level with one that
			// was there goes after it rather than push it aside.
			key[n] = sum/cnt + 0.5
		}
	}
	for pass := 0; pass < 3; pass++ {
		for l := range s.layers {
			for _, n := range s.layers[l] {
				place(n)
			}
		}
		for l := len(s.layers) - 1; l >= 0; l-- {
			for _, n := range s.layers[l] {
				place(n)
			}
		}
	}
	for l, layer := range s.layers {
		sorted := append([]*lnode(nil), layer...)
		at := func(n *lnode) float64 {
			if k, ok := key[n]; ok {
				return k
			}
			return hi + 1 + float64(n.seq)
		}
		sort.SliceStable(sorted, func(i, j int) bool {
			a, b := at(sorted[i]), at(sorted[j])
			if a != b {
				return a < b
			}
			return sorted[i].seq < sorted[j].seq
		})
		bary := map[*lnode]float64{}
		for i, n := range sorted {
			bary[n] = float64(i)
		}
		s.layers[l] = s.hsort(sorted, 0, bary)
		s.renumber(l)
	}
}

func (s *scopeLayout) renumber(l int) {
	for i, n := range s.layers[l] {
		n.order = i
	}
}

// sortLayer orders layer l by the mean positions of each node's neighbors
// in the layer above (down) or below.
func (s *scopeLayout) sortLayer(l int, down bool) {
	bary := map[*lnode]float64{}
	layer := s.layers[l]
	var other int
	if down {
		other = len(s.layers[l-1])
	} else {
		other = len(s.layers[l+1])
	}
	scale := 1.0
	if len(layer) > 1 && other > 1 {
		scale = float64(other-1) / float64(len(layer)-1)
	}
	for _, n := range layer {
		adj := n.down
		if down {
			adj = n.up
		}
		sw, sx := 0.0, 0.0
		for _, a := range adj {
			sw += a.w
			sx += a.w * float64(a.n.order)
		}
		if sw > 0 {
			bary[n] = sx / sw
		} else {
			bary[n] = float64(n.order) * scale
		}
	}
	s.layers[l] = s.hsort(layer, 0, bary)
	s.renumber(l)
}

// hsort sorts nodes by barycenter, keeping each subgraph's nodes together:
// at each depth a subgraph sorts as one unit, at the mean barycenter of
// its nodes, with its left border first and its right border last.
func (s *scopeLayout) hsort(nodes []*lnode, depth int, bary map[*lnode]float64) []*lnode {
	type unit struct {
		cluster string
		nodes   []*lnode
		key     float64
		first   int
	}
	var units []*unit
	byCluster := map[string]*unit{}
	for i, n := range nodes {
		if len(n.path) > depth {
			c := n.path[depth]
			u := byCluster[c]
			if u == nil {
				u = &unit{cluster: c, first: i}
				byCluster[c] = u
				units = append(units, u)
			}
			u.nodes = append(u.nodes, n)
			continue
		}
		units = append(units, &unit{nodes: []*lnode{n}, first: i})
	}
	for _, u := range units {
		sw, sx, all := 0.0, 0.0, 0.0
		for _, n := range u.nodes {
			all += bary[n]
			if n.kind == kBorderL || n.kind == kBorderR {
				continue
			}
			sw++
			sx += bary[n]
		}
		if sw > 0 {
			u.key = sx / sw
		} else {
			u.key = all / float64(len(u.nodes))
		}
	}
	sort.SliceStable(units, func(i, j int) bool {
		if math.Abs(units[i].key-units[j].key) > 1e-9 {
			return units[i].key < units[j].key
		}
		return units[i].first < units[j].first
	})
	var out []*lnode
	for _, u := range units {
		if u.cluster == "" {
			out = append(out, u.nodes...)
			continue
		}
		var left, right *lnode
		var inner []*lnode
		for _, n := range u.nodes {
			switch {
			case n.kind == kBorderL && n.cluster == u.cluster:
				left = n
			case n.kind == kBorderR && n.cluster == u.cluster:
				right = n
			default:
				inner = append(inner, n)
			}
		}
		if left != nil {
			out = append(out, left)
		}
		out = append(out, s.hsort(inner, depth+1, bary)...)
		if right != nil {
			out = append(out, right)
		}
	}
	return out
}

func (s *scopeLayout) snapshot() [][]*lnode {
	out := make([][]*lnode, len(s.layers))
	for l, layer := range s.layers {
		out[l] = append([]*lnode(nil), layer...)
	}
	return out
}

func (s *scopeLayout) restore(layers [][]*lnode) {
	s.layers = layers
	for l := range s.layers {
		s.renumber(l)
	}
}

// crossings counts the edge crossings between adjacent layers, with a
// Fenwick tree over the lower layer's positions.
func (s *scopeLayout) crossings() int {
	total := 0
	for l := 0; l+1 < len(s.layers); l++ {
		type pair struct{ a, b int }
		var pairs []pair
		for _, n := range s.layers[l] {
			for _, a := range n.down {
				pairs = append(pairs, pair{n.order, a.n.order})
			}
		}
		sort.Slice(pairs, func(i, j int) bool {
			if pairs[i].a != pairs[j].a {
				return pairs[i].a < pairs[j].a
			}
			return pairs[i].b < pairs[j].b
		})
		size := len(s.layers[l+1]) + 1
		tree := make([]int, size+1)
		seen := 0
		for _, p := range pairs {
			// Count earlier pairs whose lower end is to the right of p's.
			le := 0
			for i := p.b + 1; i > 0; i -= i & -i {
				le += tree[i]
			}
			total += seen - le
			for i := p.b + 1; i <= size; i += i & -i {
				tree[i]++
			}
			seen++
		}
	}
	return total
}
