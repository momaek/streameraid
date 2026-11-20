// Package er parses Mermaid entity relationship diagrams into the
// flowchart model, so that they are laid out and drawn as flowcharts are.
//
// Each entity is a node of shape flow.Sectioned whose Table holds its name
// and, below, its attributes in columns: type, name, keys and comment.
// Relationships are edges ending in crow's feet.
package er

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/momaek/streameraid/flow"
	"github.com/momaek/streameraid/label"
)

// Parse parses the source of an ER diagram.
//
// final tells whether the source is complete. When it is not, and the
// source does not end with a newline, its last line is still being
// written: Parse reads it tentatively, so that a diagram streaming in does
// not flicker.
//
//   - An entity name that reaches the end of the source may grow, so it
//     makes no node yet.
//   - A relationship whose second entity is still being written is left
//     out.
//   - Attributes show once their line is complete; a block still open
//     shows the attributes so far.
//   - An alias or label being written shows as written so far.
//   - Statements that only style (classDef, class, style, :::) apply once
//     their line is complete.
//
// Statements Parse cannot read are recorded in Graph.Problems and skipped;
// what parsed before the error in the same statement is kept.
func Parse(src string, final bool) *flow.Graph {
	p := &parser{g: flow.NewGraph(), edgeCount: map[string]int{}, entities: map[string]*entity{}}
	p.g.Final = final
	src = strings.ReplaceAll(src, "\r\n", "\n")
	if !final {
		src = trimPartialRune(src)
	}
	complete := final || strings.HasSuffix(src, "\n")
	lines := strings.Split(src, "\n")
	if strings.HasSuffix(src, "\n") {
		lines = lines[:len(lines)-1]
	}
	start, ok := p.frontMatter(lines, complete, final)
	if !ok {
		return p.g
	}
	for i := start; i < len(lines); i++ {
		p.line = i + 1
		p.parseLine(lines[i], i == len(lines)-1 && !complete)
	}
	p.finish()
	return p.g
}

var errIncomplete = errors.New("incomplete")

// trimPartialRune drops the bytes of a character cut short at the end of
// s: the rest of it is still coming.
func trimPartialRune(s string) string {
	i := len(s) - 1
	for i > 0 && i > len(s)-4 && !utf8.RuneStart(s[i]) {
		i--
	}
	if i >= 0 && !utf8.FullRuneInString(s[i:]) {
		return s[:i]
	}
	return s
}

// entity is what the source said of an entity, made into its table at the
// end.
type entity struct {
	label    string
	hasLabel bool
	partial  bool
	attrs    [][4]string // type, name, keys, comment
}

type parser struct {
	g         *flow.Graph
	entities  map[string]*entity
	order     int
	edgeCount map[string]int
	block     string // the entity whose { } block is open
	blockAt   int
	inAcc     bool // inside a multi-line accDescr { }
	line      int
	text      string
}

func (p *parser) problem(format string, args ...any) {
	p.g.Problems = append(p.g.Problems, flow.Problem{Line: p.line, Text: p.text, Msg: fmt.Sprintf(format, args...)})
}

// frontMatter skips a YAML front matter block (--- ... ---), reading its
// title. It returns the index of the first line after it, and false when
// the block has not ended yet.
func (p *parser) frontMatter(lines []string, complete, final bool) (int, bool) {
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
		if k, v, ok := strings.Cut(t, ":"); ok && strings.TrimSpace(k) == "title" {
			p.g.Title = label.Unquote(strings.TrimSpace(v))
		}
	}
	if final {
		p.line = first + 1
		p.text = lines[first]
		p.problem("front matter is not closed with ---")
	}
	return 0, false
}

func (p *parser) parseLine(line string, tail bool) {
	t := strings.TrimSpace(line)
	if p.inAcc {
		if strings.Contains(t, "}") {
			p.inAcc = false
		}
		return
	}
	if t == "" || strings.HasPrefix(t, "%%") {
		return
	}
	// Keep the trailing space of a line still being written: it tells that
	// its last word is complete.
	s := stripComment(strings.TrimLeft(line, " \t"))
	if !tail {
		s = strings.TrimSpace(s)
	}
	if strings.TrimSpace(s) == "" {
		return
	}
	p.text = strings.TrimSpace(s)
	cut := tail && !strings.HasSuffix(s, " ") && !strings.HasSuffix(s, "\t")
	if !p.g.HasHeader {
		p.header(p.text, cut)
		return
	}
	if p.block != "" {
		p.blockLine(p.text, tail)
		return
	}
	p.statement(s, tail)
}

