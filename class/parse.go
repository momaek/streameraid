// Package class parses Mermaid class diagrams into the flowchart model, so
// that they are laid out and drawn as flowcharts are.
//
// Each class is a node of shape flow.Sectioned whose Table holds its name
// (under its annotation, if any), its attributes and its methods. Relations
// are edges, with the ends of their kinds as arrows; namespaces are
// subgraphs; notes are nodes of shape flow.Note.
package class

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/momaek/streameraid/flow"
	"github.com/momaek/streameraid/label"
)

// AssocArrow is the arrow drawn for the plain ends < and > of associations
// and dependencies (-->, ..>).
const AssocArrow = flow.ArrowNormal

// Parse parses the source of a class diagram.
//
// final tells whether the source is complete. When it is not, and the
// source does not end with a newline, its last line is still being
// written: Parse reads it tentatively, so that a diagram streaming in does
// not flicker.
//
//   - A class name that reaches the end of the source may grow, so it
//     makes no node yet.
//   - A relation whose second class is still being written is left out.
//   - Members show once their line is complete; a block still open shows
//     the members so far.
//   - A label or note being written shows as written so far, its node
//     marked Partial.
//   - Statements that only style (classDef, cssClass, style, :::) and
//     annotations apply once their line is complete.
//
// Statements Parse cannot read are recorded in Graph.Problems and skipped;
// what parsed before the error in the same statement is kept.
func Parse(src string, final bool) *flow.Graph {
	p := &parser{g: flow.NewGraph(), edgeCount: map[string]int{}, classes: map[string]*classInfo{}}
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

// classInfo is what the source said of a class, made into its table at the
// end.
type classInfo struct {
	label      string // from A["label"]
	hasLabel   bool
	generic    string // from A~T~, as written
	partial    bool
	annotation string
	body       bool // the class has a { } block
	attrs      [][]string
	methods    [][]string
}

type parser struct {
	g         *flow.Graph
	classes   map[string]*classInfo
	order     int
	sgOrder   int
	notes     int
	edgeCount map[string]int
	stack     []string // open namespaces, innermost last
	body      string   // the class whose { } block is open
	bodyAt    int
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
	if p.body != "" {
		p.bodyLine(p.text, tail)
		return
	}
	p.statement(s, tail)
}

func (p *parser) header(s string, cut bool) {
	word, rest := firstWord(s)
	if cut {
		// The word may still grow: classDiagram, then classDiagram-v2.
		return
	}
	if word != "classDiagram" && word != "classDiagram-v2" {
		p.problem("expected classDiagram, found %q", word)
		return
	}
	p.g.HasHeader = true
	p.g.Dir = flow.TB
	if rest != "" {
		p.problem("unexpected %q after classDiagram", rest)
	}
}

// bodyLine reads a line of an open class block: a member, an annotation
// or the closing brace. Members show once their line is complete.
func (p *parser) bodyLine(t string, tail bool) {
	if t == "}" {
		p.body = ""
		return
	}
	if tail {
		return
	}
	info := p.classes[p.body]
	closes := false
	if strings.HasSuffix(t, "}") {
		t, closes = strings.TrimSpace(t[:len(t)-1]), true
	}
	switch {
	case strings.HasPrefix(t, "<<"):
		if a, ok := annotation(t); ok {
			setAnnotation(info, a)
		} else {
			p.problem("annotation is missing its closing >>")
		}
	case t != "":
		addMember(info, t)
	}
	if closes {
		p.body = ""
	}
}

func (p *parser) statement(s string, tail bool) {
	t := strings.TrimSpace(s)
	word, rest := firstWord(t)
	// raw is the statement after its first word, keeping a trailing space.
	raw := strings.TrimLeft(strings.TrimLeft(s, " \t")[len(word):], " \t")
	switch word {
	case "class":
		p.classStatement(raw, tail)
		return
	case "namespace":
		p.namespace(raw, tail)
		return
	case "note":
		p.note(raw, tail)
		return
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
	case "cssClass":
		if !tail {
			p.cssClass(rest)
		}
		return
	case "style":
		if !tail {
			p.style(rest)
		}
		return
	case "click", "link", "callback", "accTitle", "accTitle:", "accDescr:":
		// Interactions and accessibility text draw nothing.
		return
	case "accDescr":
		if strings.HasPrefix(rest, "{") && !strings.Contains(rest, "}") {
			p.inAcc = true
		}
		return
	case "}":
		if len(p.stack) == 0 {
			if !tail {
				p.problem("} without a block to close")
			}
			return
		}
		p.g.Subgraphs[p.stack[len(p.stack)-1]].Open = false
		p.stack = p.stack[:len(p.stack)-1]
		return
	}
	if strings.HasPrefix(word, "accTitle:") || strings.HasPrefix(word, "accDescr:") {
		return
	}
	if strings.HasPrefix(t, "<<") {
		if !tail {
			p.annotationStatement(t)
		}
		return
	}
	p.relationOrMember(s, tail)
}

// ref is a class named in a statement.
type ref struct {
	id       string
	generic  string // ~T~ as written
	label    string
	hasLabel bool
	partial  bool
	class    string // from :::name
	// bad is the problem of a label left unclosed on a complete line; the
	// class still shows, as it did while the line was written.
	bad string
}

// classRef reads a class name with its generic type, label and class.
func (sc *scanner) classRef() (ref, error) {
	sc.skipSpace()
	var r ref
	if sc.eof() {
		return r, sc.incomplete(errors.New("expected a class name"))
	}
	if sc.peek() == '`' {
		end := strings.IndexByte(sc.s[sc.i+1:], '`')
		if end < 0 {
			return r, sc.incomplete(errors.New("class name is missing its closing `"))
		}
		r.id = strings.TrimSpace(sc.s[sc.i+1 : sc.i+1+end])
		sc.i += end + 2
		if r.id == "" {
			return r, errors.New("empty class name")
		}
	} else {
		r.id = sc.scanID()
		if r.id == "" {
			return r, fmt.Errorf("expected a class name, found %q", short(sc.s[sc.i:]))
		}
		if sc.eof() && sc.tail {
			return r, errIncomplete
		}
	}
	if !sc.eof() && sc.peek() == '~' {
		g, ok := sc.generic()
		if !ok {
			return r, sc.incomplete(errors.New("generic type is missing its closing ~"))
		}
		r.generic = g
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
				r.bad = "class label is missing its closing ]"
				sc.i = len(sc.s)
			}
		}
		r.label, r.hasLabel = label.Clean(text), true
		if r.partial {
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

// touch returns the node of class r, adding it when new, and applies what
// r says of it.
func (p *parser) touch(r ref) *flow.Node {
	n := p.g.Nodes[r.id]
	info := p.classes[r.id]
	if n == nil {
		n = &flow.Node{ID: r.id, Label: r.id, Shape: flow.Sectioned, Order: p.order}
		p.order++
		p.g.Nodes[r.id] = n
		p.g.NodeOrder = append(p.g.NodeOrder, r.id)
		info = &classInfo{}
		p.classes[r.id] = info
	}
	if r.bad != "" {
		p.problem("%s", r.bad)
	}
	if info == nil {
		// A note's ID: notes have IDs no class name can have.
		return n
	}
	if len(p.stack) > 0 && n.Subgraph == "" {
		n.Subgraph = p.stack[len(p.stack)-1]
	}
	if r.hasLabel {
		info.label, info.hasLabel, info.partial = r.label, true, r.partial
	}
	if r.generic != "" {
		info.generic = r.generic
	}
	return n
}

func (p *parser) addClass(n *flow.Node, class string, tail bool) {
	if class != "" && !tail {
		n.Classes = appendUnique(n.Classes, class)
	}
}

// classStatement reads class A, class A~T~["label"]:::name <<annotation>> {.
func (p *parser) classStatement(rest string, tail bool) {
	sc := &scanner{s: rest, tail: tail}
	r, err := sc.classRef()
	if err != nil {
		if err != errIncomplete && !tail {
			p.problem("%v", err)
		}
		return
	}
	n := p.touch(r)
	n.Defined = true
	if r.partial {
		return
	}
	p.addClass(n, r.class, tail)
	info := p.classes[r.id]
	sc.skipSpace()
	if strings.HasPrefix(sc.s[sc.i:], "<<") {
		end := strings.Index(sc.s[sc.i:], ">>")
		if end < 0 {
			if !tail {
				p.problem("annotation is missing its closing >>")
			}
			return
		}
		if !tail {
			setAnnotation(info, strings.TrimSpace(sc.s[sc.i+2:sc.i+end]))
		}
		sc.i += end + 2
		sc.skipSpace()
	}
	if sc.eof() {
		return
	}
	if sc.peek() != '{' {
		if !tail {
			p.problem("unexpected %q after class %s", short(sc.s[sc.i:]), r.id)
		}
		return
	}
	info.body = true
	inner := strings.TrimSpace(sc.s[sc.i+1:])
	if end := strings.LastIndexByte(inner, '}'); end >= 0 {
		if m := strings.TrimSpace(inner[:end]); m != "" && !tail {
			addMember(info, m)
		}
		return
	}
	p.body, p.bodyAt = r.id, p.line
	if inner != "" && !tail {
		p.bodyLine(inner, false)
	}
}

func (p *parser) namespace(rest string, tail bool) {
	sc := &scanner{s: rest, tail: tail}
	sc.skipSpace()
	name := sc.scanID()
	if name == "" {
		if !tail {
			p.problem("namespace needs a name")
		}
		return
	}
	if sc.eof() && tail {
		return
	}
	sc.skipSpace()
	if sc.eof() || sc.peek() != '{' {
		if !tail {
			p.problem("namespace %s needs a {", name)
		}
		return
	}
	sg := p.g.Subgraphs[name]
	if sg == nil {
		parent := ""
		if len(p.stack) > 0 {
			parent = p.stack[len(p.stack)-1]
		}
		sg = &flow.Subgraph{ID: name, Title: name, Parent: parent, Order: p.sgOrder}
		p.sgOrder++
		p.g.Subgraphs[name] = sg
		p.g.SubgraphOrder = append(p.g.SubgraphOrder, name)
		if parent != "" {
			ps := p.g.Subgraphs[parent]
			ps.Children = append(ps.Children, name)
		}
	}
	sg.Open = true
	p.stack = append(p.stack, name)
	if strings.HasSuffix(strings.TrimSpace(sc.s[sc.i+1:]), "}") {
		sg.Open = false
		p.stack = p.stack[:len(p.stack)-1]
	}
}

// note reads note for A "text" and note "text".
func (p *parser) note(rest string, tail bool) {
	sc := &scanner{s: rest, tail: tail}
	var target *flow.Node
	if w, _ := firstWord(rest); w == "for" && len(rest) > 3 && (rest[3] == ' ' || rest[3] == '\t') {
		sc.i = 3
		r, err := sc.classRef()
		if err != nil {
			if err != errIncomplete && !tail {
				p.problem("%v", err)
			}
			return
		}
		if r.partial {
			return
		}
		target = p.touch(r)
		sc.skipSpace()
	}
	if sc.eof() || sc.peek() != '"' {
		if !tail {
			p.problem("note needs a text in quotes")
		}
		return
	}
	text := sc.s[sc.i+1:]
	partial := true
	if end := strings.IndexByte(text, '"'); end >= 0 {
		text, partial = text[:end], false
		if after := strings.TrimSpace(sc.s[sc.i+end+2:]); after != "" && !tail {
			p.problem("unexpected %q after note", short(after))
		}
	} else if !tail {
		p.problem("note text is missing its closing quote")
	}
	p.notes++
	id := fmt.Sprintf("note:%d", p.notes)
	n := &flow.Node{ID: id, Label: noteText(text), Shape: flow.Note, Order: p.order, Partial: partial && tail, Defined: true}
	p.order++
	p.g.Nodes[id] = n
	p.g.NodeOrder = append(p.g.NodeOrder, id)
	if target != nil {
		p.edge(target.ID, id, flow.Edge{Line: flow.Dotted})
	}
}

// noteText turns a note's text into plain text: \n and <br> break lines.
func noteText(s string) string {
	return label.Plain(strings.ReplaceAll(s, `\n`, "<br>"))
}

func (p *parser) annotationStatement(t string) {
	a, ok := annotation(t)
	if !ok {
		p.problem("annotation is missing its closing >>")
		return
	}
	end := strings.Index(t, ">>")
	sc := &scanner{s: t[end+2:]}
	r, err := sc.classRef()
	if err != nil {
		p.problem("%v", err)
		return
	}
	p.touch(r)
	setAnnotation(p.classes[r.id], a)
	if sc.skipSpace(); !sc.eof() {
		p.problem("unexpected %q after annotation", short(sc.s[sc.i:]))
	}
}

// annotation reads <<name>> at the start of t.
func annotation(t string) (string, bool) {
	end := strings.Index(t, ">>")
	if !strings.HasPrefix(t, "<<") || end < 0 {
		return "", false
	}
	return strings.TrimSpace(t[2:end]), true
}

func setAnnotation(info *classInfo, a string) {
	// Mermaid draws the first annotation only.
	if info != nil && info.annotation == "" && a != "" {
		info.annotation = a
	}
}

// relationOrMember reads A <|-- B : label, A "1" --> "*" B, A : +member, or
// a lone class name.
func (p *parser) relationOrMember(s string, tail bool) {
	sc := &scanner{s: strings.TrimLeft(s, " \t"), tail: tail}
	fail := func(err error) {
		if err != errIncomplete && !tail {
			p.problem("%v", err)
		}
	}
	left, err := sc.classRef()
	if err != nil {
		fail(err)
		return
	}
	a := p.touch(left)
	if left.partial {
		return
	}
	p.addClass(a, left.class, tail)
	sc.skipSpace()
	if sc.eof() {
		return
	}
	if sc.peek() == ':' {
		if tail {
			return
		}
		m := strings.TrimSpace(sc.s[sc.i+1:])
		if m == "" {
			p.problem("member of %s is empty", left.id)
			return
		}
		if info := p.classes[left.id]; info != nil {
			addMember(info, m)
		}
		return
	}
	var e flow.Edge
	if sc.peek() == '"' {
		if e.StartLabel, err = sc.quoted(); err != nil {
			fail(err)
			return
		}
		sc.skipSpace()
	}
	if err := sc.relation(&e); err != nil {
		fail(err)
		return
	}
	sc.skipSpace()
	if !sc.eof() && sc.peek() == '"' {
		if e.EndLabel, err = sc.quoted(); err != nil {
			fail(err)
			return
		}
	}
	right, err := sc.classRef()
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
			p.problem("unexpected %q after relation", short(sc.s[sc.i:]))
		}
	}
	p.edge(a.ID, b.ID, e)
}

