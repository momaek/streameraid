// Package css reads the CSS colors that Mermaid styles are written in.
package css

import (
	"math"
	"strconv"
	"strings"
)

// Color is an sRGB color with alpha.
type Color struct{ R, G, B, A uint8 }

// Hex returns c as #rrggbb, or #rrggbbaa when not opaque.
func (c Color) Hex() string {
	const digits = "0123456789abcdef"
	b := []byte{'#', digits[c.R>>4], digits[c.R&15], digits[c.G>>4], digits[c.G&15], digits[c.B>>4], digits[c.B&15]}
	if c.A != 255 {
		b = append(b, digits[c.A>>4], digits[c.A&15])
	}
	return string(b)
}

// ParseColor reads #rgb, #rgba, #rrggbb, #rrggbbaa, rgb(), rgba(), hsl(),
// hsla(), the named colors and transparent.
func ParseColor(s string) (Color, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return Color{}, false
	}
	if strings.HasPrefix(s, "#") {
		return parseHex(s[1:])
	}
	if open := strings.IndexByte(s, '('); open > 0 && strings.HasSuffix(s, ")") {
		fn := s[:open]
		args := strings.FieldsFunc(s[open+1:len(s)-1], func(r rune) bool { return r == ',' || r == ' ' || r == '/' })
		if len(args) < 3 {
			return Color{}, false
		}
		alpha := 1.0
		if len(args) >= 4 {
			alpha = number(args[3], 1)
		}
		switch fn {
		case "rgb", "rgba":
			return Color{channel(args[0]), channel(args[1]), channel(args[2]), unit(alpha)}, true
		case "hsl", "hsla":
			h := number(strings.TrimSuffix(args[0], "deg"), 360)
			sat := number(args[1], 100)
			l := number(args[2], 100)
			if !strings.HasSuffix(args[1], "%") {
				sat /= 100
			}
			if !strings.HasSuffix(args[2], "%") {
				l /= 100
			}
			r, g, b := hsl(h, sat, l)
			return Color{unit(r), unit(g), unit(b), unit(alpha)}, true
		}
		return Color{}, false
	}
	if s == "transparent" {
		return Color{}, true
	}
	if hex, ok := named[s]; ok {
		return parseHex(hex)
	}
	return Color{}, false
}

func parseHex(h string) (Color, bool) {
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return Color{}, false
	}
	switch len(h) {
	case 3:
		return Color{uint8(v>>8&15) * 17, uint8(v>>4&15) * 17, uint8(v&15) * 17, 255}, true
	case 4:
		return Color{uint8(v>>12&15) * 17, uint8(v>>8&15) * 17, uint8(v>>4&15) * 17, uint8(v&15) * 17}, true
	case 6:
		return Color{uint8(v >> 16), uint8(v >> 8), uint8(v), 255}, true
	case 8:
		return Color{uint8(v >> 24), uint8(v >> 16), uint8(v >> 8), uint8(v)}, true
	}
	return Color{}, false
}

// number reads a number or a percentage of full.
func number(s string, full float64) float64 {
	pct := strings.HasSuffix(s, "%")
	f, err := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64)
	if err != nil {
		return 0
	}
	if pct {
		return f / 100 * full
	}
	return f
}

func channel(s string) uint8 {
	return uint8(math.Round(math.Max(0, math.Min(255, number(s, 255)))))
}

func unit(f float64) uint8 { return uint8(math.Round(math.Max(0, math.Min(1, f)) * 255)) }

func hsl(h, s, l float64) (r, g, b float64) {
	h = math.Mod(h, 360) / 360
	if h < 0 {
		h++
	}
	if s == 0 {
		return l, l, l
	}
	var q float64
	if l < 0.5 {
		q = l * (1 + s)
	} else {
		q = l + s - l*s
	}
	p := 2*l - q
	return hue(p, q, h+1.0/3), hue(p, q, h), hue(p, q, h-1.0/3)
}

func hue(p, q, t float64) float64 {
	if t < 0 {
		t++
	}
	if t > 1 {
		t--
	}
	switch {
	case t < 1.0/6:
		return p + (q-p)*6*t
	case t < 0.5:
		return q
	case t < 2.0/3:
		return p + (q-p)*(2.0/3-t)*6
	}
	return p
}

// Luminance returns the relative luminance of c, from 0 for black to 1 for
// white.
func (c Color) Luminance() float64 {
	lin := func(v uint8) float64 {
		f := float64(v) / 255
		if f <= 0.04045 {
			return f / 12.92
		}
		return math.Pow((f+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.R) + 0.7152*lin(c.G) + 0.0722*lin(c.B)
}

var named = map[string]string{
	"black": "000000", "white": "ffffff", "red": "ff0000", "green": "008000", "blue": "0000ff",
	"yellow": "ffff00", "orange": "ffa500", "purple": "800080", "pink": "ffc0cb", "gray": "808080",
	"grey": "808080", "lightgray": "d3d3d3", "lightgrey": "d3d3d3", "darkgray": "a9a9a9", "darkgrey": "a9a9a9",
	"silver": "c0c0c0", "maroon": "800000", "olive": "808000", "lime": "00ff00", "aqua": "00ffff",
	"cyan": "00ffff", "teal": "008080", "navy": "000080", "fuchsia": "ff00ff", "magenta": "ff00ff",
	"brown": "a52a2a", "gold": "ffd700", "coral": "ff7f50", "salmon": "fa8072", "tomato": "ff6347",
	"crimson": "dc143c", "indigo": "4b0082", "violet": "ee82ee", "orchid": "da70d6", "plum": "dda0dd",
	"khaki": "f0e68c", "beige": "f5f5dc", "ivory": "fffff0", "lavender": "e6e6fa", "linen": "faf0e6",
	"skyblue": "87ceeb", "lightblue": "add8e6", "lightgreen": "90ee90", "lightyellow": "ffffe0",
	"lightpink": "ffb6c1", "darkred": "8b0000", "darkgreen": "006400", "darkblue": "00008b",
	"darkorange": "ff8c00", "steelblue": "4682b4", "royalblue": "4169e1", "dodgerblue": "1e90ff",
	"forestgreen": "228b22", "seagreen": "2e8b57", "limegreen": "32cd32", "turquoise": "40e0d0",
	"tan": "d2b48c", "chocolate": "d2691e", "firebrick": "b22222", "slategray": "708090",
	"slategrey": "708090", "whitesmoke": "f5f5f5", "gainsboro": "dcdcdc", "honeydew": "f0fff0",
	"mintcream": "f5fffa", "aliceblue": "f0f8ff", "lightcyan": "e0ffff", "palegreen": "98fb98",
	"lightcoral": "f08080", "lightsalmon": "ffa07a", "peachpuff": "ffdab9", "wheat": "f5deb3",
	"mistyrose": "ffe4e1", "lemonchiffon": "fffacd", "cornsilk": "fff8dc", "snow": "fffafa",
	"darkviolet": "9400d3", "mediumpurple": "9370db", "rebeccapurple": "663399", "hotpink": "ff69b4",
	"deeppink": "ff1493", "darkslategray": "2f4f4f", "dimgray": "696969", "dimgrey": "696969",
}
