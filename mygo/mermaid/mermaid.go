// Package mermaid draws Mermaid diagrams natively in MyGo, as they stream
// in: flowcharts and sequence diagrams. Shapes and lines are painted, and
// labels are text elements, which the user can select and copy. As a
// diagram grows, what is there stays where it is, new parts fade in and
// new lines are drawn from their start to their end; with the desktop's
// Reduce Motion, changes show at once.
//
// Give MyGo's Markdown view the hook, and mermaid blocks show as diagrams
// while the model is still writing them:
//
//	app.md.Code = map[string]func(*ui.Context, *markdown.CodeBlockView){
//		"mermaid": mermaid.CodeBlock,
//	}
//
// Kinds of diagrams not drawn yet show as code.
package mermaid

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"github.com/momaek/streameraid"
	"github.com/momaek/streameraid/flow"
	"github.com/momaek/streameraid/geom"
	"github.com/momaek/streameraid/layout"
	"github.com/momaek/streameraid/layout/sequence"
	"github.com/momaek/streameraid/layout/sugiyama"
	"github.com/momaek/streameraid/seq"
)

// Options configure a diagram view.
type Options struct {
	// Streaming tells that the source is still being written: its last
	// line may be cut short, and problems are not reported yet.
	Streaming bool
	// FontSize is the size of labels; 0 is the theme's font size.
	FontSize float32
	// Labels are the texts the view shows; empty fields take English
	// ones.
	Labels Labels
	// OnNodeClick, when set, is called with the ID of the node of a
	// flowchart the user clicks. The pointer then shows a hand over nodes,
	// and their labels are clicked rather than selected; the other texts
	// of the diagram can still be selected.
	OnNodeClick func(id string)
	// OnExport, when set, receives the diagram as SVG when the user asks
	// to export it. Without it the SVG goes to the clipboard.
	OnExport func(svg string)
	// NoToolbar leaves out the controls that show over a diagram while the
	// pointer is on it: zooming, copying its source, exporting it.
	NoToolbar bool
	// NoFit keeps a diagram wider than the view at its size, to scroll
	// sideways, rather than scale it down to fit once it is complete.
	NoFit bool
}

// Labels are the texts a view shows around a diagram.
type Labels struct {
	// Problems says how many statements could not be read, as a format
	// with one %d.
	Problems    string
	ShowSource  string
	ShowDiagram string
	// The controls over a diagram: the names of the zoom buttons, read out
	// by screen readers and shown as tooltips, and the texts of the others.
	ZoomIn, ZoomOut, ZoomReset string
	CopySource, ExportSVG      string
	Copied                     string
}

func (l Labels) withDefaults() Labels {
	if l.Problems == "" {
		l.Problems = "%d statements could not be drawn"
	}
	if l.ShowSource == "" {
		l.ShowSource = "Show source"
	}
	if l.ShowDiagram == "" {
		l.ShowDiagram = "Show diagram"
	}
	def := func(s *string, v string) {
		if *s == "" {
			*s = v
		}
	}
	def(&l.ZoomIn, "Zoom in")
	def(&l.ZoomOut, "Zoom out")
	def(&l.ZoomReset, "Reset zoom")
	def(&l.CopySource, "Copy source")
	def(&l.ExportSVG, "SVG")
	def(&l.Copied, "Copied")
	return l
}

// ChineseLabels are the view's texts in Simplified Chinese.
var ChineseLabels = Labels{
	Problems: "有 %d 条语句没能画出来", ShowSource: "查看源码", ShowDiagram: "查看图表",
	ZoomIn: "放大", ZoomOut: "缩小", ZoomReset: "恢复默认大小",
	CopySource: "复制源码", ExportSVG: "SVG", Copied: "已复制",
}

// Supported reports whether View draws diagrams of a kind.
func Supported(k streameraid.Kind) bool {
	return k == streameraid.Flowchart || k == streameraid.Sequence
}

// CodeBlock is a hook for MyGo's Markdown view that draws mermaid blocks
// as diagrams, while they stream in too. Blocks of kinds of diagrams not
// drawn yet show as code.
func CodeBlock(c *ui.Context, b *markdown.CodeBlockView) {
	CodeBlockWith(Options{})(c, b)
}