func (p *parser) header(s string, cut bool) {
	word, rest := firstWord(s)
	if cut {
		return
	}
	if word != "erDiagram" {
		p.problem("expected erDiagram, found %q", word)
		return
	}
	p.g.HasHeader = true
	p.g.Dir = flow.TB
	if rest != "" {
		p.problem("unexpected %q after erDiagram", rest)
	}
}

// blockLine reads a line of an open entity block: attributes or the
// closing brace. Attributes show once their line is complete.
func (p *parser) blockLine(t string, tail bool) {
	if t == "}" {
		p.block = ""
		return
	}
	if tail {
		return
	}
	closes := false
	if strings.HasSuffix(t, "}") {
		t, closes = strings.TrimSpace(t[:len(t)-1]), true
	}
	if t != "" {
		p.attributes(p.entities[p.block], t)
	}
	if closes {
		p.block = ""
	}
}

// attributes reads attributes: type name, then keys (PK, FK, UK) and a
// comment in quotes, each optional. A line may hold several.
func (p *parser) attributes(e *entity, t string) {
	toks := tokens(t)
	for i := 0; i < len(toks); {
		typ := toks[i]
		if strings.HasPrefix(typ, "\"") {
			p.problem("expected an attribute type, found %q", short(typ))
			return
		}
		if i+1 >= len(toks) || strings.HasPrefix(toks[i+1], "\"") {
			p.problem("attribute %q has no name", short(typ))
			return
		}
		a := [4]string{typ, toks[i+1]}
		i += 2
		var keys []string
		for i < len(toks) {
			ks, ok := keyList(toks[i])
			if !ok {
				break
			}
			keys = append(keys, ks...)
			i++
		}
		a[2] = strings.Join(keys, ", ")
		if i < len(toks) && strings.HasPrefix(toks[i], "\"") {
			a[3] = label.Plain(label.Unquote(toks[i]))
			i++
		}
		e.attrs = append(e.attrs, a)
	}
}

// keyList reads keys such as PK or PK,FK, or the commas between them.
func keyList(tok string) ([]string, bool) {
	var keys []string
	for _, k := range strings.Split(tok, ",") {
		k = strings.TrimSpace(k)
		switch strings.ToUpper(k) {
		case "":
		case "PK", "FK", "UK":
			keys = append(keys, k)
		default:
			return nil, false
		}
	}
	return keys, true
}

// tokens splits attributes at spaces outside quotes and parentheses, so
// that decimal(10, 2) and "a comment" are one token each.
func tokens(s string) []string {
	var out []string
	depth := 0
	inQuote := false
	start := -1
	for i := 0; i < len(s); i++ {
		c := s[i]
		if start < 0 {
			if c == ' ' || c == '\t' {
				continue
			}
			start = i
		}
		switch {
		case c == '"':
			inQuote = !inQuote
		case inQuote:
		case c == '(':
			depth++
		case c == ')':
			if depth > 0 {
				depth--
			}
		case (c == ' ' || c == '\t') && depth == 0:
			out = append(out, s[start:i])
			start = -1
		}
	}
	if start >= 0 {
		out = append(out, s[start:])
	}
	return out
}

func (p *parser) statement(s string, tail bool) {
	t := strings.TrimSpace(s)
	word, rest := firstWord(t)
	switch word {
	case "direction":
		if !tail {
			d, ok := flow.ParseDirection(rest)
			if !ok {
				p.problem("unknown direction %q", rest)
				return
			}
			p.g.Dir = d
		}
		return
	case "classDef":
		if !tail {
			p.classDef(rest)
		}
		return
	case "class":
		if !tail {
			p.class(rest)
		}
		return
	case "style":
		if !tail {
			p.style(rest)
		}
		return
	case "accTitle", "accTitle:", "accDescr:":
		return
	case "accDescr":
		if strings.HasPrefix(rest, "{") && !strings.Contains(rest, "}") {
			p.inAcc = true
		}
		return
	case "}":
		if !tail {
			p.problem("} without a block to close")
		}
		return
	}
	if strings.HasPrefix(word, "accTitle:") || strings.HasPrefix(word, "accDescr:") {
		return
	}
	p.entityStatement(strings.TrimLeft(s, " \t"), tail)
}

// ref is an entity named in a statement.
type ref struct {
	id       string
	label    string
	hasLabel bool
	partial  bool
	class    string // from :::name
	bad      string // the problem of an alias left unclosed
}

