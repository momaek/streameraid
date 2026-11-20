// Package flow is the model of a Mermaid flowchart and its parser.
//
// Parse reads the whole source each time, so that a caller streaming a
// diagram in passes the source so far and compares the graphs it gets
// (see Compare) to find what is new.
package flow

import "strings"

// Direction is the direction a flowchart's ranks go.
type Direction uint8

const (
	TB Direction = iota // top to bottom (TD is the same)
	BT                  // bottom to top
	LR                  // left to right
	RL                  // right to left
)

func (d Direction) String() string {
	switch d {
	case BT:
		return "BT"
	case LR:
		return "LR"
	case RL:
		return "RL"
	}
	return "TB"
}

// Horizontal reports whether ranks go across the page.
func (d Direction) Horizontal() bool { return d == LR || d == RL }

// ParseDirection reads TB, TD, BT, LR or RL.
func ParseDirection(s string) (Direction, bool) {
	switch strings.ToUpper(s) {
	case "TB", "TD":
		return TB, true
	case "BT":
		return BT, true
	case "LR":
		return LR, true
	case "RL":
		return RL, true
	}
	return TB, false
}

// Shape is the shape of a node.
type Shape uint8

const (
	Rect             Shape = iota // A[text]
	Round                         // A(text)
	Stadium                       // A([text])
	Subroutine                    // A[[text]]
	Cylinder                      // A[(text)]
	Circle                        // A((text))
	DoubleCircle                  // A(((text)))
	Asymmetric                    // A>text]
	Diamond                       // A{text}
	Hexagon                       // A{{text}}
	Parallelogram                 // A[/text/]
	ParallelogramAlt              // A[\text\]
	Trapezoid                     // A[/text\]
	TrapezoidAlt                  // A[\text/]

	// The shapes of other kinds of diagrams drawn as graphs.

	Start     // the start of a state diagram: a filled dot
	End       // an end of a state diagram: a dot in a ring
	Choice    // a choice of a state diagram: a small diamond, no label
	Bar       // a fork or join of a state diagram: a filled bar across the ranks
	Note      // a note: a box with a folded corner, in the colors of notes
	Sectioned // a box of sections, Node.Table: a class, an entity
	Cloud     // a mind map's cloud
	Bang      // a mind map's bang: a burst
)

var shapeNames = [...]string{"rect", "round", "stadium", "subroutine", "cylinder", "circle", "doublecircle",
	"asymmetric", "diamond", "hexagon", "parallelogram", "parallelogram-alt", "trapezoid", "trapezoid-alt",
	"start", "end", "choice", "bar", "note", "sectioned", "cloud", "bang"}

func (s Shape) String() string {
	if int(s) < len(shapeNames) {
		return shapeNames[s]
	}
	return "rect"
}

// Line is how an edge's line is drawn.
type Line uint8

const (
	Solid     Line = iota // -->
	Dotted                // -.->
	Thick                 // ==>
	Invisible             // ~~~
)

// Arrow is what ends an edge.
type Arrow uint8

const (
	ArrowNone   Arrow = iota
	ArrowNormal       // >
	ArrowCircle       // o
	ArrowCross        // x

	// The ends of relations in class diagrams.

	ArrowTriangle // <| a hollow triangle: inheritance, realization
	ArrowDiamond  // *  a filled diamond: composition
	ArrowHollow   // o  a hollow diamond: aggregation
	ArrowOpen     // <  an open arrowhead: association, dependency
	ArrowLollipop // () a hollow circle: a provided interface

	// The ends of relationships in ER diagrams, in crow's foot notation.

	ArrowOne      // || exactly one
	ArrowZeroOne  // |o zero or one
	ArrowMany     // }| one or more
	ArrowZeroMany // }o zero or more
)

// Style is the subset of CSS a flowchart's classDef, style and linkStyle
// set that this renderer draws. Colors are kept as written.
type Style struct {
	Fill        string
	Stroke      string
	Color       string // text color
	StrokeWidth float64
	Dash        string // stroke-dasharray, as written
	FontWeight  string
}

// Merge returns s with the fields o sets replacing its own.
func (s Style) Merge(o Style) Style {
	if o.Fill != "" {
		s.Fill = o.Fill
	}
	if o.Stroke != "" {
		s.Stroke = o.Stroke
	}
	if o.Color != "" {
		s.Color = o.Color
	}
	if o.StrokeWidth != 0 {
		s.StrokeWidth = o.StrokeWidth
	}
	if o.Dash != "" {
		s.Dash = o.Dash
	}
	if o.FontWeight != "" {
		s.FontWeight = o.FontWeight
	}
	return s
}

// IsZero reports whether s sets nothing.
func (s Style) IsZero() bool { return s == Style{} }