// CodeBlockWith returns a hook like CodeBlock that draws with options; it
// sets Streaming itself.
func CodeBlockWith(o Options) func(*ui.Context, *markdown.CodeBlockView) {
	return func(c *ui.Context, b *markdown.CodeBlockView) {
		src := b.Code()
		switch k := streameraid.Detect(src); {
		case Supported(k):
			o.Streaming = b.Open
			View(c, src, o)
		case b.Open && !strings.Contains(strings.TrimSpace(src), "\n") && maybeSupported(string(k)):
			// The first line is still coming: keep the place empty rather than
			// show code for a moment.
			ui.Box(c).Height(0)
		default:
			markdown.DefaultCodeBlock(c, b)
		}
	}
}

// maybeSupported reports whether word may be the start of the header of a
// diagram View draws.
func maybeSupported(word string) bool {
	for _, k := range []string{"flowchart", "graph", "sequenceDiagram"} {
		if strings.HasPrefix(k, word) || strings.HasPrefix(word, k) {
			return true
		}
	}
	return false
}

// problem is a statement that could not be drawn.
type problem struct {
	line int
	msg  string
}

// state is what a view keeps between frames: the parse and layout of the
// source it last saw, and the transition to them from what showed before.
type state struct {
	src      string
	final    bool
	fontSize float64
	parsed   bool
	kind     streameraid.Kind
	opt      layout.Options

	graph  *flow.Graph
	lay    *layout.Flow
	seq    *seq.Diagram
	seqLay *layout.Sequence

	problems   []problem
	motion     *transition
	showSource bool

	// userZoom is the zoom the user chose, or 0 to fit. The zoom showing
	// eases from zoomFrom to zoomTo.
	userZoom         float64
	zoomFrom, zoomTo float64
	zoomStart        time.Time
	// copied names the control whose text reads "Copied" until copiedUntil.
	copied      string
	copiedUntil time.Time
}

// setZoom starts easing the zoom to z.
func (s *state) setZoom(z float64, now time.Time, still bool) {
	if s.zoomTo == 0 {
		s.zoomFrom, s.zoomTo = z, z
		return
	}
	if z == s.zoomTo {
		return
	}
	s.zoomFrom = s.zoom(now)
	if still {
		s.zoomFrom = z
	}
	s.zoomTo, s.zoomStart = z, now
}

// zoom returns the zoom showing at a time.
func (s *state) zoom(now time.Time) float64 {
	if s.zoomTo == 0 {
		return 1
	}
	p := float64(now.Sub(s.zoomStart)) / float64(motionDuration)
	if p >= 1 || s.zoomFrom == s.zoomTo {
		return s.zoomTo
	}
	p = 1 - math.Pow(1-math.Max(p, 0), 3)
	return s.zoomFrom + (s.zoomTo-s.zoomFrom)*p
}

func (s *state) zooming(now time.Time) bool {
	return s.zoomFrom != s.zoomTo && now.Before(s.zoomStart.Add(motionDuration))
}

// update parses and lays out the source when it changed, following the
// layout before so that what is there stays put, and starts the transition
// from what shows now to the new layout.
func (s *state) update(src string, final bool, fontSize float64, now time.Time, still bool) {
	if s.parsed && s.src == src && s.final == final && s.fontSize == fontSize {
		return
	}
	kind := streameraid.Detect(src)
	// While the source streams in, hold what is there; once it is complete,
	// balance the layout, keeping its order.
	follow := s.parsed && s.fontSize == fontSize && s.kind == kind
	first := !s.parsed || s.kind != kind
	s.src, s.final, s.fontSize, s.kind, s.parsed = src, final, fontSize, kind, true
	s.opt = layout.Options{FontSize: fontSize}.Defaults()
	s.problems = s.problems[:0]
	var to *frame
	switch kind {
	case streameraid.Sequence:
		hints := sequence.Hints{}
		if follow {
			hints = sequence.Hints{Prev: s.seqLay, Anchor: !final}
		}
		s.seq = seq.Parse(src, final)
		s.seqLay = sequence.LayoutWith(s.seq, sharedMeasurer, s.opt, hints)
		for _, p := range s.seq.Problems {
			s.problems = append(s.problems, problem{p.Line, p.Msg})
		}
		to = frameOfSequence(s.seqLay)
	default:
		hints := sugiyama.Hints{}
		if follow {
			hints = sugiyama.Hints{Prev: s.lay, Anchor: !final}
		}
		s.graph = flow.Parse(src, final)
		s.lay = sugiyama.LayoutWith(s.graph, sharedMeasurer, s.opt, hints)
		for _, p := range s.graph.Problems {
			s.problems = append(s.problems, problem{p.Line, p.Msg})
		}
		to = frameOf(s.lay)
	}
	switch {
	case still, first && final:
		// No motion asked for, or a diagram shown whole: as one read from
		// history, which should not fade in each time it comes into view.
		s.motion = &transition{from: to, to: to}
	case first:
		s.motion = &transition{from: &frame{w: to.w, h: to.h}, to: to, start: now, dur: motionDuration}
	default:
		s.motion = &transition{from: s.motion.at(now), to: to, start: now, dur: motionDuration}
	}
}

