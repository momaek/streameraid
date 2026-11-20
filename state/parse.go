// Package state parses Mermaid state diagrams into the flowchart model,
// flow.Graph, so that they lay out and draw as flowcharts do.
//
// States are nodes: plain states round boxes, [*] the start and end dots,
// choices small diamonds, forks and joins bars, a state with descriptions
// a box of sections. Composite states are subgraphs, and the regions of a
// composite split by -- are dashed subgraphs inside it. Notes are note
// nodes joined to their state by a dotted edge.
package state

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/momaek/streameraid/flow"
	"github.com/momaek/streameraid/label"
)

// RegionDash is the dash pattern of the boxes of the regions of a
// composite state.
const RegionDash = "4 4"

// Parse parses the source of a state diagram.
//
// final tells whether the source is complete. When it is not, and the
// source does not end with a newline, its last line is still being
// written: Parse reads it tentatively, as flow.Parse does, so that a
// diagram streaming in does not flicker.
//
//   - A name that reaches the end of the source may grow, so it makes no
//     state or transition yet.
//   - A transition without its target is left out.
//   - A description or label being written shows as written so far, its
//     node marked Partial.
//   - A state statement (state X, state X <<choice>>) applies once it is
//     clear what it declares; a composite state opens at its {.
//   - Statements that only style (classDef, class, style) and direction
//     apply once their line is complete.
//   - A note block still open shows the lines it has so far.
//
// Statements Parse cannot read are recorded in Graph.Problems and skipped.
func Parse(src string, final bool) *flow.Graph {
	p := &parser{
		g:         flow.NewGraph(),
		edgeCount: map[string]int{},
		states:    map[string]*info{},
		classes:   map[string][]string{},
		styles:    map[string]flow.Style{},
	}
	p.g.Final = final
	src = strings.ReplaceAll(src, "\r\n", "\n")
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

type kind uint8

const (
	plain  kind = iota
	choice      // <<choice>>
	bar         // <<fork>> or <<join>>
)

// info is what the source says of a state.
type info struct {
	alias   string // the text of state "text" as ID
	descs   []string
	partial bool // the last description is still being written
	kind    kind
}

// frame is an open composite state.
type frame struct {
	id string
	// region is the number of the region being read, from 1, or 0 while
	// no -- has split the composite.
	region int
}

type parser struct {
	g         *flow.Graph
	stack     []frame
	order     int
	edgeCount map[string]int
	states    map[string]*info
	// classes and styles are those given to names by :::, class and style,
	// applied to nodes or subgraphs at the end.
	classes map[string][]string
	styles  map[string]flow.Style
	notes   int
	// note is the note whose block (note ... end note) is open.
	note      *flow.Node
	noteLines []string
	// inAccDescr tells that an accDescr { ... } block is open.
	inAccDescr bool
	line       int
	text       string // the statement being parsed, for problems
	tail       bool   // the statement is still being written
}

func (p *parser) problem(format string, args ...any) {
	if p.tail {
		// A statement still being written is not wrong yet.
		return
	}
	p.g.Problems = append(p.g.Problems, flow.Problem{Line: p.line, Text: p.text, Msg: fmt.Sprintf(format, args...)})
}

// frontMatter skips a YAML front matter block (--- ... ---), reading its
// title. It returns the index of the first line after it, and false when
// the block has not ended yet, which is a problem once the source is
// final.
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
	p.tail = tail
	t := strings.TrimSpace(line)
	if p.note != nil {
		p.noteLine(t, tail)
		return
	}
	if p.inAccDescr {
		if strings.Contains(t, "}") {
			p.inAccDescr = false
		}
		return
	}
	if t == "" || strings.HasPrefix(t, "%%") {
		return
	}
	// Keep the trailing space of a line still being written: it tells that
	// its last word is complete.
	s, commented := stripComment(strings.TrimLeft(line, " \t"))
	if commented && tail {
		// The comment ends the statement: its last word is complete.
		s += " "
	}
	if !tail {
		s = strings.TrimSpace(s)
	}
	if strings.TrimSpace(s) == "" {
		return
	}
	p.statement(s, tail)
}

