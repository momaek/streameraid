// Package streameraid renders Mermaid diagrams in pure Go: without a
// browser, a WebView or JavaScript.
//
// RenderSVG draws a diagram's source as SVG: flowcharts, sequence
// diagrams, state diagrams, class diagrams, ER diagrams, mind maps and pie
// charts. The packages under it parse (flow, seq, state, class, er,
// mindmap, pie), lay out (layout and its subpackages) and draw
// (render/svg) diagrams, and the module github.com/momaek/streameraid/mygo
// draws them natively in MyGo, as they stream in.
package streameraid

import (
	"errors"
	"fmt"
	"github.com/momaek/streameraid/flow"
	"strings"

	"github.com/momaek/streameraid/layout"
	"github.com/momaek/streameraid/layout/piechart"
	"github.com/momaek/streameraid/layout/sequence"
	"github.com/momaek/streameraid/measure"
	"github.com/momaek/streameraid/pie"
	"github.com/momaek/streameraid/render/svg"
	"github.com/momaek/streameraid/seq"
)

// Kind is the kind of a diagram.
type Kind string

const (
	Flowchart Kind = "flowchart"
	Sequence  Kind = "sequenceDiagram"
	State     Kind = "stateDiagram"
	Class     Kind = "classDiagram"
	ER        Kind = "erDiagram"
	Mindmap   Kind = "mindmap"
	Pie       Kind = "pie"
)

// Kinds are the kinds of diagrams drawn.
var Kinds = []Kind{Flowchart, Sequence, State, Class, ER, Mindmap, Pie}

// Supported reports whether diagrams of a kind are drawn.
func Supported(k Kind) bool {
	for _, s := range Kinds {
		if k == s {
			return true
		}
	}
	return false
}

// Detect returns the kind a diagram's source declares in its first
// statement: graph and flowchart are flowcharts. It returns the keyword as
// written for other kinds, and "" when there is none yet.
func Detect(src string) Kind {
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	inFront := false
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if t == "---" && (i == 0 || inFront) {
			inFront = !inFront
			continue
		}
		if inFront || t == "" || strings.HasPrefix(t, "%%") {
			continue
		}
		word := strings.FieldsFunc(t, func(r rune) bool { return r == ' ' || r == '\t' || r == ';' })[0]
		switch word {
		case "graph", "flowchart", "flowchart-elk":
			return Flowchart
		case "stateDiagram-v2":
			return State
		case "classDiagram-v2":
			return Class
		}
		return Kind(word)
	}
	return ""
}

// ErrUnsupported is returned for kinds of diagrams that are not drawn yet.
var ErrUnsupported = errors.New("streameraid: unsupported diagram type")

// Options configure rendering.
type Options struct {
	// Dark draws for a dark background.
	Dark bool
	// Background fills the diagram's background; "" leaves it transparent.
	Background string
	// FontSize is the size of labels; 0 is 14.
	FontSize float64
	// Theme replaces the light or dark theme.
	Theme *svg.Theme
	// Measurer measures labels; nil measures with the Go fonts.
	Measurer layout.Measurer
}

// Problem is a statement the parser could not read.
type Problem = flow.Problem

// Result is a rendered diagram.
type Result struct {
	SVG           string
	Width, Height float64
	// Problems are the statements that were skipped.
	Problems []Problem
}

var defaultMeasurer = &measure.Default{}

// Render draws a complete diagram as SVG.
func Render(src string, opt Options) (*Result, error) {
	m := opt.Measurer
	if m == nil {
		m = defaultMeasurer
	}
	lo := layout.Options{FontSize: opt.FontSize}.Defaults()
	th := svg.Light()
	if opt.Dark {
		th = svg.Dark()
	}
	if opt.Theme != nil {
		th = *opt.Theme
	}
	if opt.Background != "" {
		th.Background = opt.Background
	}
	switch k := Detect(src); {
	case IsGraph(k):
		g := ParseGraph(k, src, true)
		if !g.HasHeader {
			return nil, fmt.Errorf("streameraid: %s", g.Problems[0].Msg)
		}
		f := LayoutGraph(k, g, m, lo, nil, false)
		return &Result{SVG: svg.Flow(g, f, th, lo.FontSize), Width: f.Width, Height: f.Height, Problems: g.Problems}, nil
	case k == Pie:
		c := pie.Parse(src, true)
		if !c.HasHeader {
			return nil, fmt.Errorf("streameraid: %s", c.Problems[0].Msg)
		}
		p := piechart.Layout(c, m, lo)
		return &Result{SVG: svg.Pie(p, th, lo.FontSize), Width: p.Width, Height: p.Height, Problems: c.Problems}, nil
	case k == Sequence:
		d := seq.Parse(src, true)
		if !d.HasHeader {
			return nil, fmt.Errorf("streameraid: %s", d.Problems[0].Msg)
		}
		s := sequence.Layout(d, m, lo)
		res := &Result{SVG: svg.Sequence(s, th, lo.FontSize), Width: s.Width, Height: s.Height}
		for _, p := range d.Problems {
			res.Problems = append(res.Problems, Problem(p))
		}
		return res, nil
	case k == "":
		return nil, errors.New("streameraid: empty diagram")
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupported, k)
	}
}

// RenderSVG draws a complete diagram as SVG.
func RenderSVG(src string, opt Options) (string, error) {
	r, err := Render(src, opt)
	if err != nil {
		return "", err
	}
	return r.SVG, nil
}
