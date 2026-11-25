package flow

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/momaek/streameraid/label"
)

// Parse parses the source of a flowchart.
//
// final tells whether the source is complete. When it is not, and the
// source does not end with a newline, its last line is still being
// written: Parse reads it tentatively, so that a diagram streaming in does
// not flicker.
//
//   - An identifier that reaches the end of the source may grow (B, then
//     Ba, then Bar), so it makes no node, edge or subgraph yet.
//   - A label whose bracket has not closed shows as written so far, its
//     node marked Partial.
//   - An edge without its target is left out.
//   - Statements that only style (classDef, class, style, linkStyle) and
//     end apply once their line is complete.
//
// Statements Parse cannot read are recorded in Graph.Problems and skipped;
// what parsed before the error in the same statement is kept.
func Parse(src string, final bool) *Graph {
	p := &parser{g: newGraph(), edgeCount: map[string]int{}, linkStyles: map[int]Style{}, edgeIDs: map[string]bool{}}
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
		// A Markdown string ("`...`") may go on over several lines.
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

var (
	errIncomplete = errors.New("incomplete")
)

type parser struct {
	g          *Graph
	stack      []string // open subgraphs, innermost last
	order      int
	sgOrder    int
	edgeCount  map[string]int
	linkStyles map[int]Style
	linkAll    Style
	edgeIDs    map[string]bool // IDs given to edges, as e1 in A e1@--> B
	line       int
	text       string // the statement being parsed, for problems
}

func (p *parser) problem(format string, args ...any) {
	p.g.Problems = append(p.g.Problems, Problem{Line: p.line, Text: p.text, Msg: fmt.Sprintf(format, args...)})
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
		if k, v, ok := strings.Cut(t, ":"); ok && strings.TrimSpace(k) == "title" {
			p.g.Title = unquote(strings.TrimSpace(v))
		}
	}
	if p.g.Final {
		p.line = first + 1
		p.text = lines[first]
		p.problem("front matter is not closed with ---")
	}
	return 0, false
}

func (p *parser) parseLine(line string, tail bool) {
	t := strings.TrimSpace(line)
	if t == "" || strings.HasPrefix(t, "%%") {
		return
	}
	// Keep the trailing space of a line still being written: it tells that
	// its last word is complete.
	body := stripComment(strings.TrimLeft(line, " \t"))
	stmts := splitStatements(body)
	for k, s := range stmts {
		last := tail && k == len(stmts)-1
		if last {
			s = strings.TrimLeft(s, " \t")
		} else {
			s = strings.TrimSpace(s)
		}
		if strings.TrimSpace(s) == "" {
			continue
		}
		// Only the last statement of the last line can be cut short.
		p.statement(s, last)
	}
}