// noteLine reads a line of an open note block.
func (p *parser) noteLine(t string, tail bool) {
	if strings.Join(strings.Fields(t), " ") == "end note" {
		p.note.Partial = false
		p.note = nil
		p.noteLines = nil
		return
	}
	if tail && t != "" && strings.HasPrefix("end note", t) {
		// Maybe the end of the note, still being written.
		return
	}
	p.noteLines = append(p.noteLines, label.Plain(t))
	p.note.Label = strings.TrimSpace(strings.Join(p.noteLines, "\n"))
}

var keywords = map[string]bool{
	"state": true, "note": true, "direction": true, "classDef": true, "class": true, "style": true,
	"click": true, "hide": true, "scale": true, "link": true, "callback": true,
}

func (p *parser) statement(s string, tail bool) {
	// cut tells that the statement's last word may still grow.
	cut := tail && !strings.HasSuffix(s, " ") && !strings.HasSuffix(s, "\t")
	t := strings.TrimSpace(s)
	p.text = t
	if !p.g.HasHeader {
		p.header(t, cut)
		return
	}
	switch t {
	case "}":
		p.close()
		return
	case "--":
		if !cut {
			p.region()
		}
		return
	}
	word, rest := firstWord(t)
	if strings.HasPrefix(word, "accTitle") || strings.HasPrefix(word, "accDescr") {
		// Accessibility texts draw nothing; accDescr { ... } may go on over
		// several lines.
		if !tail && strings.HasPrefix(word, "accDescr") && strings.HasPrefix(strings.TrimLeft(t[len("accDescr"):], " \t"), "{") &&
			!strings.Contains(t, "}") {
			p.inAccDescr = true
		}
		return
	}
	// A keyword followed by --> or : is the name of a state.
	if keywords[word] && !strings.HasPrefix(rest, "-") && !strings.HasPrefix(rest, ":") {
		if word == "state" || word == "note" {
			if rest == "" {
				if !cut {
					p.problem("%s needs a name", word)
				}
				return
			}
			after := strings.TrimLeft(strings.TrimLeft(s, " \t")[len(word):], " \t")
			if word == "state" {
				p.state(after, tail, cut)
			} else {
				p.noteStmt(after, tail, cut)
			}
			return
		}
		if tail {
			// The rest apply once their line is complete.
			return
		}
		switch word {
		case "direction":
			p.direction(rest)
		case "classDef":
			p.classDef(rest)
		case "class":
			p.class(rest)
		case "style":
			p.style(rest)
		}
		// click, hide empty description and scale draw nothing.
		return
	}
	p.transition(strings.TrimLeft(s, " \t"), tail, cut)
}

func (p *parser) header(s string, cut bool) {
	word, rest := firstWord(s)
	if word != "stateDiagram" && word != "stateDiagram-v2" {
		p.problem("expected stateDiagram or stateDiagram-v2, found %q", word)
		return
	}
	if cut {
		// stateDiagram may still become stateDiagram-v2.
		return
	}
	p.g.HasHeader = true
	p.g.Dir = flow.TB
	if rest != "" {
		if d, ok := flow.ParseDirection(rest); ok {
			p.g.Dir = d
		} else {
			p.problem("unexpected %q after the header", rest)
		}
	}
}

// scope returns the subgraph new states go in: the innermost open
// composite state, or its current region, or "" at the top.
func (p *parser) scope() string {
	if len(p.stack) == 0 {
		return ""
	}
	f := p.stack[len(p.stack)-1]
	if f.region > 0 {
		return regionID(f.id, f.region)
	}
	return f.id
}

// prefix returns what the IDs of the start and end of the current scope
// begin with. The first region keeps its composite's, so that they do not
// change when a -- comes.
func (p *parser) prefix() string {
	if len(p.stack) == 0 {
		return ""
	}
	f := p.stack[len(p.stack)-1]
	if f.region > 1 {
		return regionID(f.id, f.region) + "/"
	}
	return f.id + "/"
}

func regionID(composite string, n int) string { return fmt.Sprintf("%s/region-%d", composite, n) }

