package class

import (
	"fmt"
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

// rows returns a node's table as text: sections split by |, rows by ;.
func rows(n *flow.Node) string {
	if n == nil || n.Table == nil {
		return "<none>"
	}
	var secs []string
	for _, s := range n.Table.Sections {
		var rs []string
		for _, r := range s.Rows {
			rs = append(rs, strings.Join(r, ","))
		}
		secs = append(secs, strings.Join(rs, ";"))
	}
	return strings.Join(secs, "|")
}

func edgeList(g *flow.Graph) []string {
	var out []string
	for _, e := range g.Edges {
		s := fmt.Sprintf("%s>%s %d%d%d", e.From, e.To, e.Start, e.Line, e.End)
		if e.Label != "" {
			s += "|" + e.Label
		}
		if e.StartLabel != "" || e.EndLabel != "" {
			s += " [" + e.StartLabel + "," + e.EndLabel + "]"
		}
		out = append(out, s)
	}
	return out
}

func TestHeader(t *testing.T) {
	for _, h := range []string{"classDiagram", "classDiagram-v2"} {
		g := mustParse(t, h+"\nA")
		if !g.HasHeader || g.NodeOrder[0] != "A" {
			t.Errorf("%s: %+v", h, g)
		}
	}
	if g := Parse("classDiagram", false); g.HasHeader {
		t.Error("header still being written was taken")
	}
	if g := Parse("flowchart TD\nA", true); len(g.Problems) == 0 {
		t.Error("wrong header gave no problem")
	}
	if g := Parse("", true); len(g.Problems) == 0 {
		t.Error("missing header gave no problem")
	}
	g := mustParse(t, "---\ntitle: Animals\n---\nclassDiagram\ndirection LR\nA\n")
	if g.Title != "Animals" || g.Dir != flow.LR {
		t.Errorf("title %q dir %v", g.Title, g.Dir)
	}
}

func TestClassBlock(t *testing.T) {
	g := mustParse(t, `classDiagram
class BankAccount {
    +String owner
    +BigDecimal balance$
    -List~Transaction~ history
    +deposit(amount) bool
    +withdrawal(amount)*
    +getItems() List~Item~
    #audit()$ void
}
class Empty {}
class Plain
`)
	if got := rows(g.Nodes["BankAccount"]); got != "BankAccount|+String owner;+BigDecimal balance;-List<Transaction> history|+deposit(amount) : bool;+withdrawal(amount);+getItems() : List<Item>;#audit() : void" {
		t.Errorf("BankAccount: %s", got)
	}
	if got := rows(g.Nodes["Empty"]); got != "Empty||" {
		t.Errorf("Empty: %s", got)
	}
	if got := rows(g.Nodes["Plain"]); got != "Plain" {
		t.Errorf("Plain: %s", got)
	}
	n := g.Nodes["BankAccount"]
	if n.Shape != flow.Sectioned || n.Label != "BankAccount" || !n.Table.Sections[0].Bold || !n.Table.Sections[0].Center || n.Table.Sections[1].Center {
		t.Errorf("node %+v", n)
	}
}

func TestMemberLines(t *testing.T) {
	g := mustParse(t, "classDiagram\nAnimal : +int age\nAnimal : +isMammal()\nAnimal: +mate()\n")
	if got := rows(g.Nodes["Animal"]); got != "Animal|+int age|+isMammal();+mate()" {
		t.Errorf("Animal: %s", got)
	}
}

func TestGenericsAndLabels(t *testing.T) {
	g := mustParse(t, "classDiagram\nclass Square~Shape~{\n  int id\n  List~List~int~~ grid\n  ~int count\n}\nclass Animal[\"Animal with a label\"]\nclass `Odd Name`\nSquare~Shape~ <|-- Animal\n")
	if got := rows(g.Nodes["Square"]); got != "Square<Shape>|int id;List<List<int>> grid;~int count|" {
		t.Errorf("Square: %s", got)
	}
	if n := g.Nodes["Animal"]; n.Label != "Animal with a label" {
		t.Errorf("Animal label %q", n.Label)
	}
	if g.Nodes["Odd Name"] == nil {
		t.Errorf("backtick name: %v", g.NodeOrder)
	}
	if got := edgeList(g); !reflect.DeepEqual(got, []string{"Square>Animal 400"}) {
		t.Errorf("edges %v", got)
	}
}

func TestAnnotations(t *testing.T) {
	g := mustParse(t, "classDiagram\nclass Shape\n<<interface>> Shape\nclass Color{\n<<enumeration>>\nRED\nGREEN\n}\nclass Sized <<abstract>>\n")
	s := g.Nodes["Shape"].Table.Sections
	if len(s) != 2 || s[0].Rows[0][0] != "«interface»" || !s[0].Small || !s[0].Center || s[1].Rows[0][0] != "Shape" || !s[1].Joined || !s[1].Bold {
		t.Errorf("Shape: %+v", s)
	}
	if got := rows(g.Nodes["Color"]); got != "«enumeration»|Color|RED;GREEN|" {
		t.Errorf("Color: %s", got)
	}
	if got := rows(g.Nodes["Sized"]); got != "«abstract»|Sized" {
		t.Errorf("Sized: %s", got)
	}
}

func TestRelations(t *testing.T) {
	g := mustParse(t, `classDiagram
classA <|-- classB
classC *-- classD
classE o-- classF
classG <-- classH
classI -- classJ
classK <.. classL
classM <|.. classN
classO .. classP
classQ --> classR : uses
classS ..> classT
classU ..|> classV
classW --* classX
classY --o classZ
A <|--|> B
C *--* D
bar ()-- foo
foo --() baz
Customer "1" --> "*" Ticket : buys
Student "1..*" o-- "1" Course
`)
	T, D, H, N, L := flow.ArrowTriangle, flow.ArrowDiamond, flow.ArrowHollow, AssocArrow, flow.ArrowLollipop
	want := []string{
		fmt.Sprintf("classA>classB %d00", T),
		fmt.Sprintf("classC>classD %d00", D),
		fmt.Sprintf("classE>classF %d00", H),
		fmt.Sprintf("classG>classH %d00", N),
		"classI>classJ 000",
		fmt.Sprintf("classK>classL %d10", N),
		fmt.Sprintf("classM>classN %d10", T),
		"classO>classP 010",
		fmt.Sprintf("classQ>classR 00%d|uses", N),
		fmt.Sprintf("classS>classT 01%d", N),
		fmt.Sprintf("classU>classV 01%d", T),
		fmt.Sprintf("classW>classX 00%d", D),
		fmt.Sprintf("classY>classZ 00%d", H),
		fmt.Sprintf("A>B %d0%d", T, T),
		fmt.Sprintf("C>D %d0%d", D, D),
		fmt.Sprintf("bar>foo %d00", L),
		fmt.Sprintf("foo>baz 00%d", L),
		fmt.Sprintf("Customer>Ticket 00%d|buys [1,*]", N),
		fmt.Sprintf("Student>Course %d00 [1..*,1]", H),
	}
	if got := edgeList(g); !reflect.DeepEqual(got, want) {
		t.Errorf("edges\n got %q\nwant %q", got, want)
	}
	for _, e := range g.Edges {
		if e.Length != 1 || g.Edges[e.Index] != e {
			t.Errorf("edge %+v", e)
		}
	}
	if g.Nodes["classZ"] == nil || g.Nodes["classZ"].Table == nil {
		t.Error("relation did not make its classes")
	}
}

func TestNamespaces(t *testing.T) {
	g := mustParse(t, `classDiagram
namespace BaseShapes {
    class Triangle
    class Rectangle {
      double width
    }
}
class Free
Triangle <|-- Free
`)
	sg := g.Subgraphs["BaseShapes"]
	if sg == nil || sg.Title != "BaseShapes" || sg.Open || !reflect.DeepEqual(sg.Nodes, []string{"Triangle", "Rectangle"}) {
		t.Fatalf("namespace %+v", sg)
	}
	if g.Nodes["Free"].Subgraph != "" || g.Nodes["Rectangle"].Subgraph != "BaseShapes" {
		t.Error("wrong subgraphs")
	}
	if g := Parse("classDiagram\nnamespace X {\nclass A\n", true); len(g.Problems) == 0 {
		t.Error("unclosed namespace gave no problem")
	}
	if g := Parse("classDiagram\nnamespace X {\nclass A\n", false); !g.Subgraphs["X"].Open || len(g.Problems) > 0 {
		t.Error("namespace not open while streaming")
	}
}

func TestNotes(t *testing.T) {
	g := mustParse(t, "classDiagram\nnote \"From Duck till Zebra\"\nnote for Duck \"can fly\\ncan swim\"\n")
	if len(g.NodeOrder) != 3 {
		t.Fatalf("nodes %v", g.NodeOrder)
	}
	n1, n2 := g.Nodes["note:1"], g.Nodes["note:2"]
	if n1 == nil || n1.Shape != flow.Note || n1.Label != "From Duck till Zebra" || n2 == nil || n2.Label != "can fly\ncan swim" {
		t.Fatalf("notes %+v %+v", n1, n2)
	}
	e := g.Edges[0]
	if e.From != "Duck" || e.To != "note:2" || e.Line != flow.Dotted || e.Start != flow.ArrowNone || e.End != flow.ArrowNone {
		t.Errorf("note edge %+v", e)
	}
	g = Parse("classDiagram\nnote for Duck \"can f", false)
	if n := g.Nodes["note:1"]; n == nil || n.Label != "can f" || !n.Partial || len(g.Problems) > 0 {
		t.Errorf("partial note %+v", n)
	}
}

func TestStyling(t *testing.T) {
	g := mustParse(t, `classDiagram
class Animal:::warm
class Duck
Duck:::cold --> Animal
classDef warm fill:#f96,stroke:#333
classDef cold fill:#9cf
cssClass "Animal,Duck" extra
style Duck fill:#fff,stroke-width:4px
click Duck href "https://example.com"
link Animal "https://example.com"
callback Animal "fn"
`)
	if !reflect.DeepEqual(g.Nodes["Animal"].Classes, []string{"warm", "extra"}) || !reflect.DeepEqual(g.Nodes["Duck"].Classes, []string{"cold", "extra"}) {
		t.Errorf("classes %v %v", g.Nodes["Animal"].Classes, g.Nodes["Duck"].Classes)
	}
	if g.ClassDefs["warm"].Fill != "#f96" || g.Nodes["Duck"].Style.StrokeWidth != 4 {
		t.Errorf("styles %+v %+v", g.ClassDefs, g.Nodes["Duck"].Style)
	}
}

func TestProblems(t *testing.T) {
	for _, src := range []string{
		"classDiagram\nA ~~ B",
		"classDiagram\nA -- ",
		"classDiagram\nclass A {\n+x",
		"classDiagram\nnamespace {",
		"classDiagram\nnote for A text",
		"classDiagram\nclass A[\"x\"\n",
		"classDiagram\n}",
	} {
		if g := Parse(src, true); len(g.Problems) == 0 {
			t.Errorf("%q: no problem", src)
		}
	}
}

// TestStreamingTail checks how the line being written reads.
func TestStreamingTail(t *testing.T) {
	cases := []struct {
		src   string
		nodes string
		edges int
	}{
		{"classDiagram\nAnim", "", 0},
		{"classDiagram\nAnimal ", "Animal", 0},
		{"classDiagram\nAnimal <|-- Du", "Animal", 0},
		{"classDiagram\nAnimal <|-- Duck ", "Animal Duck", 1},
		{"classDiagram\nclass Duc", "", 0},
		{"classDiagram\nclass Duck{", "Duck", 0},
		{"classDiagram\nclass Duck~Ty", "", 0},
		{"classDiagram\nAnimal \"1\" --> \"*", "Animal", 0},
		{"classDiagram\nclass Duck{\n+swim()\n+quack", "Duck", 0},
		{"classDiagram\nnamespace Sha", "", 0},
	}
	for _, c := range cases {
		g := Parse(c.src, false)
		if got := strings.Join(g.NodeOrder, " "); got != c.nodes || len(g.Edges) != c.edges || len(g.Problems) > 0 {
			t.Errorf("%q: nodes %q edges %d problems %v", c.src, got, len(g.Edges), g.Problems)
		}
	}
	g := Parse("classDiagram\nclass Duck{\n+swim()\n+quack", false)
	if got := rows(g.Nodes["Duck"]); got != "Duck||+swim()" {
		t.Errorf("open block: %s", got)
	}
	g = Parse("classDiagram\nclass Duck[\"Mallard d", false)
	if n := g.Nodes["Duck"]; n == nil || n.Label != "Mallard d" || !n.Partial {
		t.Errorf("partial label: %+v", n)
	}
}

func TestStreamedPrefixesNeverPanic(t *testing.T) {
	src := "classDiagram\nclass A~T~[\"x\"]:::c <<i>> {\n<<interface>>\n+List~int~ x$\n+f(a)* int\n}\nA \"1\" <|--|> \"*\" B : y\nnote for A \"n\"\nnamespace N {\nclass C\n}\n`q r` ()..() s\n"
	for i := range len(src) + 1 {
		Parse(src[:i], false)
		Parse(src[:i], true)
	}
}

func FuzzParse(f *testing.F) {
	f.Add("classDiagram\nclass A~T~ {\n+x\n}\nA <|-- B : y\n")
	f.Add("classDiagram\nA \"1\" o--* \"2\" B\nnote for A \"x\"\nnamespace N {\n}\n")
	f.Fuzz(func(t *testing.T, src string) {
		Parse(src, false)
		Parse(src, true)
	})
}
