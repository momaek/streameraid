package main

import (
	"image"
	"image/color"
	"image/gif"
	"os"
	"sort"
	"time"

	"github.com/egoist/mygo/ui"
)

// record plays a sample without a window, on a clock of its own, and
// writes what it shows as an animated GIF: the demo in the README is made
// this way.
//
//	go run ./examples/playground -gif demo.gif -sample 4
func record(a *app, path string, width, height int, dark bool) error {
	const step = 80 * time.Millisecond // 12.5 frames a second
	a.bare = true
	tt := ui.NewTester(a.view, width, height)
	tt.SetDark(dark)
	var frames []*image.RGBA
	grab := func() {
		src := tt.Image()
		cp := image.NewRGBA(src.Bounds())
		copy(cp.Pix, src.Pix)
		frames = append(frames, cp)
	}
	src := samples[a.sample].src
	for tail := 0; tail < 18; {
		tt.Frame()
		grab()
		tt.Advance(step)
		if a.pos >= len(src) {
			tail++ // hold the finished diagram a moment
		}
	}
	pal := palette(frames)
	out := &gif.GIF{Config: image.Config{ColorModel: pal, Width: width, Height: height}}
	lut := map[uint16]uint8{}
	var prev *image.Paletted
	for i, f := range frames {
		delay := int(step / (10 * time.Millisecond))
		if i == len(frames)-1 {
			delay = 250
		}
		full := paletted(f, pal, lut)
		// Most of the picture stays the same from a frame to the next: keep
		// only the rectangle that changed, drawn over the frame before.
		part := full
		if prev != nil {
			part = changed(prev, full)
		}
		prev = full
		out.Image = append(out.Image, part)
		out.Delay = append(out.Delay, delay)
		out.Disposal = append(out.Disposal, gif.DisposalNone)
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return gif.EncodeAll(file, out)
}

// bucket is a color to 5 bits a channel.
func bucket(r, g, b uint8) uint16 { return uint16(r>>3)<<10 | uint16(g>>3)<<5 | uint16(b>>3) }

// palette picks the 255 colors the frames use most, and a transparent one. A UI has few colors,
// and the shades at the edges of its text: the commonest cover it well.
func palette(frames []*image.RGBA) color.Palette {
	type tally struct {
		n       int
		r, g, b int
	}
	seen := map[uint16]*tally{}
	for _, f := range frames {
		for i := 0; i+3 < len(f.Pix); i += 4 * 3 { // every third pixel
			k := bucket(f.Pix[i], f.Pix[i+1], f.Pix[i+2])
			t := seen[k]
			if t == nil {
				t = &tally{}
				seen[k] = t
			}
			t.n++
			t.r += int(f.Pix[i])
			t.g += int(f.Pix[i+1])
			t.b += int(f.Pix[i+2])
		}
	}
	all := make([]*tally, 0, len(seen))
	for _, t := range seen {
		all = append(all, t)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].n != all[j].n {
			return all[i].n > all[j].n
		}
		return all[i].r*65536+all[i].g*256+all[i].b < all[j].r*65536+all[j].g*256+all[j].b
	})
	// One entry is kept for "as the frame before".
	if len(all) > 255 {
		all = all[:255]
	}
	pal := make(color.Palette, len(all), len(all)+1)
	for i, t := range all {
		pal[i] = color.RGBA{uint8(t.r / t.n), uint8(t.g / t.n), uint8(t.b / t.n), 255}
	}
	return append(pal, color.RGBA{})
}

// paletted maps a frame to the palette, remembering the nearest color of
// each bucket.
func paletted(f *image.RGBA, pal color.Palette, lut map[uint16]uint8) *image.Paletted {
	out := image.NewPaletted(f.Bounds(), pal)
	for i, j := 0, 0; i+3 < len(f.Pix); i, j = i+4, j+1 {
		k := bucket(f.Pix[i], f.Pix[i+1], f.Pix[i+2])
		idx, ok := lut[k]
		if !ok {
			idx = uint8(pal[:len(pal)-1].Index(color.RGBA{f.Pix[i], f.Pix[i+1], f.Pix[i+2], 255}))
			lut[k] = idx
		}
		out.Pix[j] = idx
	}
	return out
}

// changed returns the part of cur that differs from prev: the smallest
// rectangle holding every pixel that changed, or one pixel when none did.
// The last entry of the palette is the transparent one.
func changed(prev, cur *image.Paletted) *image.Paletted {
	b := cur.Bounds()
	box := image.Rectangle{Min: b.Max, Max: b.Min}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		row := (y - b.Min.Y) * cur.Stride
		for x := b.Min.X; x < b.Max.X; x++ {
			if cur.Pix[row+x-b.Min.X] != prev.Pix[row+x-b.Min.X] {
				box.Min.X, box.Min.Y = min(box.Min.X, x), min(box.Min.Y, y)
				box.Max.X, box.Max.Y = max(box.Max.X, x+1), max(box.Max.Y, y+1)
			}
		}
	}
	if box.Empty() {
		box = image.Rect(b.Min.X, b.Min.Y, b.Min.X+1, b.Min.Y+1)
	}
	// Inside it, what did not change is transparent, which lets the frame
	// before show through and compresses well.
	clear := uint8(len(cur.Palette) - 1)
	part := image.NewPaletted(box, cur.Palette)
	for y := box.Min.Y; y < box.Max.Y; y++ {
		from := (y-b.Min.Y)*cur.Stride + box.Min.X - b.Min.X
		to := (y - box.Min.Y) * part.Stride
		for x := 0; x < box.Dx(); x++ {
			if v := cur.Pix[from+x]; v != prev.Pix[from+x] {
				part.Pix[to+x] = v
			} else {
				part.Pix[to+x] = clear
			}
		}
	}
	return part
}
