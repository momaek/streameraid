// Package measure measures text for layouts without a UI toolkit: with the
// metrics of the Go fonts (Default), or with fixed widths (Fixed), whose
// results do not depend on any font and so suit tests.
package measure

import (
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"

	"github.com/momaek/streameraid/layout"
)

// LineHeight is the height of a line of text, in ems.
const LineHeight = 1.35

// Default measures text with the advance widths of the Go fonts, regular
// and bold. Characters they lack are measured as 1 em when wide (CJK,
// emoji) and 0.6 em otherwise. Its zero value is ready to use.
type Default struct {
	once         sync.Once
	regular      *sfnt.Font
	bold         *sfnt.Font
	mu           sync.Mutex
	cacheRegular map[rune]float64
	cacheBold    map[rune]float64
}

func (d *Default) init() {
	d.once.Do(func() {
		d.regular, _ = sfnt.Parse(goregular.TTF)
		d.bold, _ = sfnt.Parse(gobold.TTF)
		d.cacheRegular = map[rune]float64{}
		d.cacheBold = map[rune]float64{}
	})
}

// advance returns the width of r in ems.
func (d *Default) advance(r rune, bold bool) float64 {
	d.init()
	d.mu.Lock()
	defer d.mu.Unlock()
	cache, f := d.cacheRegular, d.regular
	if bold {
		cache, f = d.cacheBold, d.bold
	}
	if w, ok := cache[r]; ok {
		return w
	}
	w := fallbackWidth(r)
	if f != nil && w > 0 && !Wide(r) {
		var buf sfnt.Buffer
		if idx, err := f.GlyphIndex(&buf, r); err == nil && idx != 0 {
			upem := fixed.Int26_6(f.UnitsPerEm())
			if adv, err := f.GlyphAdvance(&buf, idx, upem, font.HintingNone); err == nil {
				w = float64(adv) / float64(upem)
			}
		}
	}
	cache[r] = w
	return w
}

// Measure measures text as layout.Measurer does.
func (d *Default) Measure(text string, style layout.TextStyle, maxWidth float64) layout.TextBox {
	return Wrap(text, style, maxWidth, func(r rune) float64 { return d.advance(r, style.Bold) * style.Size })
}

// Fixed measures every character as 0.6 em, or 1 em when wide. Its results
// depend on no font, so that layouts measured with it stay the same.
type Fixed struct{}

// Measure measures text as layout.Measurer does.
func (Fixed) Measure(text string, style layout.TextStyle, maxWidth float64) layout.TextBox {
	return Wrap(text, style, maxWidth, func(r rune) float64 { return fallbackWidth(r) * style.Size })
}

func fallbackWidth(r rune) float64 {
	switch {
	case r == 0x200D || r >= 0xFE00 && r <= 0xFE0F || unicode.Is(unicode.Mn, r):
		return 0 // joiners, variation selectors, combining marks
	case Wide(r):
		return 1
	}
	return 0.6
}

// Wide reports whether r takes the width of two Latin letters: CJK
// characters, fullwidth forms and emoji.
func Wide(r rune) bool {
	switch {
	case r >= 0x1100 && r <= 0x115F,
		r >= 0x2E80 && r <= 0x303E,
		r >= 0x3041 && r <= 0x33FF,
		r >= 0x3400 && r <= 0x4DBF,
		r >= 0x4E00 && r <= 0x9FFF,
		r >= 0xA000 && r <= 0xA4CF,
		r >= 0xAC00 && r <= 0xD7A3,
		r >= 0xF900 && r <= 0xFAFF,
		r >= 0xFE30 && r <= 0xFE4F,
		r >= 0xFF00 && r <= 0xFF60,
		r >= 0xFFE0 && r <= 0xFFE6,
		r >= 0x1F000 && r <= 0x1FAFF,
		r >= 0x20000 && r <= 0x3FFFD:
		return true
	}
	return false
}

// breaksAnywhere reports whether a line may break before or after r, as
// between CJK characters.
func breaksAnywhere(r rune) bool { return Wide(r) && !(r >= 0x1F000 && r <= 0x1FAFF) }

// Wrap breaks text into lines no wider than maxWidth (when positive), at
// spaces and around CJK characters, measuring characters with width, and
// returns the lines and their box.
func Wrap(text string, style layout.TextStyle, maxWidth float64, width func(rune) float64) layout.TextBox {
	lh := LineHeight * style.Size
	tb := layout.TextBox{LineHeight: lh, Style: style}
	measure := func(s string) float64 {
		w := 0.0
		for _, r := range s {
			w += width(r)
		}
		return w
	}
	for _, para := range strings.Split(text, "\n") {
		if maxWidth <= 0 || measure(para) <= maxWidth {
			tb.Lines = append(tb.Lines, para)
			continue
		}
		var line strings.Builder
		lineW := 0.0
		flush := func() {
			tb.Lines = append(tb.Lines, strings.TrimRight(line.String(), " "))
			line.Reset()
			lineW = 0
		}
		for _, tok := range tokens(para) {
			tw := measure(tok)
			if lineW > 0 && lineW+tw > maxWidth && strings.TrimSpace(tok) != "" {
				flush()
			}
			if lineW == 0 && strings.TrimSpace(tok) == "" {
				continue // no space at the start of a line
			}
			// A word wider than a line breaks between its characters.
			if tw > maxWidth {
				for _, r := range tok {
					rw := width(r)
					if lineW > 0 && lineW+rw > maxWidth {
						flush()
					}
					line.WriteRune(r)
					lineW += rw
				}
				continue
			}
			line.WriteString(tok)
			lineW += tw
		}
		if line.Len() > 0 {
			flush()
		}
	}
	for _, l := range tb.Lines {
		tb.W = max(tb.W, measure(l))
	}
	tb.H = float64(len(tb.Lines)) * lh
	return tb
}

// tokens splits a paragraph into the units lines break between: words,
// runs of spaces, and single CJK characters.
func tokens(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == ' ':
			if i > start {
				out = append(out, s[start:i])
			}
			j := i
			for j < len(s) && s[j] == ' ' {
				j++
			}
			out = append(out, s[i:j])
			i, start = j, j
			continue
		case breaksAnywhere(r):
			if i > start {
				out = append(out, s[start:i])
			}
			out = append(out, s[i:i+size])
			start = i + size
		}
		i += size
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}
