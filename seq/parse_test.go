package seq

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func mustParse(t *testing.T, src string) *Diagram {
	t.Helper()
	d := Parse(src, true)
	if len(d.Problems) > 0 {
		t.Fatalf("problems: %+v", d.Problems)
	}
	return d
}

func names(d *Diagram) []string {
	var out []string
	for _, p := range d.Participants {
		out = append(out, p.ID)
	}
	return out
}

func messages(d *Diagram) []string {
	var out []string
	for _, e := range d.Events {
		if e.Kind == Message {
			out = append(out, e.From+">"+e.To+":"+e.Text)
		}
	}
	return out
}

func TestParticipants(t *testing.T) {
	d := mustParse(t, `sequenceDiagram
    participant A as Alice
    actor B as Bob<br/>the builder
    participant DB@{ "type": "database", "alias": "User DB" }
    A->>Carol: hi
    B->>A: hello
`)
	if got := names(d); !reflect.DeepEqual(got, []string{"A", "B", "DB", "Carol"}) {
		t.Fatalf("participants %v", got)
	}
	if p := d.Participants[0]; p.Label != "Alice" || p.Actor {
		t.Errorf("A: %+v", p)
	}
	if p := d.Participants[1]; p.Label != "Bob\nthe builder" || !p.Actor {
		t.Errorf("B: %+v", p)
	}
	if p := d.Participants[2]; p.Label != "User DB" {
		t.Errorf("DB: %+v", p)
	}
	if p := d.Participants[3]; p.Label != "Carol" {
		t.Errorf("Carol: %+v", p)
	}
}

func TestArrows(t *testing.T) {
	d := mustParse(t, `sequenceDiagram
    A->B: a
    A-->B: b
    A->>B: c
    A-->>B: d
    A-xB: e
    A--xB: f
    A-)B: g
    A--)B: h
    A<<->>B: i
    A<<-->>B: j
    A->>+B: k
    B-->>-A: l
    web-server->>db: with: colons
`)
	type w struct {
		dotted bool
		head   Head
		both   bool
	}
	want := []w{{false, HeadNone, false}, {true, HeadNone, false}, {false, HeadArrow, false}, {true, HeadArrow, false},
		{false, HeadCross, false}, {true, HeadCross, false}, {false, HeadOpen, false}, {true, HeadOpen, false},
		{false, HeadArrow, true}, {true, HeadArrow, true}, {false, HeadArrow, false}, {true, HeadArrow, false}, {false, HeadArrow, false}}
	if len(d.Events) != len(want) {
		t.Fatalf("%d events: %v", len(d.Events), messages(d))
	}
	for i, x := range want {
		e := d.Events[i]
		if got := (w{e.Dotted, e.Head, e.Both}); got != x {
			t.Errorf("message %d (%s): %+v, want %+v", i, e.Text, got, x)
		}
	}
	if !d.Events[10].ActivateTo || !d.Events[11].DeactivateFrom {
		t.Errorf("activation marks: %+v %+v", d.Events[10], d.Events[11])
	}
	if e := d.Events[12]; e.From != "web-server" || e.To != "db" || e.Text != "with: colons" {
		t.Errorf("last message %+v", e)
	}
}

func TestBlocksNotesAndNumbers(t *testing.T) {
	d := mustParse(t, `sequenceDiagram
    autonumber 10 5
    A->>B: one
    loop Every minute
        B->>A: two
        alt ok
            A->>B: three
        else failed
            Note over A,B: retry later
        end
    end
    rect rgb(200, 220, 255)
        Note right of B: thinking
    end
    activate B
    deactivate B
`)
	var kinds []Kind
	for _, e := range d.Events {
		kinds = append(kinds, e.Kind)
	}
	want := []Kind{Message, BlockStart, Message, BlockStart, Message, Section, Note, BlockEnd, BlockEnd, BlockStart, Note, BlockEnd, Activate, Deactivate}
	if !reflect.DeepEqual(kinds, want) {
		t.Fatalf("kinds %v, want %v", kinds, want)
	}
	if d.Events[0].Number != 10 || d.Events[2].Number != 15 || d.Events[4].Number != 20 {
		t.Errorf("numbers %d %d %d", d.Events[0].Number, d.Events[2].Number, d.Events[4].Number)
	}
	if e := d.Events[1]; e.Block != "loop" || e.Text != "Every minute" || e.ID != "b0" {
		t.Errorf("loop %+v", e)
	}
	if e := d.Events[5]; e.Block != "else" || e.Text != "failed" || e.ID != "b1.s0" {
		t.Errorf("else %+v", e)
	}
	if e := d.Events[6]; e.Placement != Over || !reflect.DeepEqual(e.Actors, []string{"A", "B"}) || e.Text != "retry later" {
		t.Errorf("note %+v", e)
	}
	if e := d.Events[9]; e.Block != "rect" || e.Color != "rgb(200, 220, 255)" {
		t.Errorf("rect %+v", e)
	}
	if e := d.Events[10]; e.Placement != RightOf {
		t.Errorf("note %+v", e)
	}
}

