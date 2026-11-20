// Package mindmap parses Mermaid mind maps into the model of flowcharts,
// flow.Graph: a tree of nodes, each joined to its parent by an edge, for
// the tree layout of package layout/tree to place.
//
// A mind map is a tree by indentation:
//
//	mindmap
//	  root((Topic))
//	    Branch
//	      Leaf
//	      ::icon(fa fa-book)
//	    id[Another branch]
//	    :::urgent
//
// The first node is the root. Every later line is a child of the nearest
// line before it that is indented less. A space counts as one column of
// indentation and a tab as four.
//
// Nodes are named by their place in the tree, not by the IDs of the
// source, which may repeat or be absent: the root is "n0", its third child
// "n0.2", that child's first child "n0.2.0". Lines are only ever added at
// the end of a streaming source, so a node keeps its name.
package mindmap

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/momaek/streameraid/flow"
	"github.com/momaek/streameraid/label"
)

// tabWidth is the number of columns of indentation a tab counts for.
const tabWidth = 4

// branchColors is the number of series colors the branches of the root
// cycle through.
const branchColors = 8

// Parse parses the source of a mind map.
//
// final tells whether the source is complete. When it is not, and the
// source does not end with a newline, its last line is still being
// written, and Parse reads it so that nothing appears, goes away and comes
// back as characters arrive:
//
//   - A node's place in the tree is known from the first character of its
//     line, and its name with it, so it never changes.
//   - A line of text shows, its node marked Partial, once it has a
//     character an ID cannot have (a space, a dot, a CJK character): until
//     then it may still be an ID with a shape to come (Root, then
//     Root((Topic))), or a keyword.
//   - A node with a shape shows once its label has begun, with the label
//     written so far. Its shape may change while it is Partial, as the
//     opening of id(( arrives in two characters.
//   - Lines that start with : or % (icons, classes, comments) and
//     classDef statements apply once complete.
//
// Lines Parse cannot read are recorded in Graph.Problems. A node whose
// bracket is not closed, or with text after its closing bracket, is still
// drawn, so that the names of the nodes after it stay the same.
func Parse(src string, final bool) *flow.Graph {
	p := &parser{g: flow.NewGraph(), children: map[string]int{}, icons: map[string]string{}}
	p.g.Final = final
	src = strings.ReplaceAll(src, "\r\n", "\n")
	complete := final || strings.HasSuffix(src, "\n")
	lines := strings.Split(src, "\n")
	if strings.HasSuffix(src, "\n") {
		lines = lines[:len(lines)-1]
	}
	start, ok := p.frontMatter(lines, complete)
	if !ok {
		return p.g
	}
	for i := start; i < len(lines); i++ {
		p.line = i + 1
		text := lines[i]
		// A Markdown string ("` ... `") may go on over several lines.
		for openMarkdownString(text) && i+1 < len(lines) {
			i++
			text += "\n" + lines[i]
		}
		last := i == len(lines)-1
		p.parseLine(text, last && (!complete || !final && openMarkdownString(text)))
	}
	p.finish()
	return p.g
}

type parser struct {
	g     *flow.Graph
	line  int
	text  string
	order int
	// stack holds the lines that may be parents of the next one, by
	// indentation, innermost last. A line that made no node (one with no
	// parent) has id "": its children are skipped with it.
	stack []level
	root  string
	// children counts the children of each node so far.
	children map[string]int
	// last is the node the last node line made, which icons and classes
	// decorate.
	last string
	// icons are the glyphs of the nodes' icons, drawn before their labels.
	icons map[string]string
	// inAccDescr is set inside an accDescr { ... } block.
	inAccDescr bool
}

type level struct {
	indent int
	id     string
}

func (p *parser) problem(format string, args ...any) {
	p.g.Problems = append(p.g.Problems, flow.Problem{Line: p.line, Text: p.text, Msg: fmt.Sprintf(format, args...)})
}

// frontMatter skips a YAML front matter block (--- ... ---), reading its
// title. It returns the index of the first line after it, and false when
// the block has not ended yet.
func (p *parser) frontMatter(lines []string, complete bool) (int, bool) {
	first := -1
	for i, l := range lines {
		if strings.TrimSpace(l) != "" {
			first = i
			break
		}
	}
	if first < 0 || strings.TrimSpace(lines[first]) != "---" {
		return 0, true
	}
	if !complete && first == len(lines)-1 {
		return 0, false
	}
	for i := first + 1; i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		if t == "---" && (complete || i < len(lines)-1) {
			return i + 1, true
		}
		if k, v, ok := strings.Cut(t, ":"); ok && strings.TrimSpace(k) == "title" && lines[i] == strings.TrimLeft(lines[i], " \t") {
			p.g.Title = label.Unquote(strings.TrimSpace(v))
		}
	}
	if complete {
		p.line = first + 1
		p.text = lines[first]
		p.problem("front matter is not closed with ---")
	}
	return 0, false
}

