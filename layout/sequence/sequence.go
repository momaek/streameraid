// Package sequence lays out sequence diagrams: participants in columns in
// the order they appear, events in rows in the order of the source.
//
// The layout suits a diagram that streams in as it is: a new event goes
// under those before it, and a new participant to the right of the
// others, so that what is there moves only when a long text asks for more
// room between two lifelines.
package sequence

import (
	"math"
	"strconv"

	"github.com/momaek/streameraid/geom"
	"github.com/momaek/streameraid/layout"
	"github.com/momaek/streameraid/seq"
)

// Hints make a layout follow an earlier one of the same diagram, as its
// source streams in.
type Hints struct {
	// Prev is the layout of the diagram before its last change.
	Prev *layout.Sequence
	// Anchor keeps the lifelines Prev holds from moving back to the left:
	// they only move apart, to make room. Without it a text that wraps as
	// it grows, and so gets narrower, would pull them back.
	Anchor bool
}

// Layout lays out a sequence diagram.
func Layout(d *seq.Diagram, m layout.Measurer, opt layout.Options) *layout.Sequence {
	return LayoutWith(d, m, opt, Hints{})
}

// LayoutWith lays out a sequence diagram following an earlier layout of
// it.
func LayoutWith(d *seq.Diagram, m layout.Measurer, opt layout.Options, h Hints) *layout.Sequence {
	opt = opt.Defaults()
	l := &layouter{d: d, m: m, opt: opt, em: opt.FontSize, out: &layout.Sequence{},
		minX: math.Inf(1), maxX: math.Inf(-1), stacks: map[string][]activation{}, actCount: map[string]int{}}
	if h.Anchor && h.Prev != nil {
		l.prevX = map[string]float64{}
		for _, p := range h.Prev.Participants {
			l.prevX[p.ID] = p.LifeTop.X
		}
	}
	if len(d.Participants) == 0 && len(d.Events) == 0 && d.Title == "" {
		return l.out
	}
	l.measure()
	l.columns()
	l.rows()
	l.finish()
	return l.out
}

type activation struct {
	id     string
	startY float64
	depth  int
}

type block struct {
	slot     int // its place in out.Blocks
	ev       *seq.Event
	startY   float64
	headerH  float64
	minX     float64
	maxX     float64
	title    layout.TextBox
	kind     layout.TextBox
	sections []layout.SeqSection
}

type layouter struct {
	d   *seq.Diagram
	m   layout.Measurer
	opt layout.Options
	em  float64
	out *layout.Sequence

	headW  []float64
	headHs []float64        // each head's own height
	headH  float64          // the tallest head: heads stand on one line
	labels []layout.TextBox // participants' labels
	texts  []layout.TextBox // events' texts
	x      []float64        // lifelines
	title  layout.TextBox

	cursor     float64 // the top of the next row
	lastY      float64 // the last message's line, where activations start and end
	minX, maxX float64
	stacks     map[string][]activation
	actCount   map[string]int
	blocks     []*block
	headTop    float64
	lifeTop    []float64
	lifeBottom []float64 // 0 until destroyed
	// prevX is where the lifelines were in the layout to follow, if any.
	prevX map[string]float64
}

func (l *layouter) style() layout.TextStyle { return l.opt.TextStyle() }

func (l *layouter) small() layout.TextStyle {
	s := l.opt.TextStyle()
	s.Size *= 0.85
	s.Bold = true
	return s
}

