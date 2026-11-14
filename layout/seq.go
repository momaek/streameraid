package layout

import "github.com/momaek/streameraid/geom"

// Sequence is the geometry of a sequence diagram, in screen coordinates
// with the origin at the top left of the diagram.
type Sequence struct {
	Width, Height float64
	// Title is the diagram's title, centered on TitleAt, when it has one.
	Title   TextBox
	TitleAt geom.Point
	// Groups are the boxes around participants; the other parts are in the
	// order of the diagram's participants and events.
	Groups       []SeqGroup
	Participants []SeqParticipant
	Blocks       []SeqBlock
	Activations  []SeqActivation
	Messages     []SeqMessage
	Notes        []SeqNote
}

// SeqGroup is a box around participants side by side.
type SeqGroup struct {
	ID      string
	Rect    geom.Rect
	Title   TextBox
	TitleAt geom.Point
	Color   string // a CSS color, or ""
}

// SeqParticipant is a participant's head and lifeline.
type SeqParticipant struct {
	ID   string
	Rect geom.Rect
	// Actor tells that the head is a stick figure, drawn in Figure, with
	// the label under it; otherwise the head is the box Rect.
	Actor   bool
	Figure  geom.Path
	Label   TextBox
	LabelAt geom.Point
	// LifeTop and LifeBottom are the ends of the lifeline. Destroyed tells
	// that it ends with a cross.
	LifeTop, LifeBottom geom.Point
	Destroyed           bool
}

// SeqHead is the head of a message's arrow.
type SeqHead struct {
	Shape  geom.Path
	Filled bool
}

// SeqMessage is a message's arrow and text.
type SeqMessage struct {
	ID string
	// Path is the message's line, pulled back from its heads.
	Path   geom.Path
	Dotted bool
	Heads  []SeqHead
	// Label is the message's text, centered on LabelAt.
	Label   TextBox
	LabelAt geom.Point
	// Number is the message's number, shown in a badge of radius
	// NumberRadius centered on NumberAt, or 0.
	Number       int
	NumberAt     geom.Point
	NumberRadius float64
}

// SeqNote is a note.
type SeqNote struct {
	ID      string
	Rect    geom.Rect
	Label   TextBox
	LabelAt geom.Point
}

// SeqActivation is the bar on a lifeline while its participant is active.
type SeqActivation struct {
	ID   string
	Rect geom.Rect
}

// SeqBlock is the frame of a loop, alt, opt, par, critical or break block,
// or the background of a rect.
type SeqBlock struct {
	ID   string
	Rect geom.Rect
	// Kind is the block's keyword. A rect has no frame: only Fill.
	Kind string
	Fill string // a rect's CSS color
	// Tab is the corner holding the keyword, KindLabel, centered on KindAt.
	Tab       geom.Rect
	KindLabel TextBox
	KindAt    geom.Point
	// Title is the block's label, centered on TitleAt.
	Title    TextBox
	TitleAt  geom.Point
	Sections []SeqSection
}

// SeqSection is a section of a block after its first: a dashed line
// across the block at Y, with a title under it.
type SeqSection struct {
	ID      string
	Y       float64
	Title   TextBox
	TitleAt geom.Point
}
