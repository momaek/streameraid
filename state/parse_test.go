package state

import (
	"reflect"
	"strings"
	"testing"

	"github.com/momaek/streameraid/flow"
)

func mustParse(t *testing.T, src string) *flow.Graph {
	t.Helper()
	g := Parse(src, true)
	if len(g.Problems) > 0 {
		t.Fatalf("problems: %+v", g.Problems)
	}
	return g
}

func edgeList(g *flow.Graph) []string {
	var out []string
	for _, e := range g.Edges {
		s := e.From + ">" + e.To
		if e.Label != "" {
			s += "|" + e.Label
		}
		out = append(out, s)
	}
	return out
}

func TestHeader(t *testing.T) {
	for _, h := range []string{"stateDiagram", "stateDiagram-v2"} {
		g := mustParse(t, h+"\nA --> B\n")
		if !g.HasHeader || g.Dir != flow.TB || len(g.Nodes) != 2 {
			t.Errorf("%s: header %v dir %v nodes %d", h, g.HasHeader, g.Dir, len(g.Nodes))
		}
	}
	if g := Parse("stateDiagram", false); g.HasHeader {
		t.Error("a header still being written counts")
	}
	if g := Parse("graph TD\nA-->B\n", true); len(g.Problems) == 0 || g.HasHeader {
		t.Errorf("a flowchart header: %+v", g.Problems)
	}
	if g := Parse("", true); len(g.Problems) != 1 {
		t.Errorf("an empty source: %+v", g.Problems)
	}
	g := mustParse(t, "---\ntitle: Doors\n---\nstateDiagram-v2\n%%{init: {}}%%\ndirection LR\nA --> B\n")
	if g.Title != "Doors" || g.Dir != flow.LR {
		t.Errorf("title %q dir %v", g.Title, g.Dir)
	}
}

func TestStatesAndTransitions(t *testing.T) {
	g := mustParse(t, `stateDiagram-v2
    [*] --> Still
    Still --> [*]
    Still --> Moving : push
    Moving-->Crash:fall over
    Crash --> [*]
    Lone
`)
	want := []string{"[*]start>Still", "Still>[*]end", "Still>Moving|push", "Moving>Crash|fall over", "Crash>[*]end"}
	if got := edgeList(g); !reflect.DeepEqual(got, want) {
		t.Fatalf("edges %v", got)
	}
	if got := g.NodeOrder; !reflect.DeepEqual(got, []string{"[*]start", "Still", "[*]end", "Moving", "Crash", "Lone"}) {
		t.Fatalf("nodes %v", got)
	}
	if n := g.Nodes["[*]start"]; n.Shape != flow.Start || n.Label != "" {
		t.Errorf("start %+v", n)
	}
	if n := g.Nodes["[*]end"]; n.Shape != flow.End || n.Label != "" {
		t.Errorf("end %+v", n)
	}
	if n := g.Nodes["Still"]; n.Shape != flow.Round || n.Label != "Still" {
		t.Errorf("Still %+v", n)
	}
	e := g.Edges[2]
	if e.ID != "Still->Moving#0" || e.Line != flow.Solid || e.End != flow.ArrowNormal || e.Start != flow.ArrowNone || e.Length != 1 || e.Index != 2 {
		t.Errorf("edge %+v", e)
	}
}

func TestDescriptions(t *testing.T) {
	g := mustParse(t, `stateDiagram-v2
    state "Long name" as L
    S1 : only one
    S2 : first
    S2 : second<br/>line
    state "Waiting" as W
    W : timeout 30m
    状态 : 中文描述
`)
	if n := g.Nodes["L"]; n.Shape != flow.Round || n.Label != "Long name" || n.Table != nil {
		t.Errorf("L %+v", n)
	}
	if n := g.Nodes["S1"]; n.Shape != flow.Round || n.Label != "only one" {
		t.Errorf("S1 %+v", n)
	}
	n := g.Nodes["S2"]
	want := &flow.Table{Sections: []flow.Section{
		{Rows: [][]string{{"S2"}}, Center: true, Bold: true},
		{Rows: [][]string{{"first"}, {"second\nline"}}},
	}}
	if n.Shape != flow.Sectioned || n.Label != "S2" || !reflect.DeepEqual(n.Table, want) {
		t.Errorf("S2 %+v %+v", n, n.Table)
	}
	n = g.Nodes["W"]
	if n.Shape != flow.Sectioned || n.Label != "Waiting" || n.Table.Sections[0].Rows[0][0] != "Waiting" ||
		!reflect.DeepEqual(n.Table.Sections[1].Rows, [][]string{{"timeout 30m"}}) {
		t.Errorf("W %+v %+v", n, n.Table)
	}
	if n := g.Nodes["状态"]; n == nil || n.Label != "中文描述" {
		t.Errorf("状态 %+v", n)
	}
}

