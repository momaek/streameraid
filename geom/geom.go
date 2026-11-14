// Package geom holds the geometry shared by layout and the renderers:
// points, rectangles and paths, in screen coordinates (y grows downward).
package geom

import "math"

// Point is a point in screen coordinates.
type Point struct{ X, Y float64 }

// Add returns p+q.
func (p Point) Add(q Point) Point { return Point{p.X + q.X, p.Y + q.Y} }

// Sub returns p-q.
func (p Point) Sub(q Point) Point { return Point{p.X - q.X, p.Y - q.Y} }

// Scale returns p*k.
func (p Point) Scale(k float64) Point { return Point{p.X * k, p.Y * k} }

// Len returns the distance of p from the origin.
func (p Point) Len() float64 { return math.Hypot(p.X, p.Y) }

// Lerp returns the point t of the way from p to q.
func (p Point) Lerp(q Point, t float64) Point {
	return Point{p.X + (q.X-p.X)*t, p.Y + (q.Y-p.Y)*t}
}

// Rect is an axis-aligned rectangle.
type Rect struct{ X, Y, W, H float64 }

// RectCentered returns the rectangle of size w×h centered on c.
func RectCentered(c Point, w, h float64) Rect { return Rect{c.X - w/2, c.Y - h/2, w, h} }

// Center returns the center of r.
func (r Rect) Center() Point { return Point{r.X + r.W/2, r.Y + r.H/2} }

// Right returns the x of r's right edge.
func (r Rect) Right() float64 { return r.X + r.W }

// Bottom returns the y of r's bottom edge.
func (r Rect) Bottom() float64 { return r.Y + r.H }

// Empty reports whether r has no area.
func (r Rect) Empty() bool { return r.W <= 0 || r.H <= 0 }

// Union returns the smallest rectangle holding r and o. An empty rectangle
// adds nothing.
func (r Rect) Union(o Rect) Rect {
	if r.Empty() {
		return o
	}
	if o.Empty() {
		return r
	}
	x0, y0 := math.Min(r.X, o.X), math.Min(r.Y, o.Y)
	x1, y1 := math.Max(r.Right(), o.Right()), math.Max(r.Bottom(), o.Bottom())
	return Rect{x0, y0, x1 - x0, y1 - y0}
}

// Inset returns r shrunk by d on every side (grown for negative d).
func (r Rect) Inset(d float64) Rect { return Rect{r.X + d, r.Y + d, r.W - 2*d, r.H - 2*d} }

// Translate returns r moved by d.
func (r Rect) Translate(d Point) Rect { return Rect{r.X + d.X, r.Y + d.Y, r.W, r.H} }

// Contains reports whether p is inside r.
func (r Rect) Contains(p Point) bool {
	return p.X >= r.X && p.X <= r.Right() && p.Y >= r.Y && p.Y <= r.Bottom()
}

// OpKind is the kind of a path operation.
type OpKind uint8

const (
	MoveTo OpKind = iota
	LineTo
	CubicTo // Pts[0], Pts[1] are control points, Pts[2] the end point
	Close
)

// Op is one operation of a path.
type Op struct {
	Kind OpKind
	Pts  [3]Point
}

// Path is a sequence of subpaths made of lines and cubic curves.
type Path struct{ Ops []Op }

// MoveTo starts a new subpath at p.
func (p *Path) MoveTo(pt Point) *Path {
	p.Ops = append(p.Ops, Op{Kind: MoveTo, Pts: [3]Point{pt}})
	return p
}

// LineTo adds a line to pt.
func (p *Path) LineTo(pt Point) *Path {
	p.Ops = append(p.Ops, Op{Kind: LineTo, Pts: [3]Point{pt}})
	return p
}

// CubicTo adds a cubic Bézier curve.
func (p *Path) CubicTo(c1, c2, pt Point) *Path {
	p.Ops = append(p.Ops, Op{Kind: CubicTo, Pts: [3]Point{c1, c2, pt}})
	return p
}

// Close closes the current subpath.
func (p *Path) Close() *Path {
	p.Ops = append(p.Ops, Op{Kind: Close})
	return p
}

// Polygon adds a closed subpath through pts.
func (p *Path) Polygon(pts ...Point) *Path {
	for i, pt := range pts {
		if i == 0 {
			p.MoveTo(pt)
		} else {
			p.LineTo(pt)
		}
	}
	return p.Close()
}

// kappa is the control point distance of a quarter circle drawn as a cubic.
const kappa = 0.5522847498307936

// Ellipse adds a closed ellipse centered on c with radii rx, ry.
func (p *Path) Ellipse(c Point, rx, ry float64) *Path {
	kx, ky := rx*kappa, ry*kappa
	p.MoveTo(Point{c.X + rx, c.Y})
	p.CubicTo(Point{c.X + rx, c.Y + ky}, Point{c.X + kx, c.Y + ry}, Point{c.X, c.Y + ry})
	p.CubicTo(Point{c.X - kx, c.Y + ry}, Point{c.X - rx, c.Y + ky}, Point{c.X - rx, c.Y})
	p.CubicTo(Point{c.X - rx, c.Y - ky}, Point{c.X - kx, c.Y - ry}, Point{c.X, c.Y - ry})
	p.CubicTo(Point{c.X + kx, c.Y - ry}, Point{c.X + rx, c.Y - ky}, Point{c.X + rx, c.Y})
	return p.Close()
}