// keywords start lines that are statements rather than nodes.
var keywords = []string{"classDef", "accTitle", "accDescr"}

func (p *parser) parseLine(line string, tail bool) {
	t := strings.TrimSpace(line)
	if t == "" || strings.HasPrefix(t, "%%") {
		return
	}
	p.text = t
	if p.inAccDescr {
		if strings.Contains(line, "}") {
			p.inAccDescr = false
		}
		return
	}
	if !p.g.HasHeader {
		p.header(t, tail)
		return
	}
	if tail && (t[0] == ':' || t[0] == '%') {
		// An icon, classes or a comment, still being written.
		return
	}
	switch {
	case strings.HasPrefix(t, "::icon("):
		p.icon(t[len("::icon("):])
		return
	case strings.HasPrefix(t, ":::"):
		p.classes(t[3:])
		return
	}
	word, rest := firstWord(t)
	switch strings.TrimSuffix(word, ":") {
	case "classDef":
		if !tail {
			p.classDef(rest)
		}
		return
	case "accTitle":
		return
	case "accDescr":
		if !tail && strings.Contains(t, "{") && !strings.Contains(t, "}") {
			p.inAccDescr = true
		}
		return
	}
	p.node(line, tail)
}

func (p *parser) header(t string, tail bool) {
	word, _ := firstWord(t)
	if tail {
		// The header may still be coming.
		return
	}
	if word != "mindmap" {
		p.problem("expected mindmap, found %q", word)
		return
	}
	p.g.HasHeader = true
}

// icon reads the classes of ::icon(classes) and gives the last node the
// glyph of the icon, if it has one: see Glyph.
func (p *parser) icon(rest string) {
	classes, closed := strings.CutSuffix(strings.TrimSpace(rest), ")")
	if !closed {
		p.problem("::icon( is missing its closing )")
		return
	}
	if p.last == "" {
		if p.root == "" {
			p.problem("an icon needs a node before it")
		}
		return
	}
	if g := Glyph(classes); g != "" {
		p.icons[p.last] = g
	}
}

func (p *parser) classes(rest string) {
	names := strings.Fields(rest)
	if len(names) == 0 {
		p.problem("::: needs class names")
		return
	}
	if p.last == "" {
		if p.root == "" {
			p.problem("classes need a node before them")
		}
		return
	}
	n := p.g.Nodes[p.last]
	for _, c := range names {
		n.Classes = appendUnique(n.Classes, c)
	}
}

func (p *parser) classDef(rest string) {
	names, styles := firstWord(rest)
	if names == "" || styles == "" {
		p.problem("classDef needs a name and styles")
		return
	}
	st := flow.ParseStyle(styles)
	for _, n := range strings.Split(names, ",") {
		if n = strings.TrimSpace(n); n != "" {
			p.g.ClassDefs[n] = p.g.ClassDefs[n].Merge(st)
		}
	}
}

// indentation returns the columns of indentation of line.
func indentation(line string) int {
	n := 0
	for _, c := range line {
		switch c {
		case ' ':
			n++
		case '\t':
			n += tabWidth
		default:
			return n
		}
	}
	return n
}

