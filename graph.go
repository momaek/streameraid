package streameraid

import (
	"github.com/momaek/streameraid/class"
	"github.com/momaek/streameraid/er"
	"github.com/momaek/streameraid/flow"
	"github.com/momaek/streameraid/layout"
	"github.com/momaek/streameraid/layout/sugiyama"
	"github.com/momaek/streameraid/layout/tree"
	"github.com/momaek/streameraid/mindmap"
	"github.com/momaek/streameraid/state"
)

// IsGraph reports whether diagrams of a kind are drawn as graphs: nodes
// and the edges between them, parsed into a flow.Graph. Flowcharts, state
// diagrams, class diagrams, ER diagrams and mind maps are.
func IsGraph(k Kind) bool {
	_, ok := graphParsers[k]
	return ok
}

var graphParsers = map[Kind]func(src string, final bool) *flow.Graph{
	Flowchart: flow.Parse,
	State:     state.Parse,
	Class:     class.Parse,
	ER:        er.Parse,
	Mindmap:   mindmap.Parse,
}

// ParseGraph parses a diagram drawn as a graph. final tells whether the
// source is complete, as flow.Parse says. It returns nil for other kinds.
func ParseGraph(k Kind, src string, final bool) *flow.Graph {
	if p := graphParsers[k]; p != nil {
		return p(src, final)
	}
	return nil
}

// LayoutGraph lays out a diagram drawn as a graph. prev, when not nil, is
// the layout of the diagram before its last change: the new layout follows
// it, and with anchor also keeps what it holds in place, for a diagram
// streaming in.
func LayoutGraph(k Kind, g *flow.Graph, m layout.Measurer, opt layout.Options, prev *layout.Flow, anchor bool) *layout.Flow {
	if k == Mindmap {
		return tree.LayoutWith(g, m, opt, tree.Hints{Prev: prev, Anchor: anchor && prev != nil})
	}
	if k == Class || k == ER {
		// Relations carry texts at both ends and in the middle: they need
		// longer edges to keep them apart.
		opt = opt.Defaults()
		opt.RankSep = max(opt.RankSep, 4.6*opt.FontSize)
	}
	return sugiyama.LayoutWith(g, m, opt, sugiyama.Hints{Prev: prev, Anchor: anchor && prev != nil})
}
