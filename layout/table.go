package layout

import (
	"math"

	"github.com/momaek/streameraid/flow"
	"github.com/momaek/streameraid/geom"
)

// TableBox is the geometry of a box of sections (flow.Sectioned), relative
// to the box's top left corner, so that it follows the box as it moves.
type TableBox struct {
	// W and H are the box's size.
	W, H  float64
	Cells []TableCell
	// Dividers are the heights of the lines between sections.
	Dividers []float64
}

// TableCell is a text of a table: one cell of a row, on one line.
type TableCell struct {
	Text TextBox
	// At is the center of the text, from the box's top left corner.
	At geom.Point
	// Left tells that the text is aligned left, on At.X - Text.W/2: a
	// renderer measuring with other fonts starts it there.
	Left bool
}

// MeasureTable sizes a box of sections and places its cells: rows one
// above the other, their cells in columns across each section.
func MeasureTable(t *flow.Table, m Measurer, opt Options) TableBox {
	opt = opt.Defaults()
	fs := opt.FontSize
	padX, padY := 0.8*fs, 0.35*fs
	colGap := 1.0 * fs
	type sec struct {
		cells  [][]TextBox
		widths []float64
		w, h   float64
		lineH  float64
	}
	var secs []sec
	width := 4 * fs
	for _, s := range t.Sections {
		st := opt.TextStyle()
		st.Bold = s.Bold
		if s.Small {
			st.Size = math.Round(fs*0.85*100) / 100
		}
		var sc sec
		sc.lineH = m.Measure("M", st, 0).H
		for r, row := range s.Rows {
			rs := s.RowStyle(r)
			st := st
			st.Italic, st.Underline = rs&flow.RowItalic != 0, rs&flow.RowUnderline != 0
			var cells []TextBox
			for i, text := range row {
				tb := m.Measure(text, st, 0)
				cells = append(cells, tb)
				if i >= len(sc.widths) {
					sc.widths = append(sc.widths, 0)
				}
				sc.widths[i] = math.Max(sc.widths[i], tb.W)
			}
			sc.cells = append(sc.cells, cells)
		}
		for i, w := range sc.widths {
			sc.w += w
			if i > 0 && w > 0 {
				sc.w += colGap
			}
		}
		for _, cells := range sc.cells {
			h := sc.lineH
			for _, c := range cells {
				h = math.Max(h, c.H)
			}
			sc.h += h
		}
		if len(sc.cells) == 0 {
			// An empty section of a class still shows, as a thin band.
			sc.h = 0.5 * fs
		} else {
			sc.h += 2 * padY
		}

		width = math.Max(width, sc.w+2*padX)
		secs = append(secs, sc)
	}
	// A joined section and the one above it share their padding.
	topPad := make([]float64, len(secs))
	for i := range secs {
		if len(secs[i].cells) > 0 {
			topPad[i] = padY
		}
		if i > 0 && t.Sections[i].Joined {
			if len(secs[i].cells) > 0 {
				secs[i].h -= padY
				topPad[i] = 0
			}
			if len(secs[i-1].cells) > 0 {
				secs[i-1].h -= padY / 2
				secs[i].h += padY / 2
				topPad[i] += padY / 2
			}
		}
	}
	tb := TableBox{W: math.Ceil(width)}
	y := 0.0
	for i, sc := range secs {
		if i > 0 && !t.Sections[i].Joined {
			tb.Dividers = append(tb.Dividers, y)
		}
		ry := y + topPad[i]
		for _, cells := range sc.cells {
			h := sc.lineH
			for _, c := range cells {
				h = math.Max(h, c.H)
			}
			x := padX
			if t.Sections[i].Center {
				x = (width - sc.w) / 2
			}
			for k, c := range cells {
				if len(c.Lines) > 0 {
					tb.Cells = append(tb.Cells, TableCell{Text: c, At: geom.Point{X: x + c.W/2, Y: ry + h/2}, Left: !t.Sections[i].Center})
				}
				if sc.widths[k] > 0 {
					x += sc.widths[k] + colGap
				}
			}
			ry += h
		}
		y += sc.h
	}
	// A title alone (a class with no members) gets a little more room,
	// its rows centered in it.
	if len(tb.Dividers) == 0 && y < 2.4*fs {
		shift := (2.4*fs - y) / 2
		for i := range tb.Cells {
			tb.Cells[i].At.Y += shift
		}
		y = 2.4 * fs
	}
	tb.H = math.Ceil(y)
	return tb
}

// CellAt returns where a cell's text is centered in a box drawn in rect,
// which may differ from the box's own size while it moves or grows.
func (t *TableBox) CellAt(c TableCell, rect geom.Rect) geom.Point {
	kx, ky := 1.0, 1.0
	if t.W > 0 {
		kx = rect.W / t.W
	}
	if t.H > 0 {
		ky = rect.H / t.H
	}
	return geom.Point{X: rect.X + c.At.X*kx, Y: rect.Y + c.At.Y*ky}
}

// NodeLines returns the lines a node draws over its outline, in rect: the
// dividers of a box of sections, the decorations of other shapes.
func NodeLines(nb NodeBox, rect geom.Rect, fontSize float64) geom.Path {
	if nb.Table == nil {
		return Decorations(nb.Shape, rect, fontSize)
	}
	var p geom.Path
	ky := 1.0
	if nb.Table.H > 0 {
		ky = rect.H / nb.Table.H
	}
	for _, y := range nb.Table.Dividers {
		p.MoveTo(geom.Point{X: rect.X, Y: rect.Y + y*ky}).LineTo(geom.Point{X: rect.Right(), Y: rect.Y + y*ky})
	}
	return p
}