func (l *layouter) measure() {
	em := l.em
	for _, p := range l.d.Participants {
		tb := l.m.Measure(p.Label, l.style(), 10*em)
		w, h := math.Max(tb.W+2*em, 4.5*em), tb.H+1.1*em
		if p.Actor {
			w, h = math.Max(tb.W, 2.2*em)+0.6*em, actorFigure*em+0.3*em+tb.H
		}
		l.labels = append(l.labels, tb)
		l.headW = append(l.headW, math.Ceil(w))
		l.headHs = append(l.headHs, math.Ceil(h))
		if p.Created == -1 {
			// The heads at the top stand on one line.
			l.headH = math.Max(l.headH, math.Ceil(h))
		}
	}
	for _, e := range l.d.Events {
		var tb layout.TextBox
		switch e.Kind {
		case seq.Message:
			if e.Text != "" {
				tb = l.m.Measure(e.Text, l.style(), l.opt.WrapWidth)
			}
		case seq.Note:
			tb = l.m.Measure(e.Text, l.style(), 12*em)
		case seq.BlockStart, seq.Section:
			if e.Text != "" {
				tb = l.m.Measure("["+e.Text+"]", l.style(), 22*em)
			}
		}
		l.texts = append(l.texts, tb)
	}
	if l.d.Title != "" {
		st := l.style()
		st.Bold = true
		l.title = l.m.Measure(l.d.Title, st, 30*em)
	}
}

// actorFigure is the height of an actor's stick figure, in ems.
const actorFigure = 2.6

func (l *layouter) noteWidth(tb layout.TextBox) float64 { return math.Max(tb.W+1.2*l.em, 4*l.em) }

// columns places the lifelines: side by side with room for their heads,
// and further apart where a message's text, a note or a loop back needs
// it.
func (l *layouter) columns() {
	n := len(l.d.Participants)
	em := l.em
	need := make([][]float64, n)
	for i := range need {
		need[i] = make([]float64, n)
	}
	for i := 0; i+1 < n; i++ {
		gap := 2 * em
		if l.d.Participants[i].Box != l.d.Participants[i+1].Box {
			gap += 1.4 * em
		}
		need[i][i+1] = (l.headW[i]+l.headW[i+1])/2 + gap
		// Lifelines that were there do not move closer.
		a, okA := l.prevX[l.d.Participants[i].ID]
		b, okB := l.prevX[l.d.Participants[i+1].ID]
		if okA && okB {
			need[i][i+1] = math.Max(need[i][i+1], b-a)
		}
	}
	ask := func(i, j int, v float64) {
		if i > j {
			i, j = j, i
		}
		if i >= 0 && j < n && i != j {
			need[i][j] = math.Max(need[i][j], v)
		}
	}
	for k, e := range l.d.Events {
		tb := l.texts[k]
		switch e.Kind {
		case seq.Message:
			_, a := l.d.Participant(e.From)
			pb, b := l.d.Participant(e.To)
			if a == b {
				ask(a, a+1, 0.6*em+math.Max(tb.W, selfW*em)+1.4*em)
				continue
			}
			w := tb.W + 2.4*em
			if pb.Created == k {
				w += l.headW[b] / 2
			}
			ask(a, b, w)
		case seq.Note:
			w := l.noteWidth(tb)
			_, a := l.d.Participant(e.Actors[0])
			switch {
			case e.Placement == seq.RightOf:
				ask(a, a+1, w+1.8*em)
			case e.Placement == seq.LeftOf:
				ask(a-1, a, w+1.8*em)
			case len(e.Actors) == 1:
				ask(a-1, a, w/2+em)
				ask(a, a+1, w/2+em)
			default:
				_, b := l.d.Participant(e.Actors[1])
				ask(a, b, w-2.4*em)
			}
		}
	}
	l.x = make([]float64, n)
	for j := 1; j < n; j++ {
		for i := 0; i < j; i++ {
			if need[i][j] > 0 {
				l.x[j] = math.Max(l.x[j], l.x[i]+need[i][j])
			}
		}
	}
	for i := range l.x {
		l.x[i] = math.Round(l.x[i])
		l.span(l.x[i]-l.headW[i]/2, l.x[i]+l.headW[i]/2)
	}
}

// selfW and selfH are the size of a message's loop back to its sender, in
// ems.
const (
	selfW = 2.4
	selfH = 1.5
)