// node reads a line that is a node, adding it under its parent.
func (p *parser) node(line string, tail bool) {
	indent := indentation(line)
	for len(p.stack) > 0 && p.stack[len(p.stack)-1].indent >= indent {
		p.stack = p.stack[:len(p.stack)-1]
	}
	parent := ""
	switch {
	case len(p.stack) > 0:
		parent = p.stack[len(p.stack)-1].id
		if parent == "" {
			// Under a line that made no node.
			if !tail {
				p.stack = append(p.stack, level{indent, ""})
				p.last = ""
			}
			return
		}
	case p.root != "":
		if !tail {
			p.problem("a mind map has one root: this line is not indented under it")
			p.stack = append(p.stack, level{indent, ""})
			p.last = ""
		}
		return
	}
	text := stripComment(strings.TrimLeft(line, " \t"))
	if !tail {
		text = strings.TrimSpace(text)
	}
	ref, msg := scanNode(text, tail)
	if !ref.show {
		return
	}
	if msg != "" {
		p.problem("%s", msg)
	}
	id, series := "n0", 0
	if parent == "" {
		p.root = id
	} else {
		k := p.children[parent]
		p.children[parent] = k + 1
		id = parent + "." + strconv.Itoa(k)
		series = p.g.Nodes[parent].Series
		if parent == p.root {
			series = 1 + k%branchColors
		}
	}
	p.g.Nodes[id] = &flow.Node{ID: id, Label: ref.label, Shape: ref.shape, Order: p.order,
		Partial: ref.partial, Defined: true, Series: series}
	p.order++
	p.g.NodeOrder = append(p.g.NodeOrder, id)
	if parent != "" {
		p.g.Edges = append(p.g.Edges, &flow.Edge{
			ID: fmt.Sprintf("%s->%s#0", parent, id), From: parent, To: id,
			Line: flow.Solid, Start: flow.ArrowNone, End: flow.ArrowNone,
			Length: 1, Index: len(p.g.Edges), Series: series,
		})
	}
	if !tail {
		p.stack = append(p.stack, level{indent, id})
	}
	p.last = id
}

func (p *parser) finish() {
	g := p.g
	for id, glyph := range p.icons {
		if n := g.Nodes[id]; n.Label != "" {
			n.Label = glyph + " " + n.Label
		} else {
			n.Label = glyph
		}
	}
	if g.Final && !g.HasHeader && len(g.Problems) == 0 {
		g.Problems = append(g.Problems, flow.Problem{Msg: "missing mindmap header"})
	}
}

// nodeRef is a node line as read.
type nodeRef struct {
	shape   flow.Shape
	label   string
	partial bool
	// show tells whether the line makes a node yet: a line still being
	// written may not.
	show bool
}

// shapes are the brackets of the shapes of nodes, longest first.
var shapes = []struct {
	open, close string
	shape       flow.Shape
}{
	{"((", "))", flow.Circle},
	{"))", "((", flow.Bang},
	{"{{", "}}", flow.Hexagon},
	// (-text-) is Mermaid's default shape, as plain text is.
	{"(-", "-)", flow.Round},
	{"(", ")", flow.Round},
	{")", "(", flow.Cloud},
	{"[", "]", flow.Rect},
}

// scanNode reads a node: plain text, or an optional ID followed by a label
// in the brackets of a shape. tail tells that s may still grow. It returns
// a problem with the line, or "".
func scanNode(s string, tail bool) (nodeRef, string) {
	text := func() nodeRef {
		return nodeRef{shape: flow.Round, label: cleanLabel(s), partial: tail, show: !tail || !idLike(s)}
	}
	i := openerAt(s)
	if i < 0 {
		return text(), ""
	}
	idPart, rest := s[:i], s[i:]
	var sh flow.Shape
	var close string
	for _, b := range shapes {
		if strings.HasPrefix(rest, b.open) {
			sh, close, rest = b.shape, b.close, rest[len(b.open):]
			break
		}
	}
	if close == "" {
		// A single {: not a shape of a mind map.
		if tail && rest == "{" {
			s = idPart
		}
		return text(), ""
	}
	if tail && rest == "" && (close == ")" || close == "(") {
		// ( may yet be ((, and ) may be )).
		s = idPart
		return text(), ""
	}
	content, after, closed := splitLabel(rest, close)
	if !closed && !tail && close == "-)" {
		// id(-1 °C) is a round node whose label starts with a dash.
		sh, close = flow.Round, ")"
		content, after, closed = splitLabel("-"+rest, close)
	}
	if !closed {
		if tail {
			// What may be the start of the closing bracket is not label.
			for k := len(close) - 1; k > 0; k-- {
				if strings.HasSuffix(content, close[:k]) {
					content = content[:len(content)-k]
					break
				}
			}
			if strings.Trim(content, " \t\"`") == "" {
				s = idPart
				return text(), ""
			}
			return nodeRef{shape: sh, label: cleanLabel(content), partial: true, show: true}, ""
		}
		return nodeRef{shape: sh, label: cleanLabel(content), show: true}, fmt.Sprintf("node is missing its closing %s", close)
	}
	if strings.TrimSpace(after) != "" {
		// Not a shape after all: draw the line as text.
		ref := nodeRef{shape: flow.Round, label: cleanLabel(s), partial: tail, show: true}
		if tail {
			return ref, ""
		}
		return ref, fmt.Sprintf("unexpected text after %s", close)
	}
	return nodeRef{shape: sh, label: cleanLabel(content), show: true}, ""
}

