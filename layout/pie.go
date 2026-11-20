package layout

import (
	"math"

	"github.com/momaek/streameraid/geom"
)

// Pie is the geometry of a pie chart, in screen coordinates with the origin
// at the top left of the chart: the pie, and a legend to its right.
type Pie struct {
	Width, Height float64
	// Title is the chart's title, centered on TitleAt above the pie, when
	// it has one.
	Title   TextBox
	TitleAt geom.Point
	Center  geom.Point
	Radius  float64
	Slices  []PieSlice
}

// PieSlice is a slice and its row of the legend.
type PieSlice struct {
	ID string
	// Series is the slice's color, from 1.
	Series int
	// Start and End are the slice's angles, in radians clockwise from
	// straight up.
	Start, End float64
	// Swatch is the slice's color in the legend, beside its label and its
	// share (and value), each centered on its point.
	Swatch  geom.Rect
	Label   TextBox
	LabelAt geom.Point
	Share   TextBox
	ShareAt geom.Point
}

// Wedge returns the outline of a slice of a circle, between two angles in
// radians clockwise from straight up. A slice of the whole circle is the
// circle.
func Wedge(c geom.Point, r, a0, a1 float64) geom.Path {
	var p geom.Path
	if a1-a0 >= 2*math.Pi-1e-9 {
		p.Ellipse(c, r, r)
		return p
	}
	if a1 <= a0 {
		return p
	}
	at := func(a float64) geom.Point { return geom.Point{X: c.X + r*math.Sin(a), Y: c.Y - r*math.Cos(a)} }
	p.MoveTo(c).LineTo(at(a0))
	// Arcs of at most a quarter turn, each a cubic.
	n := int(math.Ceil((a1 - a0) / (math.Pi / 2)))
	step := (a1 - a0) / float64(n)
	k := 4.0 / 3 * math.Tan(step/4) * r
	for i := 0; i < n; i++ {
		s, e := a0+float64(i)*step, a0+float64(i+1)*step
		ps, pe := at(s), at(e)
		ts := geom.Point{X: math.Cos(s), Y: math.Sin(s)}
		te := geom.Point{X: math.Cos(e), Y: math.Sin(e)}
		p.CubicTo(ps.Add(ts.Scale(k)), pe.Sub(te.Scale(k)), pe)
	}
	p.Close()
	return p
}