// span widens the diagram, and the open blocks, to hold x0 to x1.
func (l *layouter) span(x0, x1 float64) {
	l.minX, l.maxX = math.Min(l.minX, x0), math.Max(l.maxX, x1)
	for _, b := range l.blocks {
		b.minX, b.maxX = math.Min(b.minX, x0), math.Max(b.maxX, x1)
	}
}

// actHalf is half the width of an activation bar, in ems.
const actHalf = 0.38

// edges returns where arrows meet participant i's lifeline at the left
// and at the right: the sides of its activation bars, if any.
func (l *layouter) edges(i int) (left, right float64) {
	x := l.x[i]
	st := l.stacks[l.d.Participants[i].ID]
	if len(st) == 0 {
		return x, x
	}
	a := actHalf * l.em
	return x - a, x + a + float64(len(st)-1)*a
}

func (l *layouter) activate(id string, y float64) {
	st := l.stacks[id]
	n := l.actCount[id]
	l.actCount[id] = n + 1
	l.stacks[id] = append(st, activation{id: "act:" + id + "#" + strconv.Itoa(n), startY: y, depth: len(st)})
}

func (l *layouter) deactivate(id string, y float64) {
	st := l.stacks[id]
	if len(st) == 0 {
		return
	}
	a := st[len(st)-1]
	l.stacks[id] = st[:len(st)-1]
	l.addActivation(id, a, y)
}

func (l *layouter) addActivation(id string, a activation, endY float64) {
	_, i := l.d.Participant(id)
	w := actHalf * l.em
	h := math.Max(endY-a.startY, 0.6*l.em)
	l.out.Activations = append(l.out.Activations, layout.SeqActivation{
		ID:   a.id,
		Rect: geom.Rect{X: l.x[i] - w + float64(a.depth)*w, Y: a.startY, W: 2 * w, H: h},
	})
}

// rows places the events down the page.
func (l *layouter) rows() {
	em := l.em
	d := l.d
	n := len(d.Participants)
	top := 0.0
	if len(l.title.Lines) > 0 {
		top = l.title.H + 0.8*em
	}
	for _, b := range d.Boxes {
		if b.Title != "" {
			top += 1.9 * em
			break
		}
	}
	if len(d.Boxes) > 0 {
		top += 0.5 * em
	}
	l.headTop = top
	l.lifeTop = make([]float64, n)
	l.lifeBottom = make([]float64, n)
	for i := range l.lifeTop {
		l.lifeTop[i] = top + l.headH
	}
	l.cursor = top + l.headH + 1.1*em
	l.lastY = l.cursor - 0.4*em

	for k, e := range d.Events {
		tb := l.texts[k]
		switch e.Kind {
		case seq.Message:
			l.message(k, e, tb)
		case seq.Note:
			l.note(e, tb)
		case seq.Activate:
			l.activate(e.Actor, l.lastY)
		case seq.Deactivate:
			l.deactivate(e.Actor, l.lastY)
		case seq.BlockStart:
			b := &block{slot: len(l.out.Blocks), ev: e, startY: l.cursor, minX: math.Inf(1), maxX: math.Inf(-1), title: tb}
			l.out.Blocks = append(l.out.Blocks, layout.SeqBlock{})
			if e.Block == "rect" {
				b.headerH = 0.5 * em
			} else {
				b.kind = l.m.Measure(e.Block, l.small(), 0)
				b.headerH = math.Max(tabH*em, tb.H+0.3*em) + 0.5*em
			}
			l.blocks = append(l.blocks, b)
			l.cursor += b.headerH
		case seq.Section:
			if len(l.blocks) == 0 {
				continue
			}
			b := l.blocks[len(l.blocks)-1]
			sec := layout.SeqSection{ID: e.ID, Y: l.cursor - 0.2*em, Title: tb}
			sec.TitleAt = geom.Point{Y: sec.Y + 0.3*em + tb.H/2}
			b.sections = append(b.sections, sec)
			l.cursor = sec.Y + 0.3*em + tb.H + 0.6*em
		case seq.BlockEnd:
			l.closeBlock()
		}
	}
	// A diagram still streaming in has blocks and activations open: they
	// reach down to where it has got to.
	for len(l.blocks) > 0 {
		l.closeBlock()
	}
	for _, p := range d.Participants {
		for _, a := range l.stacks[p.ID] {
			l.addActivation(p.ID, a, math.Max(l.lastY, a.startY)+0.6*em)
			l.cursor = math.Max(l.cursor, math.Max(l.lastY, a.startY)+1.2*em)
		}
	}
}

