// Package label turns the markup of Mermaid labels into plain text.
package label

import (
	"strconv"
	"strings"
	"unicode"
)

// Unquote removes the double quotes around s, or one at its start.
func Unquote(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return strings.TrimPrefix(s, "\"")
}

var labelTags = []string{"b", "i", "u", "s", "em", "strong", "small", "sub", "sup", "code", "span", "p", "div", "font", "big", "del", "ins", "mark"}

// Clean turns label markup into plain text: quotes and backticks of
// Markdown strings go, <br> becomes a line break, formatting tags and
// Font Awesome icons go, and entities are decoded.
func Clean(s string) string {
	s = Unquote(strings.TrimSpace(s))
	if len(s) >= 2 && s[0] == '`' && s[len(s)-1] == '`' {
		s = s[1 : len(s)-1]
	}
	return Plain(s)
}

// Plain turns the markup of a text into plain text as Clean does, keeping
// quotes: the text of a message may well be in quotes.
func Plain(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == '<' {
			if end := strings.IndexByte(s[i:], '>'); end > 0 {
				tag := strings.ToLower(strings.TrimSpace(s[i+1 : i+end]))
				name := strings.TrimSuffix(strings.TrimPrefix(tag, "/"), "/")
				name, _, _ = strings.Cut(strings.TrimSpace(name), " ")
				if name == "br" {
					b.WriteByte('\n')
					i += end + 1
					continue
				}
				for _, t := range labelTags {
					if name == t {
						i += end + 1
						goto next
					}
				}
			}
		}
		if s[i] == '#' || s[i] == '&' {
			if r, n := entity(s[i:]); n > 0 {
				b.WriteString(r)
				i += n
				continue
			}
		}
		if strings.HasPrefix(s[i:], "fa:fa-") {
			j := i + 6
			for j < len(s) && (s[j] == '-' || s[j] >= 'a' && s[j] <= 'z' || s[j] >= '0' && s[j] <= '9') {
				j++
			}
			i = j
			continue
		}
		b.WriteByte(s[i])
		i++
	next:
	}
	lines := strings.Split(b.String(), "\n")
	for i, l := range lines {
		lines[i] = strings.Join(strings.Fields(l), " ")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

var namedEntities = map[string]string{
	"amp": "&", "lt": "<", "gt": ">", "quot": "\"", "apos": "'", "nbsp": " ", "copy": "©", "reg": "®",
	"hellip": "…", "mdash": "—", "ndash": "–", "rarr": "→", "larr": "←", "harr": "↔", "check": "✓",
}

// entity decodes an HTML entity (&amp; &#35;) or a Mermaid one (#quot;
// #35;) at the start of s.
func entity(s string) (string, int) {
	end := strings.IndexByte(s, ';')
	if end < 2 || end > 10 {
		return "", 0
	}
	body := s[1:end]
	if s[0] == '&' && strings.HasPrefix(body, "#") {
		body = body[1:]
	}
	if n, err := strconv.Atoi(body); err == nil && n > 0 && n <= unicode.MaxRune {
		return string(rune(n)), end + 1
	}
	if strings.HasPrefix(body, "x") || strings.HasPrefix(body, "X") {
		if n, err := strconv.ParseInt(body[1:], 16, 32); err == nil && n > 0 {
			return string(rune(n)), end + 1
		}
	}
	if r, ok := namedEntities[body]; ok {
		return r, end + 1
	}
	return "", 0
}
