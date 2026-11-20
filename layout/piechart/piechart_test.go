package piechart

import (
	"math"
	"testing"

	"github.com/momaek/streameraid/layout"
	"github.com/momaek/streameraid/measure"
	"github.com/momaek/streameraid/pie"
)

func TestLayout(t *testing.T) {
	c := pie.Parse("pie title Pets\n\"Dogs\" : 386\n\"Cats\" : 85\n\"Rats\" : 15\n", true)
	lay := Layout(c, measure.Fixed{}, layout.Options{})
	if len(lay.Slices) != 3 || lay.Width <= 0 || lay.Height <= 0 {
		t.Fatalf("%+v", lay)
	}
	if end := lay.Slices[2].End; math.Abs(end-2*math.Pi) > 1e-9 {
		t.Errorf("the slices turn %v, not a whole circle", end)
	}
	for _, s := range lay.Slices {
		if s.Swatch.Right() > lay.Width || s.ShareAt.X+s.Share.W/2 > lay.Width+0.01 {
			t.Errorf("slice %s's row overflows the chart", s.ID)
		}
	}
}

// TestStreaming checks that what is there holds still as slices come in:
// the pie and the legend's rows; only the angles change.
func TestStreaming(t *testing.T) {
	src := "pie title Pets adopted by volunteers\n\"Dogs\" : 386\n\"Cats\" : 85\n\"Rats\" : 15\n\"Hamsters\" : 7\n"
	var prev *layout.Pie
	for end := 1; end <= len(src); end++ {
		lay := Layout(pie.Parse(src[:end], end == len(src)), measure.Fixed{}, layout.Options{})
		if prev != nil {
			if lay.Center != prev.Center || lay.Radius != prev.Radius {
				t.Fatalf("at %d the pie moved", end)
			}
			for i, s := range prev.Slices {
				if lay.Slices[i].Swatch.Y != s.Swatch.Y {
					t.Fatalf("at %d legend row %s moved", end, s.ID)
				}
			}
		}
		if len(lay.Slices) > 0 {
			prev = lay
		}
	}
}