// tabH is the height of a block's keyword tab, in ems.
const tabH = 1.5

func (l *layouter) closeBlock() {
	em := l.em
	b := l.blocks[len(l.blocks)-1]
	l.blocks = l.blocks[:len(l.blocks)-1]
	x0, x1 := b.minX-0.8*em, b.maxX+0.8*em
	if math.IsInf(b.minX, 1) {
		n := len(l.x)
		if n == 0 {
			l.out.Blocks[b.slot].ID = b.ev.ID
			return
		}
		x0, x1 = l.x[0]-1.5*em, l.x[n-1]+1.5*em
	}
	sb := layout.SeqBlock{ID: b.ev.ID, Kind: b.ev.Block, Fill: b.ev.Color, Title: b.title, KindLabel: b.kind}
	if b.ev.Block != "rect" {
		tabW := b.kind.W + em
		x1 = math.Max(x1, x0+tabW+0.6*em+b.title.W+0.8*em)
		for _, s := range b.sections {
			x1 = math.Max(x1, x0+0.8*em+s.Title.W+0.8*em)
		}
		sb.Tab = geom.Rect{X: x0, Y: b.startY, W: tabW, H: tabH * em}
		sb.KindAt = sb.Tab.Center()
		sb.TitleAt = geom.Point{X: sb.Tab.Right() + 0.6*em + b.title.W/2, Y: b.startY + (b.headerH-0.5*em)/2}
	}
	bottom := l.cursor + 0.3*em
	sb.Rect = geom.Rect{X: x0, Y: b.startY, W: x1 - x0, H: bottom - b.startY}
	for _, s := range b.sections {
		s.TitleAt.X = x0 + 0.8*em + s.Title.W/2
		sb.Sections = append(sb.Sections, s)
	}
	l.out.Blocks[b.slot] = sb
	l.cursor = bottom + 0.8*em
	l.span(x0, x1)
}

