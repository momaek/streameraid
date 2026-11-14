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
	}
	return math.Ceil(w), math.Ceil(h)
}

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
	case flow.Circle:
		p.Ellipse(c, r.W/2, r.H/2)
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