// Node is a node of the graph.
type Node struct {
	ID    string
	Label string // "\n" separates lines
	Shape Shape
	// Classes are the node's classes in the order given; Style its own
	// style statement. Graph.NodeStyle resolves them.
	Classes []string
	Style   Style
	// Subgraph is the innermost subgraph holding the node, or "".
	Subgraph string
	// Order is the order the node first appeared in.
	Order int
	// Partial marks a label still being written: its closing bracket has
	// not come yet.
	Partial bool
	// Defined tells whether the source gave the node a shape or label,
	// rather than only naming it.
	Defined bool
	// Table holds the sections of a Sectioned node; Label is then its first
	// line, for what names the node (a tooltip, a click target).
	Table *Table
	// Series, when above 0, colors the node with the series color of that
	// number (from 1), unless a style sets its colors: a mind map's
	// branches.
	Series int
}

// Table is the content of a box of sections, one above the other, split
// by lines: the name, attributes and methods of a class; the name and
// attributes of an entity.
type Table struct {
	Sections []Section
}

// Section is a part of a Table. Its rows have cells, which line up in
// columns across the rows of the section.
type Section struct {
	Rows [][]string
	// Center centers the rows, rather than aligning them left.
	Center bool
	// Bold draws the section's text bold; Small draws it smaller (a
	// class's annotation, as <<interface>>).
	Bold, Small bool
	// Joined draws the section with no line above it, as a part of the
	// section before: a class's name under its annotation.
	Joined bool
}

// Edge is an edge of the graph.
type Edge struct {
	// ID is stable while the source grows: the ends and the count of
	// earlier edges between the same ends.
	ID       string
	From, To string // node or subgraph IDs
	Label    string
	Line     Line
	Start    Arrow
	End      Arrow
	// Length is how many ranks the edge asks to span, from the number of
	// dashes: 1 for -->, 2 for --->.
	Length int
	// Index is the edge's number in the order edges were declared, which
	// linkStyle counts by.
	Index int
	Style Style
	// StartLabel and EndLabel are texts at the ends of the edge: the
	// multiplicities of a relation of classes.
	StartLabel, EndLabel string
	// Series, when above 0, colors the edge as Node.Series does.
	Series int
}

// Subgraph is a subgraph: a box grouping nodes and other subgraphs.
type Subgraph struct {
	ID     string
	Title  string
	Dir    Direction
	HasDir bool // a direction statement set Dir
	Parent string
	// Nodes are the IDs of the nodes directly inside, Children those of the
	// subgraphs directly inside, each in order.
	Nodes    []string
	Children []string
	Classes  []string
	Style    Style
	Order    int
	// Open marks a subgraph whose end has not come yet.
	Open bool
}

// Problem is a statement the parser could not read, or something missing
// at the end of the source.
type Problem struct {
	Line int // from 1
	Text string
	Msg  string
}

// Graph is a parsed flowchart.
type Graph struct {
	// Title is the title of the source's front matter.
	Title string
	Dir   Direction
	// HasHeader tells whether the source's header (flowchart or graph) was
	// complete. A graph without one is empty.
	HasHeader bool
	Nodes     map[string]*Node
	NodeOrder []string
	Edges     []*Edge
	Subgraphs map[string]*Subgraph
	// SubgraphOrder lists subgraphs in the order they were opened.
	SubgraphOrder []string
	ClassDefs     map[string]Style
	Problems      []Problem
	// Final tells whether the source was complete when parsed.
	Final bool
}

// NewGraph returns an empty graph, for the parsers of the other kinds of
// diagrams drawn as graphs.
func NewGraph() *Graph { return newGraph() }

func newGraph() *Graph {
	return &Graph{
		Nodes:     map[string]*Node{},
		Subgraphs: map[string]*Subgraph{},
		ClassDefs: map[string]Style{},
	}
}

// NodeStyle resolves the style a node is drawn with: the default class,
// then its classes in order, then its own style statement.
func (g *Graph) NodeStyle(n *Node) Style {
	st := g.ClassDefs["default"]
	for _, c := range n.Classes {
		st = st.Merge(g.ClassDefs[c])
	}
	return st.Merge(n.Style)
}

// SubgraphStyle resolves the style a subgraph's box is drawn with.
func (g *Graph) SubgraphStyle(s *Subgraph) Style {
	var st Style
	for _, c := range s.Classes {
		st = st.Merge(g.ClassDefs[c])
	}
	return st.Merge(s.Style)
}

// RootSubgraphs returns the subgraphs not inside another, in order.
func (g *Graph) RootSubgraphs() []string {
	var out []string
	for _, id := range g.SubgraphOrder {
		if g.Subgraphs[id].Parent == "" {
			out = append(out, id)
		}
	}
	return out
}

// Contains reports whether subgraph sg holds node or subgraph id, at any
// depth.
func (g *Graph) Contains(sg, id string) bool {
	parent := ""
	if n := g.Nodes[id]; n != nil {
		parent = n.Subgraph
	} else if s := g.Subgraphs[id]; s != nil {
		parent = s.Parent
	}
	for parent != "" {
		if parent == sg {
			return true
		}
		parent = g.Subgraphs[parent].Parent
	}
	return false
}