func (l *layouter) message(k int, e *seq.Event, tb layout.TextBox) {
	em := l.em
	d := l.d
	pa, a := d.Participant(e.From)
	pb, b := d.Participant(e.To)
	msg := layout.SeqMessage{ID: e.ID, Dotted: e.Dotted, Label: tb, Number: e.Number, NumberRadius: 0.72 * em}
	labelTop := l.cursor
	lineY := labelTop + tb.H + 0.3*em
	if len(tb.Lines) == 0 {
		lineY = labelTop + 0.4*em
	}
	if a == b {
		_, right := l.edges(a)
		x0 := right
		w, h := selfW*em, selfH*em
		end := geom.Point{X: x0, Y: lineY + h}
		shape, filled, trim := headShape(e.Head, end, -1, em)
		var p geom.Path
		p.MoveTo(geom.Point{X: x0, Y: lineY})
		p.CubicTo(geom.Point{X: x0 + w*1.35, Y: lineY}, geom.Point{X: x0 + w*1.35, Y: lineY + h}, geom.Point{X: end.X + trim, Y: end.Y})
		msg.Path = p
		if e.Head != seq.HeadNone {
			msg.Heads = append(msg.Heads, layout.SeqHead{Shape: shape, Filled: filled})
		}
		msg.LabelAt = geom.Point{X: x0 + 0.5*em + tb.W/2, Y: labelTop + tb.H/2}
		msg.NumberAt = geom.Point{X: x0, Y: lineY}
		l.span(x0, math.Max(x0+w*1.05, x0+0.5*em+tb.W))
		l.out.Messages = append(l.out.Messages, msg)
		l.lastY = lineY + h
		l.cursor = lineY + h + 0.9*em
		if e.ActivateTo {
			l.activate(e.To, lineY+h)
		}
		if e.DeactivateFrom {
			l.deactivate(e.From, lineY)
		}
		l.destroy(k, pa, a, lineY+h)
		return
	}
	created := pb.Created == k
	if created {
		lineY = math.Max(lineY, labelTop+l.headHs[b]/2)
	}
	if e.ActivateTo {
		l.activate(e.To, lineY)
	}
	dir := 1.0
	if l.x[b] < l.x[a] {
		dir = -1
	}
	la, ra := l.edges(a)
	lb, rb := l.edges(b)
	xa, xb := ra, lb
	if dir < 0 {
		xa, xb = la, rb
	}
	if created {
		xb = l.x[b] - dir*l.headW[b]/2
		l.lifeTop[b] = lineY + l.headHs[b]/2
	}
	start, end := geom.Point{X: xa, Y: lineY}, geom.Point{X: xb, Y: lineY}
	shape, filled, trim := headShape(e.Head, end, dir, em)
	if e.Head != seq.HeadNone {
		msg.Heads = append(msg.Heads, layout.SeqHead{Shape: shape, Filled: filled})
	}
	from := start
	if e.Both {
		s2, f2, t2 := headShape(e.Head, start, -dir, em)
		msg.Heads = append(msg.Heads, layout.SeqHead{Shape: s2, Filled: f2})
		from.X += dir * t2
	}
	var p geom.Path
	p.MoveTo(from).LineTo(geom.Point{X: end.X - dir*trim, Y: lineY})
	msg.Path = p
	mid := (xa + xb) / 2
	msg.LabelAt = geom.Point{X: mid, Y: labelTop + tb.H/2}
	msg.NumberAt = start
	l.span(math.Min(xa, xb), math.Max(xa, xb))
	l.span(mid-tb.W/2, mid+tb.W/2)
	l.out.Messages = append(l.out.Messages, msg)
	l.lastY = lineY
	l.cursor = lineY + 0.9*em
	if created {
		l.cursor = math.Max(l.cursor, lineY+l.headHs[b]/2+0.7*em)
	}
	if e.DeactivateFrom {
		l.deactivate(e.From, lineY)
	}
	l.destroy(k, pa, a, lineY)
	l.destroy(k, pb, b, lineY)
}

// destroy ends participant i's lifeline when event k destroys it.
func (l *layouter) destroy(k int, p *seq.Participant, i int, y float64) {
	if p.Destroyed == k {
		l.lifeBottom[i] = y + 1.1*l.em
		l.cursor = math.Max(l.cursor, l.lifeBottom[i]+0.6*l.em)
	}
}

// headShape returns the head of an arrow whose tip is at tip, pointing
// along dir (1 to the right, -1 to the left), whether it is filled, and
// how far the line is pulled back for it.
func headShape(h seq.Head, tip geom.Point, dir, em float64) (shape geom.Path, filled bool, trim float64) {
	pt := func(dx, dy float64) geom.Point { return geom.Point{X: tip.X - dir*dx, Y: tip.Y + dy} }
	switch h {
	case seq.HeadArrow:
		length, half := 0.72*em, 0.3*em
		shape.Polygon(tip, pt(length, -half), pt(length, half))
		return shape, true, length * 0.9
	case seq.HeadOpen:
		length, half := 0.7*em, 0.36*em
		shape.MoveTo(pt(length, -half)).LineTo(tip).LineTo(pt(length, half))
		return shape, false, 0
	case seq.HeadCross:
		r := 0.3 * em
		c := pt(0.5*em, 0)
		shape.MoveTo(geom.Point{X: c.X - r, Y: c.Y - r}).LineTo(geom.Point{X: c.X + r, Y: c.Y + r})
		shape.MoveTo(geom.Point{X: c.X - r, Y: c.Y + r}).LineTo(geom.Point{X: c.X + r, Y: c.Y - r})
		return shape, false, 0.5 * em
	}
	return shape, false, 0
}

