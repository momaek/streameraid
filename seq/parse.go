package seq

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/momaek/streameraid/label"
	"github.com/momaek/streameraid/render/css"
)

// Parse parses the source of a sequence diagram.
//
// final tells whether the source is complete. When it is not, and the
// source does not end with a newline, its last line is still being
// written, and Parse reads it tentatively, so that a diagram streaming in
// does not flicker.
//
//   - A message or a note shows once its colon has come: the names before
//     it are then complete, and its text grows as it is written.
//   - A participant shows once its line is complete, or its "as" has come.
//   - A block (loop, alt, ...) shows once its keyword is followed by a
//     space; its label grows as it is written.
//   - Statements of one word and a name (activate, end, autonumber, ...)
//     apply once their line is complete.
//
// Statements Parse cannot read are recorded in Diagram.Problems and
// skipped.
func Parse(src string, final bool) *Diagram {
	p := &parser{d: &Diagram{index: map[string]int{}, Final: final}, box: -1, counts: map[string]int{},
		creating: map[string]bool{}, destroying: map[string]bool{}}
	src = strings.ReplaceAll(src, "\r\n", "\n")
	complete := final || strings.HasSuffix(src, "\n")
	lines := strings.Split(src, "\n")
	if strings.HasSuffix(src, "\n") {
		lines = lines[:len(lines)-1]
	}
	start, ok := p.frontMatter(lines, complete)
	if !ok {
		return p.d
	}
	for i := start; i < len(lines); i++ {
		p.line = i + 1
		t := strings.TrimSpace(lines[i])
		if t == "" || strings.HasPrefix(t, "%%") {
			continue
		}
		tail := !complete && i == len(lines)-1
		p.text = t
		p.statement(strings.TrimLeft(lines[i], " \t"), tail)
	}
	p.finish()
	return p.d
}

type parser struct {
	d      *Diagram
	line   int
	text   string
	box    int // the open box, or -1
	blocks []string
	counts map[string]int
	// numbering messages
	numbering bool
	next      int
	step      int
	// participants a create or destroy statement named, waiting for the
	// message that does it
	creating, destroying map[string]bool
}

func (p *parser) problem(format string, args ...any) {
	p.d.Problems = append(p.d.Problems, Problem{Line: p.line, Text: p.text, Msg: fmt.Sprintf(format, args...)})
}

func (p *parser) id(prefix string) string {
	n := p.counts[prefix]
	p.counts[prefix] = n + 1
	return prefix + strconv.Itoa(n)
}

// frontMatter skips a YAML front matter block, reading its title. It
// returns the index of the first line after it, and false when the block
// has not ended yet.
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
	for i := first + 1; i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		if t == "---" && (complete || i < len(lines)-1) {
			return i + 1, true
		}
		if k, v, ok := strings.Cut(t, ":"); ok && strings.TrimSpace(k) == "title" {
			p.d.Title = label.Unquote(strings.TrimSpace(v))
		}
	}
	if complete {
		p.line, p.text = first+1, lines[first]
		p.problem("front matter is not closed with ---")
	}
	return 0, false
}

func firstWord(s string) (word, rest string) {
	s = strings.TrimSpace(s)
	i := strings.IndexAny(s, " \t")
	if i < 0 {
		return s, ""
	}
	return s[:i], strings.TrimSpace(s[i+1:])
}

func (p *parser) statement(s string, tail bool) {
	// cut tells that the statement's last word may still grow.
	cut := tail && !strings.HasSuffix(s, " ") && !strings.HasSuffix(s, "\t")
	word, rest := firstWord(s)
	if !p.d.HasHeader {
		switch {
		case strings.TrimSuffix(word, ";") != "sequenceDiagram":
			if !tail {
				p.problem("expected sequenceDiagram, found %q", word)
			}
		case !cut:
			p.d.HasHeader = true
		}
		return
	}
	// A statement of a keyword alone may be a name that starts like it.
	if cut && rest == "" {
		return
	}
	switch strings.ToLower(word) {
	case "participant", "actor":
		p.participant(rest, word == "actor", tail)
		return
	case "create":
		kind, r := firstWord(rest)
		if kind != "participant" && kind != "actor" {
			if !tail {
				p.problem("expected participant or actor after create")
			}
			return
		}
		if tail && r == "" {
			return
		}
		if id := p.participant(r, kind == "actor", tail); id != "" {
			p.creating[id] = true
			if pt, _ := p.d.Participant(id); pt.Created == -1 {
				pt.Created = NotYetCreated
			}
		}
		return
	case "destroy":
		if !tail {
			p.destroying[strings.TrimSpace(rest)] = true
		}
		return
	case "box":
		if !tail {
			p.openBox(rest)
		}
		return
	case "end":
		if rest == "" {
			if !cut {
				p.end()
			}
			return
		}
	case "autonumber":
		if !tail {
			p.autonumber(rest)
		}
		return
	case "activate", "deactivate":
		if tail {
			return
		}
		if rest == "" {
			p.problem("%s needs a participant", word)
			return
		}
		kind := Activate
		if strings.ToLower(word) == "deactivate" {
			kind = Deactivate
		}
		p.touch(rest)
		p.d.Events = append(p.d.Events, &Event{Kind: kind, ID: p.id("a"), Actor: rest})
		return
	case "note":
		if p.note(rest, tail) {
			return
		}
	case "loop", "alt", "opt", "par", "par_over", "critical", "break", "rect":
		p.blockStart(strings.ToLower(word), rest)
		return
	case "else", "and", "option":
		if len(p.blocks) == 0 {
			if !tail {
				p.problem("%s outside a block", word)
			}
			return
		}
		block := p.blocks[len(p.blocks)-1]
		n := p.counts[block+".s"]
		p.counts[block+".s"] = n + 1
		p.d.Events = append(p.d.Events, &Event{Kind: Section, ID: block + ".s" + strconv.Itoa(n), Block: strings.ToLower(word), Text: label.Plain(rest)})
		return
	case "title", "title:":
		if !tail {
			p.d.Title = label.Clean(strings.TrimPrefix(strings.TrimSpace(rest), ":"))
		}
		return
	case "links", "link", "properties", "details", "acctitle", "acctitle:", "accdescr", "accdescr:":
		// Menus and accessibility text draw nothing.
		return
	}
	if strings.HasPrefix(strings.ToLower(s), "title:") {
		if !tail {
			p.d.Title = label.Clean(s[len("title:"):])
		}
		return
	}
	if err := p.message(s, tail); err != "" && !tail {
		p.problem("%s", err)
	}
}

