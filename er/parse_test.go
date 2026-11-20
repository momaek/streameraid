package er

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
		out = append(out, fmt.Sprintf("%s>%s %d%d%d|%s", e.From, e.To, e.Start, e.Line, e.End, e.Label))
	}
	return out
}

func TestHeader(t *testing.T) {
	g := mustParse(t, "---\ntitle: Orders\n---\nerDiagram\ndirection LR\nCUSTOMER\n")
	if !g.HasHeader || g.Title != "Orders" || g.Dir != flow.LR || g.NodeOrder[0] != "CUSTOMER" {
		t.Errorf("%+v", g)
	}
	if g := Parse("erDiagram", false); g.HasHeader {
		t.Error("header still being written was taken")
	}
	if g := Parse("classDiagram\nA", true); len(g.Problems) == 0 {
		t.Error("wrong header gave no problem")
	}
	if g := Parse("", true); len(g.Problems) == 0 {
		t.Error("missing header gave no problem")
	}
}

func TestRelationships(t *testing.T) {
	g := mustParse(t, `erDiagram
CUSTOMER ||--o{ ORDER : places
ORDER ||--|{ LINE-ITEM : contains
CUSTOMER }|..|{ DELIVERY-ADDRESS : uses
A |o--o| B : "zero or one"
C }o--|| D : ""
id1||--||id2 : label
CAR 1 to zero or more NAMED-DRIVER : allows
PERSON many(0) optionally to 0+ NAMED-DRIVER : is
X only one to one or more Y : a
X zero or one to 1+ Y : b
X one or zero to many(1) Y : c
X one or many to zero or many Y : d
P ||--o{ Q
`)
	O, ZO, M, ZM := flow.ArrowOne, flow.ArrowZeroOne, flow.ArrowMany, flow.ArrowZeroMany
	want := []string{
		fmt.Sprintf("CUSTOMER>ORDER %d0%d|places", O, ZM),
		fmt.Sprintf("ORDER>LINE-ITEM %d0%d|contains", O, M),
		fmt.Sprintf("CUSTOMER>DELIVERY-ADDRESS %d1%d|uses", M, M),
		fmt.Sprintf("A>B %d0%d|zero or one", ZO, ZO),
		fmt.Sprintf("C>D %d0%d|", ZM, O),
		fmt.Sprintf("id1>id2 %d0%d|label", O, O),
		fmt.Sprintf("CAR>NAMED-DRIVER %d0%d|allows", O, ZM),
		fmt.Sprintf("PERSON>NAMED-DRIVER %d1%d|is", ZM, ZM),
		fmt.Sprintf("X>Y %d0%d|a", O, M),
		fmt.Sprintf("X>Y %d0%d|b", ZO, M),
		fmt.Sprintf("X>Y %d0%d|c", ZO, M),
		fmt.Sprintf("X>Y %d0%d|d", M, ZM),
		fmt.Sprintf("P>Q %d0%d|", O, ZM),
	}
	if got := edgeList(g); !reflect.DeepEqual(got, want) {
		t.Errorf("edges\n got %q\nwant %q", got, want)
	}
	if g.Edges[11].ID != "X->Y#3" {
		t.Errorf("edge ID %s", g.Edges[11].ID)
	}
}

func TestAttributes(t *testing.T) {
	g := mustParse(t, `erDiagram
CUSTOMER {
    string name
    string custNumber
}
PERSON {
    string driversLicense PK "The license #"
    string(99) firstName "Only 99 characters are allowed"
    string lastName
    string phone UK
    decimal(10, 2) balance
    string[] tags
}
NAMED-DRIVER {
    string carRegistrationNumber PK, FK
    string driverLicence PK,FK
}
NOTE {
    string body "just a comment"
}
EMPTY {
}
ONELINE { int id PK string name }
`)
	cases := map[string]string{
		"CUSTOMER":     "CUSTOMER|string,name;string,custNumber",
		"PERSON":       "PERSON|string,driversLicense,PK,The license #;string(99),firstName,,Only 99 characters are allowed;string,lastName,,;string,phone,UK,;decimal(10, 2),balance,,;string[],tags,,",
		"NAMED-DRIVER": "NAMED-DRIVER|string,carRegistrationNumber,PK, FK;string,driverLicence,PK, FK",
		"NOTE":         "NOTE|string,body,just a comment",
		"EMPTY":        "EMPTY",
		"ONELINE":      "ONELINE|int,id,PK;string,name,",
	}
	for id, want := range cases {
		if got := rows(g.Nodes[id]); got != want {
			t.Errorf("%s:\n got %s\nwant %s", id, got, want)
		}
	}
	s := g.Nodes["PERSON"].Table.Sections
	if !s[0].Center || !s[0].Bold || s[1].Center || s[1].Bold {
		t.Errorf("sections %+v", s)
	}
}

