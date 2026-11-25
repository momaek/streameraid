package mindmap

import (
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/momaek/streameraid/flow"
)

// outline returns the nodes of g as "id shape label" lines, indented by depth.
func outline(g *flow.Graph) string {
	var b strings.Builder
	for _, id := range g.NodeOrder {
		n := g.Nodes[id]
		b.WriteString(strings.Repeat("  ", strings.Count(id, ".")))
		b.WriteString(id + " " + n.Shape.String() + " " + strings.ReplaceAll(n.Label, "\n", "|"))
		if n.Partial {
			b.WriteString(" …")
		}
		b.WriteString("\n")
	}
	return b.String()
}

func TestTree(t *testing.T) {
	g := Parse("mindmap\n  root((Topic))\n    A\n      A1\n      A2\n    B\n      B1\n    C\n", true)
	want := `n0 circle Topic
  n0.0 round A
    n0.0.0 round A1
    n0.0.1 round A2
  n0.1 round B
    n0.1.0 round B1
  n0.2 round C
`
	if got := outline(g); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	if !g.HasHeader || len(g.Problems) > 0 {
		t.Errorf("header %v, problems %+v", g.HasHeader, g.Problems)
	}
	var edges []string
	for i, e := range g.Edges {
		if e.Index != i || e.Line != flow.Solid || e.Start != flow.ArrowNone || e.End != flow.ArrowNone || e.Length != 1 {
			t.Errorf("edge %+v", e)
		}
		edges = append(edges, e.ID)
	}
	wantEdges := []string{"n0->n0.0#0", "n0.0->n0.0.0#0", "n0.0->n0.0.1#0", "n0->n0.1#0", "n0.1->n0.1.0#0", "n0->n0.2#0"}
	if !slices.Equal(edges, wantEdges) {
		t.Errorf("edges %v, want %v", edges, wantEdges)
	}
}

func TestShapes(t *testing.T) {
	src := `mindmap
  root
    a[Square]
    b(Rounded)
    c((Circle))
    d))Bang((
    e)Cloud(
    f{{Hexagon}}
    Plain text
    [No id]
    g["Quoted (with brackets)"]
    h["` + "`**Bold** and *italic*`" + `"]
    i[Line one<br/>line two]
    Python (Django)
`
	want := `n0 round root
  n0.0 rect Square
  n0.1 round Rounded
  n0.2 circle Circle
  n0.3 bang Bang
  n0.4 cloud Cloud
  n0.5 hexagon Hexagon
  n0.6 round Plain text
  n0.7 rect No id
  n0.8 rect Quoted (with brackets)
  n0.9 rect Bold and italic
  n0.10 rect Line one|line two
  n0.11 round Django
`
	g := Parse(src, true)
	if got := outline(g); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	if len(g.Problems) > 0 {
		t.Errorf("problems %+v", g.Problems)
	}
}

func TestIndentation(t *testing.T) {
	// C is indented less than B but more than A: its parent is A, the
	// nearest line before it indented less.
	g := Parse("mindmap\nRoot\n    A\n        B\n      C\n   D\n", true)
	want := `n0 round Root
  n0.0 round A
    n0.0.0 round B
    n0.0.1 round C
  n0.1 round D
`
	if got := outline(g); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	// A tab counts as four spaces.
	g = Parse("mindmap\nRoot\n\tA\n    B\n\t\tC\n", true)
	want = `n0 round Root
  n0.0 round A
  n0.1 round B
    n0.1.0 round C
`
	if got := outline(g); got != want {
		t.Errorf("tabs: got\n%s\nwant\n%s", got, want)
	}
}

func TestSecondRoot(t *testing.T) {
	g := Parse("mindmap\n  Root\n    A\n  Other\n    B\n    C\n  \n    D\n", true)
	if len(g.Problems) != 1 || g.Problems[0].Line != 4 {
		t.Errorf("problems %+v", g.Problems)
	}
	// The children of the line with no parent go with it.
	if got := outline(g); got != "n0 round Root\n  n0.0 round A\n" {
		t.Errorf("got\n%s", got)
	}
}

func TestSeries(t *testing.T) {
	var b strings.Builder
	b.WriteString("mindmap\n  root\n")
	for i := 0; i < 10; i++ {
		b.WriteString("    branch x\n      leaf x\n")
	}
	g := Parse(b.String(), true)
	if s := g.Nodes["n0"].Series; s != 0 {
		t.Errorf("root series %d", s)
	}
	for i := 0; i < 10; i++ {
		want := 1 + i%8
		br := g.Nodes["n0."+strconv.Itoa(i)]
		leaf := g.Nodes["n0."+strconv.Itoa(i)+".0"]
		if br.Series != want || leaf.Series != want {
			t.Errorf("branch %d: series %d and %d, want %d", i, br.Series, leaf.Series, want)
		}
	}
	for _, e := range g.Edges {
		if e.Series != g.Nodes[e.To].Series {
			t.Errorf("edge %s series %d", e.ID, e.Series)
		}
	}
}

func TestIconsAndClasses(t *testing.T) {
	src := "mindmap\n  Root\n    A[A]\n    ::icon(fa fa-book)\n    :::urgent large\n    B\n  :::big\nclassDef urgent fill:#f00,color:#fff\n"
	g := Parse(src, true)
	if got := outline(g); got != "n0 round Root\n  n0.0 rect A\n  n0.1 round B\n" {
		t.Errorf("got\n%s", got)
	}
	if c := g.Nodes["n0.0"].Classes; !slices.Equal(c, []string{"urgent", "large"}) {
		t.Errorf("A's classes %v", c)
	}
	if c := g.Nodes["n0.1"].Classes; !slices.Equal(c, []string{"big"}) {
		t.Errorf("B's classes %v", c)
	}
	if st := g.ClassDefs["urgent"]; st.Fill != "#f00" || st.Color != "#fff" {
		t.Errorf("classDef %+v", st)
	}
	if len(g.Problems) > 0 {
		t.Errorf("problems %+v", g.Problems)
	}
}