// touch returns the index of participant id, adding it when new.
func (p *parser) touch(id string) int {
	if i, ok := p.d.index[id]; ok {
		return i
	}
	p.d.index[id] = len(p.d.Participants)
	p.d.Participants = append(p.d.Participants, &Participant{ID: id, Label: id, Box: p.box, Created: -1, Destroyed: -1})
	return len(p.d.Participants) - 1
}

// participant reads a participant statement after its keyword, and returns
// the participant's ID, or "" when the statement is not complete enough.
func (p *parser) participant(rest string, actor bool, tail bool) string {
	id, alias, hasAlias := strings.Cut(rest, " as ")
	if tail && !hasAlias {
		return "" // the name may still grow
	}
	id = strings.TrimSpace(id)
	// Alice@{ "type": "database", "alias": "..." }
	if at := strings.Index(id, "@{"); at >= 0 {
		attrs := id[at+2:]
		id = strings.TrimSpace(id[:at])
		if v, ok := jsonValue(attrs, "alias"); ok && !hasAlias {
			alias, hasAlias = v, true
		}
		if v, _ := jsonValue(attrs, "type"); v == "actor" {
			actor = true
		}
	}
	if id == "" {
		if !tail {
			p.problem("participant needs a name")
		}
		return ""
	}
	pt := p.d.Participants[p.touch(id)]
	pt.Actor = pt.Actor || actor
	if p.box >= 0 {
		pt.Box = p.box
	}
	if hasAlias {
		pt.Label = label.Clean(alias)
	}
	return id
}

// jsonValue finds the string value of key in the body of a JSON-like
// object: "type": "database", "alias": "x".
func jsonValue(body, key string) (string, bool) {
	i := strings.Index(body, `"`+key+`"`)
	if i < 0 {
		return "", false
	}
	rest := body[i+len(key)+2:]
	c := strings.IndexByte(rest, ':')
	if c < 0 {
		return "", false
	}
	rest = strings.TrimSpace(rest[c+1:])
	if !strings.HasPrefix(rest, `"`) {
		return "", false
	}
	end := strings.IndexByte(rest[1:], '"')
	if end < 0 {
		return "", false
	}
	return rest[1 : 1+end], true
}

func (p *parser) openBox(rest string) {
	b := &Box{}
	rest = strings.TrimSpace(rest)
	switch {
	case strings.HasPrefix(rest, "rgb") || strings.HasPrefix(rest, "hsl"):
		if end := strings.IndexByte(rest, ')'); end > 0 {
			b.Color, rest = rest[:end+1], strings.TrimSpace(rest[end+1:])
		}
	default:
		if w, r := firstWord(rest); w != "" {
			if _, ok := css.ParseColor(w); ok {
				b.Color, rest = w, r
			}
		}
	}
	if strings.EqualFold(b.Color, "transparent") {
		b.Color = ""
	}
	b.Title = label.Clean(rest)
	p.d.Boxes = append(p.d.Boxes, b)
	p.box = len(p.d.Boxes) - 1
}

func (p *parser) end() {
	switch {
	case p.box >= 0:
		p.box = -1
	case len(p.blocks) > 0:
		id := p.blocks[len(p.blocks)-1]
		p.blocks = p.blocks[:len(p.blocks)-1]
		p.d.Events = append(p.d.Events, &Event{Kind: BlockEnd, ID: id})
	default:
		p.problem("end without a block to close")
	}
}