func (p *parser) statement(s string, tail bool) {
	// cut tells that the statement's last word may still grow.
	cut := tail && !strings.HasSuffix(s, " ") && !strings.HasSuffix(s, "\t")
	p.text = strings.TrimSpace(s)
	if !p.g.HasHeader {
		p.header(p.text, cut)
		return
	}
	word, rest := firstWord(s)
	switch word {
	case "subgraph":
		p.subgraph(strings.TrimLeft(strings.TrimLeft(s, " \t")[len(word):], " \t"), tail)
		return
	case "end":
		if rest == "" {
			if !cut {
				p.end()
			}
			return
		}
	case "direction":
		if !tail {
			p.direction(rest)
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
	case "linkStyle":
		if !tail {
			p.linkStyle(rest)
		}
		return
	case "click", "accTitle:", "accDescr:", "accTitle", "accDescr":
		// Interactions and accessibility text draw nothing.
		return
	}
	if err := p.chain(s, tail); err != nil && !tail {
		p.problem("%v", err)
	}
}

func (p *parser) header(s string, tail bool) {
	word, rest := firstWord(s)
	switch word {
	case "graph", "flowchart", "flowchart-elk":
	default:
		if !tail {
			p.problem("expected flowchart or graph, found %q", word)
		}
		return
	}
	if tail {
		// The direction may still be coming.
		return
	}
	p.g.HasHeader = true
	p.g.Dir = TB
	if rest != "" {
		d, ok := ParseDirection(strings.TrimSpace(rest))
		if !ok {
			p.problem("unknown direction %q", rest)
		}
		p.g.Dir = d
	}
}

func (p *parser) top() *Subgraph {
	if len(p.stack) == 0 {
		return nil
	}
	return p.g.Subgraphs[p.stack[len(p.stack)-1]]
}

func (p *parser) subgraph(rest string, tail bool) {
	sc := &scanner{s: rest, tail: tail}
	var id, title string
	switch {
	case strings.TrimSpace(rest) == "":
		if tail {
			return
		}
		id = fmt.Sprintf("subgraph-%d", p.sgOrder+1)
	case rest[0] == '"':
		t, err := sc.quoted()
		if err != nil {
			if !tail {
				p.problem("subgraph title is missing its closing quote")
			}
			return
		}
		id, title = t, t
	default:
		id = sc.scanID()
		if id != "" && sc.eof() && tail {
			return // the identifier may grow
		}
		sc.skipSpace()
		switch {
		case id == "":
			id = strings.TrimSpace(rest)
			title = id
			if tail {
				return
			}
		case sc.eof():
			title = id
		case sc.peek() == '[':
			sc.i++
			raw, closed := sc.until("]")
			if !closed && !tail {
				p.problem("subgraph title is missing its closing ]")
				return
			}
			title = cleanLabel(raw)
		default:
			if tail {
				return
			}
			id = strings.TrimSpace(rest)
			title = id
		}
	}
	title = cleanLabel(title)
	sg := p.g.Subgraphs[id]
	if sg == nil {
		sg = &Subgraph{ID: id, Order: p.sgOrder}
		p.sgOrder++
		p.g.Subgraphs[id] = sg
		p.g.SubgraphOrder = append(p.g.SubgraphOrder, id)
		if parent := p.top(); parent != nil {
			sg.Parent = parent.ID
			parent.Children = append(parent.Children, id)
		}
	}
	sg.Title = title
	sg.Open = true
	p.stack = append(p.stack, id)
}

func (p *parser) end() {
	sg := p.top()
	if sg == nil {
		p.problem("end without a subgraph to close")
		return
	}
	sg.Open = false
	p.stack = p.stack[:len(p.stack)-1]
}

func (p *parser) direction(rest string) {
	d, ok := ParseDirection(strings.TrimSpace(rest))
	if !ok {
		p.problem("unknown direction %q", rest)
		return
	}
	if sg := p.top(); sg != nil {
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
	st := parseStyle(styles)
	for _, n := range strings.Split(names, ",") {
		if n = strings.TrimSpace(n); n != "" {
			p.g.ClassDefs[n] = p.g.ClassDefs[n].Merge(st)
		}
	}
}

func (p *parser) class(rest string) {
	ids, name := firstWord(rest)
	name = strings.TrimSpace(name)
	if ids == "" || name == "" {
		p.problem("class needs node IDs and a class name")
		return
	}
	for _, id := range strings.Split(ids, ",") {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if sg := p.g.Subgraphs[id]; sg != nil {
			sg.Classes = appendUnique(sg.Classes, name)
			continue
		}
		n := p.touch(id)
		n.Classes = appendUnique(n.Classes, name)
	}
}

func (p *parser) style(rest string) {
	id, styles := firstWord(rest)
	if id == "" || styles == "" {
		p.problem("style needs an ID and styles")
		return
	}
	st := parseStyle(styles)
	if sg := p.g.Subgraphs[id]; sg != nil {
		sg.Style = sg.Style.Merge(st)
		return
	}
	n := p.touch(id)
	n.Style = n.Style.Merge(st)
}

func (p *parser) linkStyle(rest string) {
	which, styles := firstWord(rest)
	if which == "" || styles == "" {
		p.problem("linkStyle needs edge numbers and styles")
		return
	}
	// "interpolate basis" and the like choose the curve; edges are always
	// drawn smooth here.
	if w, _ := firstWord(styles); w == "interpolate" {
		return
	}
	st := parseStyle(styles)
	if which == "default" {
		p.linkAll = p.linkAll.Merge(st)
		return
	}
	for _, n := range strings.Split(which, ",") {
		i, err := strconv.Atoi(strings.TrimSpace(n))
		if err != nil || i < 0 {
			p.problem("linkStyle: %q is not an edge number", n)
			continue
		}
		p.linkStyles[i] = p.linkStyles[i].Merge(st)
	}
}

// touch returns the node id, adding it when new and placing it in the open
// subgraph when it has none yet.
func (p *parser) touch(id string) *Node {
	n := p.g.Nodes[id]
	if n == nil {
		n = &Node{ID: id, Label: id, Order: p.order}
		p.order++
		p.g.Nodes[id] = n
		p.g.NodeOrder = append(p.g.NodeOrder, id)
	}
	if sg := p.top(); sg != nil && n.Subgraph == "" && id != sg.ID && !p.isAncestor(id) {
		n.Subgraph = sg.ID
	}
	return n
}

// isAncestor reports whether id names an open subgraph.
func (p *parser) isAncestor(id string) bool {
	for _, s := range p.stack {
		if s == id {
			return true
		}
	}
	return false
}

type nodeRef struct {
	id       string
	hasShape bool
	shape    Shape
	label    string
	partial  bool
	class    string
	// attrsOnly marks A@{ ... } with neither shape nor label: settings of
	// an edge or subgraph with that ID (animation, collapsed view), which
	// draw nothing here.
	attrsOnly bool
}

type link struct {
	line       Line
	start, end Arrow
	length     int
	label      string
}

// chain parses a statement of nodes and edges: A --> B & C -.-> D.
func (p *parser) chain(s string, tail bool) error {
	sc := &scanner{s: s, tail: tail}
	group, err := p.group(sc)
	if err != nil {
		return ignoreIncomplete(err)
	}
	for {
		sc.skipSpace()
		if sc.eof() || group[len(group)-1].partial {
			return nil
		}
		if id := sc.edgeID(); id != "" {
			p.edgeIDs[id] = true
		}
		lk, err := sc.scanLink()
		if err != nil {
			return ignoreIncomplete(err)
		}
		sc.skipSpace()
		if !sc.eof() && sc.peek() == '|' {
			sc.i++
			raw, closed := sc.until("|")
			if !closed {
				if tail {
					return nil
				}
				return errors.New("edge label is missing its closing |")
			}
			lk.label = cleanLabel(raw)
			sc.skipSpace()
		}
		if sc.eof() {
			if tail {
				return nil
			}
			return errors.New("edge has no target")
		}
		next, err := p.group(sc)
		if len(next) > 0 {
			for _, a := range group {
				for _, b := range next {
					p.edge(a.id, b.id, lk)
				}
			}
			group = next
		}
		if err != nil {
			return ignoreIncomplete(err)
		}
	}
}

func ignoreIncomplete(err error) error {
	if errors.Is(err, errIncomplete) {
		return nil
	}
	return err
}

// group parses nodes joined by &. It adds the nodes it reads, and returns
// them with the error that stopped it, if any.
func (p *parser) group(sc *scanner) ([]nodeRef, error) {
	var refs []nodeRef
	for {
		sc.skipSpace()
		ref, err := sc.node()
		if err != nil {
			return refs, err
		}
		if ref.attrsOnly || (p.edgeIDs[ref.id] && !ref.hasShape) {
			// Settings of an edge or a subgraph: nothing to draw.
			return nil, errIncomplete
		}
		p.apply(ref)
		refs = append(refs, ref)
		if ref.partial {
			return refs, nil
		}
		save := sc.i
		sc.skipSpace()
		if sc.eof() || sc.peek() != '&' {
			sc.i = save
			return refs, nil
		}
		sc.i++
	}
}

func (p *parser) apply(ref nodeRef) {
	n := p.touch(ref.id)
	if ref.hasShape {
		n.Shape, n.Label, n.Partial, n.Defined = ref.shape, ref.label, ref.partial, true
	}
	if ref.class != "" {
		n.Classes = appendUnique(n.Classes, ref.class)
	}
}

func (p *parser) edge(from, to string, lk link) {
	key := from + "\x00" + to
	n := p.edgeCount[key]
	p.edgeCount[key] = n + 1
	p.g.Edges = append(p.g.Edges, &Edge{
		ID:     fmt.Sprintf("%s->%s#%d", from, to, n),
		From:   from,
		To:     to,
		Label:  lk.label,
		Line:   lk.line,
		Start:  lk.start,
		End:    lk.end,
		Length: max(lk.length, 1),
		Index:  len(p.g.Edges),
	})
}

func (p *parser) finish() {
	g := p.g
	// A name that is a subgraph's ID, given no shape of its own, is the
	// subgraph: an edge to it goes to the subgraph's box.
	if len(g.Subgraphs) > 0 {
		kept := g.NodeOrder[:0]
		for _, id := range g.NodeOrder {
			n := g.Nodes[id]
			if g.Subgraphs[id] != nil && !n.Defined {
				delete(g.Nodes, id)
				continue
			}
			kept = append(kept, id)
		}
		g.NodeOrder = kept
	}
	for _, id := range g.NodeOrder {
		n := g.Nodes[id]
		if sg := g.Subgraphs[n.Subgraph]; sg != nil {
			sg.Nodes = append(sg.Nodes, id)
		} else {
			n.Subgraph = ""
		}
	}
	for _, e := range g.Edges {
		e.Style = p.linkAll.Merge(p.linkStyles[e.Index])
	}
	if g.Final {
		if !g.HasHeader && len(g.Problems) == 0 {
			g.Problems = append(g.Problems, Problem{Msg: "missing flowchart header"})
		}
		for _, id := range p.stack {
			g.Problems = append(g.Problems, Problem{Msg: fmt.Sprintf("subgraph %q is missing its end", id)})
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

// scanID reads an identifier: letters, digits and underscores, with single
// hyphens between them (my-node, but not the -- of a link).
func (sc *scanner) scanID() string {
	start := sc.i
	for sc.i < len(sc.s) {
		r, size := utf8.DecodeRuneInString(sc.s[sc.i:])
		if isIDRune(r) {
			sc.i += size
			continue
		}
		if r == '-' && sc.i > start && sc.i+1 < len(sc.s) {
			if next, _ := utf8.DecodeRuneInString(sc.s[sc.i+1:]); isIDRune(next) {
				sc.i++
				continue
			}
		}
		break
	}
	return sc.s[start:sc.i]
}

// until reads up to the next occurrence of close, skipping text in double
// quotes, and returns what came before it. Without close it returns the
// rest and false.
func (sc *scanner) until(close string) (string, bool) {
	start := sc.i
	inQuote := false
	for i := sc.i; i < len(sc.s); i++ {
		if sc.s[i] == '"' {
			inQuote = !inQuote
			continue
		}
		if !inQuote && strings.HasPrefix(sc.s[i:], close) {
			sc.i = i + len(close)
			return sc.s[start:i], true
		}
	}
	sc.i = len(sc.s)
	return sc.s[start:], false
}

// quoted reads a string in double quotes at the scanner's position.
func (sc *scanner) quoted() (string, error) {
	sc.i++ // opening quote
	end := strings.IndexByte(sc.s[sc.i:], '"')
	if end < 0 {
		rest := sc.s[sc.i:]
		sc.i = len(sc.s)
		return rest, errIncomplete
	}
	out := sc.s[sc.i : sc.i+end]
	sc.i += end + 1
	return out, nil
}

// node reads a node: an ID with an optional shape and class.
func (sc *scanner) node() (nodeRef, error) {
	id := sc.scanID()
	if id == "" {
		if sc.eof() {
			return nodeRef{}, sc.incomplete(errors.New("expected a node"))
		}
		return nodeRef{}, fmt.Errorf("unexpected %q", rest(sc.s[sc.i:]))
	}
	if sc.eof() && sc.tail {
		return nodeRef{}, errIncomplete // the ID may grow
	}
	ref := nodeRef{id: id}
	if strings.HasPrefix(sc.s[sc.i:], "@{") {
		if err := sc.attrs(&ref); err != nil {
			return ref, err
		}
		if ref.partial {
			return ref, nil
		}
	} else if !sc.eof() && strings.IndexByte("([{>", sc.peek()) >= 0 {
		if err := sc.shape(&ref); err != nil {
			return ref, err
		}
		if ref.partial {
			return ref, nil
		}
	}
	if strings.HasPrefix(sc.s[sc.i:], ":::") {
		sc.i += 3
		cls := sc.scanID()
		if sc.eof() && sc.tail {
			return ref, nil // the class name may grow
		}
		if cls == "" {
			return ref, errors.New("expected a class name after :::")
		}
		ref.class = cls
	}
	return ref, nil
}

// shapeAliases maps the shape names of Mermaid's A@{ shape: ... } syntax
// to the shapes drawn here. Names not listed draw as rectangles.
var shapeAliases = map[string]Shape{
	"rect": Rect, "proc": Rect, "process": Rect, "rectangle": Rect,
	"rounded": Round, "event": Round,
	"stadium": Stadium, "pill": Stadium, "terminal": Stadium,
	"subproc": Subroutine, "subprocess": Subroutine, "subroutine": Subroutine, "fr-rect": Subroutine, "framed-rectangle": Subroutine,
	"cyl": Cylinder, "cylinder": Cylinder, "database": Cylinder, "db": Cylinder,
	"circle": Circle, "circ": Circle,
	"dbl-circ": DoubleCircle, "double-circle": DoubleCircle,
	"odd":  Asymmetric,
	"diam": Diamond, "diamond": Diamond, "decision": Diamond, "question": Diamond,
	"hex": Hexagon, "hexagon": Hexagon, "prepare": Hexagon,
	"lean-r": Parallelogram, "lean-right": Parallelogram, "in-out": Parallelogram,
	"lean-l": ParallelogramAlt, "lean-left": ParallelogramAlt, "out-in": ParallelogramAlt,
	"trap-b": Trapezoid, "trapezoid-bottom": Trapezoid, "priority": Trapezoid, "trapezoid": Trapezoid,
	"trap-t": TrapezoidAlt, "trapezoid-top": TrapezoidAlt, "manual": TrapezoidAlt, "inv-trapezoid": TrapezoidAlt,
}

// attrs reads the A@{ shape: ..., label: "..." } syntax at the scanner's
// position.
func (sc *scanner) attrs(ref *nodeRef) error {
	sc.i += 2
	body, closed := sc.until("}")
	if !closed {
		if sc.tail {
			ref.partial = true
			if v, ok := attrValue(body, "label"); ok {
				ref.hasShape, ref.label = true, cleanLabel(v)
			}
			return nil
		}
		return fmt.Errorf("node %s is missing its closing }", ref.id)
	}
	ref.attrsOnly = true
	if v, ok := attrValue(body, "shape"); ok {
		ref.hasShape = true
		ref.shape = shapeAliases[strings.ToLower(v)]
		ref.attrsOnly = false
	}
	if v, ok := attrValue(body, "label"); ok {
		ref.hasShape = true
		ref.label = cleanLabel(v)
		ref.attrsOnly = false
	}
	if ref.hasShape && ref.label == "" {
		ref.label = ref.id
	}
	return nil
}

// attrValue finds key's value in "key: value, other: "quoted, value"".
func attrValue(body, key string) (string, bool) {
	inQuote := false
	start := 0
	for i := 0; i <= len(body); i++ {
		if i < len(body) && body[i] == '"' {
			inQuote = !inQuote
			continue
		}
		if i == len(body) || (!inQuote && body[i] == ',') {
			k, v, ok := strings.Cut(body[start:i], ":")
			if ok && strings.TrimSpace(k) == key {
				return unquote(strings.TrimSpace(v)), true
			}
			start = i + 1
		}
	}
	return "", false
}

// edgeID skips the ID of an edge, as in A e1@--> B.
func (sc *scanner) edgeID() string {
	save := sc.i
	id := sc.scanID()
	if id != "" && sc.i+1 < len(sc.s) && sc.s[sc.i] == '@' && strings.IndexByte("-=.~<ox", sc.s[sc.i+1]) >= 0 {
		sc.i++
		return id
	}
	sc.i = save
	return ""
}

type opener struct {
	open, close string
	shape       Shape
}

var openers = []opener{
	{"(((", ")))", DoubleCircle},
	{"((", "))", Circle},
	{"([", "])", Stadium},
	{"(", ")", Round},
	{"[[", "]]", Subroutine},
	{"[(", ")]", Cylinder},
	{"[/", "/]", Parallelogram},
	{"[\\", "\\]", ParallelogramAlt},
	{"[", "]", Rect},
	{"{{", "}}", Hexagon},
	{"{", "}", Diamond},
	{">", "]", Asymmetric},
}

// shape reads a node's shape and label at the scanner's position.
func (sc *scanner) shape(ref *nodeRef) error {
	rest := sc.s[sc.i:]
	var op opener
	for _, o := range openers {
		if strings.HasPrefix(rest, o.open) {
			op = o
			break
		}
	}
	sc.i += len(op.open)
	ref.hasShape = true
	ref.shape = op.shape

	// A quoted label may hold the closing brackets.
	save := sc.i
	sc.skipSpace()
	if !sc.eof() && sc.peek() == '"' {
		text, err := sc.quoted()
		if err == nil {
			sc.skipSpace()
			if c, ok := sc.closeShape(op, ref); ok {
				_ = c
				ref.label = cleanLabel(text)
				return nil
			}
			if sc.eof() && sc.tail {
				ref.label, ref.partial = cleanLabel(text), true
				return nil
			}
		} else if sc.tail {
			ref.label, ref.partial = cleanLabel(text), true
			return nil
		}
		sc.i = save // read it as unquoted text
	}

	closes := []string{op.close}
	switch op.shape {
	case Parallelogram:
		closes = []string{"/]", "\\]"}
	case ParallelogramAlt:
		closes = []string{"\\]", "/]"}
	}
	best, which := -1, ""
	for _, c := range closes {
		if k := indexOutsideQuotes(sc.s[sc.i:], c); k >= 0 && (best < 0 || k < best) {
			best, which = k, c
		}
	}
	if best < 0 && (op.shape == Parallelogram || op.shape == ParallelogramAlt) {
		// A[/api/users] is a rectangle whose label starts with a slash.
		if k := indexOutsideQuotes(sc.s[sc.i:], "]"); k >= 0 {
			ref.shape = Rect
			ref.label = cleanLabel(op.open[1:] + sc.s[sc.i:sc.i+k])
			sc.i += k + 1
			return nil
		}
	}
	if best < 0 {
		text := sc.s[sc.i:]
		sc.i = len(sc.s)
		if sc.tail {
			ref.label, ref.partial = cleanLabel(text), true
			return nil
		}
		return fmt.Errorf("node %s is missing its closing %s", ref.id, op.close)
	}
	ref.label = cleanLabel(sc.s[sc.i : sc.i+best])
	sc.i += best + len(which)
	switch {
	case op.shape == Parallelogram && which == "\\]":
		ref.shape = Trapezoid
	case op.shape == ParallelogramAlt && which == "/]":
		ref.shape = TrapezoidAlt
	}
	return nil
}

// closeShape consumes the closing bracket of op after a quoted label.
func (sc *scanner) closeShape(op opener, ref *nodeRef) (string, bool) {
	rest := sc.s[sc.i:]
	closes := []string{op.close}
	switch op.shape {
	case Parallelogram:
		closes = []string{"/]", "\\]"}
	case ParallelogramAlt:
		closes = []string{"\\]", "/]"}
	}
	for _, c := range closes {
		if strings.HasPrefix(rest, c) {
			sc.i += len(c)
			if op.shape == Parallelogram && c == "\\]" {
				ref.shape = Trapezoid
			}
			if op.shape == ParallelogramAlt && c == "/]" {
				ref.shape = TrapezoidAlt
			}
			return c, true
		}
	}
	return "", false
}

func indexOutsideQuotes(s, sub string) int {
	inQuote := false
	for i := 0; i < len(s); i++ {
		if s[i] == '"' {
			inQuote = !inQuote
			continue
		}
		if !inQuote && strings.HasPrefix(s[i:], sub) {
			return i
		}
	}
	return -1
}

func run(s string, i int, c byte) int {
	n := 0
	for i+n < len(s) && s[i+n] == c {
		n++
	}
	return n
}

// endsToken reports whether position i ends a link token: x and o end an
// edge only when no identifier follows them directly.
func endsToken(s string, i int) bool {
	return i >= len(s) || s[i] == ' ' || s[i] == '\t' || s[i] == '|' || s[i] == ';'
}

func arrowFor(c byte) Arrow {
	switch c {
	case '>':
		return ArrowNormal
	case 'x':
		return ArrowCross
	case 'o':
		return ArrowCircle
	}
	return ArrowNone
}

// scanLink reads a link: -->, ---, -.->, ==>, ~~~, <-->, --o, --x, with
// more dashes for longer edges, and the text forms -- text -->,
// -. text .-> and == text ==>.
func (sc *scanner) scanLink() (link, error) {
	s := sc.s
	i := sc.i
	lk := link{length: 1}
	if i < len(s) {
		switch s[i] {
		case '<':
			lk.start = ArrowNormal
			i++
		case 'x', 'o':
			if i+1 < len(s) && (s[i+1] == '-' || s[i+1] == '=') {
				lk.start = arrowFor(s[i])
				i++
			}
		}
	}
	if i >= len(s) {
		return lk, sc.incomplete(errors.New("expected an edge"))
	}
	switch s[i] {
	case '-':
		n := run(s, i, '-')
		j := i + n
		if n == 1 {
			return sc.dotted(lk, j)
		}
		lk.line = Solid
		return sc.solidOrThick(lk, '-', n, j)
	case '=':
		n := run(s, i, '=')
		if n < 2 {
			return lk, sc.incompleteAt(i+n, "expected an edge")
		}
		lk.line = Thick
		return sc.solidOrThick(lk, '=', n, i+n)
	case '~':
		n := run(s, i, '~')
		if n < 3 || (i+n >= len(s) && sc.tail) {
			return lk, sc.incompleteAt(i+n, "expected ~~~")
		}
		lk.line = Invisible
		lk.length = n - 2
		sc.i = i + n
		return lk, nil
	}
	return lk, fmt.Errorf("expected an edge, found %q", rest(s[i:]))
}

func (sc *scanner) incompleteAt(i int, msg string) error {
	if i >= len(sc.s) && sc.tail {
		return errIncomplete
	}
	return errors.New(msg)
}

// solidOrThick reads the rest of a -- or == link whose run of n dashes (or
// equals signs) ends at j.
func (sc *scanner) solidOrThick(lk link, c byte, n, j int) (link, error) {
	s := sc.s
	if j >= len(s) {
		if n >= 3 && !sc.tail {
			lk.length = n - 2
			sc.i = j
			return lk, nil
		}
		return lk, sc.incomplete(errors.New("edge has no target"))
	}
	switch next := s[j]; {
	case next == '>':
		lk.end = ArrowNormal
		lk.length = n - 1
		sc.i = j + 1
		return lk, nil
	case (next == 'x' || next == 'o') && endsToken(s, j+1):
		if j+1 >= len(s) && sc.tail {
			return lk, errIncomplete
		}
		lk.end = arrowFor(next)
		lk.length = n - 1
		sc.i = j + 1
		return lk, nil
	case n >= 3:
		lk.length = n - 2
		sc.i = j
		return lk, nil
	}
	// The text form: -- text --> or == text ==>.
	for k := j; k < len(s); k++ {
		if s[k] != c {
			continue
		}
		m := run(s, k, c)
		if m < 2 {
			continue
		}
		after := k + m
		label := cleanLabel(unquote(strings.TrimSpace(s[j:k])))
		switch {
		case after < len(s) && s[after] == '>':
			lk.end, lk.length, lk.label = ArrowNormal, m-1, label
			sc.i = after + 1
			return lk, nil
		case after < len(s) && (s[after] == 'x' || s[after] == 'o') && endsToken(s, after+1):
			lk.end, lk.length, lk.label = arrowFor(s[after]), m-1, label
			sc.i = after + 1
			return lk, nil
		case m >= 3 && (after < len(s) || !sc.tail):
			lk.length, lk.label = m-2, label
			sc.i = after
			return lk, nil
		}
		k = after - 1
	}
	return lk, sc.incomplete(fmt.Errorf("edge text is missing its closing %c%c>", c, c))
}

// dotted reads a dotted link after its first dash, which ends at j: -.-,
// -.->, -..-> or -. text .->.
func (sc *scanner) dotted(lk link, j int) (link, error) {
	s := sc.s
	lk.line = Dotted
	if j >= len(s) {
		return lk, sc.incomplete(errors.New("expected an edge"))
	}
	if s[j] != '.' {
		return lk, fmt.Errorf("expected an edge, found %q", rest(s[j-1:]))
	}
	d := run(s, j, '.')
	k := j + d
	if k >= len(s) {
		return lk, sc.incomplete(errors.New("expected an edge"))
	}
	if s[k] == '-' {
		lk.length = d
		k++
		if k < len(s) && (s[k] == '>' || ((s[k] == 'x' || s[k] == 'o') && endsToken(s, k+1))) {
			lk.end = arrowFor(s[k])
			k++
		} else if k >= len(s) && sc.tail {
			return lk, errIncomplete
		}
		sc.i = k
		return lk, nil
	}
	// The text form: -. text .->
	for m := k; m+1 < len(s); m++ {
		if s[m] == '.' && s[m+1] == '-' {
			label := cleanLabel(unquote(strings.TrimSpace(s[k:m])))
			end := m + 2
			if end < len(s) && (s[end] == '>' || ((s[end] == 'x' || s[end] == 'o') && endsToken(s, end+1))) {
				lk.end = arrowFor(s[end])
				end++
			} else if end >= len(s) && sc.tail {
				return lk, errIncomplete
			}
			lk.label = label
			sc.i = end
			return lk, nil
		}
	}
	return lk, sc.incomplete(errors.New("edge text is missing its closing .->"))
}

// rest shortens s for an error message.
func rest(s string) string {
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

// splitStatements splits a line at semicolons that are not inside quotes,
// brackets or an edge label.
func splitStatements(s string) []string {
	var out []string
	depth := 0
	inQuote, inPipe := false, false
	start := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"':
			inQuote = !inQuote
		case inQuote:
		case c == '|':
			inPipe = !inPipe
		case inPipe:
		case c == '[' || c == '(' || c == '{':
			depth++
		case c == ']' || c == ')' || c == '}':
			if depth > 0 {
				depth--
			}
		case c == ';' && depth == 0:
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}

func unquote(s string) string { return label.Unquote(s) }

// cleanLabel turns label markup into plain text.
func cleanLabel(s string) string { return label.Clean(s) }

// ParseStyle reads CSS-like declarations: fill:#f9f,stroke:#333,stroke-width:4px.
func ParseStyle(s string) Style { return parseStyle(s) }

func parseStyle(s string) Style {
	var st Style
	depth := 0
	start := 0
	var parts []string
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
		case ',', ';':
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, s[start:])
	for _, part := range parts {
		k, v, ok := strings.Cut(part, ":")
		if !ok {
			continue
		}
		k = strings.ToLower(strings.TrimSpace(k))
		v = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(v), "!important"))
		switch k {
		case "fill", "background", "background-color":
			st.Fill = v
		case "stroke", "border-color":
			st.Stroke = v
		case "color":
			st.Color = v
		case "stroke-width":
			if f, err := strconv.ParseFloat(strings.TrimSuffix(v, "px"), 64); err == nil {
				st.StrokeWidth = f
			}
		case "stroke-dasharray":
			st.Dash = v
		case "font-weight":
			st.FontWeight = v
		}
	}
	return st
}
