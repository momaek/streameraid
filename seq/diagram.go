// Package seq is the model of a Mermaid sequence diagram and its parser.
//
// As with flowcharts, Parse reads the whole source each time: a caller
// streaming a diagram in passes the source so far, and the IDs of the
// diagram's parts stay the same as it grows.
package seq

// Participant is a participant of the diagram: a lifeline with a head.
type Participant struct {
	ID    string
	Label string // "\n" separates lines
	// Actor tells that the participant shows as a stick figure.
	Actor bool
	// Box is the index of the box grouping the participant, or -1.
	Box int
	// Created is the index of the event (a message) that creates the
	// participant, whose head then shows there rather than at the top; -1
	// for a participant there from the start; and NotYetCreated for one a
	// create statement announced, whose message has not come. Destroyed is
	// the index of the event after which its lifeline ends, or -1.
	Created, Destroyed int
}

// NotYetCreated is the Created of a participant announced by a create
// statement that no message has created yet: it does not show.
const NotYetCreated = -2

// Box groups participants side by side under a title.
type Box struct {
	Title string
	Color string // a CSS color, or ""
}

// Head is what ends a message's line.
type Head uint8

const (
	HeadNone  Head = iota // ->   a line
	HeadArrow             // ->>  a filled arrowhead
	HeadCross             // -x   a cross
	HeadOpen              // -)   an open arrowhead, for asynchronous messages
)

// Placement is where a note goes.
type Placement uint8

const (
	Over Placement = iota
	LeftOf
	RightOf
)

// Kind is the kind of an event.
type Kind uint8

const (
	Message    Kind = iota
	Note            // a note beside or over participants
	Activate        // an activation starts on Actor
	Deactivate      // the last activation of Actor ends
	BlockStart      // loop, alt, opt, par, critical, break, rect
	Section         // else, and, option: a new section of the block
	BlockEnd
)

// Event is one step of the diagram, in the order of the source.
type Event struct {
	Kind Kind
	// ID is stable while the source grows: the kind and the count of
	// earlier events of that kind (m3, n0, b1, b1.s0).
	ID string

	// Message.
	From, To string
	Dotted   bool
	Head     Head
	// Both tells that the message has a head at each end (<<->>).
	Both bool
	// Number is the message's number when the diagram numbers them, or 0.
	Number int
	// ActivateTo starts an activation on To (->>+), DeactivateFrom ends one
	// on From (->>-).
	ActivateTo, DeactivateFrom bool

	// Note: its placement and the one or two participants it refers to.
	Placement Placement
	Actors    []string

	// Activate, Deactivate.
	Actor string

	// BlockStart: its keyword (loop, alt, ...), and for rect its color.
	// Section: its keyword (else, and, option).
	Block string
	Color string

	// Text is the text of a message or note, or the label of a block or
	// section. "\n" separates lines.
	Text string
}

// Problem is a statement the parser could not read, or something missing
// at the end of the source.
type Problem struct {
	Line int // from 1
	Text string
	Msg  string
}

// Diagram is a parsed sequence diagram.
type Diagram struct {
	Title string
	// HasHeader tells whether the source's header (sequenceDiagram) was
	// complete. A diagram without one is empty.
	HasHeader    bool
	Participants []*Participant
	Boxes        []*Box
	Events       []*Event
	Problems     []Problem
	// Open counts the blocks whose end has not come.
	Open int
	// Final tells whether the source was complete when parsed.
	Final bool

	index map[string]int
}

// Participant returns the participant id and its index, or nil and -1.
func (d *Diagram) Participant(id string) (*Participant, int) {
	if i, ok := d.index[id]; ok {
		return d.Participants[i], i
	}
	return nil, -1
}