// RoundRect adds a closed rectangle with corners rounded by radius.
func (p *Path) RoundRect(r Rect, radius float64) *Path {
	radius = math.Min(radius, math.Min(r.W, r.H)/2)
	if radius <= 0 {
		return p.Polygon(Point{r.X, r.Y}, Point{r.Right(), r.Y}, Point{r.Right(), r.Bottom()}, Point{r.X, r.Bottom()})
	}
	k := radius * (1 - kappa)
	x0, y0, x1, y1 := r.X, r.Y, r.Right(), r.Bottom()
	p.MoveTo(Point{x0 + radius, y0})
	p.LineTo(Point{x1 - radius, y0})
	p.CubicTo(Point{x1 - k, y0}, Point{x1, y0 + k}, Point{x1, y0 + radius})
	p.LineTo(Point{x1, y1 - radius})
	p.CubicTo(Point{x1, y1 - k}, Point{x1 - k, y1}, Point{x1 - radius, y1})
	p.LineTo(Point{x0 + radius, y1})
	p.CubicTo(Point{x0 + k, y1}, Point{x0, y1 - k}, Point{x0, y1 - radius})
	p.LineTo(Point{x0, y0 + radius})
	p.CubicTo(Point{x0, y0 + k}, Point{x0 + k, y0}, Point{x0 + radius, y0})
	return p.Close()
}

// Append adds the operations of q to p.
func (p *Path) Append(q Path) *Path {
	p.Ops = append(p.Ops, q.Ops...)
	return p
}

// Transform returns p with every point mapped by f.
func (p Path) Transform(f func(Point) Point) Path {
	out := Path{Ops: make([]Op, len(p.Ops))}
	for i, op := range p.Ops {
		out.Ops[i] = op
		n := 1
		if op.Kind == CubicTo {
			n = 3
		} else if op.Kind == Close {
			n = 0
		}
		for j := 0; j < n; j++ {
			out.Ops[i].Pts[j] = f(op.Pts[j])
		}
	}
	return out
}

// Flatten returns the subpaths of p as polylines, approximating curves with
// segments. Closed subpaths repeat their first point at the end.
func (p Path) Flatten() [][]Point {
	var out [][]Point
	var cur []Point
	for _, op := range p.Ops {
		switch op.Kind {
		case MoveTo:
			if len(cur) > 0 {
				out = append(out, cur)
			}
			cur = []Point{op.Pts[0]}
		case LineTo:
			cur = append(cur, op.Pts[0])
		case CubicTo:
			if len(cur) == 0 {
				cur = []Point{op.Pts[0]}
			}
			p0 := cur[len(cur)-1]
			const steps = 12
			for i := 1; i <= steps; i++ {
				cur = append(cur, cubicAt(p0, op.Pts[0], op.Pts[1], op.Pts[2], float64(i)/steps))
			}
		case Close:
			if len(cur) > 0 {
				cur = append(cur, cur[0])
				out = append(out, cur)
				cur = nil
			}
		}
	}
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out
}

func cubicAt(p0, p1, p2, p3 Point, t float64) Point {
	u := 1 - t
	a, b, c, d := u*u*u, 3*u*u*t, 3*u*t*t, t*t*t
	return Point{a*p0.X + b*p1.X + c*p2.X + d*p3.X, a*p0.Y + b*p1.Y + c*p2.Y + d*p3.Y}
}

// SegmentIntersect returns where segment a0-a1 meets segment b0-b1, with t
// the fraction along a0-a1.
func SegmentIntersect(a0, a1, b0, b1 Point) (pt Point, t float64, ok bool) {
	d1 := a1.Sub(a0)
	d2 := b1.Sub(b0)
	den := d1.X*d2.Y - d1.Y*d2.X
	if math.Abs(den) < 1e-12 {
		return Point{}, 0, false
	}
	w := b0.Sub(a0)
	t = (w.X*d2.Y - w.Y*d2.X) / den
	u := (w.X*d1.Y - w.Y*d1.X) / den
	if t < 0 || t > 1 || u < 0 || u > 1 {
		return Point{}, 0, false
	}
	return a0.Lerp(a1, t), t, true
}