func (p *parser) autonumber(rest string) {
	fields := strings.Fields(rest)
	if len(fields) == 1 && fields[0] == "off" {
		p.numbering = false
		return
	}
	p.numbering, p.step = true, 1
	if p.next == 0 {
		p.next = 1
	}
	if len(fields) >= 1 {
		if n, err := strconv.Atoi(fields[0]); err == nil {
			p.next = n
		}
	}
	if len(fields) >= 2 {
		if n, err := strconv.Atoi(fields[1]); err == nil {
			p.step = n
		}
	}
}

func (p *parser) blockStart(kind, rest string) {
	id := p.id("b")
	p.blocks = append(p.blocks, id)
	e := &Event{Kind: BlockStart, ID: id, Block: kind}
	if kind == "rect" {
		e.Color = strings.TrimSpace(rest)
	} else {
		e.Text = label.Plain(rest)
	}
	p.d.Events = append(p.d.Events, e)
}

// note reads a note statement after its keyword. It returns false when the
// statement does not read as a note, for the caller to try it as a
// message from a participant named Note.
func (p *parser) note(rest string, tail bool) bool {
	lower := strings.ToLower(rest)
	var place Placement
	switch {
	case strings.HasPrefix(lower, "left of "):
		place, rest = LeftOf, rest[len("left of "):]
	case strings.HasPrefix(lower, "right of "):
		place, rest = RightOf, rest[len("right of "):]
	case strings.HasPrefix(lower, "over "):
		place, rest = Over, rest[len("over "):]
	default:
		// "Note l" may be on its way to "Note left of".
		return tail
	}
	who, text, ok := strings.Cut(rest, ":")
	if !ok {
		if !tail {
			p.problem("note is missing its colon and text")
		}
		return true
	}
	var actors []string
	for _, a := range strings.Split(who, ",") {
		if a = strings.TrimSpace(a); a != "" {
			actors = append(actors, a)
			p.touch(a)
		}
	}
	if len(actors) == 0 || len(actors) > 2 {
		if !tail {
			p.problem("a note is beside one participant, or over one or two")
		}
		return true
	}
	p.d.Events = append(p.d.Events, &Event{Kind: Note, ID: p.id("n"), Placement: place, Actors: actors, Text: label.Plain(text)})
	return true
}

type arrow struct {
	tok    string
	dotted bool
	head   Head
	both   bool
}

// arrows are the arrows of messages, longest first.
var arrows = []arrow{
	{"<<-->>", true, HeadArrow, true},
	{"<<->>", false, HeadArrow, true},
	{"-->>", true, HeadArrow, false},
	{"->>", false, HeadArrow, false},
	{"--x", true, HeadCross, false},
	{"--)", true, HeadOpen, false},
	{"-->", true, HeadNone, false},
	{"-x", false, HeadCross, false},
	{"-)", false, HeadOpen, false},
	{"->", false, HeadNone, false},
}

// message reads a message: Alice->>+Bob: text. It returns what is wrong
// with the statement, or "".
func (p *parser) message(s string, tail bool) string {
	head, text, hasColon := strings.Cut(s, ":")
	if tail && !hasColon {
		return "" // the names may still grow
	}
	at, ar := -1, arrow{}
scan:
	for i := 0; i < len(head); i++ {
		for _, a := range arrows {
			if strings.HasPrefix(head[i:], a.tok) {
				at, ar = i, a
				break scan
			}
		}
	}
	if at < 0 {
		return fmt.Sprintf("unrecognized statement %q", p.text)
	}
	from := strings.TrimSpace(head[:at])
	to := strings.TrimSpace(head[at+len(ar.tok):])
	e := &Event{Kind: Message, Dotted: ar.dotted, Head: ar.head, Both: ar.both}
	switch {
	case strings.HasPrefix(to, "+"):
		e.ActivateTo, to = true, strings.TrimSpace(to[1:])
	case strings.HasPrefix(to, "-"):
		e.DeactivateFrom, to = true, strings.TrimSpace(to[1:])
	}
	// The markers of central connections draw nothing here.
	from = strings.TrimSpace(strings.TrimSuffix(from, "()"))
	to = strings.TrimSpace(strings.TrimPrefix(to, "()"))
	if from == "" || to == "" {
		return "a message needs a sender and a receiver"
	}
	p.touch(from)
	p.touch(to)
	e.From, e.To, e.Text = from, to, label.Plain(text)
	e.ID = p.id("m")
	if p.numbering {
		e.Number = p.next
		p.next += p.step
	}
	idx := len(p.d.Events)
	for _, id := range []string{from, to} {
		pt, _ := p.d.Participant(id)
		if p.creating[id] {
			delete(p.creating, id)
			pt.Created = idx
		}
		if p.destroying[id] {
			delete(p.destroying, id)
			pt.Destroyed = idx
		}
	}
	p.d.Events = append(p.d.Events, e)
	return ""
}

func (p *parser) finish() {
	d := p.d
	d.Open = len(p.blocks)
	if !d.Final {
		return
	}
	if !d.HasHeader && len(d.Problems) == 0 {
		d.Problems = append(d.Problems, Problem{Msg: "missing sequenceDiagram header"})
	}
	if len(p.blocks) > 0 {
		d.Problems = append(d.Problems, Problem{Msg: fmt.Sprintf("%d blocks are missing their end", len(p.blocks))})
	}
}
