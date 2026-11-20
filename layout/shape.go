package layout

import (
	"math"

	"github.com/momaek/streameraid/flow"
	"github.com/momaek/streameraid/geom"
)

// NodeSize returns the size of a node of the given shape around a label.
func NodeSize(shape flow.Shape, label TextBox, fontSize float64) (w, h float64) {
	px, py := 1.1*fontSize, 0.65*fontSize
	lw, lh := label.W, label.H
	if lh == 0 {
		lh = 1.4 * fontSize
	}
	w, h = lw+2*px, lh+2*py
	switch shape {
	case flow.Round:
	case flow.Stadium:
		w = lw + h + px/2
	case flow.Subroutine:
		w += 1.2 * fontSize
	case flow.Cylinder:
		// The caps take a fixed share of the height, so that the height
		// does not change with the label's width.
		h /= 1 - 2*cylinderCap
	case flow.Circle, flow.DoubleCircle:
		d := math.Hypot(lw+fontSize, lh+0.6*fontSize)
		d = math.Max(d, 2.6*fontSize)
		if shape == flow.DoubleCircle {
			d += 0.6 * fontSize
		}
		w, h = d, d
	case flow.Asymmetric:
		w += h / 3
	case flow.Diamond:
		// The label's box sits inside the diamond: a/(2P) + b/(2Q) <= 1.
		a, b := lw+fontSize, lh+0.6*fontSize
		w, h = a/0.6, b/0.4
		h = math.Max(h, 3*fontSize)
		w = math.Max(w, h)
	case flow.Hexagon:
		w += h / 2
	case flow.Parallelogram, flow.ParallelogramAlt, flow.Trapezoid, flow.TrapezoidAlt:
		w += h * 0.8
	case flow.Start, flow.End:
		w, h = 1.3*fontSize, 1.3*fontSize
	case flow.Choice:
		w, h = 1.8*fontSize, 1.8*fontSize
	case flow.Bar:
		// Across a top-to-bottom diagram; layouts going across swap it.
		w, h = 5*fontSize, 0.5*fontSize
	case flow.Note:
		w += 0.6 * fontSize
	case flow.Cloud, flow.Bang:
		// The label's box sits inside an ellipse; a cloud's bumps and a
		// bang's spikes go around it.
		rx, ry := (lw/2+0.5*fontSize)*math.Sqrt2, (lh/2+0.3*fontSize)*math.Sqrt2
		k := 1.12
		if shape == flow.Bang {
			k = 1.3
		}
		w, h = 2*rx*k, 2*ry*k
	}
	return math.Ceil(w), math.Ceil(h)
}

// FilledShape reports whether a shape is filled with the color of lines
// rather than of nodes: the start of a state diagram, a fork or join.
func FilledShape(shape flow.Shape) bool { return shape == flow.Start || shape == flow.Bar }

// FilledDecorations reports whether a shape's decorations are filled with
// the color of lines rather than stroked: the dot of a state diagram's
// end.
func FilledDecorations(shape flow.Shape) bool { return shape == flow.End }

// cylinderCap is the height of a cylinder's cap (half an ellipse), as a
// share of the cylinder's height.
const cylinderCap = 0.15

func cylinderRY(h float64) float64 { return h * cylinderCap }

// Outline returns a node's shape in r.
func Outline(shape flow.Shape, r geom.Rect) geom.Path {
	var p geom.Path
	x0, y0, x1, y1 := r.X, r.Y, r.Right(), r.Bottom()
	c := r.Center()
	pt := func(x, y float64) geom.Point { return geom.Point{X: x, Y: y} }
	switch shape {
	case flow.Round:
		p.RoundRect(r, math.Min(r.H/4, 8))
	case flow.Stadium:
		p.RoundRect(r, r.H/2)
	case flow.Circle, flow.Start, flow.End:
		p.Ellipse(c, r.W/2, r.H/2)
	case flow.Choice:
		p.Polygon(pt(c.X, y0), pt(x1, c.Y), pt(c.X, y1), pt(x0, c.Y))
	case flow.Bar:
		p.RoundRect(r, math.Min(r.W, r.H)/4)
	case flow.Note:
		f := math.Min(r.H/3, 0.6*noteFold(r))
		p.Polygon(pt(x0, y0), pt(x1-f, y0), pt(x1, y0+f), pt(x1, y1), pt(x0, y1))
	case flow.Sectioned:
		p.RoundRect(r, math.Min(r.H/8, 4))
	case flow.Cloud:
		cloud(&p, r)
	case flow.Bang:
		bang(&p, r)
	case flow.DoubleCircle:
		p.Ellipse(c, r.W/2, r.H/2)
	case flow.Cylinder:
		ry := cylinderRY(r.H)
		rx := r.W / 2
		k := 0.5522847498307936
		p.MoveTo(pt(x0, y0+ry))
		p.CubicTo(pt(x0, y0+ry-ry*k), pt(c.X-rx*k, y0), pt(c.X, y0))
		p.CubicTo(pt(c.X+rx*k, y0), pt(x1, y0+ry-ry*k), pt(x1, y0+ry))
		p.LineTo(pt(x1, y1-ry))
		p.CubicTo(pt(x1, y1-ry+ry*k), pt(c.X+rx*k, y1), pt(c.X, y1))
		p.CubicTo(pt(c.X-rx*k, y1), pt(x0, y1-ry+ry*k), pt(x0, y1-ry))
		p.Close()
	case flow.Asymmetric:
		n := r.H / 3
		p.Polygon(pt(x0, y0), pt(x1, y0), pt(x1, y1), pt(x0, y1), pt(x0+n, c.Y))
	case flow.Diamond:
		p.Polygon(pt(c.X, y0), pt(x1, c.Y), pt(c.X, y1), pt(x0, c.Y))
	case flow.Hexagon:
		n := r.H / 4
		p.Polygon(pt(x0+n, y0), pt(x1-n, y0), pt(x1, c.Y), pt(x1-n, y1), pt(x0+n, y1), pt(x0, c.Y))
	case flow.Parallelogram:
		n := r.H * 0.4
		p.Polygon(pt(x0+n, y0), pt(x1, y0), pt(x1-n, y1), pt(x0, y1))
	case flow.ParallelogramAlt:
		n := r.H * 0.4
		p.Polygon(pt(x0, y0), pt(x1-n, y0), pt(x1, y1), pt(x0+n, y1))
	case flow.Trapezoid:
		n := r.H * 0.4
		p.Polygon(pt(x0+n, y0), pt(x1-n, y0), pt(x1, y1), pt(x0, y1))
	case flow.TrapezoidAlt:
		n := r.H * 0.4
		p.Polygon(pt(x0, y0), pt(x1, y0), pt(x1-n, y1), pt(x0+n, y1))
	default: // Rect, Subroutine
		p.RoundRect(r, 0)
	}
	return p
}