func TestNames(t *testing.T) {
	g := mustParse(t, `erDiagram
p[Person] {
    string firstName
}
a["Customer Account"] {
    string email
}
p ||--o| a : has
"This ❤ Unicode"
用户 ||--o{ 订单 : 下单
`)
	if g.Nodes["p"].Label != "Person" || g.Nodes["a"].Label != "Customer Account" || rows(g.Nodes["a"]) != "Customer Account|string,email" {
		t.Errorf("aliases %q %q", g.Nodes["p"].Label, g.Nodes["a"].Label)
	}
	if n := g.Nodes["This ❤ Unicode"]; n == nil || n.Label != "This ❤ Unicode" {
		t.Errorf("quoted name: %v", g.NodeOrder)
	}
	if got := edgeList(g); len(got) != 2 || !strings.HasPrefix(got[1], "用户>订单") {
		t.Errorf("edges %v", got)
	}
}

func TestStyling(t *testing.T) {
	g := mustParse(t, `erDiagram
CAR:::someclass {
    string make
}
HOUSE:::other
CAR:::third ||--o{ PERSON : allows
classDef someclass fill:#f96
class CAR,PERSON extra
style PERSON fill:#fff,stroke-width:4px
accTitle: Cars
accDescr {
    A long
    description
}
`)
	if !reflect.DeepEqual(g.Nodes["CAR"].Classes, []string{"someclass", "third", "extra"}) || !reflect.DeepEqual(g.Nodes["HOUSE"].Classes, []string{"other"}) {
		t.Errorf("classes %v %v", g.Nodes["CAR"].Classes, g.Nodes["HOUSE"].Classes)
	}
	if g.ClassDefs["someclass"].Fill != "#f96" || g.Nodes["PERSON"].Style.StrokeWidth != 4 || len(g.NodeOrder) != 3 {
		t.Errorf("styles %+v %+v %v", g.ClassDefs, g.Nodes["PERSON"].Style, g.NodeOrder)
	}
}

func TestProblems(t *testing.T) {
	for _, src := range []string{
		"erDiagram\nA ||--x{ B : c",
		"erDiagram\nA ||~~o{ B",
		"erDiagram\nA ||--o{",
		"erDiagram\nA {\nstring",
		"erDiagram\nA {\nstring x",
		"erDiagram\np[Person",
		"erDiagram\n}",
		"erDiagram\nA B",
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
		{"erDiagram\nCUST", "", 0},
		{"erDiagram\nCUSTOMER ", "CUSTOMER", 0},
		{"erDiagram\nCUSTOMER |", "CUSTOMER", 0},
		{"erDiagram\nCUSTOMER ||--o{ ORD", "CUSTOMER", 0},
		{"erDiagram\nCUSTOMER ||--o{ ORDER ", "CUSTOMER ORDER", 1},
		{"erDiagram\nCUSTOMER ||--o{ ORDER : pla", "CUSTOMER ORDER", 1},
		{"erDiagram\nCAR 1", "CAR", 0},
		{"erDiagram\nCAR 1 to zero or", "CAR", 0},
		{"erDiagram\nCAR 1 to zero or more NAMED", "CAR", 0},
		{"erDiagram\nCAR{", "CAR", 0},
		{"erDiagram\nCAR {\nstring make\nstring mo", "CAR", 0},
		{"erDiagram\n\"Customer Acc", "", 0},
		{"erDiagram\np[Pers", "p", 0},
	}
	for _, c := range cases {
		g := Parse(c.src, false)
		if got := strings.Join(g.NodeOrder, " "); got != c.nodes || len(g.Edges) != c.edges || len(g.Problems) > 0 {
			t.Errorf("%q: nodes %q edges %d problems %v", c.src, got, len(g.Edges), g.Problems)
		}
	}
	if g := Parse("erDiagram\nCAR {\nstring make\nstring mo", false); rows(g.Nodes["CAR"]) != "CAR|string,make" {
		t.Errorf("open block: %s", rows(g.Nodes["CAR"]))
	}
	if g := Parse("erDiagram\nA ||--o{ B : pla", false); g.Edges[0].Label != "pla" {
		t.Errorf("label being written: %q", g.Edges[0].Label)
	}
	if n := Parse("erDiagram\np[Pers", false).Nodes["p"]; n.Label != "Pers" || !n.Partial {
		t.Errorf("alias being written: %+v", n)
	}
}

func TestStreamedPrefixesNeverPanic(t *testing.T) {
	src := "erDiagram\np[\"P q\"]:::c {\nstring(9) x PK, FK \"c\"\n}\n\"Q r\" |o..o{ p : \"y\"\nA one or more optionally to many(0) B\naccDescr {\nx\n}\n"
	for i := range len(src) + 1 {
		Parse(src[:i], false)
		Parse(src[:i], true)
	}
}

func FuzzParse(f *testing.F) {
	f.Add("erDiagram\nA ||--o{ B : c\nA {\nint id PK \"x\"\n}\n")
	f.Add("erDiagram\nA 1 to 0+ B\np[\"x\"] }|..|{ q\n")
	f.Fuzz(func(t *testing.T, src string) {
		Parse(src, false)
		Parse(src, true)
	})
}
