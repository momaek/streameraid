package flow

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func mustParse(t *testing.T, src string) *Graph {
	t.Helper()
	g := Parse(src, true)
	if len(g.Problems) > 0 {
		t.Fatalf("problems: %+v", g.Problems)
	}
	return g
}

func edgeList(g *Graph) []string {
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

func TestHeaderAndDirection(t *testing.T) {
	for src, want := range map[string]Direction{
		"flowchart TD\nA": TB, "graph LR\nA": LR, "flowchart BT\nA": BT,
		"graph RL;A": RL, "flowchart\nA": TB,
	} {
		g := mustParse(t, src)
		if g.Dir != want {
			t.Errorf("%q: dir %v, want %v", src, g.Dir, want)
		}
	}
}

func TestShapes(t *testing.T) {
	src := `flowchart TD
a[rect] --> b(round) --> c([stadium]) --> d[[sub]] --> e[(db)]
f((circle)) --> g{diamond} --> h{{hex}} --> i[/para/] --> j[\alt\]
k[/trap\] --> l[\trapalt/] --> m>flag] --> n(((dbl)))
o[/api/users]`
	g := mustParse(t, src)
	want := map[string]Shape{"a": Rect, "b": Round, "c": Stadium, "d": Subroutine, "e": Cylinder,
		"f": Circle, "g": Diamond, "h": Hexagon, "i": Parallelogram, "j": ParallelogramAlt,
		"k": Trapezoid, "l": TrapezoidAlt, "m": Asymmetric, "n": DoubleCircle, "o": Rect}
	for id, s := range want {
		n := g.Nodes[id]
		if n == nil {
			t.Errorf("node %s missing", id)
			continue
		}
		if n.Shape != s {
			t.Errorf("node %s: shape %v, want %v", id, n.Shape, s)
		}
	}
	if l := g.Nodes["o"].Label; l != "/api/users" {
		t.Errorf("o label %q", l)
	}
	if l := g.Nodes["e"].Label; l != "db" {
		t.Errorf("e label %q", l)
	}
}

func TestLabels(t *testing.T) {
	g := mustParse(t, `flowchart LR
A["Call (with parens) and [brackets]"] --> B["Line one<br/>Line two"]
C[#quot;quoted#quot; &amp; done] --> D["<b>bold</b> fa:fa-car text"]
E[中文 标签] --> F{"是否成功?"}`)
	for id, want := range map[string]string{
		"A": "Call (with parens) and [brackets]",
		"B": "Line one\nLine two",
		"C": `"quoted" & done`,
		"D": "bold text",
		"E": "中文 标签",
		"F": "是否成功?",
	} {
		if got := g.Nodes[id].Label; got != want {
			t.Errorf("%s: label %q, want %q", id, got, want)
		}
	}
}

func TestEdges(t *testing.T) {
	g := mustParse(t, `flowchart TD
A --> B
A --- C
A -.-> D
A ==> E
A ~~~ F
A <--> G
A --o H
A --x I
A ---> J
A -->|yes| K
A -- no --> L
A -. maybe .-> M
A == sure ==> N
A -->|"quoted | pipe"| O
A-->P`)
	type want struct {
		line       Line
		start, end Arrow
		length     int
		label      string
	}
	wants := []want{
		{Solid, ArrowNone, ArrowNormal, 1, ""},
		{Solid, ArrowNone, ArrowNone, 1, ""},
		{Dotted, ArrowNone, ArrowNormal, 1, ""},
		{Thick, ArrowNone, ArrowNormal, 1, ""},
		{Invisible, ArrowNone, ArrowNone, 1, ""},
		{Solid, ArrowNormal, ArrowNormal, 1, ""},
		{Solid, ArrowNone, ArrowCircle, 1, ""},
		{Solid, ArrowNone, ArrowCross, 1, ""},
		{Solid, ArrowNone, ArrowNormal, 2, ""},
		{Solid, ArrowNone, ArrowNormal, 1, "yes"},
		{Solid, ArrowNone, ArrowNormal, 1, "no"},
		{Dotted, ArrowNone, ArrowNormal, 1, "maybe"},
		{Thick, ArrowNone, ArrowNormal, 1, "sure"},
		{Solid, ArrowNone, ArrowNormal, 1, "quoted | pipe"},
		{Solid, ArrowNone, ArrowNormal, 1, ""},
	}
	if len(g.Edges) != len(wants) {
		t.Fatalf("got %d edges: %v", len(g.Edges), edgeList(g))
	}
	for i, w := range wants {
		e := g.Edges[i]
		got := want{e.Line, e.Start, e.End, e.Length, e.Label}
		if got != w {
			t.Errorf("edge %d (%s->%s): %+v, want %+v", i, e.From, e.To, got, w)
		}
	}
}

func TestChainsAndAmpersand(t *testing.T) {
	g := mustParse(t, "graph TD\nA --> B --> C\nD & E --> F & G\n")
	got := edgeList(g)
	want := []string{"A>B", "B>C", "D>F", "D>G", "E>F", "E>G"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("edges %v, want %v", got, want)
	}
}

func TestSemicolonsAndComments(t *testing.T) {
	g := mustParse(t, "graph TD;\n  A-->B; %% a comment\n  %% whole line\n  B-->C;")
	if got := edgeList(g); !reflect.DeepEqual(got, []string{"A>B", "B>C"}) {
		t.Errorf("edges %v", got)
	}
}

func TestSubgraphs(t *testing.T) {
	g := mustParse(t, `flowchart LR
A[Client] --> api
subgraph backend [Backend services]
  direction TB
  api[API] --> db[(DB)]
  subgraph cache ["Cache layer"]
    redis
  end
end
subgraph "Plain title"
  x
end
subgraph Two words
  y
end
A --> cache`)
	b := g.Subgraphs["backend"]
	if b == nil || b.Title != "Backend services" || !b.HasDir || b.Dir != TB {
		t.Fatalf("backend: %+v", b)
	}
	if !reflect.DeepEqual(b.Nodes, []string{"api", "db"}) || !reflect.DeepEqual(b.Children, []string{"cache"}) {
		t.Errorf("backend members %v %v", b.Nodes, b.Children)
	}
	if c := g.Subgraphs["cache"]; c.Parent != "backend" || c.Title != "Cache layer" || !reflect.DeepEqual(c.Nodes, []string{"redis"}) {
		t.Errorf("cache: %+v", c)
	}
	if g.Nodes["api"].Subgraph != "backend" {
		t.Errorf("api moved into its subgraph: %q", g.Nodes["api"].Subgraph)
	}
	if g.Subgraphs["Plain title"] == nil || g.Subgraphs["Two words"] == nil {
		t.Errorf("subgraphs %v", g.SubgraphOrder)
	}
	if g.Nodes["cache"] != nil {
		t.Errorf("an edge to a subgraph's ID made a node")
	}
	if last := g.Edges[len(g.Edges)-1]; last.To != "cache" {
		t.Errorf("edge to subgraph: %+v", last)
	}
}

func TestStyles(t *testing.T) {
	g := mustParse(t, `flowchart TD
A:::warn --> B
classDef warn fill:#f96,stroke:#333,stroke-width:2px,color:#fff
classDef default fill:#eee
class B ok
classDef ok fill:rgb(1,2,3)
style A stroke:#f00
linkStyle 0 stroke:#0f0,stroke-width:3px`)
	a := g.NodeStyle(g.Nodes["A"])
	if a.Fill != "#f96" || a.Stroke != "#f00" || a.Color != "#fff" || a.StrokeWidth != 2 {
		t.Errorf("A style %+v", a)
	}
	if b := g.NodeStyle(g.Nodes["B"]); b.Fill != "rgb(1,2,3)" {
		t.Errorf("B style %+v", b)
	}
	if e := g.Edges[0].Style; e.Stroke != "#0f0" || e.StrokeWidth != 3 {
		t.Errorf("edge style %+v", e)
	}
}

func TestProblems(t *testing.T) {
	g := Parse("flowchart TD\nA --> B\nA -->> ???\nsubgraph S\nC\n", true)
	if len(g.Problems) != 2 {
		t.Fatalf("problems %+v", g.Problems)
	}
	if g.Problems[0].Line != 3 {
		t.Errorf("problem line %d", g.Problems[0].Line)
	}
	if g.Nodes["A"] == nil || g.Nodes["B"] == nil || g.Nodes["C"] == nil {
		t.Errorf("good statements kept: %v", g.NodeOrder)
	}
	if g := Parse("sequenceDiagram\nA->>B: hi", true); g.HasHeader || len(g.Problems) == 0 {
		t.Errorf("not a flowchart: %+v", g)
	}
}

func TestTentativeTail(t *testing.T) {
	cases := []struct {
		src   string
		nodes []string
		edges []string
	}{
		{"flowchart TD\nA --> B", []string{"A"}, nil},
		{"flowchart TD\nA --> B ", []string{"A", "B"}, []string{"A>B"}},
		{"flowchart TD\nA --> Ba", []string{"A"}, nil},
		{"flowchart TD\nA -->", []string{"A"}, nil},
		{"flowchart TD\nA --", []string{"A"}, nil},
		{"flowchart TD\nA -->|Ye", []string{"A"}, nil},
		{"flowchart TD\nA[Sta", []string{"A"}, nil},
		{"flowchart TD\nA --> B{Is it", []string{"A", "B"}, []string{"A>B"}},
		{"flowchart TD\nA & B", []string{"A"}, nil},
		{"flowchart T", nil, nil},
		{"flowchart TD\nsubgraph Back", nil, nil},
		{"flowchart TD\nA -- yes", []string{"A"}, nil},
	}
	for _, c := range cases {
		g := Parse(c.src, false)
		if !reflect.DeepEqual(g.NodeOrder, c.nodes) && !(len(g.NodeOrder) == 0 && len(c.nodes) == 0) {
			t.Errorf("%q: nodes %v, want %v", c.src, g.NodeOrder, c.nodes)
		}
		if got := edgeList(g); !reflect.DeepEqual(got, c.edges) {
			t.Errorf("%q: edges %v, want %v", c.src, got, c.edges)
		}
	}
	g := Parse("flowchart TD\nA[Sta", false)
	if n := g.Nodes["A"]; !n.Partial || n.Label != "Sta" {
		t.Errorf("partial label %+v", n)
	}
}

const streamSample = `---
title: Order flow
---
flowchart TD
    %% an AI-style diagram
    Start([开始]) --> Login{"已登录?"}
    Login -->|是| Cart[查看购物车]
    Login -- 否 --> SignIn[/登录页面/]
    SignIn --> Login
    Cart --> Pay[[支付]] & Cancel(取消)
    subgraph payment [支付流程]
        direction LR
        Pay --> Gateway[(支付网关)]
        Gateway -.->|回调| Notify>通知]
    end
    Notify ==> Done(((完成)))
    classDef danger fill:#fdd,stroke:#c00
    class Cancel danger
    style Done fill:#dfd
    linkStyle 0 stroke:#888
`

// TestStreaming feeds the source a piece at a time, as a model writes it,
// and checks that every step parses, that nothing appears, goes and comes
// back, and that the end matches parsing the whole source at once.
func TestStreaming(t *testing.T) {
	whole := Parse(streamSample, true)
	if len(whole.Problems) > 0 {
		t.Fatalf("problems: %+v", whole.Problems)
	}
	for _, step := range []int{1, 3, 7} {
		t.Run(fmt.Sprint("step", step), func(t *testing.T) {
			seen := map[string]string{} // id -> "present" or "gone"
			var prev *Graph
			for end := 0; end <= len(streamSample); end += step {
				src := streamSample[:end]
				g := Parse(src, false)
				d := Compare(prev, g)
				for _, id := range append(d.AddedNodes, d.AddedEdges...) {
					if seen[id] == "gone" {
						t.Fatalf("at %d (%q): %s came back", end, tailOf(src), id)
					}
					seen[id] = "present"
				}
				for _, id := range append(d.RemovedNodes, d.RemovedEdges...) {
					seen[id] = "gone"
				}
				prev = g
			}
			final := Parse(streamSample, true)
			if !reflect.DeepEqual(final.NodeOrder, whole.NodeOrder) || !reflect.DeepEqual(edgeList(final), edgeList(whole)) {
				t.Errorf("final graph differs")
			}
		})
	}
}

func tailOf(s string) string {
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return s[i+1:]
	}
	return s
}