func (l *layouter) note(e *seq.Event, tb layout.TextBox) {
	em := l.em
	w := l.noteWidth(tb)
	h := tb.H + 0.7*em
	_, a := l.d.Participant(e.Actors[0])
	var x float64
	switch {
	case e.Placement == seq.RightOf:
		_, right := l.edges(a)
		x = right + 0.8*em
	case e.Placement == seq.LeftOf:
		left, _ := l.edges(a)
		x = left - 0.8*em - w
	case len(e.Actors) == 1:
		x = l.x[a] - w/2
	default:
		_, b := l.d.Participant(e.Actors[1])
		lo, hi := math.Min(l.x[a], l.x[b]), math.Max(l.x[a], l.x[b])
		w = math.Max(w, hi-lo+2.4*em)
		x = (lo+hi)/2 - w/2
	}
	r := geom.Rect{X: x, Y: l.cursor, W: w, H: h}
	l.out.Notes = append(l.out.Notes, layout.SeqNote{ID: e.ID, Rect: r, Label: tb, LabelAt: r.Center()})
	l.span(r.X, r.Right())
	l.lastY = r.Bottom()
	l.cursor = r.Bottom() + 0.8*em
}

// finish places the heads, lifelines and groups, now that the rows give
// the diagram's height, and moves everything inside the margin.
func (l *layouter) finish() {
	em := l.em
	d := l.d
	out := l.out
	bottom := l.cursor + 0.2*em
	for i, p := range d.Participants {
		if p.Created == seq.NotYetCreated {
			continue // it shows with the message that creates it
		}
		// Heads stand on one line, each at its own height.
		r := geom.Rect{X: l.x[i] - l.headW[i]/2, Y: l.lifeTop[i] - l.headHs[i], W: l.headW[i], H: l.headHs[i]}
		sp := layout.SeqParticipant{ID: p.ID, Rect: r, Actor: p.Actor, Label: l.labels[i], LabelAt: r.Center()}
		if p.Actor {
			sp.Figure = Actor(geom.Point{X: l.x[i], Y: r.Y}, em)
			sp.LabelAt = geom.Point{X: l.x[i], Y: r.Bottom() - l.labels[i].H/2}
		}
		sp.LifeTop = geom.Point{X: l.x[i], Y: l.lifeTop[i]}
		sp.LifeBottom = geom.Point{X: l.x[i], Y: bottom}
		if l.lifeBottom[i] > 0 {
			sp.LifeBottom.Y, sp.Destroyed = l.lifeBottom[i], true
		}
		out.Participants = append(out.Participants, sp)
	}
	for g, b := range d.Boxes {
		lo, hi := math.Inf(1), math.Inf(-1)
		for i, p := range d.Participants {
			if p.Box == g {
				lo, hi = math.Min(lo, l.x[i]-l.headW[i]/2), math.Max(hi, l.x[i]+l.headW[i]/2)
			}
		}
		if math.IsInf(lo, 1) {
			continue
		}
		title := l.m.Measure(b.Title, l.style(), 0)
		lo, hi = lo-0.6*em, hi+0.6*em
		if d := title.W + 1.2*em - (hi - lo); d > 0 {
			lo, hi = lo-d/2, hi+d/2
		}
		top := l.headTop - 0.5*em
		if b.Title != "" {
			top -= 1.9 * em
		}
		r := geom.Rect{X: lo, Y: top, W: hi - lo, H: bottom + 0.3*em - top}
		out.Groups = append(out.Groups, layout.SeqGroup{ID: "g" + strconv.Itoa(g), Rect: r, Title: title, Color: b.Color,
			TitleAt: geom.Point{X: (lo + hi) / 2, Y: top + 0.95*em}})
		l.span(lo, hi)
		bottom = math.Max(bottom, r.Bottom()-0.3*em)
	}
	if len(out.Groups) > 0 {
		bottom += 0.3 * em
	}
	if math.IsInf(l.minX, 1) {
		l.minX, l.maxX = 0, 0
	}
	if len(l.title.Lines) > 0 {
		mid := (l.minX + l.maxX) / 2
		l.span(mid-l.title.W/2, mid+l.title.W/2)
		out.Title = l.title
		out.TitleAt = geom.Point{X: (l.minX + l.maxX) / 2, Y: l.title.H / 2}
	}
	mg := l.opt.Margin
	dx := mg - l.minX
	if len(d.Participants) > 0 {
		// Nor does the first lifeline move back.
		if was, ok := l.prevX[d.Participants[0].ID]; ok {
			dx = math.Max(dx, was-l.x[0])
		}
	}
	shift(out, geom.Point{X: dx, Y: mg})
	out.Width = math.Ceil(l.maxX + dx + mg)
	out.Height = math.Ceil(bottom + 2*mg)
}