func TestChoiceForkJoin(t *testing.T) {
	g := mustParse(t, `stateDiagram-v2
    state c <<choice>>
    state f <<fork>>
    state j [[join]]
    state c2 [[choice]]
    [*] --> c
    c --> f
    f --> j
    j --> c2
`)
	for id, shape := range map[string]flow.Shape{"c": flow.Choice, "f": flow.Bar, "j": flow.Bar, "c2": flow.Choice} {
		if n := g.Nodes[id]; n.Shape != shape || n.Label != "" {
			t.Errorf("%s: %+v", id, n)
		}
	}
	if g := Parse("stateDiagram-v2\nstate x <<maybe>>\n", true); len(g.Problems) != 1 {
		t.Errorf("unknown type: %+v", g.Problems)
	}
}

func TestComposite(t *testing.T) {
	g := mustParse(t, `stateDiagram-v2
    [*] --> Outer
    state "The outer one" as Outer {
        direction LR
        [*] --> a
        a --> Inner
        state Inner {
            [*] --> b
            b --> [*]
        }
        Inner --> [*]
    }
    Outer --> Done
`)
	if g.Nodes["Outer"] != nil || g.Nodes["Inner"] != nil {
		t.Fatal("a composite state has a node")
	}
	o, in := g.Subgraphs["Outer"], g.Subgraphs["Inner"]
	if o == nil || in == nil || o.Title != "The outer one" || in.Title != "Inner" || in.Parent != "Outer" ||
		!o.HasDir || o.Dir != flow.LR || o.Open || in.Open {
		t.Fatalf("subgraphs %+v %+v", o, in)
	}
	if !reflect.DeepEqual(o.Nodes, []string{"Outer/[*]start", "a", "Outer/[*]end"}) || !reflect.DeepEqual(o.Children, []string{"Inner"}) {
		t.Errorf("Outer holds %v %v", o.Nodes, o.Children)
	}
	if !reflect.DeepEqual(in.Nodes, []string{"Inner/[*]start", "b", "Inner/[*]end"}) {
		t.Errorf("Inner holds %v", in.Nodes)
	}
	want := []string{"[*]start>Outer", "Outer/[*]start>a", "a>Inner", "Inner/[*]start>b", "b>Inner/[*]end", "Inner>Outer/[*]end", "Outer>Done"}
	if got := edgeList(g); !reflect.DeepEqual(got, want) {
		t.Errorf("edges %v", got)
	}
	if n := g.Nodes["Done"]; n.Subgraph != "" {
		t.Errorf("Done is in %q", n.Subgraph)
	}

	g = Parse("stateDiagram-v2\nstate X {\nA --> B\n", true)
	if len(g.Problems) != 1 || !strings.Contains(g.Problems[0].Msg, "}") {
		t.Errorf("unclosed: %+v", g.Problems)
	}
	if g := Parse("stateDiagram-v2\n}\n", true); len(g.Problems) != 1 {
		t.Errorf("stray }: %+v", g.Problems)
	}
}

func TestConcurrency(t *testing.T) {
	g := mustParse(t, `stateDiagram-v2
    state Active {
        [*] --> A1
        state Sub {
            x --> y
        }
        --
        [*] --> B1
        --
        C1 --> [*]
    }
`)
	r1, r2, r3 := g.Subgraphs["Active/region-1"], g.Subgraphs["Active/region-2"], g.Subgraphs["Active/region-3"]
	if r1 == nil || r2 == nil || r3 == nil {
		t.Fatalf("regions %v", g.SubgraphOrder)
	}
	for _, r := range []*flow.Subgraph{r1, r2, r3} {
		if r.Title != "" || r.Style.Dash == "" || r.Parent != "Active" || r.Open {
			t.Errorf("region %+v", r)
		}
	}
	if !reflect.DeepEqual(g.SubgraphOrder, []string{"Active", "Active/region-1", "Sub", "Active/region-2", "Active/region-3"}) {
		t.Errorf("order %v", g.SubgraphOrder)
	}
	if !reflect.DeepEqual(r1.Nodes, []string{"Active/[*]start", "A1"}) || !reflect.DeepEqual(r1.Children, []string{"Sub"}) {
		t.Errorf("region 1 holds %v %v", r1.Nodes, r1.Children)
	}
	if !reflect.DeepEqual(r2.Nodes, []string{"Active/region-2/[*]start", "B1"}) {
		t.Errorf("region 2 holds %v", r2.Nodes)
	}
	if !reflect.DeepEqual(r3.Nodes, []string{"C1", "Active/region-3/[*]end"}) {
		t.Errorf("region 3 holds %v", r3.Nodes)
	}
	if a := g.Subgraphs["Active"]; len(a.Nodes) != 0 || !reflect.DeepEqual(a.Children, []string{"Active/region-1", "Active/region-2", "Active/region-3"}) {
		t.Errorf("Active holds %v %v", a.Nodes, a.Children)
	}
	if g := Parse("stateDiagram-v2\nA\n--\n", true); len(g.Problems) != 1 {
		t.Errorf("-- at the top: %+v", g.Problems)
	}
}