func TestStreamedPrefixesNeverPanic(t *testing.T) {
	srcs := []string{streamSample, "graph LR\nA-->B-->C\nC-.text.->D\nD==text==>E\nE~~~F\nx--xy\no--o p"}
	for _, src := range srcs {
		for i := range len(src) + 1 {
			Parse(src[:i], false)
			Parse(src[:i], true)
		}
	}
}

func FuzzParse(f *testing.F) {
	f.Add(streamSample)
	f.Add("graph TD\nA[\"x\"]-->|y|B{z}")
	f.Fuzz(func(t *testing.T, src string) {
		Parse(src, false)
		Parse(src, true)
	})
}

func TestCompare(t *testing.T) {
	a := Parse("flowchart TD\nA --> B[Lab", false)
	b := Parse("flowchart TD\nA --> B[Label]\nB --> C\n", false)
	d := Compare(a, b)
	if !reflect.DeepEqual(d.ChangedNodes, []string{"B"}) || !reflect.DeepEqual(d.AddedNodes, []string{"C"}) ||
		!reflect.DeepEqual(d.AddedEdges, []string{"B->C#0"}) || len(d.RemovedNodes) > 0 {
		t.Errorf("diff %+v", d)
	}
}

// TestFrontMatterStreaming checks that front matter still being written
// is no problem, and that one never closed is, once the source is complete.
func TestFrontMatterStreaming(t *testing.T) {
	if g := Parse("---\ntitle: x\n", false); len(g.Problems) > 0 {
		t.Errorf("streaming: %+v", g.Problems)
	}
	if g := Parse("---\ntitle: x\n", true); len(g.Problems) == 0 {
		t.Error("an unclosed front matter is not reported")
	}
}
