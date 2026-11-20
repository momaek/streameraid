package er

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/momaek/streameraid/flow"
	"github.com/momaek/streameraid/layout"
	"github.com/momaek/streameraid/layout/sugiyama"
	"github.com/momaek/streameraid/measure"
)

// corpus returns the ER diagrams of testdata/corpus.
func corpus(t *testing.T) map[string]string {
	t.Helper()
	files, err := filepath.Glob("../testdata/corpus/*/er-*.mmd")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		out[f] = string(b)
	}
	if len(out) == 0 {
		t.Fatal("no ER diagrams in testdata/corpus")
	}
	return out
}

// TestCorpusAI checks that the AI-style samples parse without problems.
func TestCorpusAI(t *testing.T) {
	for f, src := range corpus(t) {
		if !strings.Contains(f, "/ai/") {
			continue
		}
		if g := Parse(src, true); len(g.Problems) > 0 || len(g.Nodes) == 0 {
			t.Errorf("%s: %d nodes, problems %+v", filepath.Base(f), len(g.Nodes), g.Problems)
		}
	}
}

// TestCorpusDocs checks that the examples of Mermaid's documentation parse
// without problems.
func TestCorpusDocs(t *testing.T) {
	for f, src := range corpus(t) {
		if g := Parse(src, true); len(g.Problems) > 0 || len(g.Nodes) == 0 {
			t.Errorf("%s: %d nodes, problems %+v", filepath.Base(f), len(g.Nodes), g.Problems)
		}
	}
}

// TestCorpusStreaming streams every sample a character at a time and
// checks that nothing appears, goes and comes back, that no member row
// goes away, that the line being written makes no problems, and that the
// last step matches the whole source.
func TestCorpusStreaming(t *testing.T) {
	for f, src := range corpus(t) {
		name := filepath.Base(f)
		seen := map[string]string{}
		rowCount := map[string]int{}
		var prev *flow.Graph
		for end := 0; end <= len(src); end++ {
			g := Parse(src[:end], false)
			d := flow.Compare(prev, g)
			for _, id := range append(d.AddedNodes, d.AddedEdges...) {
				if seen[id] == "gone" {
					t.Errorf("%s: at %d (%q) %s came back", name, end, tailOf(src[:end]), id)
				}
				seen[id] = "present"
			}
			for _, id := range append(d.RemovedNodes, d.RemovedEdges...) {
				seen[id] = "gone"
			}
			for _, id := range g.NodeOrder {
				n := countRows(g.Nodes[id])
				if n < rowCount[id] {
					t.Errorf("%s: at %d (%q) rows of %s went away", name, end, tailOf(src[:end]), id)
				}
				rowCount[id] = n
			}
			if len(g.Problems) > 0 && strings.Contains(f, "/ai/") {
				t.Errorf("%s: at %d (%q) problems %+v", name, end, tailOf(src[:end]), g.Problems)
			}
			prev = g
		}
		whole := Parse(src, true)
		if !flow.Compare(prev, whole).Empty() || !reflect.DeepEqual(prev.NodeOrder, whole.NodeOrder) {
			t.Errorf("%s: the streamed graph differs from the whole one: %+v", name, flow.Compare(prev, whole))
		}
	}
}

// TestCorpusLayout lays out every sample, whole and while it streams in.
func TestCorpusLayout(t *testing.T) {
	for f, src := range corpus(t) {
		g := Parse(src, true)
		l := sugiyama.Layout(g, measure.Fixed{}, layout.Options{})
		if len(l.Nodes) != len(g.Nodes) {
			t.Errorf("%s: %d nodes laid out of %d", filepath.Base(f), len(l.Nodes), len(g.Nodes))
		}
		for end := 0; end <= len(src); end += 7 {
			sugiyama.Layout(Parse(src[:end], false), measure.Fixed{}, layout.Options{})
		}
	}
}

func countRows(n *flow.Node) int {
	if n.Table == nil {
		return 0
	}
	c := 0
	for _, s := range n.Table.Sections {
		c += len(s.Rows)
	}
	return c
}

func tailOf(s string) string {
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return s[i+1:]
	}
	return s
}