// View draws the diagram src: a flowchart or a sequence diagram.
func View(c *ui.Context, src string, o Options) ui.Element {
	th := c.Theme()
	fs := o.FontSize
	if fs <= 0 {
		fs = th.FontSize
	}
	if fs <= 0 {
		fs = 14
	}
	labels := o.Labels.withDefaults()
	root := ui.Column(c).Gap(6).AlignSelf(ui.Stretch)
	st := *ui.Local(root, "streameraid", func() *state { return &state{} })
	st.update(src, !o.Streaming, float64(fs), c.Now(), c.Preferences().ReduceMotion)
	return root.Children(func() {
		if st.showSource {
			sourceView(c, src, th)
		} else {
			diagram(c, st, th, root, o, labels)
		}
		if o.Streaming || len(st.problems) == 0 {
			return
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			var tips []string
			for _, p := range st.problems {
				if p.line > 0 {
					tips = append(tips, fmt.Sprintf("%d: %s", p.line, p.msg))
				} else {
					tips = append(tips, p.msg)
				}
			}
			ui.Text(c, fmt.Sprintf(labels.Problems, len(st.problems))).
				FontSize(fs * 0.85).TextColor(th.TextMuted).Tooltip(strings.Join(tips, "\n"))
			label := labels.ShowSource
			if st.showSource {
				label = labels.ShowDiagram
			}
			if ui.Button(c, label).Clicked() {
				st.showSource = !st.showSource
			}
		})
	})
}

func sourceView(c *ui.Context, src string, th *ui.Theme) {
	ui.Column(c).AlignSelf(ui.Stretch).Padding(10).Radius(6).Border(1, th.Border).Children(func() {
		ui.Text(c, strings.TrimRight(src, "\n")).Font("monospace").Selectable()
	})
}

// Zoom limits, and the factor of a step of the zoom buttons.
const (
	minZoom  = 0.25
	maxZoom  = 4
	zoomStep = 1.25
	// minFit is the smallest a diagram is scaled down to fit: smaller, its
	// texts could not be read, and it scrolls sideways instead.
	minFit = 0.45
)

// diagram builds the diagram: a box that paints shapes and lines, holding
// the labels as text elements placed over them, and the controls over it.
func diagram(c *ui.Context, st *state, th *ui.Theme, root ui.Element, o Options, labels Labels) {
	now := c.Now()
	still := c.Preferences().ReduceMotion
	f := st.motion.at(now)
	if f.w == 0 || f.h == 0 {
		return
	}
	// A complete diagram wider than the view is scaled down to fit, unless
	// the user chose a zoom. A diagram still streaming in keeps its size, so
	// that it does not shrink as it grows.
	target := 1.0
	avail := float64(root.Bounds().W)
	switch {
	case st.userZoom > 0:
		target = st.userZoom
	case st.final && !o.NoFit && avail > 0 && st.motion.to.w > avail:
		target = math.Max(avail/st.motion.to.w, minFit)
	}
	st.setZoom(target, now, still)
	z := st.zoom(now)
	if !st.motion.done(now) || st.zooming(now) || (st.final && avail == 0) {
		c.After(time.Second / 60)
	}
	pal := newPalette(th)
	fs := st.opt.FontSize
	wrap := ui.Column(c).AlignSelf(ui.Stretch)
	over := wrap.Hovered()
	wrap.Children(func() {
		ui.ScrollHorizontal(c).AlignSelf(ui.Stretch).Children(func() {
			box := ui.Box(c).Size(float32(f.w*z), float32(f.h*z)).Selectable()
			scaled := func(p *ui.Painter, r ui.Rect, draw func()) {
				if z == 1 {
					draw()
					return
				}
				p.Transform(ui.Scaling(float32(z), float32(z)), r.X, r.Y, draw)
			}
			if st.kind == streameraid.Sequence {
				lay, to := st.seqLay, st.motion.to
				box.Draw(func(p *ui.Painter, r ui.Rect) {
					scaled(p, r, func() { paintSequence(p, r, lay, f, to, pal, fs) })
				})
				box.Children(func() { sequenceLabels(c, lay, f, pal, fs, z) })
				return
			}
			lay, g := st.lay, st.graph
			// The node under the pointer: its edges stand out, and a click
			// on it is reported.
			hover := ""
			if x, y, in := box.PointerPosition(); in {
				hover = nodeAt(lay, f, geom.Point{X: float64(x) / z, Y: float64(y) / z})
			}
			box.Draw(func(p *ui.Painter, r ui.Rect) {
				scaled(p, r, func() { paint(p, r, g, lay, f, pal, fs, hover) })
			})
			box.Children(func() {
				for _, cb := range lay.Clusters {
					cf := f.clusters[cb.ID]
					_, _, text := pal.cluster(g, g.Subgraphs[cb.ID])
					labelText(c, cb.Title, cf.titleAt, text, cf.alpha, z)
				}
				for _, e := range lay.Edges {
					if e.HasLabel && g.Edges[e.Index].Line != flow.Invisible {
						ef := f.edges[e.ID]
						labelText(c, e.Label, ef.labelAt, pal.styled(g.Edges[e.Index].Style.Color, pal.text), ef.drawn, z)
					}
				}
				for _, nb := range lay.Nodes {
					nf := f.nodes[nb.ID]
					_, _, text := pal.node(g, g.Nodes[nb.ID])
					labelText(c, nb.Label, layout.LabelCenter(nb.Shape, nf.rect), text, nf.alpha, z)
				}
				if o.OnNodeClick == nil {
					return
				}
				// Nodes that take clicks: a box over each takes the pointer,
				// so their labels are clicked rather than selected.
				for _, nb := range lay.Nodes {
					id, r := nb.ID, f.nodes[nb.ID].rect
					ui.Box(c).Absolute().Left(float32(r.X*z)).Top(float32(r.Y*z)).Size(float32(r.W*z), float32(r.H*z)).
						Cursor(ui.CursorPointer).Label(strings.ReplaceAll(nb.Label.Lines[0], "\n", " ")).
						OnClick(func() { o.OnNodeClick(id) })
				}
			})
		})
		if !o.NoToolbar && (over || now.Before(st.copiedUntil)) {
			toolbar(c, st, th, o, labels, z)
		}
	})
}