// openerAt returns the index of the bracket that opens a node's shape in
// s, or -1. Text in double quotes at the start of s is skipped.
func openerAt(s string) int {
	from := 0
	if strings.HasPrefix(s, "\"") {
		end := strings.IndexByte(s[1:], '"')
		if end < 0 {
			return -1
		}
		from = end + 2
	}
	i := strings.IndexAny(s[from:], "()[{")
	if i < 0 {
		return -1
	}
	return from + i
}

// splitLabel splits the rest of a node after its opening bracket into the
// label, up to close, and what follows close. A label in double quotes, or
// a Markdown string ("` ... `"), may hold the closing bracket.
func splitLabel(rest, close string) (content, after string, closed bool) {
	from := 0
	if t := strings.TrimLeft(rest, " \t"); strings.HasPrefix(t, "\"") {
		lead := len(rest) - len(t)
		end := -1
		if strings.HasPrefix(t, "\"`") {
			if j := strings.Index(t[2:], "`\""); j >= 0 {
				end = 2 + j + 2
			}
		} else if j := strings.IndexByte(t[1:], '"'); j >= 0 {
			end = 1 + j + 1
		}
		if end < 0 {
			return rest, "", false
		}
		from = lead + end
	}
	j := strings.Index(rest[from:], close)
	if j < 0 {
		return rest, "", false
	}
	return rest[:from+j], rest[from+j+len(close):], true
}

// idLike reports whether s could still be an ID with a shape to come, or a
// keyword: whether it has only the characters of IDs.
func idLike(s string) bool {
	for _, r := range s {
		if !(r == '_' || r == '-' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

// cleanLabel turns a label into plain text: quotes go, a Markdown string
// loses its backticks and the stars of bold and italic text, <br> becomes
// a line break. A label still being written (with no closing quote yet)
// cleans the same way.
func cleanLabel(s string) string {
	s = strings.TrimSpace(s)
	md := false
	switch {
	case strings.HasPrefix(s, "\"`"):
		s, md = strings.TrimSuffix(strings.TrimSuffix(s[2:], "\""), "`"), true
	case strings.HasPrefix(s, "`"):
		s, md = strings.TrimSuffix(s[1:], "`"), true
	}
	if !md {
		return label.Clean(s)
	}
	return stripEmphasis(label.Plain(s))
}

// stripEmphasis removes the stars of Markdown's bold and italic text.
func stripEmphasis(s string) string {
	s = strings.ReplaceAll(s, "**", "")
	if !strings.Contains(s, "*") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '*' {
			before, after := i > 0 && s[i-1] != ' ' && s[i-1] != '\n', i+1 < len(s) && s[i+1] != ' ' && s[i+1] != '\n'
			if before != after {
				continue // opens or closes emphasis
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// openMarkdownString reports whether s opens a Markdown string ("` ...)
// that it does not close.
func openMarkdownString(s string) bool {
	if strings.HasPrefix(strings.TrimSpace(s), "%%") {
		return false
	}
	open := false
	for i := 0; i < len(s); i++ {
		if s[i] != '"' {
			continue
		}
		if !open {
			if i+1 < len(s) && s[i+1] == '`' {
				open = true
			} else if j := strings.IndexByte(s[i+1:], '"'); j >= 0 {
				i += j + 1 // a plain string: skip it
			}
			continue
		}
		if i > 0 && s[i-1] == '`' {
			open = false
		}
	}
	return open
}

// stripComment removes a %% comment that follows a node.
func stripComment(s string) string {
	inQuote := false
	for i := 0; i+1 < len(s); i++ {
		switch {
		case s[i] == '"':
			inQuote = !inQuote
		case !inQuote && s[i] == '%' && s[i+1] == '%':
			return strings.TrimRight(s[:i], " \t")
		}
	}
	return s
}

func firstWord(s string) (word, rest string) {
	s = strings.TrimSpace(s)
	i := strings.IndexAny(s, " \t")
	if i < 0 {
		return s, ""
	}
	return s[:i], strings.TrimSpace(s[i+1:])
}

func appendUnique(list []string, s string) []string {
	for _, x := range list {
		if x == s {
			return list
		}
	}
	return append(list, s)
}
