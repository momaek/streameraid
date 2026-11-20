// Package piechart lays out pie charts: the pie on the left, a legend of
// its slices on the right, the title above the pie.
//
// The pie stays where it is as slices stream in: rows are added to the
// bottom of the legend, and only the angles of the slices change.
package piechart

import (
	"math"
	"strconv"

	"github.com/momaek/streameraid/geom"
	"github.com/momaek/streameraid/layout"
	"github.com/momaek/streameraid/pie"
)

// Layout lays out a pie chart.
func Layout(c *pie.Chart, m layout.Measurer, opt layout.Options) *layout.Pie {
	opt = opt.Defaults()
	fs := opt.FontSize
	st := opt.TextStyle()
	margin := opt.Margin
	r := 6.5 * fs
	out := &layout.Pie{Radius: r}
	top := margin
	cx := margin + r
	if c.Title != "" {
		ts := st
		ts.Bold = true
		out.Title = m.Measure(c.Title, ts, 2.5*opt.WrapWidth)
		cx = margin + math.Max(r, out.Title.W/2)
		out.TitleAt = geom.Point{X: cx, Y: top + out.Title.H/2}
		top += out.Title.H + 0.8*fs
	}
	out.Center = geom.Point{X: cx, Y: top + r}
	right := cx + math.Max(r, out.Title.W/2)
	bottom := top + 2*r

	total := c.Total()
	legendX := cx + r + 2*fs
	rowH := m.Measure("M", st, 0).H * 1.6
	sw := 0.85 * fs
	// The shares line up at the right of a column after the widest label.
	labelW := 0.0
	labels := make([]layout.TextBox, len(c.Slices))
	shares := make([]layout.TextBox, len(c.Slices))
	for i, s := range c.Slices {
		labels[i] = m.Measure(s.Label, st, opt.WrapWidth)
		labelW = math.Max(labelW, labels[i].W)
		shares[i] = m.Measure(share(s.Value, total, c.ShowData), st, 0)
	}
	shareW := 0.0
	for _, tb := range shares {
		shareW = math.Max(shareW, tb.W)
	}
	// Rows start at the top of the pie, not centered beside it, so that
	// a new row leaves those above it where they are.
	y := top
	angle := 0.0
	for i, s := range c.Slices {
		ps := layout.PieSlice{ID: s.ID, Series: i + 1, Start: angle, End: angle}
		if total > 0 {
			ps.End = angle + 2*math.Pi*s.Value/total
		}
		angle = ps.End
		h := math.Max(rowH, labels[i].H+0.6*fs)
		mid := y + h/2
		ps.Swatch = geom.RectCentered(geom.Point{X: legendX + sw/2, Y: mid}, sw, sw)
		lx := legendX + sw + 0.6*fs
		ps.Label, ps.LabelAt = labels[i], geom.Point{X: lx + labels[i].W/2, Y: mid}
		sx := lx + labelW + 1.5*fs + shareW
		ps.Share, ps.ShareAt = shares[i], geom.Point{X: sx - shares[i].W/2, Y: mid}
		right = math.Max(right, sx)
		y += h
		out.Slices = append(out.Slices, ps)
	}
	bottom = math.Max(bottom, y)
	out.Width = math.Ceil(right + margin)
	out.Height = math.Ceil(bottom + margin)
	return out
}

// share writes a slice's share of the total, and its value when asked.
func share(v, total float64, showData bool) string {
	pct := 0.0
	if total > 0 {
		pct = v / total * 100
	}
	s := strconv.FormatFloat(math.Round(pct*10)/10, 'f', -1, 64) + "%"
	if showData {
		s = strconv.FormatFloat(v, 'f', -1, 64) + " · " + s
	}
	return s
}
