package flow

import "slices"

// Diff lists what changed between two parses of a growing source, by the
// stable IDs of nodes, edges and subgraphs.
type Diff struct {
	AddedNodes, ChangedNodes, RemovedNodes             []string
	AddedEdges, ChangedEdges, RemovedEdges             []string
	AddedSubgraphs, ChangedSubgraphs, RemovedSubgraphs []string
}

// Empty reports whether nothing changed.
func (d Diff) Empty() bool {
	return len(d.AddedNodes)+len(d.ChangedNodes)+len(d.RemovedNodes)+
		len(d.AddedEdges)+len(d.ChangedEdges)+len(d.RemovedEdges)+
		len(d.AddedSubgraphs)+len(d.ChangedSubgraphs)+len(d.RemovedSubgraphs) == 0
}

// Compare returns what changed from old to cur. A nil old is an empty
// graph.
func Compare(old, cur *Graph) Diff {
	if old == nil {
		old = newGraph()
	}
	var d Diff
	for _, id := range cur.NodeOrder {
		n, o := cur.Nodes[id], old.Nodes[id]
		switch {
		case o == nil:
			d.AddedNodes = append(d.AddedNodes, id)
		case !sameNode(o, n):
			d.ChangedNodes = append(d.ChangedNodes, id)
		}
	}
	for _, id := range old.NodeOrder {
		if cur.Nodes[id] == nil {
			d.RemovedNodes = append(d.RemovedNodes, id)
		}
	}
	oldEdges := map[string]*Edge{}
	for _, e := range old.Edges {
		oldEdges[e.ID] = e
	}
	curEdges := map[string]bool{}
	for _, e := range cur.Edges {
		curEdges[e.ID] = true
		o := oldEdges[e.ID]
		switch {
		case o == nil:
			d.AddedEdges = append(d.AddedEdges, e.ID)
		case !sameEdge(o, e):
			d.ChangedEdges = append(d.ChangedEdges, e.ID)
		}
	}
	for _, e := range old.Edges {
		if !curEdges[e.ID] {
			d.RemovedEdges = append(d.RemovedEdges, e.ID)
		}
	}
	for _, id := range cur.SubgraphOrder {
		s, o := cur.Subgraphs[id], old.Subgraphs[id]
		switch {
		case o == nil:
			d.AddedSubgraphs = append(d.AddedSubgraphs, id)
		case o.Title != s.Title || o.Parent != s.Parent || o.Dir != s.Dir || o.Style != s.Style || !slices.Equal(o.Classes, s.Classes):
			d.ChangedSubgraphs = append(d.ChangedSubgraphs, id)
		}
	}
	for _, id := range old.SubgraphOrder {
		if cur.Subgraphs[id] == nil {
			d.RemovedSubgraphs = append(d.RemovedSubgraphs, id)
		}
	}
	return d
}

func sameNode(a, b *Node) bool {
	return a.Label == b.Label && a.Shape == b.Shape && a.Subgraph == b.Subgraph &&
		a.Style == b.Style && a.Partial == b.Partial && slices.Equal(a.Classes, b.Classes) &&
		a.Series == b.Series && sameTable(a.Table, b.Table)
}

func sameTable(a, b *Table) bool {
	if a == nil || b == nil {
		return a == b
	}
	return slices.EqualFunc(a.Sections, b.Sections, func(x, y Section) bool {
		return x.Center == y.Center && x.Bold == y.Bold && x.Small == y.Small && x.Joined == y.Joined &&
			slices.EqualFunc(x.Rows, y.Rows, slices.Equal)
	})
}

func sameEdge(a, b *Edge) bool {
	return a.Label == b.Label && a.Line == b.Line && a.Start == b.Start && a.End == b.End &&
		a.Length == b.Length && a.Style == b.Style &&
		a.StartLabel == b.StartLabel && a.EndLabel == b.EndLabel && a.Series == b.Series
}
