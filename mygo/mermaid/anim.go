package mermaid

import (
	"math"
	"strings"
	"time"

	"github.com/momaek/streameraid/geom"
	"github.com/momaek/streameraid/layout"
)

// A diagram that changes does not jump to its new layout: what was there
// moves to where it now belongs, new nodes fade in, and new edges are
// drawn from their source to their target.
//
// A frame is what a diagram shows at a moment, by the stable IDs of its
// parts. A transition goes from the frame showing when the layout changed
// to the frame of the new layout.

// motionDuration is how long a transition takes.
const motionDuration = 240 * time.Millisecond

// edgeSamples is how many points along an edge are kept to move it from
// one route to another.
const edgeSamples = 24

type frame struct {
	w, h     float64
	nodes    map[string]nodeFrame
	clusters map[string]clusterFrame
	edges    map[string]edgeFrame
}

type nodeFrame struct {
	rect  geom.Rect
	alpha float64
}

type clusterFrame struct {
	rect    geom.Rect
	titleAt geom.Point
	alpha   float64
}

type edgeFrame struct {
	pts     []geom.Point // edgeSamples points along the edge
	labelAt geom.Point
	// drawn is how much of the edge's length shows, from its source; moving
	// tells that the points are between two routes, so that the edge is
	// drawn through them rather than along its exact path.
	drawn  float64
	moving bool
}

// frameOf returns the frame showing all of a layout.
func frameOf(lay *layout.Flow) *frame {
	f := &frame{
		w: lay.Width, h: lay.Height,
		nodes:    make(map[string]nodeFrame, len(lay.Nodes)),
		clusters: make(map[string]clusterFrame, len(lay.Clusters)),
		edges:    make(map[string]edgeFrame, len(lay.Edges)),
	}
	for _, n := range lay.Nodes {
		f.nodes[n.ID] = nodeFrame{rect: n.Rect, alpha: 1}
	}
	for _, c := range lay.Clusters {
		f.clusters[c.ID] = clusterFrame{rect: c.Rect, titleAt: c.TitleAt, alpha: 1}
	}
	for _, e := range lay.Edges {
		f.edges[e.ID] = edgeFrame{pts: sample(e.Path, edgeSamples), labelAt: e.LabelAt, drawn: 1}
	}
	return f
}

// sample returns n points evenly spaced along a path.
func sample(p geom.Path, n int) []geom.Point {
	var line []geom.Point
	for _, part := range p.Flatten() {
		line = append(line, part...)
	}
	out := make([]geom.Point, n)
	if len(line) == 0 {
		return out
	}
	total := 0.0
	for i := 1; i < len(line); i++ {
		total += line[i].Sub(line[i-1]).Len()
	}
	if total == 0 {
		for i := range out {
			out[i] = line[0]
		}
		return out
	}
	seg, walked := 1, 0.0
	for i := range out {
		want := total * float64(i) / float64(n-1)
		for seg < len(line)-1 && walked+line[seg].Sub(line[seg-1]).Len() < want {
			walked += line[seg].Sub(line[seg-1]).Len()
			seg++
		}
		l := line[seg].Sub(line[seg-1]).Len()
		t := 0.0
		if l > 0 {
			t = math.Min(1, (want-walked)/l)
		}
		out[i] = line[seg-1].Lerp(line[seg], t)
	}
	return out
}

// transition moves a diagram from one frame to another.
type transition struct {
	from, to *frame
	start    time.Time
	dur      time.Duration
}

// done reports whether the transition has reached its end.
func (t *transition) done(now time.Time) bool {
	return t.dur <= 0 || !now.Before(t.start.Add(t.dur))
}

// at returns the frame showing at a time.
func (t *transition) at(now time.Time) *frame {
	if t.done(now) {
		return t.to
	}
	p := float64(now.Sub(t.start)) / float64(t.dur)
	p = math.Max(0, math.Min(1, p))
	p = 1 - math.Pow(1-p, 3) // ease out
	lerp := func(a, b float64) float64 { return a + (b-a)*p }
	rect := func(a, b geom.Rect) geom.Rect {
		return geom.Rect{X: lerp(a.X, b.X), Y: lerp(a.Y, b.Y), W: lerp(a.W, b.W), H: lerp(a.H, b.H)}
	}
	// A diagram that grows takes its room at once, so that what is new is
	// not cut off while it comes in; one that shrinks gives its room back
	// as its parts move.
	size := func(a, b float64) float64 {
		if b >= a {
			return b
		}
		return lerp(a, b)
	}
	f := &frame{
		w: size(t.from.w, t.to.w), h: size(t.from.h, t.to.h),
		nodes:    make(map[string]nodeFrame, len(t.to.nodes)),
		clusters: make(map[string]clusterFrame, len(t.to.clusters)),
		edges:    make(map[string]edgeFrame, len(t.to.edges)),
	}
	for id, to := range t.to.nodes {
		if from, ok := t.from.nodes[id]; ok {
			f.nodes[id] = nodeFrame{rect: rect(from.rect, to.rect), alpha: lerp(from.alpha, 1)}
		} else if strings.HasPrefix(id, "slice:") {
			// A new slice of a pie turns open from its start.
			f.nodes[id] = nodeFrame{rect: geom.Rect{X: to.rect.X, W: to.rect.W * p}, alpha: p}
		} else {
			f.nodes[id] = nodeFrame{rect: to.rect, alpha: p}
		}
	}
	for id, to := range t.to.clusters {
		if from, ok := t.from.clusters[id]; ok {
			f.clusters[id] = clusterFrame{rect: rect(from.rect, to.rect), titleAt: from.titleAt.Lerp(to.titleAt, p), alpha: lerp(from.alpha, 1)}
		} else {
			f.clusters[id] = clusterFrame{rect: to.rect, titleAt: to.titleAt, alpha: p}
		}
	}
	for id, to := range t.to.edges {
		from, ok := t.from.edges[id]
		if !ok {
			f.edges[id] = edgeFrame{pts: to.pts, labelAt: to.labelAt, drawn: p}
			continue
		}
		e := edgeFrame{labelAt: from.labelAt.Lerp(to.labelAt, p), drawn: lerp(from.drawn, 1), pts: to.pts}
		if !samePoints(from.pts, to.pts) {
			e.moving = true
			e.pts = make([]geom.Point, len(to.pts))
			for i := range e.pts {
				e.pts[i] = from.pts[i].Lerp(to.pts[i], p)
			}
		}
		f.edges[id] = e
	}
	return f
}

func samePoints(a, b []geom.Point) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if math.Abs(a[i].X-b[i].X) > 0.01 || math.Abs(a[i].Y-b[i].Y) > 0.01 {
			return false
		}
	}
	return true
}

// polyline returns the path through points.
func polyline(pts []geom.Point) geom.Path {
	var p geom.Path
	for i, pt := range pts {
		if i == 0 {
			p.MoveTo(pt)
		} else {
			p.LineTo(pt)
		}
	}
	return p
}

// grow returns r scaled about its center.
func grow(r geom.Rect, k float64) geom.Rect {
	return geom.RectCentered(r.Center(), r.W*k, r.H*k)
}