// touch returns the node of state id, adding it in the current scope when
// new. It returns nil when id names a composite state: a transition to it
// goes to its box.
func (p *parser) touch(id string) *flow.Node {
	if p.g.Subgraphs[id] != nil {
		return nil
	}
	n := p.g.Nodes[id]
	if n == nil {
		n = &flow.Node{ID: id, Label: id, Shape: flow.Round, Order: p.order, Subgraph: p.scope()}
		p.order++
		p.g.Nodes[id] = n
		p.g.NodeOrder = append(p.g.NodeOrder, id)
		p.states[id] = &info{}
	}
	return n
}

// pseudo returns the start or the end of the current scope, adding it when
// new.
func (p *parser) pseudo(start bool) string {
	id, shape := p.prefix()+"[*]end", flow.End
	if start {
		id, shape = p.prefix()+"[*]start", flow.Start
	}
	if p.g.Nodes[id] == nil {
		p.g.Nodes[id] = &flow.Node{ID: id, Shape: shape, Order: p.order, Subgraph: p.scope(), Defined: true}
		p.order++
		p.g.NodeOrder = append(p.g.NodeOrder, id)
	}
	return id
}

func (p *parser) addClass(id, class string) {
	if !slices.Contains(p.classes[id], class) {
		p.classes[id] = append(p.classes[id], class)
	}
}

func (p *parser) edge(from, to, text string, line flow.Line, end flow.Arrow) *flow.Edge {
	key := from + "\x00" + to
	n := p.edgeCount[key]
	p.edgeCount[key] = n + 1
	e := &flow.Edge{
		ID:     fmt.Sprintf("%s->%s#%d", from, to, n),
		From:   from,
		To:     to,
		Label:  text,
		Line:   line,
		End:    end,
		Length: 1,
		Index:  len(p.g.Edges),
	}
	p.g.Edges = append(p.g.Edges, e)
	return e
}

// openSubgraph adds subgraph id under parent, or returns it when it
// exists. after, when set, is the subgraph it is listed after.
func (p *parser) openSubgraph(id, parent, after string) *flow.Subgraph {
	if sg := p.g.Subgraphs[id]; sg != nil {
		return sg
	}
	sg := &flow.Subgraph{ID: id, Parent: parent, Open: true}
	p.g.Subgraphs[id] = sg
	at := len(p.g.SubgraphOrder)
	if after != "" {
		if i := slices.Index(p.g.SubgraphOrder, after); i >= 0 {
			at = i + 1
		}
	}
	p.g.SubgraphOrder = slices.Insert(p.g.SubgraphOrder, at, id)
	return sg
}

// composite opens composite state id.
func (p *parser) composite(id string) {
	sg := p.openSubgraph(id, p.scope(), "")
	sg.Open = true
	f := frame{id: id}
	// Opened again: go on in its last region.
	for p.g.Subgraphs[regionID(id, f.region+1)] != nil {
		f.region++
	}
	if f.region > 0 {
		p.g.Subgraphs[regionID(id, f.region)].Open = true
	}
	p.stack = append(p.stack, f)
}

func (p *parser) close() {
	if len(p.stack) == 0 {
		p.problem("} without a composite state to close")
		return
	}
	f := p.stack[len(p.stack)-1]
	p.g.Subgraphs[f.id].Open = false
	if f.region > 0 {
		p.g.Subgraphs[regionID(f.id, f.region)].Open = false
	}
	p.stack = p.stack[:len(p.stack)-1]
}

// region starts a new region of the open composite state at a --. The
// first -- also moves what came before it into a first region.
func (p *parser) region() {
	if len(p.stack) == 0 {
		p.problem("-- outside a composite state")
		return
	}
	f := &p.stack[len(p.stack)-1]
	if f.region == 0 {
		first := regionID(f.id, 1)
		sg := p.openSubgraph(first, f.id, f.id)
		sg.Open = false
		sg.Style.Dash = RegionDash
		for _, id := range p.g.NodeOrder {
			if n := p.g.Nodes[id]; n.Subgraph == f.id {
				n.Subgraph = first
			}
		}
		for _, id := range p.g.SubgraphOrder {
			if s := p.g.Subgraphs[id]; id != first && s.Parent == f.id {
				s.Parent = first
			}
		}
		f.region = 1
	} else {
		p.g.Subgraphs[regionID(f.id, f.region)].Open = false
	}
	f.region++
	sg := p.openSubgraph(regionID(f.id, f.region), f.id, "")
	sg.Open = true
	sg.Style.Dash = RegionDash
}