func TestNotes(t *testing.T) {
	g := mustParse(t, `stateDiagram-v2
    A --> B
    note right of A : on the right
    note left of B
        line one
        line <b>two</b>
    end note
    note "floating" as N1
`)
	n1, n2 := g.Nodes["note-1"], g.Nodes["note-2"]
	if n1 == nil || n1.Shape != flow.Note || n1.Label != "on the right" || n1.Partial {
		t.Fatalf("note-1 %+v", n1)
	}
	if n2 == nil || n2.Label != "line one\nline two" || n2.Partial {
		t.Fatalf("note-2 %+v", n2)
	}
	if n := g.Nodes["N1"]; n == nil || n.Shape != flow.Note || n.Label != "floating" {
		t.Errorf("N1 %+v", n)
	}
	want := []string{"A>B", "A>note-1", "note-2>B"}
	if got := edgeList(g); !reflect.DeepEqual(got, want) {
		t.Fatalf("edges %v", got)
	}
	if e := g.Edges[1]; e.Line != flow.Dotted || e.Start != flow.ArrowNone || e.End != flow.ArrowNone {
		t.Errorf("note edge %+v", e)
	}
	// An open note block shows what it has so far.
	g = Parse("stateDiagram-v2\nnote right of A\n  first\n  sec", false)
	if n := g.Nodes["note-1"]; n == nil || n.Label != "first\nsec" || !n.Partial {
		t.Errorf("open note %+v", n)
	}
	g = Parse("stateDiagram-v2\nnote right of A\n  first\n  end no", false)
	if n := g.Nodes["note-1"]; n.Label != "first" {
		t.Errorf("end note being written shows: %q", n.Label)
	}
	g = Parse("stateDiagram-v2\nnote right of A\n  first\n", true)
	if len(g.Problems) != 1 || g.Nodes["note-1"].Partial {
		t.Errorf("unclosed note: %+v", g.Problems)
	}
}

func TestStyling(t *testing.T) {
	g := mustParse(t, `stateDiagram-v2
    classDef hot fill:#f00,color:white
    classDef cold,frozen fill:#00f
    [*] --> A:::hot
    A:::cold --> B
    class B, C hot
    style C stroke:#333,stroke-width:2px
    state Box {
        x
    }
    class Box frozen
    style Box fill:#eee
`)
	if g.ClassDefs["hot"].Fill != "#f00" || g.ClassDefs["frozen"].Fill != "#00f" {
		t.Errorf("classDefs %+v", g.ClassDefs)
	}
	if got := g.Nodes["A"].Classes; !reflect.DeepEqual(got, []string{"hot", "cold"}) {
		t.Errorf("A classes %v", got)
	}
	if got := g.Nodes["B"].Classes; !reflect.DeepEqual(got, []string{"hot"}) {
		t.Errorf("B classes %v", got)
	}
	if g.Nodes["C"] != nil {
		t.Error("class made a state")
	}
	if sg := g.Subgraphs["Box"]; !reflect.DeepEqual(sg.Classes, []string{"frozen"}) || sg.Style.Fill != "#eee" {
		t.Errorf("Box %+v", sg)
	}
	g = mustParse(t, "stateDiagram-v2\nC\nstyle C stroke:#333,stroke-width:2px\n")
	if st := g.Nodes["C"].Style; st.Stroke != "#333" || st.StrokeWidth != 2 {
		t.Errorf("C style %+v", st)
	}
}

func TestIgnored(t *testing.T) {
	mustParse(t, `stateDiagram-v2
    hide empty description
    scale 350 width
    accTitle: A title
    accDescr: A description
    accDescr {
        over
        lines
    }
    click A "https://example.com"
    A --> B %% a comment
    %% a comment line
`)
}

