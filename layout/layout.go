// Package layout holds what the layouts of every kind of diagram share:
// the text measurer they size labels with, their options, the shapes of
// nodes, and the geometry they produce for renderers.
package layout

import (
	"github.com/momaek/streameraid/flow"
	"github.com/momaek/streameraid/geom"
)

// TextStyle is the style a text is measured in.
type TextStyle struct {
	Family       string
	Size         float64
	Bold, Italic bool
	// Underline underlines the text. It does not change its size.
	Underline bool
}

// TextBox is a measured text: its size and its lines after wrapping.
type TextBox struct {
	W, H       float64
	Lines      []string
	LineHeight float64
	Style      TextStyle
}

// Measurer measures text. Line breaks in text always break; when maxWidth
// is positive, lines wider than it wrap between words (and between CJK
// characters).
type Measurer interface {
	Measure(text string, style TextStyle, maxWidth float64) TextBox
}

// Options are the sizes a layout works with. Zero fields take the
// defaults of Defaults.
type Options struct {
	FontSize   float64
	FontFamily string
	// WrapWidth is the width labels wrap at.
	WrapWidth float64
	// NodeSep is the space between nodes side by side, RankSep the space
	// between ranks, EdgeSep the space between edges passing side by side.
	NodeSep, RankSep, EdgeSep float64
	// ClusterPad is the space inside a subgraph's box around its content.
	ClusterPad float64
	// Margin is the space around the whole diagram.
	Margin float64
}

// Defaults returns o with its zero fields set to the defaults, which scale
// with the font size.
func (o Options) Defaults() Options {
	if o.FontSize <= 0 {
		o.FontSize = 14
	}
	em := o.FontSize
	if o.WrapWidth <= 0 {
		o.WrapWidth = 16 * em
	}
	if o.NodeSep <= 0 {
		o.NodeSep = 2.5 * em
	}
	if o.RankSep <= 0 {
		o.RankSep = 3.2 * em
	}
	if o.EdgeSep <= 0 {
		o.EdgeSep = 1.2 * em
	}
	if o.ClusterPad <= 0 {
		o.ClusterPad = 1.1 * em
	}
	if o.Margin <= 0 {
		o.Margin = 0.6 * em
	}
	return o
}

// TextStyle returns the style of labels at the options' font.
func (o Options) TextStyle() TextStyle {
	return TextStyle{Family: o.FontFamily, Size: o.FontSize}
}

// Flow is the geometry of a flowchart, in screen coordinates with the
// origin at the top left of the diagram.
type Flow struct {
	Width, Height float64
	// Nodes are in the graph's NodeOrder, Edges in the order of the graph's
	// edges, Clusters with every subgraph after the one holding it.
	Nodes    []NodeBox
	Edges    []EdgeRoute
	Clusters []ClusterBox
	// Crossings counts the crossings of edges between ranks, a measure of
	// how tangled the layout is.
	Crossings int
}

// NodeBox is where a node is drawn.
type NodeBox struct {
	ID    string
	Rect  geom.Rect
	Shape flow.Shape
	Label TextBox
	// Outline is the node's shape, filled and stroked by renderers.
	Outline geom.Path
	// Lines are extra lines drawn over the shape: the bars of a
	// subroutine, the lower rim of a cylinder, the dividers of a box of
	// sections.
	Lines geom.Path
	// Table holds the texts of a box of sections, whose Label is empty.
	Table *TableBox
}

// EdgeRoute is where an edge is drawn.
type EdgeRoute struct {
	ID string
	// Index is the edge's index in the graph's Edges.
	Index int
	// Path goes from the edge's source to its target, ending on their
	// outlines. Points are the points it was smoothed from.
	Path   geom.Path
	Points []geom.Point
	// Label is the edge's text, centered on LabelAt, if HasLabel.
	HasLabel bool
	Label    TextBox
	LabelAt  geom.Point
	// StartLabel and EndLabel are the texts at the edge's ends, when they
	// have lines, centered on StartAt and EndAt.
	StartLabel, EndLabel TextBox
	StartAt, EndAt       geom.Point
}

// ClusterBox is where a subgraph's box is drawn.
type ClusterBox struct {
	ID    string
	Rect  geom.Rect
	Title TextBox
	// TitleAt is the center of the title.
	TitleAt geom.Point
	Depth   int
}

// Node returns the box of node id.
func (f *Flow) Node(id string) (NodeBox, bool) {
	for _, n := range f.Nodes {
		if n.ID == id {
			return n, true
		}
	}
	return NodeBox{}, false
}