// entityRef reads an entity's name, or a name in quotes, with its alias
// and class.
func (sc *scanner) entityRef() (ref, error) {
	sc.skipSpace()
	var r ref
	if sc.eof() {
		return r, sc.incomplete(errors.New("expected an entity name"))
	}
	if sc.peek() == '"' {
		end := strings.IndexByte(sc.s[sc.i+1:], '"')
		if end < 0 {
			return r, sc.incomplete(errors.New("entity name is missing its closing quote"))
		}
		r.id = sc.s[sc.i+1 : sc.i+1+end]
		sc.i += end + 2
		if strings.TrimSpace(r.id) == "" {
			return r, errors.New("empty entity name")
		}
		r.label, r.hasLabel = label.Plain(r.id), true
	} else {
		r.id = sc.scanID()
		if r.id == "" {
			return r, fmt.Errorf("expected an entity name, found %q", short(sc.s[sc.i:]))
		}
		if sc.eof() && sc.tail {
			return r, errIncomplete
		}
	}
	if !sc.eof() && sc.peek() == '[' {
		sc.i++
		inner := strings.TrimLeft(sc.s[sc.i:], " \t")
		sc.i = len(sc.s) - len(inner)
		closed := false
		var text string
		if strings.HasPrefix(inner, "\"") {
			if q := strings.IndexByte(inner[1:], '"'); q >= 0 {
				text = inner[1 : q+1]
				sc.i += q + 2
				sc.skipSpace()
				if !sc.eof() && sc.peek() == ']' {
					sc.i++
					closed = true
				}
			} else {
				text = inner[1:]
				sc.i = len(sc.s)
			}
		} else if end := strings.IndexByte(inner, ']'); end >= 0 {
			text = inner[:end]
			sc.i += end + 1
			closed = true
		} else {
			text = inner
			sc.i = len(sc.s)
		}
		if !closed {
			if sc.tail && sc.eof() {
				r.partial = true
			} else {
				r.bad = "entity alias is missing its closing ]"
				sc.i = len(sc.s)
			}
		}
		r.label, r.hasLabel = label.Clean(text), true
		if r.partial || r.bad != "" {
			return r, nil
		}
	}
	if strings.HasPrefix(sc.s[sc.i:], ":::") {
		sc.i += 3
		start := sc.i
		for !sc.eof() && sc.peek() != ' ' && sc.peek() != '\t' && sc.peek() != '{' {
			sc.i++
		}
		r.class = sc.s[start:sc.i]
	}
	return r, nil
}

// touch returns the node of entity r, adding it when new, and applies what
// r says of it.
func (p *parser) touch(r ref) *flow.Node {
	n := p.g.Nodes[r.id]
	if n == nil {
		n = &flow.Node{ID: r.id, Label: r.id, Shape: flow.Sectioned, Order: p.order}
		p.order++
		p.g.Nodes[r.id] = n
		p.g.NodeOrder = append(p.g.NodeOrder, r.id)
		p.entities[r.id] = &entity{}
	}
	e := p.entities[r.id]
	if r.hasLabel {
		e.label, e.hasLabel, e.partial = r.label, true, r.partial
	}
	if r.bad != "" {
		p.problem("%s", r.bad)
	}
	return n
}

func (p *parser) addClass(n *flow.Node, class string, tail bool) {
	if class != "" && !tail {
		n.Classes = appendUnique(n.Classes, class)
	}
}

// entityStatement reads an entity (A, A["Label"], A {) or a relationship
// (A ||--o{ B : label).
func (p *parser) entityStatement(s string, tail bool) {
	sc := &scanner{s: s, tail: tail}
	fail := func(err error) {
		if err != errIncomplete && !tail {
			p.problem("%v", err)
		}
	}
	left, err := sc.entityRef()
	if err != nil {
		fail(err)
		return
	}
	a := p.touch(left)
	if left.partial || left.bad != "" {
		return
	}
	p.addClass(a, left.class, tail)
	sc.skipSpace()
	if sc.eof() {
		return
	}
	if sc.peek() == '{' {
		inner := strings.TrimSpace(sc.s[sc.i+1:])
		if end := strings.LastIndexByte(inner, '}'); end >= 0 {
			if t := strings.TrimSpace(inner[:end]); t != "" && !tail {
				p.attributes(p.entities[left.id], t)
			}
			return
		}
		p.block, p.blockAt = left.id, p.line
		if inner != "" && !tail {
			p.attributes(p.entities[left.id], inner)
		}
		return
	}
	var e flow.Edge
	if e.Start, err = sc.cardinality(); err != nil {
		fail(err)
		return
	}
	if e.Line, err = sc.erLine(); err != nil {
		fail(err)
		return
	}
	if e.End, err = sc.cardinality(); err != nil {
		fail(err)
		return
	}
	right, err := sc.entityRef()
	if err != nil {
		fail(err)
		return
	}
	if right.partial {
		return
	}
	b := p.touch(right)
	p.addClass(b, right.class, tail)
	sc.skipSpace()
	if !sc.eof() {
		if sc.peek() == ':' {
			e.Label = label.Clean(sc.s[sc.i+1:])
		} else if !tail {
			p.problem("unexpected %q after relationship", short(sc.s[sc.i:]))
		}
	}
	p.edge(a.ID, b.ID, e)
}

