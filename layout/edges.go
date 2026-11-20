package layout

import (
	"math"

	"github.com/momaek/streameraid/flow"
	"github.com/momaek/streameraid/geom"
)

// Marker is a part of what ends an edge: an arrowhead, a circle, a
// cross, a diamond, a crow's foot.
type Marker struct {
	Kind flow.Arrow
	// Shape is the marker's outline: filled with the edge's color when
	// Filled, filled with the background and stroked when Hollow, and only
	// stroked otherwise.
	Shape          geom.Path
	Filled, Hollow bool
	// AtEnd tells that the marker is at the edge's target, rather than at
	// its source.
	AtEnd bool
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
		var ms []Marker
		body, ms = marker(body, e.End, size)
		for i := range ms {
			ms[i].AtEnd = true
		}
		markers = append(markers, ms...)
	}
	if e.Start != flow.ArrowNone {
		rev, ms := marker(geom.Reverse(body), e.Start, size)
		body = geom.Reverse(rev)
		markers = append(markers, ms...)
	}
	return body, markers
}

// markerLength returns how far a marker reaches back from the end of its
// edge, for the texts placed beside it.
func markerLength(kind flow.Arrow, size float64) float64 {
	switch kind {
	case flow.ArrowNone:
		return 0
	case flow.ArrowDiamond, flow.ArrowHollow:
		return 1.5 * size
	case flow.ArrowZeroOne:
		return 1.6 * size
	case flow.ArrowZeroMany:
		return 1.8 * size
	}
	return size
}

// marker makes the marker at the end of p and returns p trimmed for it.
func marker(p geom.Path, kind flow.Arrow, size float64) (geom.Path, []Marker) {
	tip, before, ok := geom.PathEnd(p)
	m := Marker{Kind: kind}
	if !ok {
		return p, nil
	}
	d := tip.Sub(before)
	d = d.Scale(1 / d.Len())
	n := geom.Point{X: -d.Y, Y: d.X}
	back := func(k float64) geom.Point { return tip.Sub(d.Scale(k * size)) }
	// bar is a line across the edge at k sizes from the tip.
	bar := func(path *geom.Path, k float64) {
		c := back(k)
		path.MoveTo(c.Add(n.Scale(size * 0.5))).LineTo(c.Sub(n.Scale(size * 0.5)))
	}
	ring := func(k float64) Marker {
		r := size * 0.32
		o := Marker{Kind: kind, Hollow: true}
		o.Shape.Ellipse(back(k), r, r)
		return o
	}
	// foot is a crow's foot: three lines from a point k sizes back to the
	// tip, spread across it.
	foot := func(path *geom.Path, k float64) {
		c := back(k)
		w := size * 0.6
		path.MoveTo(c).LineTo(tip.Add(n.Scale(w)))
		path.MoveTo(c).LineTo(tip.Sub(n.Scale(w)))
	}
	switch kind {
	case flow.ArrowNormal:
		base := back(1)
		m.Shape.Polygon(tip, base.Add(n.Scale(size*0.45)), base.Sub(n.Scale(size*0.45)))
		m.Filled = true
		return geom.TrimEnd(p, size*0.85), []Marker{m}
	case flow.ArrowCircle:
		r := size * 0.38
		m.Shape.Ellipse(tip.Sub(d.Scale(r)), r, r)
		m.Filled = true
		return geom.TrimEnd(p, 2*r), []Marker{m}
	case flow.ArrowCross:
		r := size * 0.4
		c := tip.Sub(d.Scale(r * 1.1))
		a, b := d.Add(n).Scale(r*0.75), d.Sub(n).Scale(r*0.75)
		m.Shape.MoveTo(c.Sub(a)).LineTo(c.Add(a))
		m.Shape.MoveTo(c.Sub(b)).LineTo(c.Add(b))
		return geom.TrimEnd(p, r*0.2), []Marker{m}
	case flow.ArrowTriangle:
		base := back(1.1)
		m.Shape.Polygon(tip, base.Add(n.Scale(size*0.55)), base.Sub(n.Scale(size*0.55)))
		m.Hollow = true
		return geom.TrimEnd(p, size*1.1), []Marker{m}
	case flow.ArrowDiamond, flow.ArrowHollow:
		mid := back(0.75)
		m.Shape.Polygon(tip, mid.Add(n.Scale(size*0.4)), back(1.5), mid.Sub(n.Scale(size*0.4)))
		m.Filled, m.Hollow = kind == flow.ArrowDiamond, kind == flow.ArrowHollow
		return geom.TrimEnd(p, size*1.5), []Marker{m}
	case flow.ArrowOpen:
		base := back(1)
		m.Shape.MoveTo(base.Add(n.Scale(size * 0.5))).LineTo(tip).LineTo(base.Sub(n.Scale(size * 0.5)))
		return p, []Marker{m}
	case flow.ArrowLollipop:
		r := size * 0.4
		m.Shape.Ellipse(tip.Sub(d.Scale(r)), r, r)
		m.Hollow = true
		return geom.TrimEnd(p, 2*r), []Marker{m}
	case flow.ArrowOne:
		bar(&m.Shape, 0.55)
		bar(&m.Shape, 0.9)
		return p, []Marker{m}
	case flow.ArrowZeroOne:
		bar(&m.Shape, 0.55)
		return p, []Marker{m, ring(1.25)}
	case flow.ArrowMany:
		foot(&m.Shape, 0.9)
		bar(&m.Shape, 1.15)
		return p, []Marker{m}
	case flow.ArrowZeroMany:
		foot(&m.Shape, 0.9)
		return p, []Marker{m, ring(1.45)}
	}
	return p, nil
}

// EndLabelAt returns where a text at an end of an edge is centered:
// beside the edge, past its marker. path is the edge's route, from its
// source to its target.
func EndLabelAt(path geom.Path, kind flow.Arrow, tb TextBox, atEnd bool, fontSize float64) geom.Point {
	if !atEnd {
		path = geom.Reverse(path)
	}
	var line []geom.Point
	for _, part := range path.Flatten() {
		line = append(line, part...)
	}
	if len(line) < 2 {
		return geom.Point{}
	}
	// Walk back from the end to just past the marker.
	size := 0.72 * fontSize
	want := markerLength(kind, size) + 0.4*fontSize
	at, dir := line[len(line)-1], line[len(line)-1].Sub(line[len(line)-2])
	for i := len(line) - 1; i > 0; i-- {
		seg := line[i].Sub(line[i-1])
		l := seg.Len()
		if l == 0 {
			continue
		}
		dir = seg
		if want <= l {
			at = line[i].Sub(seg.Scale(want / l))
			break
		}
		want -= l
		at = line[i-1]
	}
	if l := dir.Len(); l > 0 {
		dir = dir.Scale(1 / l)
	}
	// Back along the edge by half the text's extent that way, and to the
	// side by the other half: the text stands clear of the line.
	n := geom.Point{X: -dir.Y, Y: dir.X}
	if n.X < 0 || n.X == 0 && n.Y < 0 {
		n = n.Scale(-1)
	}
	along := 0.5 * (math.Abs(dir.X)*tb.W + math.Abs(dir.Y)*tb.H)
	side := 0.5*(math.Abs(n.X)*tb.W+math.Abs(n.Y)*tb.H) + 0.3*fontSize
	return at.Sub(dir.Scale(along)).Add(n.Scale(side))
}