// Decorations returns the lines a shape draws over its outline: the inner
// ring of a double circle, the bars of a subroutine, the front rim of a
// cylinder's top.
func Decorations(shape flow.Shape, r geom.Rect, fontSize float64) geom.Path {
	var p geom.Path
	switch shape {
	case flow.Subroutine:
		d := 0.5 * fontSize
		p.MoveTo(geom.Point{X: r.X + d, Y: r.Y}).LineTo(geom.Point{X: r.X + d, Y: r.Bottom()})
		p.MoveTo(geom.Point{X: r.Right() - d, Y: r.Y}).LineTo(geom.Point{X: r.Right() - d, Y: r.Bottom()})
	case flow.DoubleCircle:
		d := 0.3 * fontSize
		c := r.Center()
		p.Ellipse(c, r.W/2-d, r.H/2-d)
	case flow.End:
		c := r.Center()
		p.Ellipse(c, r.W*0.3, r.H*0.3)
	case flow.Note:
		f := math.Min(r.H/3, 0.6*noteFold(r))
		p.MoveTo(geom.Point{X: r.Right() - f, Y: r.Y}).LineTo(geom.Point{X: r.Right() - f, Y: r.Y + f}).LineTo(geom.Point{X: r.Right(), Y: r.Y + f})
	case flow.Cylinder:
		ry := cylinderRY(r.H)
		rx := r.W / 2
		c := r.Center()
		k := 0.5522847498307936
		y := r.Y + ry
		p.MoveTo(geom.Point{X: r.X, Y: y})
		p.CubicTo(geom.Point{X: r.X, Y: y + ry*k}, geom.Point{X: c.X - rx*k, Y: y + ry}, geom.Point{X: c.X, Y: y + ry})
		p.CubicTo(geom.Point{X: c.X + rx*k, Y: y + ry}, geom.Point{X: r.Right(), Y: y + ry*k}, geom.Point{X: r.Right(), Y: y})
	}
	return p
}

// LabelCenter returns where a node's label is centered: the middle of the
// box, below a cylinder's top.
func LabelCenter(shape flow.Shape, r geom.Rect) geom.Point {
	c := r.Center()
	if shape == flow.Cylinder {
		c.Y += cylinderRY(r.H) / 2
	}
	return c
}

// noteFold is the size of a note's folded corner, before it is capped by
// the note's height.
func noteFold(r geom.Rect) float64 { return math.Min(r.W, r.H) * 0.5 }

// cloud traces a cloud in r: bumps along an ellipse a little inside it.
func cloud(p *geom.Path, r geom.Rect) {
	c := r.Center()
	rx, ry := r.W/2/1.12, r.H/2/1.12
	// About one bump per 1.3 times the height of the ellipse's band.
	per := math.Pi * (rx + ry) / max(ry*0.7, 1)
	n := max(8, int(per))
	at := func(t float64) geom.Point {
		return geom.Point{X: c.X + rx*math.Cos(t), Y: c.Y + ry*math.Sin(t)}
	}
	for i := 0; i < n; i++ {
		t0 := -math.Pi/2 + 2*math.Pi*float64(i)/float64(n)
		t1 := -math.Pi/2 + 2*math.Pi*float64(i+1)/float64(n)
		a, b := at(t0), at(t1)
		if i == 0 {
			p.MoveTo(a)
		}
		// The bump bulges outward, to the edge of r.
		m := at((t0 + t1) / 2)
		out := m.Sub(c)
		out = geom.Point{X: out.X / rx * (r.W/2 - rx), Y: out.Y / ry * (r.H/2 - ry)}.Scale(2.2)
		p.CubicTo(a.Add(out.Scale(0.9)).Lerp(m, 0.15), b.Add(out.Scale(0.9)).Lerp(m, 0.15), b)
	}
	p.Close()
}

// bang traces a burst in r: spikes out from an ellipse inside it.
func bang(p *geom.Path, r geom.Rect) {
	c := r.Center()
	const n = 14
	var pts []geom.Point
	for i := 0; i < 2*n; i++ {
		t := -math.Pi/2 + math.Pi*float64(i)/n
		k := 1.0
		if i%2 == 1 {
			k = 1 / 1.3
		}
		pts = append(pts, geom.Point{X: c.X + r.W/2*k*math.Cos(t), Y: c.Y + r.H/2*k*math.Sin(t)})
	}
	p.Polygon(pts...)
}