func TestProblems(t *testing.T) {
	for _, src := range []string{
		"stateDiagram-v2\nA -> B\n",
		"stateDiagram-v2\nA -->\n",
		"stateDiagram-v2\n[*]\n",
		"stateDiagram-v2\nnote over A : x\n",
		"stateDiagram-v2\nstate \"open\n",
		"stateDiagram-v2\nA --> B C\n",
	} {
		if g := Parse(src, true); len(g.Problems) == 0 {
			t.Errorf("%q: no problems", src)
		}
	}
}

// TestStreamingTail checks how a line still being written reads.
func TestStreamingTail(t *testing.T) {
	const h = "stateDiagram-v2\n"
	cases := []struct {
		src   string
		nodes []string
		edges []string
	}{
		{h + "A", nil, nil},
		{h + "A ", []string{"A"}, nil},
		{h + "A --> B", []string{"A"}, nil},
		{h + "A --> B ", []string{"A", "B"}, []string{"A>B"}},
		{h + "A --> B:", []string{"A", "B"}, []string{"A>B"}},
		{h + "A --> B::", []string{"A", "B"}, []string{"A>B"}},
		{h + "A --> B : la", []string{"A", "B"}, []string{"A>B|la"}},
		{h + "[*", nil, nil},
		{h + "[*] --> [*", []string{"[*]start"}, nil},
		{h + "[*] --> [*]", []string{"[*]start", "[*]end"}, []string{"[*]start>[*]end"}},
		{h + "state X", nil, nil},
		{h + "state X ", nil, nil},
		{h + "state X <<cho", nil, nil},
		{h + "state X <<choice>>", []string{"X"}, nil},
		{h + "state \"Long", nil, nil},
		{h + "state \"Long\" as X", nil, nil},
		{h + "note right of A", nil, nil},
		{h + "note right of A ", nil, nil},
		{h + "note right of A : hel", []string{"A", "note-1"}, []string{"A>note-1"}},
		{h + "订单", nil, nil},
		{h + "订\xe5\x8d", nil, nil},
		{h + "class", nil, nil},
	}
	for _, c := range cases {
		g := Parse(c.src, false)
		if len(g.Problems) > 0 {
			t.Errorf("%q: problems %+v", c.src, g.Problems)
		}
		if !reflect.DeepEqual(g.NodeOrder, c.nodes) && !(len(g.NodeOrder) == 0 && len(c.nodes) == 0) {
			t.Errorf("%q: nodes %v, want %v", c.src, g.NodeOrder, c.nodes)
		}
		if got := edgeList(g); !reflect.DeepEqual(got, c.edges) {
			t.Errorf("%q: edges %v, want %v", c.src, got, c.edges)
		}
	}
	// A class name being written does not apply yet.
	g := Parse(h+"classDef hot fill:red\nA --> B:::ho", false)
	if n := g.Nodes["B"]; n == nil || len(n.Classes) != 0 {
		t.Errorf("B %+v", n)
	}
	// A composite opens at its brace.
	g = Parse(h+"state X {", false)
	if sg := g.Subgraphs["X"]; sg == nil || !sg.Open {
		t.Errorf("X %+v", sg)
	}
	// A description being written shows, marked partial.
	g = Parse(h+"A : wai", false)
	if n := g.Nodes["A"]; n.Label != "wai" || !n.Partial {
		t.Errorf("A %+v", n)
	}
	// A comment ends the name before it.
	g = Parse(h+"A --> B %%", false)
	if got := edgeList(g); !reflect.DeepEqual(got, []string{"A>B"}) {
		t.Errorf("edges before a comment %v", got)
	}
}

// TestStreamedPrefixesNeverPanic parses every prefix of a source that uses
// all the syntax.
func TestStreamedPrefixesNeverPanic(t *testing.T) {
	for i := range len(fuzzSample) + 1 {
		Parse(fuzzSample[:i], false)
		Parse(fuzzSample[:i], true)
	}
}

const fuzzSample = `---
title: all
---
stateDiagram-v2
    direction LR
    classDef hot fill:#f00
    state "Long" as L {
        [*] --> a:::hot
        --
        b --> [*] : done
    }
    state c <<choice>>
    state f [[fork]]
    L --> c
    c --> f : yes
    note right of f
        text
    end note
    note left of c : left
    a : desc
    class a hot
    style a fill:#fff
    hide empty description
`

func FuzzParse(f *testing.F) {
	f.Add(fuzzSample)
	f.Add("stateDiagram\n[*]-->A:::x:y\nstate \"a\" as B {\n--\n}")
	f.Fuzz(func(t *testing.T, src string) {
		Parse(src, false)
		Parse(src, true)
	})
}
