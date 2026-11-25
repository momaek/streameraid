package mermaid

import (
	"math"

	"github.com/egoist/mygo/ui"

	"github.com/momaek/streameraid/geom"
	"github.com/momaek/streameraid/layout"
)

// The parts of a pie chart in a frame: a slice's angles are kept as the
// X and W of a box under "slice:" (its start, and how far it turns), so that
// slices turn to their new angles as values come in; legend rows are
// boxes under "legend:".

// frameOfPie returns the frame showing all of a pie chart's layout.
func frameOfPie(lay *layout.Pie) *frame {
	f := &frame{w: lay.Width, h: lay.Height, nodes: map[string]nodeFrame{}, clusters: map[string]clusterFrame{}, edges: map[string]edgeFrame{}}
	for _, s := range lay.Slices {
		f.nodes["slice:"+s.ID] = nodeFrame{rect: geom.Rect{X: s.Start, W: s.End - s.Start}, alpha: 1}
		f.nodes["legend:"+s.ID] = nodeFrame{rect: s.Swatch, alpha: 1}
	}
	return f
}

// paintPie draws a pie chart as frame f shows it, without its texts.
func paintPie(p *ui.Painter, r ui.Rect, lay *layout.Pie, f *frame, pal palette, fs float64) {
	dx, dy := r.X, r.Y
	turned := 0.0
	for _, s := range lay.Slices {
		turned += f.nodes["slice:"+s.ID].rect.W
	}
	if turned <= 1e-9 {
		var ring geom.Path
		ring.Ellipse(lay.Center, lay.Radius, lay.Radius)
		p.StrokePath(toPath(ring, dx, dy), 1, pal.clusterStroke)
	}
	for _, s := range lay.Slices {
		sf, lf := f.nodes["slice:"+s.ID], f.nodes["legend:"+s.ID]
		color := pal.seriesColor(s.Series)
		if sf.rect.W > 1e-9 {
			// A new slice grows from nothing as the others make room.
			a0, a1 := sf.rect.X, sf.rect.X+sf.rect.W
			wedge := toPath(layout.Wedge(lay.Center, lay.Radius, a0, math.Min(a1, 2*math.Pi)), dx, dy)
			p.FillPath(wedge, color)
			p.StrokePath(wedge, 2, pal.bg)
		}
		p.Fill(toRect(lf.rect, dx, dy), fade(color, lf.alpha), float32(0.2*fs))
	}
}

// pieLabels places a pie chart's texts.
func pieLabels(c *ui.Context, lay *layout.Pie, f *frame, pal palette, z float64) {
	labelText(c, lay.Title, lay.TitleAt, pal.text, 1, z)
	for _, s := range lay.Slices {
		lf := f.nodes["legend:"+s.ID]
		shift := geom.Point{X: lf.rect.X - s.Swatch.X, Y: lf.rect.Y - s.Swatch.Y}
		labelText(c, s.Label, s.LabelAt.Add(shift), pal.text, lf.alpha, z)
		labelText(c, s.Share, s.ShareAt.Add(shift), pal.clusterText, lf.alpha, z)
	}
}