func TestBoxesCreateDestroy(t *testing.T) {
	d := mustParse(t, `sequenceDiagram
    box Purple Alice & John
    participant A
    participant J
    end
    box Another Group
    participant B
    end
    A->>J: hi
    create participant C
    A->>C: welcome
    destroy C
    C->>A: bye
`)
	if len(d.Boxes) != 2 || d.Boxes[0].Color != "Purple" || d.Boxes[0].Title != "Alice & John" || d.Boxes[1].Title != "Another Group" {
		t.Fatalf("boxes %+v %+v", d.Boxes[0], d.Boxes[1])
	}
	if d.Participants[0].Box != 0 || d.Participants[2].Box != 1 {
		t.Errorf("box membership %d %d", d.Participants[0].Box, d.Participants[2].Box)
	}
	c, _ := d.Participant("C")
	if c.Created != 1 || c.Destroyed != 2 || c.Box != -1 {
		t.Errorf("C: %+v", c)
	}
}

func TestProblems(t *testing.T) {
	d := Parse("sequenceDiagram\n    A->>B: ok\n    this is not a statement\n    loop forever\n", true)
	if len(d.Problems) != 2 || d.Problems[0].Line != 3 {
		t.Fatalf("problems %+v", d.Problems)
	}
	if got := messages(d); len(got) != 1 {
		t.Errorf("messages %v", got)
	}
	if d := Parse("flowchart TD\nA-->B", true); d.HasHeader || len(d.Problems) == 0 {
		t.Errorf("not a sequence diagram: %+v", d)
	}
}

func TestTentativeTail(t *testing.T) {
	head := "sequenceDiagram\n"
	cases := []struct {
		tail         string
		participants []string
		events       int
	}{
		{"participant Al", nil, 0},
		{"participant A as Al", []string{"A"}, 0},
		{"Alice->>Bo", nil, 0},
		{"Alice->>Bob", nil, 0},
		{"Alice->>Bob: He", []string{"Alice", "Bob"}, 1},
		{"Note over Ali", nil, 0},
		{"Note over Alice: hm", []string{"Alice"}, 1},
		{"loop", nil, 0},
		{"loop ", nil, 1},
		{"loop Every", nil, 1},
		{"activate Ali", nil, 0},
		{"autonumber", nil, 0},
	}
	for _, c := range cases {
		d := Parse(head+c.tail, false)
		if got := names(d); !reflect.DeepEqual(got, c.participants) {
			t.Errorf("%q: participants %v, want %v", c.tail, got, c.participants)
		}
		if len(d.Events) != c.events {
			t.Errorf("%q: %d events, want %d", c.tail, len(d.Events), c.events)
		}
	}
	if d := Parse("sequenceDiagra", false); d.HasHeader {
		t.Errorf("a header still being written was accepted")
	}
}

func corpus(t testing.TB) map[string]string {
	t.Helper()
	files, _ := filepath.Glob("../testdata/corpus/*/*.mmd")
	out := map[string]string{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if d := Parse(string(b), true); d.HasHeader {
			out[f] = string(b)
		}
	}
	if len(out) == 0 {
		t.Fatal("no sequence diagrams in testdata/corpus")
	}
	return out
}

// TestCorpus checks that every sample parses without problems.
func TestCorpus(t *testing.T) {
	for f, src := range corpus(t) {
		if d := Parse(src, true); len(d.Problems) > 0 {
			t.Errorf("%s: %+v", filepath.Base(f), d.Problems)
		}
	}
}

// TestCorpusStreaming streams every sample a character at a time and
// checks that nothing appears, goes and comes back, and that the last step
// matches the whole source.
func TestCorpusStreaming(t *testing.T) {
	for f, src := range corpus(t) {
		seen := map[string]string{}
		var prev *Diagram
		for end := 0; end <= len(src); end++ {
			if end < len(src) && !utf8.RuneStart(src[end]) {
				continue
			}
			d := Parse(src[:end], false)
			diff := Compare(prev, d)
			for _, id := range diff.Added {
				if seen[id] == "gone" {
					tail := src[:end]
					if i := strings.LastIndexByte(tail, '\n'); i >= 0 {
						tail = tail[i+1:]
					}
					t.Errorf("%s: at %q, %s came back", filepath.Base(f), tail, id)
				}
				seen[id] = "present"
			}
			for _, id := range diff.Removed {
				seen[id] = "gone"
			}
			prev = d
		}
		whole := Parse(src, true)
		if !reflect.DeepEqual(names(prev), names(whole)) || !reflect.DeepEqual(messages(prev), messages(whole)) || len(prev.Events) != len(whole.Events) {
			t.Errorf("%s: the streamed diagram differs from the whole one", filepath.Base(f))
		}
	}
}

func FuzzParse(f *testing.F) {
	f.Add("sequenceDiagram\n    A->>+B: hi\n    Note over A,B: x\n    loop y\n    B-->>-A: z\n    end\n")
	f.Fuzz(func(t *testing.T, src string) {
		Parse(src, false)
		Parse(src, true)
	})
}
