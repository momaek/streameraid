// Package pie is the model of a Mermaid pie chart and its parser.
//
// As with the other kinds, Parse reads the whole source each time, and the
// IDs of the slices stay the same as the source grows.
package pie

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/momaek/streameraid/flow"
	"github.com/momaek/streameraid/label"
)

// Slice is a slice of the pie.
type Slice struct {
	// ID is stable while the source grows: s0, s1, ... in the order of the
	// source.
	ID    string
	Label string
	Value float64
}

// Chart is a parsed pie chart.
type Chart struct {
	Title string
	// ShowData shows the slices' values beside their labels.
	ShowData bool
	Slices   []Slice
	// HasHeader tells whether the header (pie) was complete.
	HasHeader bool
	Problems  []flow.Problem
	// Final tells whether the source was complete when parsed.
	Final bool
}

// Total returns the sum of the slices' values.
func (c *Chart) Total() float64 {
	t := 0.0
	for _, s := range c.Slices {
		t += s.Value
	}
	return t
}

// Parse parses the source of a pie chart.
//
// final tells whether the source is complete. When it is not, its last
// line may be cut short: a slice shows once its value is complete, as a
// number that is still being written would change the whole pie with each
// digit, and a title shows as written so far.
func Parse(src string, final bool) *Chart {
	c := &Chart{Final: final}
	src = strings.ReplaceAll(src, "\r\n", "\n")
	complete := final || strings.HasSuffix(src, "\n")
	lines := strings.Split(src, "\n")
	if strings.HasSuffix(src, "\n") {
		lines = lines[:len(lines)-1]
	}
	problem := func(n int, text, format string, args ...any) {
		c.Problems = append(c.Problems, flow.Problem{Line: n, Text: text, Msg: fmt.Sprintf(format, args...)})
	}
	inFront, front := false, true
	for i, line := range lines {
		n := i + 1
		tail := i == len(lines)-1 && !complete
		t := strings.TrimSpace(line)
		if t == "---" && (front || inFront) {
			inFront = !inFront
			front = false
			continue
		}
		if inFront {
			if k, v, ok := strings.Cut(t, ":"); ok && strings.TrimSpace(k) == "title" && !tail {
				c.Title = label.Clean(v)
			}
			continue
		}
		if t == "" || strings.HasPrefix(t, "%%") {
			continue
		}
		front = false
		if !c.HasHeader {
			word, rest, _ := strings.Cut(t, " ")
			if word != "pie" {
				if !tail {
					problem(n, t, "expected pie, found %q", word)
				}
				continue
			}
			if tail && rest == "" {
				continue
			}
			c.HasHeader = true
			c.header(strings.TrimSpace(rest), tail)
			continue
		}
		word, rest, _ := strings.Cut(t, " ")
		switch word {
		case "title":
			c.Title = label.Clean(rest)
			continue
		case "showData":
			if !tail {
				c.ShowData = true
			}
			continue
		case "accTitle:", "accDescr:", "accTitle", "accDescr":
			continue
		}
		if t[0] != '"' && t[0] != '\'' {
			if !tail {
				problem(n, t, "expected a slice: \"label\" : value")
			}
			continue
		}
		name, value, ok := cutSlice(t)
		if !ok {
			if !tail {
				problem(n, t, "expected a slice: \"label\" : value")
			}
			continue
		}
		if tail {
			// The value may still grow.
			continue
		}
		v, err := strconv.ParseFloat(value, 64)
		switch {
		case err != nil:
			problem(n, t, "the value of %q is not a number: %q", name, value)
			continue
		case v < 0:
			problem(n, t, "the value of %q is negative", name)
			continue
		}
		c.Slices = append(c.Slices, Slice{ID: fmt.Sprintf("s%d", len(c.Slices)), Label: label.Clean(name), Value: v})
	}
	if final && !c.HasHeader && len(c.Problems) == 0 {
		c.Problems = append(c.Problems, flow.Problem{Msg: "missing pie header"})
	}
	return c
}

// header reads what follows pie: showData, then a title.
func (c *Chart) header(rest string, tail bool) {
	for rest != "" {
		word, more, _ := strings.Cut(rest, " ")
		switch {
		case word == "showData":
			c.ShowData = true
		case word == "title":
			c.Title = label.Clean(more)
			return
		case tail && (strings.HasPrefix("showData", word) || strings.HasPrefix("title", word)):
			return
		default:
			return
		}
		rest = strings.TrimSpace(more)
	}
}

// cutSlice reads "label" : value, the label in double or single quotes.
func cutSlice(t string) (name, value string, ok bool) {
	q := t[0]
	end := strings.IndexByte(t[1:], q)
	if end < 0 {
		return "", "", false
	}
	name = t[1 : end+1]
	rest := strings.TrimSpace(t[end+2:])
	if !strings.HasPrefix(rest, ":") {
		return "", "", false
	}
	value = strings.TrimSpace(rest[1:])
	if value == "" {
		return "", "", false
	}
	return name, value, true
}