// cardinalities are the word forms of cardinalities, longest first.
var cardinalities = []struct {
	word  string
	arrow flow.Arrow
}{
	{"zero or more", flow.ArrowZeroMany},
	{"zero or many", flow.ArrowZeroMany},
	{"zero or one", flow.ArrowZeroOne},
	{"one or more", flow.ArrowMany},
	{"one or many", flow.ArrowMany},
	{"one or zero", flow.ArrowZeroOne},
	{"only one", flow.ArrowOne},
	{"many(0)", flow.ArrowZeroMany},
	{"many(1)", flow.ArrowMany},
	{"0+", flow.ArrowZeroMany},
	{"1+", flow.ArrowMany},
	{"1", flow.ArrowOne},
}

// symbols are the crow's foot cardinalities, as written on either side.
var symbols = map[string]flow.Arrow{
	"||": flow.ArrowOne,
	"|o": flow.ArrowZeroOne, "o|": flow.ArrowZeroOne,
	"}|": flow.ArrowMany, "|{": flow.ArrowMany,
	"}o": flow.ArrowZeroMany, "o{": flow.ArrowZeroMany,
}

// cardinality reads a cardinality: ||, |o, }o, }| (or their mirror
// images), or a word form as "zero or more".
func (sc *scanner) cardinality() (flow.Arrow, error) {
	sc.skipSpace()
	rest := sc.s[sc.i:]
	if rest == "" {
		return 0, sc.incomplete(errors.New("relationship has no cardinality"))
	}
	if len(rest) >= 2 {
		if a, ok := symbols[rest[:2]]; ok {
			sc.i += 2
			return a, nil
		}
	}
	lower := strings.ToLower(rest)
	if sc.tail {
		for _, c := range cardinalities {
			if strings.HasPrefix(c.word, lower) {
				// The word may still grow: 1, then 1+.
				return 0, errIncomplete
			}
		}
		if len(rest) == 1 && strings.ContainsAny(rest, "|}o") {
			return 0, errIncomplete
		}
	}
	for _, c := range cardinalities {
		if strings.HasPrefix(lower, c.word) && endsWord(rest, len(c.word)) {
			sc.i += len(c.word)
			return c.arrow, nil
		}
	}
	return 0, fmt.Errorf("expected a cardinality, found %q", short(rest))
}

// erLine reads the line of a relationship: -- or "to" (identifying), ..
// or "optionally to" (non-identifying).
func (sc *scanner) erLine() (flow.Line, error) {
	sc.skipSpace()
	rest := sc.s[sc.i:]
	lower := strings.ToLower(rest)
	if sc.tail && (rest == "" || rest == "-" || rest == "." ||
		strings.HasPrefix("to", lower) || strings.HasPrefix("optionally to", lower)) {
		return 0, errIncomplete
	}
	switch {
	case strings.HasPrefix(rest, "--"):
		sc.i += 2
		return flow.Solid, nil
	case strings.HasPrefix(rest, ".."), strings.HasPrefix(rest, ".-"), strings.HasPrefix(rest, "-."):
		sc.i += 2
		return flow.Dotted, nil
	case strings.HasPrefix(lower, "optionally to") && endsWord(rest, 13):
		sc.i += 13
		return flow.Dotted, nil
	case strings.HasPrefix(lower, "to") && endsWord(rest, 2):
		sc.i += 2
		return flow.Solid, nil
	}
	if rest == "" {
		return 0, errors.New("relationship has no line")
	}
	return 0, fmt.Errorf("expected -- or .., found %q", short(rest))
}

// endsWord reports whether a word of s ends at i.
func endsWord(s string, i int) bool {
	if i >= len(s) {
		return true
	}
	r, _ := utf8.DecodeRuneInString(s[i:])
	return !isIDRune(r) && r != '+'
}

