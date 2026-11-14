package svg

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/momaek/streameraid/geom"
	"github.com/momaek/streameraid/layout"
)

// Sequence draws a laid out sequence diagram. fontSize is the size the
// layout was made with.
func Sequence(s *layout.Sequence, th Theme, fontSize float64) string {
	var b strings.Builder
	em := fontSize
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%s" height="%s" viewBox="0 0 %s %s" font-family="%s" font-size="%s">`,
		num(s.Width), num(s.Height), num(s.Width), num(s.Height), attr(th.FontFamily), num(fontSize))
	b.WriteByte('\n')
	if th.Background != "" {
		fmt.Fprintf(&b, `<rect width="100%%" height="100%%" fill="%s"/>`+"\n", attr(th.Background))
	}
	rect := func(r geom.Rect, radius float64, extra string) {
		fmt.Fprintf(&b, `<rect x="%s" y="%s" width="%s" height="%s" rx="%s" %s/>`, num(r.X), num(r.Y), num(r.W), num(r.H), num(radius), extra)
	}
	line := 0.09 * em
	for _, g := range s.Groups {
		fill, text := th.ClusterFill, th.ClusterText
		if g.Color != "" {
			fill, text = g.Color, textOn(g.Color, th)
		}
		fmt.Fprintf(&b, `<g class="group" data-id="%s">`, attr(g.ID))
		rect(g.Rect, 0.4*em, fmt.Sprintf(`fill="%s" stroke="%s"`, attr(fill), attr(th.ClusterStroke)))
		writeText(&b, g.Title, g.TitleAt, text, "")
		b.WriteString("</g>\n")
	}
	for _, bl := range s.Blocks {
		if bl.Kind == "rect" && bl.ID != "" {
			fill := pick(bl.Fill, th.ClusterFill)
			rect(bl.Rect, 0.3*em, fmt.Sprintf(`class="block-fill" data-id="%s" fill="%s"`, attr(bl.ID), attr(fill)))
			b.WriteByte('\n')
		}
	}
	for _, p := range s.Participants {
		fmt.Fprintf(&b, `<path class="lifeline" data-id="%s" d="M%s,%sL%s,%s" stroke="%s" stroke-width="%s"/>`,
			attr(p.ID), num(p.LifeTop.X), num(p.LifeTop.Y), num(p.LifeBottom.X), num(p.LifeBottom.Y), attr(th.ClusterStroke), num(line))
		if p.Destroyed {
			r := 0.45 * em
			c := p.LifeBottom
			fmt.Fprintf(&b, `<path d="M%s,%sL%s,%sM%s,%sL%s,%s" stroke="%s" stroke-width="%s" stroke-linecap="round"/>`,
				num(c.X-r), num(c.Y-r), num(c.X+r), num(c.Y+r), num(c.X-r), num(c.Y+r), num(c.X+r), num(c.Y-r), attr(th.Edge), num(2*line))
		}
		b.WriteByte('\n')
	}
	for _, a := range s.Activations {
		rect(a.Rect, 0.1*em, fmt.Sprintf(`class="activation" data-id="%s" fill="%s" stroke="%s" stroke-width="%s"`, attr(a.ID), attr(th.NodeFill), attr(th.NodeStroke), num(line)))
		b.WriteByte('\n')
	}
	for _, bl := range s.Blocks {
		if bl.Kind == "rect" || bl.ID == "" {
			continue
		}
		fmt.Fprintf(&b, `<g class="block" data-id="%s">`, attr(bl.ID))
		rect(bl.Rect, 0.3*em, fmt.Sprintf(`fill="none" stroke="%s" stroke-width="%s"`, attr(th.Edge), num(line)))
		t := bl.Tab
		cut := 0.45 * em
		fmt.Fprintf(&b, `<path d="M%s,%sL%s,%sL%s,%sL%s,%sL%s,%sZ" fill="%s" stroke="%s" stroke-width="%s"/>`,
			num(t.X), num(t.Y), num(t.Right()), num(t.Y), num(t.Right()), num(t.Bottom()-cut), num(t.Right()-cut), num(t.Bottom()), num(t.X), num(t.Bottom()),
			attr(th.ClusterFill), attr(th.Edge), num(line))
		writeText(&b, bl.KindLabel, bl.KindAt, th.Text, ` font-weight="bold"`)
		writeText(&b, bl.Title, bl.TitleAt, th.Text, "")
		for _, sec := range bl.Sections {
			fmt.Fprintf(&b, `<path d="M%s,%sL%s,%s" stroke="%s" stroke-width="%s" stroke-dasharray="%s %s"/>`,
				num(bl.Rect.X), num(sec.Y), num(bl.Rect.Right()), num(sec.Y), attr(th.Edge), num(line), num(0.3*em), num(0.3*em))
			writeText(&b, sec.Title, sec.TitleAt, th.Text, "")
		}
		b.WriteString("</g>\n")
	}
	for _, m := range s.Messages {
		fmt.Fprintf(&b, `<g class="message" data-id="%s">`, attr(m.ID))
		dash := ""
		if m.Dotted {
			dash = fmt.Sprintf(` stroke-dasharray="%s %s"`, num(0.3*em), num(0.3*em))
		}
		fmt.Fprintf(&b, `<path d="%s" fill="none" stroke="%s" stroke-width="%s"%s/>`, pathData(m.Path), attr(th.Edge), num(0.11*em), dash)
		for _, h := range m.Heads {
			if h.Filled {
				fmt.Fprintf(&b, `<path d="%s" fill="%s"/>`, pathData(h.Shape), attr(th.Edge))
			} else {
				fmt.Fprintf(&b, `<path d="%s" fill="none" stroke="%s" stroke-width="%s" stroke-linecap="round" stroke-linejoin="round"/>`, pathData(h.Shape), attr(th.Edge), num(0.11*em))
			}
		}
		if len(m.Label.Lines) > 0 {
			bg := geom.RectCentered(m.LabelAt, m.Label.W+0.4*em, m.Label.H)
			rect(bg, 0.2*em, fmt.Sprintf(`fill="%s" opacity="0.85"`, attr(th.EdgeLabelBg)))
			writeText(&b, m.Label, m.LabelAt, th.Text, "")
		}
		if m.Number != 0 {
			fmt.Fprintf(&b, `<circle cx="%s" cy="%s" r="%s" fill="%s"/>`, num(m.NumberAt.X), num(m.NumberAt.Y), num(m.NumberRadius), attr(th.Edge))
			fmt.Fprintf(&b, `<text x="%s" y="%s" text-anchor="middle" font-size="%s" font-weight="bold" fill="%s">%s</text>`,
				num(m.NumberAt.X), num(m.NumberAt.Y+0.28*em), num(0.78*em), attr(th.EdgeLabelBg), strconv.Itoa(m.Number))
		}
		b.WriteString("</g>\n")
	}
	for _, n := range s.Notes {
		fmt.Fprintf(&b, `<g class="note" data-id="%s">`, attr(n.ID))
		rect(n.Rect, 0.2*em, fmt.Sprintf(`fill="%s" stroke="%s" stroke-width="%s"`, attr(th.NoteFill), attr(th.NoteStroke), num(line)))
		writeText(&b, n.Label, n.LabelAt, th.NoteText, "")
		b.WriteString("</g>\n")
	}
	for _, p := range s.Participants {
		fmt.Fprintf(&b, `<g class="participant" data-id="%s">`, attr(p.ID))
		if p.Actor {
			fmt.Fprintf(&b, `<path d="%s" fill="%s" stroke="%s" stroke-width="%s" stroke-linecap="round" stroke-linejoin="round"/>`,
				pathData(p.Figure), attr(th.NodeFill), attr(th.NodeStroke), num(0.12*em))
		} else {
			rect(p.Rect, 0.25*em, fmt.Sprintf(`fill="%s" stroke="%s" stroke-width="%s"`, attr(th.NodeFill), attr(th.NodeStroke), num(line)))
		}
		writeText(&b, p.Label, p.LabelAt, th.Text, "")
		b.WriteString("</g>\n")
	}
	if len(s.Title.Lines) > 0 {
		writeText(&b, s.Title, s.TitleAt, th.Text, ` font-weight="bold"`)
		b.WriteByte('\n')
	}
	b.WriteString("</svg>\n")
	return b.String()
}