func TestMarkdownOverLines(t *testing.T) {
	src := "mindmap\n    id1[\"`**Root** with\na second line\nUnicode works too: 🤓`\"]\n      id2[\"`The dog in **the** hog`\"]\n"
	g := Parse(src, true)
	want := "n0 rect Root with|a second line|Unicode works too: 🤓\n  n0.0 rect The dog in the hog\n"
	if got := outline(g); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestHeaderFrontMatterComments(t *testing.T) {
	g := Parse("---\ntitle: Notes\nconfig:\n  title: no\n---\n%%{init: {}}%%\nmindmap\n  %% a comment\n  Root %% trailing\n    accTitle: A title\n    accDescr {\n      not a node\n    }\n    A\n", true)
	if g.Title != "Notes" || !g.HasHeader || len(g.Problems) > 0 {
		t.Errorf("title %q, header %v, problems %+v", g.Title, g.HasHeader, g.Problems)
	}
	if got := outline(g); got != "n0 round Root\n  n0.0 round A\n" {
		t.Errorf("got\n%s", got)
	}
	if g := Parse("graph TD\n  A\n", true); g.HasHeader || len(g.Problems) == 0 {
		t.Errorf("a flowchart: header %v, problems %+v", g.HasHeader, g.Problems)
	}
	if g := Parse("", true); len(g.Problems) != 1 {
		t.Errorf("empty: problems %+v", g.Problems)
	}
	if g := Parse("mindmap", false); g.HasHeader {
		t.Error("a header still being written is complete")
	}
}

func TestProblems(t *testing.T) {
	for _, src := range []string{
		"mindmap\n  Root\n    a[unclosed\n",
		"mindmap\n  Root\n    a[text] more\n",
		"mindmap\n  Root\n    classDef\n",
		"mindmap\n:::orphan\n",
	} {
		g := Parse(src, true)
		if len(g.Problems) != 1 {
			t.Errorf("%q: problems %+v", src, g.Problems)
		}
		// The node is still drawn, so that the names of the nodes after
		// it do not change.
		if strings.Contains(src, "a[") && g.Nodes["n0.0"] == nil {
			t.Errorf("%q: no node", src)
		}
	}
}

// TestStreamingTail checks how the line being written shows.
func TestStreamingTail(t *testing.T) {
	base := "mindmap\n  root((Topic))\n    "
	for _, c := range []struct {
		tail string
		want string // the last node, "" for none
	}{
		{"", ""},
		{"A", ""},     // may be an ID
		{"Alpha", ""}, // may be an ID
		{"Alpha ", "n0.0 round Alpha …"},
		{"Long hist", "n0.0 round Long hist …"},
		{"学习", "n0.0 round 学习 …"},
		{"id(", ""}, // may be ((
		{"id((", ""},
		{"id((Ci", "n0.0 circle Ci …"},
		{"id((Circle)", "n0.0 circle Circle …"},
		{"id((Circle))", "n0.0 circle Circle"},
		{"id))Ba", "n0.0 bang Ba …"},
		{"id))Bang(", "n0.0 bang Bang …"},
		{"id)Cl", "n0.0 cloud Cl …"},
		{"id[", ""},
		{"id[\"", ""},
		{"id[\"Quo", "n0.0 rect Quo …"},
		{"id{", ""},
		{"id{{He", "n0.0 hexagon He …"},
		{"id{{Hex}", "n0.0 hexagon Hex …"},
		{"my node(", "n0.0 round my node …"},
		{":", ""},
		{"::icon(fa", ""},
		{"%", ""},
		{"classDef x fill:#f00", ""},
		{"accTitle: x", ""},
	} {
		g := Parse(base+c.tail, false)
		got := ""
		if n := len(g.NodeOrder); n > 1 {
			got = strings.TrimSpace(strings.Split(strings.TrimSpace(outline(g)), "\n")[n-1])
		}
		if got != c.want {
			t.Errorf("%q: got %q, want %q", c.tail, got, c.want)
		}
		if len(g.Problems) > 0 {
			t.Errorf("%q: problems %+v", c.tail, g.Problems)
		}
	}
}

func TestStreamedPrefixesNeverPanic(t *testing.T) {
	src := "---\ntitle: x\n---\nmindmap\n  root((a))\n    b[\"`x\ny`\"]\n    c))d((\n    ::icon(x)\n    :::y\n\te{{f}}\n  g\n"
	for i := range len(src) + 1 {
		Parse(src[:i], false)
		Parse(src[:i], true)
	}
}

func FuzzParse(f *testing.F) {
	f.Add("mindmap\n  root((a))\n    b[c]\n      d)e(\n    ::icon(x)\n")
	f.Add("mindmap\n\"`a\n`\"\n  x((y\n")
	f.Fuzz(func(t *testing.T, src string) {
		Parse(src, false)
		Parse(src, true)
	})
}

func TestCompare(t *testing.T) {
	a := Parse("mindmap\n  root\n    A[Lab", false)
	b := Parse("mindmap\n  root\n    A[Label]\n    B\n", false)
	d := flow.Compare(a, b)
	if !reflect.DeepEqual(d.ChangedNodes, []string{"n0.0"}) || !reflect.DeepEqual(d.AddedNodes, []string{"n0.1"}) ||
		!reflect.DeepEqual(d.AddedEdges, []string{"n0->n0.1#0"}) || len(d.RemovedNodes) > 0 {
		t.Errorf("diff %+v", d)
	}
}