// relation reads the arrow of a relation: an end, a line (-- or ..) and an
// end.
func (sc *scanner) relation(e *flow.Edge) error {
	if sc.eof() {
		return sc.incomplete(errors.New("expected a relation"))
	}
	rest := sc.s[sc.i:]
	switch {
	case strings.HasPrefix(rest, "<|"):
		e.Start, sc.i = flow.ArrowTriangle, sc.i+2
	case strings.HasPrefix(rest, "()"):
		e.Start, sc.i = flow.ArrowLollipop, sc.i+2
	case rest[0] == '*':
		e.Start, sc.i = flow.ArrowDiamond, sc.i+1
	case rest[0] == 'o':
		e.Start, sc.i = flow.ArrowHollow, sc.i+1
	case rest[0] == '<':
		e.Start, sc.i = AssocArrow, sc.i+1
	}
	if sc.eof() {
		return sc.incomplete(errors.New("relation has no line"))
	}
	c := sc.peek()
	if c != '-' && c != '.' {
		return fmt.Errorf("expected a relation, found %q", short(rest))
	}
	n := 0
	for sc.i+n < len(sc.s) && sc.s[sc.i+n] == c {
		n++
	}
	sc.i += n
	if n < 2 {
		if sc.eof() {
			return sc.incomplete(errors.New("relation has no line"))
		}
		return fmt.Errorf("expected a relation, found %q", short(rest))
	}
	e.Line = flow.Solid
	if c == '.' {
		e.Line = flow.Dotted
	}
	if sc.eof() {
		return sc.incomplete(errors.New("relation has no second class"))
	}
	rest = sc.s[sc.i:]
	switch {
	case strings.HasPrefix(rest, "|>"):
		e.End, sc.i = flow.ArrowTriangle, sc.i+2
	case strings.HasPrefix(rest, "()"):
		e.End, sc.i = flow.ArrowLollipop, sc.i+2
	case rest == "|" || rest == "(":
		return sc.incomplete(errors.New("relation has no second class"))
	case rest[0] == '*':
		e.End, sc.i = flow.ArrowDiamond, sc.i+1
	case rest[0] == '>':
		e.End, sc.i = AssocArrow, sc.i+1
	case rest[0] == 'o':
		// --o B is aggregation; --oscar is a class named oscar.
		if len(rest) == 1 || rest[1] == ' ' || rest[1] == '\t' || rest[1] == '"' {
			e.End, sc.i = flow.ArrowHollow, sc.i+1
		}
	}
	return nil
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

// addMember adds a member to a class: a method when it has parentheses,
// an attribute otherwise.
func addMember(info *classInfo, text string) {
	m, method := member(text)
	if m == "" {
		return
	}
	if method {
		info.methods = append(info.methods, []string{m})
	} else {
		info.attrs = append(info.attrs, []string{m})
	}
}

// member returns the text a member is shown with, and whether it is a
// method. The classifiers $ (static) and * (abstract) go, ~T~ becomes <T>
// and a method's return type follows a colon, as Mermaid shows them.
func member(text string) (string, bool) {
	text = strings.TrimRight(strings.TrimSpace(text), "$*")
	text = strings.TrimSpace(text)
	vis := ""
	if text != "" && strings.ContainsRune("+-#~", rune(text[0])) {
		vis, text = text[:1], text[1:]
	}
	method := strings.Contains(text, "(")
	text = generics(text)
	if method {
		if j := strings.LastIndexByte(text, ')'); j >= 0 {
			ret := strings.TrimSpace(strings.TrimLeft(text[j+1:], "$*"))
			text = text[:j+1]
			if ret != "" {
				if strings.HasPrefix(ret, ":") {
					text += " " + ret
				} else {
					text += " : " + ret
				}
			}
		}
	}
	return vis + text, method
}

// generics turns Mermaid's generic types (List~int~, Map~K,List~V~~) into
// angle brackets. A ~ closes a type when one is open and no name follows
// it, and opens one otherwise.
func generics(s string) string {
	if !strings.Contains(s, "~") {
		return s
	}
	var b strings.Builder
	depth := 0
	for i := 0; i < len(s); i++ {
		if s[i] != '~' {
			b.WriteByte(s[i])
			continue
		}
		next, _ := utf8.DecodeRuneInString(s[i+1:])
		if depth > 0 && !isIDRune(next) {
			b.WriteByte('>')
			depth--
		} else {
			b.WriteByte('<')
			depth++
		}
	}
	return b.String()
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

// cssClass reads cssClass "A,B" name.
func (p *parser) cssClass(rest string) {
	var ids, name string
	if strings.HasPrefix(rest, "\"") {
		end := strings.IndexByte(rest[1:], '"')
		if end < 0 {
			p.problem("cssClass: class list is missing its closing quote")
			return
		}
		ids, name = rest[1:end+1], strings.TrimSpace(rest[end+2:])
	} else {
		ids, name = firstWord(rest)
	}
	if strings.TrimSpace(ids) == "" || name == "" {
		p.problem("cssClass needs class names and a style class")
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
		p.problem("style needs a class name and styles")
		return
	}
	n := p.touch(ref{id: id})
	n.Style = n.Style.Merge(flow.ParseStyle(styles))
}

func (p *parser) finish() {
	g := p.g
	for _, id := range g.NodeOrder {
		n := g.Nodes[id]
		if info := p.classes[id]; info != nil {
			name := info.label
			if !info.hasLabel {
				name = id + generics(info.generic)
			}
			var secs []flow.Section
			if info.annotation != "" {
				secs = append(secs,
					flow.Section{Rows: [][]string{{"«" + info.annotation + "»"}}, Center: true, Small: true},
					flow.Section{Rows: [][]string{{name}}, Center: true, Bold: true, Joined: true})
			} else {
				secs = append(secs, flow.Section{Rows: [][]string{{name}}, Center: true, Bold: true})
			}
			if info.body || len(info.attrs)+len(info.methods) > 0 {
				secs = append(secs, flow.Section{Rows: info.attrs}, flow.Section{Rows: info.methods})
			}
			n.Label, n.Partial = name, info.partial
			n.Table = &flow.Table{Sections: secs}
		}
		if sg := g.Subgraphs[n.Subgraph]; sg != nil {
			sg.Nodes = append(sg.Nodes, id)
		} else {
			n.Subgraph = ""
		}
	}
	if g.Final {
		if !g.HasHeader && len(g.Problems) == 0 {
			g.Problems = append(g.Problems, flow.Problem{Msg: "missing classDiagram header"})
		}
		if p.body != "" {
			g.Problems = append(g.Problems, flow.Problem{Line: p.bodyAt, Msg: fmt.Sprintf("class %q is missing its closing }", p.body)})
		}
		for _, id := range p.stack {
			g.Problems = append(g.Problems, flow.Problem{Msg: fmt.Sprintf("namespace %q is missing its closing }", id)})
		}
	}
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

// scanID reads a name: letters, digits and _, joined by single - or .
// (com.example.Shape).
func (sc *scanner) scanID() string {
	start := sc.i
	for sc.i < len(sc.s) {
		r, n := utf8.DecodeRuneInString(sc.s[sc.i:])
		if isIDRune(r) {
			sc.i += n
			continue
		}
		if (r == '-' || r == '.') && sc.i > start {
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

// generic reads a generic type at a ~, to the ~ that closes it.
func (sc *scanner) generic() (string, bool) {
	depth := 0
	for j := sc.i; j < len(sc.s); j++ {
		if sc.s[j] != '~' {
			continue
		}
		next, _ := utf8.DecodeRuneInString(sc.s[j+1:])
		if depth > 0 && !isIDRune(next) {
			depth--
			if depth == 0 {
				g := sc.s[sc.i : j+1]
				sc.i = j + 1
				return g, true
			}
		} else {
			depth++
		}
	}
	return "", false
}

// quoted reads a string in double quotes.
func (sc *scanner) quoted() (string, error) {
	end := strings.IndexByte(sc.s[sc.i+1:], '"')
	if end < 0 {
		return "", sc.incomplete(errors.New("string is missing its closing quote"))
	}
	s := sc.s[sc.i+1 : sc.i+1+end]
	sc.i += end + 2
	return label.Plain(s), nil
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