func (p *parser) direction(rest string) {
	d, ok := flow.ParseDirection(strings.TrimSpace(rest))
	if !ok {
		p.problem("unknown direction %q", rest)
		return
	}
	if len(p.stack) > 0 {
		sg := p.g.Subgraphs[p.stack[len(p.stack)-1].id]
		sg.Dir, sg.HasDir = d, true
		return
	}
	p.g.Dir = d
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

// class reads class A,B name; the names may have spaces after their
// commas.
func (p *parser) class(rest string) {
	i := strings.LastIndexAny(rest, " \t")
	if i < 0 {
		p.problem("class needs state names and a class name")
		return
	}
	ids, name := rest[:i], strings.TrimSpace(rest[i+1:])
	for _, id := range strings.Split(ids, ",") {
		if id = strings.TrimSpace(id); id != "" {
			p.addClass(id, name)
		}
	}
}

func (p *parser) style(rest string) {
	id, styles := firstWord(rest)
	if id == "" || styles == "" {
		p.problem("style needs a state name and styles")
		return
	}
	for _, id := range strings.Split(id, ",") {
		if id = strings.TrimSpace(id); id != "" {
			p.styles[id] = p.styles[id].Merge(flow.ParseStyle(styles))
		}
	}
}

// state reads what follows the keyword state: a declaration, an alias
// (state "text" as ID), a composite (state ID {) or a choice, fork or
// join (state ID <<choice>>).
func (p *parser) state(s string, tail, cut bool) {
	sc := &scanner{s: s}
	alias, hasAlias := "", false
	if sc.peek() == '"' {
		text, closed := sc.quoted()
		if !closed {
			p.problem("state description is missing its closing quote")
			return
		}
		alias, hasAlias = label.Clean(text), true
		sc.skipSpace()
		if w := sc.scanID(); w != "as" {
			if !(cut && sc.eof() && strings.HasPrefix("as", w)) {
				p.problem("expected as after the state description")
			}
			return
		}
		sc.skipSpace()
	}
	id := sc.scanID()
	if id == "" {
		p.problem("state needs a name")
		return
	}
	if sc.eof() && cut {
		return // the name may grow
	}
	var classes []string
	for strings.HasPrefix(sc.s[sc.i:], ":::") {
		sc.i += 3
		c := sc.scanClass()
		if sc.eof() && cut {
			return
		}
		if c != "" {
			classes = append(classes, c)
		}
	}
	sc.skipSpace()
	after := strings.TrimSpace(sc.s[sc.i:])
	k := plain
	switch {
	case after == "{":
		if hasAlias {
			p.info(id).alias = alias
		}
		p.composite(id)
		p.applyClasses(id, classes)
		return
	case strings.HasPrefix(after, "<<") || strings.HasPrefix(after, "[["):
		closer := ">>"
		if after[0] == '[' {
			closer = "]]"
		}
		body, _, closed := strings.Cut(after[2:], closer)
		if !closed {
			p.problem("%s is missing its closing %s", after[:2], closer)
			return
		}
		switch strings.ToLower(strings.TrimSpace(body)) {
		case "choice":
			k = choice
		case "fork", "join":
			k = bar
		default:
			p.problem("unknown state type %q", body)
			return
		}
		if rest := strings.TrimSpace(after[2+len(body)+2:]); rest != "" {
			p.problem("unexpected %q after the state type", rest)
		}
	case strings.HasPrefix(after, ":"):
		// state ID : description, as ID : description.
		n := p.touch(id)
		if n == nil {
			return
		}
		n.Defined = true
		if hasAlias {
			p.info(id).alias = alias
		}
		p.applyClasses(id, classes)
		p.describe(id, after[1:], tail)
		return
	case after != "":
		if !tail {
			p.problem("unexpected %q after state %s", after, id)
		}
		return
	case tail:
		// A { or a type may still come.
		return
	}
	n := p.touch(id)
	if n == nil {
		p.applyClasses(id, classes)
		return
	}
	n.Defined = true
	in := p.info(id)
	if hasAlias {
		in.alias = alias
	}
	if k != plain {
		in.kind = k
	}
	p.applyClasses(id, classes)
}

func (p *parser) info(id string) *info {
	in := p.states[id]
	if in == nil {
		in = &info{}
		p.states[id] = in
	}
	return in
}

func (p *parser) applyClasses(id string, classes []string) {
	for _, c := range classes {
		p.addClass(id, c)
	}
}

// describe adds a description to state id.
func (p *parser) describe(id, text string, tail bool) {
	d := label.Clean(text)
	if d == "" {
		return
	}
	in := p.info(id)
	in.descs = append(in.descs, d)
	in.partial = tail
}

// noteStmt reads what follows the keyword note: left of X : text,
// right of X : text, the same opening a block that end note closes, or a
// floating note, "text" as N.
func (p *parser) noteStmt(s string, tail, cut bool) {
	sc := &scanner{s: s}
	if sc.peek() == '"' {
		text, closed := sc.quoted()
		if !closed {
			p.problem("note text is missing its closing quote")
			return
		}
		sc.skipSpace()
		if w := sc.scanID(); w != "as" {
			p.problem("expected as after the note text")
			return
		}
		sc.skipSpace()
		id := sc.scanID()
		if id == "" || sc.eof() && cut {
			p.problem("note needs a name")
			return
		}
		if p.g.Nodes[id] == nil {
			p.g.Nodes[id] = &flow.Node{ID: id, Shape: flow.Note, Order: p.order, Subgraph: p.scope(), Defined: true}
			p.order++
			p.g.NodeOrder = append(p.g.NodeOrder, id)
		}
		p.g.Nodes[id].Label = label.Plain(text)
		return
	}
	side := sc.scanID()
	if side != "left" && side != "right" {
		if !(cut && sc.eof() && (strings.HasPrefix("left", side) || strings.HasPrefix("right", side))) {
			p.problem("expected left of or right of after note")
		}
		return
	}
	sc.skipSpace()
	if w := sc.scanID(); w != "of" {
		if !(cut && sc.eof() && strings.HasPrefix("of", w)) {
			p.problem("expected of after note %s", side)
		}
		return
	}
	sc.skipSpace()
	target := sc.scanID()
	if target == "" {
		p.problem("note needs a state")
		return
	}
	if sc.eof() && cut {
		return // the name may grow
	}
	sc.skipSpace()
	block := sc.eof()
	var text string
	switch {
	case block:
		if tail {
			// A : and its text may still come.
			return
		}
	case sc.peek() == ':':
		text = label.Plain(sc.s[sc.i+1:])
		if text == "" {
			if !tail {
				p.problem("note has no text")
			}
			return
		}
	default:
		p.problem("unexpected %q after note %s of %s", sc.s[sc.i:], side, target)
		return
	}
	p.touch(target)
	p.notes++
	id := fmt.Sprintf("note-%d", p.notes)
	n := &flow.Node{ID: id, Label: text, Shape: flow.Note, Order: p.order, Subgraph: p.scope(), Defined: true, Partial: tail}
	p.order++
	p.g.Nodes[id] = n
	p.g.NodeOrder = append(p.g.NodeOrder, id)
	if side == "left" {
		p.edge(id, target, "", flow.Dotted, flow.ArrowNone)
	} else {
		p.edge(target, id, "", flow.Dotted, flow.ArrowNone)
	}
	if block {
		n.Partial = true
		p.note = n
	}
}

// ref is a state named in a transition or declaration.
type ref struct {
	id      string
	star    bool // [*]
	classes []string
}

var errIncomplete = errors.New("incomplete")

// ref reads [*] or a name, with the classes ::: gives it.
func (sc *scanner) ref(cut bool) (ref, error) {
	var r ref
	switch {
	case strings.HasPrefix(sc.s[sc.i:], "[*]"):
		r.star = true
		sc.i += 3
	case sc.peek() == '[':
		if cut && strings.HasPrefix("[*]", sc.s[sc.i:]) {
			return r, errIncomplete
		}
		return r, fmt.Errorf("expected a state, found %q", short(sc.s[sc.i:]))
	default:
		r.id = sc.scanID()
		if r.id == "" {
			return r, fmt.Errorf("expected a state, found %q", short(sc.s[sc.i:]))
		}
		if sc.eof() && cut {
			return r, errIncomplete
		}
	}
	for strings.HasPrefix(sc.s[sc.i:], ":::") {
		sc.i += 3
		c := sc.scanClass()
		if sc.eof() && cut {
			// The class name may grow: the state shows without it.
			return r, nil
		}
		if c != "" {
			r.classes = append(r.classes, c)
		}
	}
	if cut && (sc.s[sc.i:] == ":" || sc.s[sc.i:] == "::") {
		// Maybe the start of :::.
		sc.i = len(sc.s)
	}
	return r, nil
}

// use adds the state r names: the start or end of the scope for [*].
func (p *parser) use(r ref, start bool) string {
	if r.star {
		return p.pseudo(start)
	}
	p.touch(r.id)
	p.applyClasses(r.id, r.classes)
	return r.id
}

// transition reads A --> B : label, a declaration A, or a description
// A : text.
func (p *parser) transition(s string, tail, cut bool) {
	sc := &scanner{s: s}
	left, err := sc.ref(cut)
	if err != nil {
		if !errors.Is(err, errIncomplete) {
			p.problem("%v", err)
		}
		return
	}
	sc.skipSpace()
	switch {
	case sc.eof():
		if left.star {
			p.problem("[*] needs a transition")
			return
		}
		from := p.use(left, true)
		if n := p.g.Nodes[from]; n != nil && !tail {
			n.Defined = true
		}
		return
	case strings.HasPrefix(sc.s[sc.i:], "-->"):
		sc.i += 3
	case sc.peek() == ':':
		if left.star {
			p.problem("[*] cannot have a description")
			return
		}
		p.use(left, true)
		p.describe(left.id, sc.s[sc.i+1:], tail)
		return
	default:
		if tail && strings.HasPrefix("-->", sc.s[sc.i:]) {
			p.use(left, true)
			return
		}
		p.problem("expected --> or : after %s, found %q", leftName(left), short(sc.s[sc.i:]))
		return
	}
	from := p.use(left, true)
	sc.skipSpace()
	if sc.eof() {
		p.problem("transition has no target")
		return
	}
	right, err := sc.ref(cut)
	if err != nil {
		if !errors.Is(err, errIncomplete) {
			p.problem("%v", err)
		}
		return
	}
	to := p.use(right, false)
	e := p.edge(from, to, "", flow.Solid, flow.ArrowNormal)
	sc.skipSpace()
	switch {
	case sc.eof():
	case sc.peek() == ':':
		e.Label = label.Clean(sc.s[sc.i+1:])
	default:
		p.problem("unexpected %q after the transition", short(sc.s[sc.i:]))
	}
}

func leftName(r ref) string {
	if r.star {
		return "[*]"
	}
	return r.id
}

func (p *parser) finish() {
	g := p.g
	// A name that is a composite state's is the composite: transitions to
	// it go to its box, and its classes and styles are the box's.
	kept := g.NodeOrder[:0]
	for _, id := range g.NodeOrder {
		if g.Subgraphs[id] != nil {
			delete(g.Nodes, id)
			continue
		}
		kept = append(kept, id)
	}
	g.NodeOrder = kept
	for _, id := range g.NodeOrder {
		n := g.Nodes[id]
		n.Classes = slices.Clone(p.classes[id])
		n.Style = p.styles[id]
		in := p.states[id]
		if in == nil {
			continue
		}
		switch {
		case in.kind == choice:
			n.Shape, n.Label = flow.Choice, ""
		case in.kind == bar:
			n.Shape, n.Label = flow.Bar, ""
		case len(in.descs) == 1 && in.alias == "":
			// As Mermaid does, a single description names the state.
			n.Shape, n.Label, n.Partial = flow.Round, in.descs[0], in.partial
		case len(in.descs) > 0:
			name := id
			if in.alias != "" {
				name = in.alias
			}
			rows := make([][]string, len(in.descs))
			for i, d := range in.descs {
				rows[i] = []string{d}
			}
			n.Shape, n.Label, n.Partial = flow.Sectioned, name, in.partial
			n.Table = &flow.Table{Sections: []flow.Section{
				{Rows: [][]string{{name}}, Center: true, Bold: true},
				{Rows: rows},
			}}
		case in.alias != "":
			n.Label = in.alias
		}
	}
	for i, id := range g.SubgraphOrder {
		sg := g.Subgraphs[id]
		sg.Order = i
		sg.Nodes, sg.Children = nil, nil
		if parent := g.Subgraphs[sg.Parent]; parent != nil {
			parent.Children = append(parent.Children, id)
		}
		if sg.Style.Dash == RegionDash && strings.Contains(id, "/region-") {
			continue
		}
		sg.Title = id
		if in := p.states[id]; in != nil {
			if in.alias != "" {
				sg.Title = in.alias
			} else if len(in.descs) > 0 {
				sg.Title = in.descs[0]
			}
		}
		sg.Classes = slices.Clone(p.classes[id])
		sg.Style = p.styles[id]
	}
	for _, id := range g.NodeOrder {
		n := g.Nodes[id]
		if sg := g.Subgraphs[n.Subgraph]; sg != nil {
			sg.Nodes = append(sg.Nodes, id)
		} else {
			n.Subgraph = ""
		}
	}
	if g.Final {
		if p.note != nil {
			p.note.Partial = false
			g.Problems = append(g.Problems, flow.Problem{Msg: "note is missing its end note"})
		}
		for _, n := range g.Nodes {
			n.Partial = false
		}
		if !g.HasHeader && len(g.Problems) == 0 {
			g.Problems = append(g.Problems, flow.Problem{Msg: "missing stateDiagram header"})
		}
		for _, f := range p.stack {
			g.Problems = append(g.Problems, flow.Problem{Msg: fmt.Sprintf("composite state %q is missing its }", f.id)})
		}
	}
}

// scanner reads one statement.
type scanner struct {
	s string
	i int
}

func (sc *scanner) eof() bool { return sc.i >= len(sc.s) }

func (sc *scanner) peek() byte {
	if sc.eof() {
		return 0
	}
	return sc.s[sc.i]
}

func (sc *scanner) skipSpace() {
	for sc.i < len(sc.s) && (sc.s[sc.i] == ' ' || sc.s[sc.i] == '\t') {
		sc.i++
	}
}

// isIDRune reports whether r may be part of a state's name: letters
// (of any script), digits, _ and dots.
func isIDRune(r rune) bool {
	return r == '_' || r == '.' || unicode.IsLetter(r) || unicode.IsDigit(r) ||
		r >= utf8.RuneSelf && !unicode.IsSpace(r) && !unicode.IsPunct(r) && r != utf8.RuneError
}

// scanID reads a name. A character cut short at the end of the statement,
// as a source streamed in bytes may be, counts as part of it.
func (sc *scanner) scanID() string {
	start := sc.i
	for sc.i < len(sc.s) {
		if !utf8.FullRuneInString(sc.s[sc.i:]) {
			sc.i = len(sc.s)
			break
		}
		r, size := utf8.DecodeRuneInString(sc.s[sc.i:])
		if !isIDRune(r) {
			break
		}
		sc.i += size
	}
	return sc.s[start:sc.i]
}

// scanClass reads a class name: as a name, with hyphens.
func (sc *scanner) scanClass() string {
	start := sc.i
	for sc.i < len(sc.s) {
		r, size := utf8.DecodeRuneInString(sc.s[sc.i:])
		if !isIDRune(r) && r != '-' {
			break
		}
		sc.i += size
	}
	return sc.s[start:sc.i]
}

// quoted reads a string in double quotes at the scanner's position. It
// returns false when the closing quote has not come.
func (sc *scanner) quoted() (string, bool) {
	sc.i++ // opening quote
	end := strings.IndexByte(sc.s[sc.i:], '"')
	if end < 0 {
		rest := sc.s[sc.i:]
		sc.i = len(sc.s)
		return rest, false
	}
	out := sc.s[sc.i : sc.i+end]
	sc.i += end + 1
	return out, true
}

// short shortens s for an error message.
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

// stripComment removes a %% comment that follows a statement, telling
// whether there was one.
func stripComment(s string) (string, bool) {
	inQuote := false
	for i := 0; i+1 < len(s); i++ {
		switch {
		case s[i] == '"':
			inQuote = !inQuote
		case !inQuote && s[i] == '%' && s[i+1] == '%':
			return strings.TrimRight(s[:i], " \t"), true
		}
	}
	return s, false
}