// nodeAt returns the ID of the node whose box holds pt, or "".
func nodeAt(lay *layout.Flow, f *frame, pt geom.Point) string {
	for i := len(lay.Nodes) - 1; i >= 0; i-- {
		if nf, ok := f.nodes[lay.Nodes[i].ID]; ok && nf.rect.Contains(pt) {
			return lay.Nodes[i].ID
		}
	}
	return ""
}

// toolbar builds the controls at the top right of a diagram: zoom out,
// the zoom (which resets it), zoom in, copy the source, export as SVG.
func toolbar(c *ui.Context, st *state, th *ui.Theme, o Options, labels Labels, z float64) {
	now := c.Now()
	step := func(k float64) {
		st.userZoom = math.Max(minZoom, math.Min(maxZoom, st.zoomTo*k))
	}
	text := func(name, label string) string {
		if st.copied == name && now.Before(st.copiedUntil) {
			return labels.Copied
		}
		return label
	}
	done := func(name string) {
		st.copied, st.copiedUntil = name, now.Add(1500*time.Millisecond)
		c.After(1500 * time.Millisecond)
	}
	// Small, so that the controls cover little of the diagram.
	size := float32(st.fontSize) * 0.8
	button := func(text, name string) bool {
		b := ui.Button(c, text).FontSize(size).Padding(2, 7)
		if name != "" {
			b.Label(name).Tooltip(name)
		}
		return b.Clicked()
	}
	bar := ui.Row(c).Absolute().Top(2).Right(2).Gap(2).Padding(2).Radius(7).
		Background(th.Background).Border(1, th.Border).AlignItems(ui.Center).Unselectable()
	bar.Children(func() {
		if button("−", labels.ZoomOut) {
			step(1 / zoomStep)
		}
		if button(fmt.Sprintf("%.0f%%", z*100), labels.ZoomReset) {
			st.userZoom = 0
		}
		if button("+", labels.ZoomIn) {
			step(zoomStep)
		}
		if button(text("source", labels.CopySource), "") {
			c.WriteClipboard(st.src)
			done("source")
		}
		if button(text("svg", labels.ExportSVG), "") {
			if svg, err := streameraid.RenderSVG(st.src, streameraid.Options{Dark: th.Dark, FontSize: st.fontSize}); err == nil {
				if o.OnExport != nil {
					o.OnExport(svg)
				} else {
					c.WriteClipboard(svg)
				}
				done("svg")
			}
		}
	})
}