func (p *parser) edge(from, to string, e flow.Edge) {
	key := from + "\x00" + to
	n := p.edgeCount[key]
	p.edgeCount[key] = n + 1
	e.ID = fmt.Sprintf("%s->%s#%d", from, to, n)
	e.From, e.To = from, to
	e.Length = 1
	e.Index = len(p.g.Edges)
	p.g.Edges = append(p.g.Edges, &e)
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

// class reads class A,B name.
func (p *parser) class(rest string) {
	ids, name := firstWord(rest)
	if ids == "" || name == "" {
		p.problem("class needs entity names and a class name")
		return
	}
	for _, id := range strings.Split(ids, ",") {
		if id = strings.TrimSpace(id); id != "" {
			n := p.touch(ref{id: id})
			n.Classes = appendUnique(n.Classes, name)
		}
	}
}

func (p *parser) style(rest string) {
	id, styles := firstWord(rest)
	if id == "" || styles == "" {
		p.problem("style needs an entity name and styles")
		return
	}
	n := p.touch(ref{id: id})
	n.Style = n.Style.Merge(flow.ParseStyle(styles))
}

func (p *parser) finish() {
	g := p.g
	for _, id := range g.NodeOrder {
		n, e := g.Nodes[id], p.entities[id]
		name := id
		if e.hasLabel {
			name = e.label
		}
		secs := []flow.Section{{Rows: [][]string{{name}}, Center: true, Bold: true}}
		if len(e.attrs) > 0 {
			secs = append(secs, flow.Section{Rows: attributeRows(e.attrs)})
		}
		n.Label, n.Partial = name, e.partial
		n.Table = &flow.Table{Sections: secs}
	}
	if g.Final {
		if !g.HasHeader && len(g.Problems) == 0 {
			g.Problems = append(g.Problems, flow.Problem{Msg: "missing erDiagram header"})
		}
		if p.block != "" {
			g.Problems = append(g.Problems, flow.Problem{Line: p.blockAt, Msg: fmt.Sprintf("entity %q is missing its closing }", p.block)})
		}
	}
}

// attributeRows makes the rows of an entity's attributes: type and name,
// then keys and comment when any attribute has them, so that the columns
// line up.
func attributeRows(attrs [][4]string) [][]string {
	var hasKeys, hasComment bool
	for _, a := range attrs {
		hasKeys = hasKeys || a[2] != ""
		hasComment = hasComment || a[3] != ""
	}
	rows := make([][]string, len(attrs))
	for i, a := range attrs {
		row := []string{a[0], a[1]}
		if hasKeys {
			row = append(row, a[2])
		}
		if hasComment {
			row = append(row, a[3])
		}
		rows[i] = row
	}
	return rows
}

// scanner reads one statement.
type scanner struct {
	s    string
	i    int
	tail bool // the statement may be cut short at its end
}

func (sc *scanner) eof() bool  { return sc.i >= len(sc.s) }
func (sc *scanner) peek() byte { return sc.s[sc.i] }

func (sc *scanner) skipSpace() {
	for sc.i < len(sc.s) && (sc.s[sc.i] == ' ' || sc.s[sc.i] == '\t') {
		sc.i++
	}
}

// incomplete returns errIncomplete when the statement may still grow, and
// err otherwise.
func (sc *scanner) incomplete(err error) error {
	if sc.tail {
		return errIncomplete
	}
	return err
}

func isIDRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// scanID reads an entity name: letters, digits and _, joined by single -
// (LINE-ITEM).
func (sc *scanner) scanID() string {
	start := sc.i
	for sc.i < len(sc.s) {
		r, n := utf8.DecodeRuneInString(sc.s[sc.i:])
		if isIDRune(r) {
			sc.i += n
			continue
		}
		if r == '-' && sc.i > start {
			if next, _ := utf8.DecodeRuneInString(sc.s[sc.i+1:]); isIDRune(next) || sc.tail && sc.i+1 == len(sc.s) {
				// At the end of a line being written, the name may go on.
				sc.i++
				continue
			}
		}
		break
	}
	return sc.s[start:sc.i]
}

func short(s string) string {
	if len(s) > 20 {
		return s[:20] + "…"
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

// stripComment removes a %% comment that follows a statement.
func stripComment(s string) string {
	inQuote := false
	for i := 0; i+1 < len(s); i++ {
		switch {
		case s[i] == '"':
			inQuote = !inQuote
		case !inQuote && s[i] == '%' && s[i+1] == '%':
			return strings.TrimSpace(s[:i])
		}
	}
	return s
}
