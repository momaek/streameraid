package layout

import (
	"github.com/momaek/streameraid/flow"
	"github.com/momaek/streameraid/geom"
)

// Marker is what ends an edge: an arrowhead, a circle or a cross.
type Marker struct {
	Kind flow.Arrow
	// Shape is the marker's outline: filled for arrowheads and circles,
	// stroked for crosses.
	Shape  geom.Path
	Filled bool
}

// EdgeWidth returns the width of an edge's line.
func EdgeWidth(e *flow.Edge, fontSize float64) float64 {
	if e.Style.StrokeWidth > 0 {
		return e.Style.StrokeWidth
	}
	if e.Line == flow.Thick {
		return 0.25 * fontSize
	}
	return 0.11 * fontSize
}

// EdgeShapes returns an edge's line, pulled back from its ends to make room
// for its markers, and the markers.
func EdgeShapes(e *flow.Edge, r EdgeRoute, fontSize float64) (geom.Path, []Marker) {
	body := r.Path
	size := 0.72 * fontSize
	if e.Line == flow.Thick {
		size *= 1.25
	}
	var markers []Marker
	if e.End != flow.ArrowNone {
		var m Marker
		body, m = marker(body, e.End, size)
		markers = append(markers, m)
	}
	if e.Start != flow.ArrowNone {
		var m Marker
		rev, m2 := marker(geom.Reverse(body), e.Start, size)
		body, m = geom.Reverse(rev), m2
		markers = append(markers, m)
	}
	return body, markers
}

// marker makes the marker at the end of p and returns p trimmed for it.
func marker(p geom.Path, kind flow.Arrow, size float64) (geom.Path, Marker) {
	tip, before, ok := geom.PathEnd(p)
	m := Marker{Kind: kind}
	if !ok {
		return p, m
	}
	d := tip.Sub(before)
	d = d.Scale(1 / d.Len())
	n := geom.Point{X: -d.Y, Y: d.X}
	switch kind {
	case flow.ArrowNormal:
		base := tip.Sub(d.Scale(size))
		m.Shape.Polygon(tip, base.Add(n.Scale(size*0.45)), base.Sub(n.Scale(size*0.45)))
		m.Filled = true
		return geom.TrimEnd(p, size*0.85), m
	case flow.ArrowCircle:
		r := size * 0.38
		m.Shape.Ellipse(tip.Sub(d.Scale(r)), r, r)
		m.Filled = true
		return geom.TrimEnd(p, 2*r), m
	case flow.ArrowCross:
		r := size * 0.4
		c := tip.Sub(d.Scale(r * 1.1))
		a, b := d.Add(n).Scale(r*0.75), d.Sub(n).Scale(r*0.75)
		m.Shape.MoveTo(c.Sub(a)).LineTo(c.Add(a))
		m.Shape.MoveTo(c.Sub(b)).LineTo(c.Add(b))
		return geom.TrimEnd(p, r*0.2), m
	}
	return p, m
}