// Actor returns the strokes of a stick figure whose head's top is at top.
func Actor(top geom.Point, em float64) geom.Path {
	var p geom.Path
	r := 0.42 * em
	head := geom.Point{X: top.X, Y: top.Y + r}
	p.Ellipse(head, r, r)
	neck := head.Y + r
	hip := top.Y + 1.75*em
	p.MoveTo(geom.Point{X: top.X, Y: neck}).LineTo(geom.Point{X: top.X, Y: hip})
	arms := neck + 0.4*em
	p.MoveTo(geom.Point{X: top.X - 0.75*em, Y: arms}).LineTo(geom.Point{X: top.X + 0.75*em, Y: arms})
	feet := top.Y + actorFigure*em
	p.MoveTo(geom.Point{X: top.X - 0.65*em, Y: feet}).LineTo(geom.Point{X: top.X, Y: hip}).LineTo(geom.Point{X: top.X + 0.65*em, Y: feet})
	return p
}

// shift moves everything in a layout by d.
func shift(s *layout.Sequence, d geom.Point) {
	move := func(p geom.Path) geom.Path { return p.Transform(func(q geom.Point) geom.Point { return q.Add(d) }) }
	s.TitleAt = s.TitleAt.Add(d)
	for i := range s.Groups {
		g := &s.Groups[i]
		g.Rect, g.TitleAt = g.Rect.Translate(d), g.TitleAt.Add(d)
	}
	for i := range s.Participants {
		p := &s.Participants[i]
		p.Rect, p.LabelAt = p.Rect.Translate(d), p.LabelAt.Add(d)
		p.LifeTop, p.LifeBottom = p.LifeTop.Add(d), p.LifeBottom.Add(d)
		p.Figure = move(p.Figure)
	}
	for i := range s.Blocks {
		b := &s.Blocks[i]
		b.Rect, b.Tab = b.Rect.Translate(d), b.Tab.Translate(d)
		b.KindAt, b.TitleAt = b.KindAt.Add(d), b.TitleAt.Add(d)
		for j := range b.Sections {
			b.Sections[j].Y += d.Y
			b.Sections[j].TitleAt = b.Sections[j].TitleAt.Add(d)
		}
	}
	for i := range s.Activations {
		s.Activations[i].Rect = s.Activations[i].Rect.Translate(d)
	}
	for i := range s.Messages {
		m := &s.Messages[i]
		m.Path = move(m.Path)
		m.LabelAt, m.NumberAt = m.LabelAt.Add(d), m.NumberAt.Add(d)
		for j := range m.Heads {
			m.Heads[j].Shape = move(m.Heads[j].Shape)
		}
	}
	for i := range s.Notes {
		s.Notes[i].Rect, s.Notes[i].LabelAt = s.Notes[i].Rect.Translate(d), s.Notes[i].LabelAt.Add(d)
	}
}