// ClipToOutline returns the point where the segment from inside (a point
// inside outline) to outside first crosses the outline. When it does not
// cross, it returns inside.
func ClipToOutline(outline Path, inside, outside Point) Point {
	best, bestT := inside, math.Inf(1)
	for _, poly := range outline.Flatten() {
		for i := 1; i < len(poly); i++ {
			if pt, t, ok := SegmentIntersect(inside, outside, poly[i-1], poly[i]); ok && t < bestT {
				best, bestT = pt, t
			}
		}
	}
	// The segment may end inside the outline (a target that overlaps the
	// shape): extend it to find the boundary in that direction.
	if math.IsInf(bestT, 1) {
		d := outside.Sub(inside)
		if l := d.Len(); l > 0 {
			far := inside.Add(d.Scale(1e4 / l))
			for _, poly := range outline.Flatten() {
				for i := 1; i < len(poly); i++ {
					if pt, t, ok := SegmentIntersect(inside, far, poly[i-1], poly[i]); ok && t < bestT {
						best, bestT = pt, t
					}
				}
			}
		}
	}
	return best
}

// BasisPath returns a smooth curve through the ends of pts that follows the
// points between them, as d3's curveBasis draws: a uniform cubic B-spline
// with clamped ends. Two points give a straight line.
func BasisPath(pts []Point) Path {
	var p Path
	if len(pts) == 0 {
		return p
	}
	p.MoveTo(pts[0])
	if len(pts) == 2 {
		p.LineTo(pts[1])
		return p
	}
	if len(pts) == 1 {
		return p
	}
	// Same construction as d3.curveBasis.
	var x0, y0, x1, y1 float64
	state := 0
	bez := func(x, y float64) {
		p.CubicTo(
			Point{(2*x0 + x1) / 3, (2*y0 + y1) / 3},
			Point{(x0 + 2*x1) / 3, (y0 + 2*y1) / 3},
			Point{(x0 + 4*x1 + x) / 6, (y0 + 4*y1 + y) / 6},
		)
	}
	for _, pt := range pts {
		x, y := pt.X, pt.Y
		switch state {
		case 0:
			state = 1
		case 1:
			state = 2
		case 2:
			state = 3
			p.LineTo(Point{(5*x0 + x1) / 6, (5*y0 + y1) / 6})
			bez(x, y)
		default:
			bez(x, y)
		}
		x0, x1 = x1, x
		y0, y1 = y1, y
	}
	if state == 3 {
		bez(x1, y1)
	}
	p.LineTo(Point{x1, y1})
	return p
}

// PathEnd returns the last point of p and the direction it arrives from: a
// point before the end along the path's tangent.
func PathEnd(p Path) (end, before Point, ok bool) {
	var prev Point
	var last Point
	have := false
	for _, op := range p.Ops {
		switch op.Kind {
		case MoveTo, LineTo:
			if have {
				prev = last
			}
			last = op.Pts[0]
			have = true
		case CubicTo:
			prev = op.Pts[1]
			if prev == op.Pts[2] {
				prev = op.Pts[0]
			}
			last = op.Pts[2]
			have = true
		}
	}
	if !have || prev == last {
		return last, last, false
	}
	return last, prev, true
}

// Reverse returns the path drawn the other way. Only a single open subpath
// of lines and curves is supported, which is what edges are.
func Reverse(p Path) Path {
	type seg struct {
		kind  OpKind
		start Point
		c1    Point
		c2    Point
		end   Point
	}
	var segs []seg
	var cur Point
	for _, op := range p.Ops {
		switch op.Kind {
		case MoveTo:
			cur = op.Pts[0]
		case LineTo:
			segs = append(segs, seg{kind: LineTo, start: cur, end: op.Pts[0]})
			cur = op.Pts[0]
		case CubicTo:
			segs = append(segs, seg{kind: CubicTo, start: cur, c1: op.Pts[0], c2: op.Pts[1], end: op.Pts[2]})
			cur = op.Pts[2]
		}
	}
	var out Path
	out.MoveTo(cur)
	for i := len(segs) - 1; i >= 0; i-- {
		s := segs[i]
		if s.kind == LineTo {
			out.LineTo(s.start)
		} else {
			out.CubicTo(s.c2, s.c1, s.start)
		}
	}
	return out
}

// TrimEnd returns p with its end pulled back by d along the final segment,
// for an arrowhead to take that room.
func TrimEnd(p Path, d float64) Path {
	out := Path{Ops: append([]Op(nil), p.Ops...)}
	for i := len(out.Ops) - 1; i >= 0; i-- {
		op := &out.Ops[i]
		switch op.Kind {
		case LineTo:
			var prev Point
			if i > 0 {
				prev = opEnd(out.Ops[i-1])
			}
			v := op.Pts[0].Sub(prev)
			if l := v.Len(); l > d {
				op.Pts[0] = op.Pts[0].Sub(v.Scale(d / l))
			}
			return out
		case CubicTo:
			v := op.Pts[2].Sub(op.Pts[1])
			if v.Len() < 1e-9 {
				v = op.Pts[2].Sub(op.Pts[0])
			}
			if l := v.Len(); l > 0 {
				shift := v.Scale(math.Min(d, l) / l)
				op.Pts[2] = op.Pts[2].Sub(shift)
				op.Pts[1] = op.Pts[1].Sub(shift)
			}
			return out
		case MoveTo:
			return out
		}
	}
	return out
}

func opEnd(op Op) Point {
	if op.Kind == CubicTo {
		return op.Pts[2]
	}
	return op.Pts[0]
}
