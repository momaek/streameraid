package mindmap

import (
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/momaek/streameraid/flow"
	"github.com/momaek/streameraid/layout"
	"github.com/momaek/streameraid/layout/tree"
	"github.com/momaek/streameraid/measure"
)

// corpus returns the mind maps of testdata/corpus, by file.
func corpus(t *testing.T) map[string]string {
	t.Helper()
	files, err := filepath.Glob("../testdata/corpus/*/mindmap-*.mmd")
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
		t.Fatal("no mind maps in testdata/corpus")
	}
	return out
}

// TestCorpusAI checks that the AI-style samples parse without problems.
func TestCorpusAI(t *testing.T) {
	n := 0
	for f, src := range corpus(t) {
		if !strings.Contains(f, "/ai/") {
			continue
		}
		n++
		if g := Parse(src, true); len(g.Problems) > 0 || len(g.Nodes) < 5 {
			t.Errorf("%s: %d nodes, problems %+v", filepath.Base(f), len(g.Nodes), g.Problems)
		}
	}
	if n < 8 {
		t.Errorf("%d AI samples", n)
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
// checks that nothing that appeared goes away, and that the last step
// matches the whole source.
func TestCorpusStreaming(t *testing.T) {
	for f, src := range corpus(t) {
		name := filepath.Base(f)
		var prev *flow.Graph
		for end := 0; end <= len(src); end++ {
			if end < len(src) && !utf8.RuneStart(src[end]) {
				continue
			}
			g := Parse(src[:end], false)
			d := flow.Compare(prev, g)
			if len(d.RemovedNodes)+len(d.RemovedEdges) > 0 {
				t.Errorf("%s: at %d (%q) %v %v went away", name, end, tailOf(src[:end]), d.RemovedNodes, d.RemovedEdges)
			}
			if !strings.HasSuffix(src[:end], "\n") && end < len(src) && len(g.Problems) > 0 && len(Parse(src, true).Problems) == 0 {
				t.Errorf("%s: at %d (%q) problems %+v", name, end, tailOf(src[:end]), g.Problems)
			}
			prev = g
		}
		whole := Parse(src, true)
		if !reflect.DeepEqual(prev.NodeOrder, whole.NodeOrder) || !reflect.DeepEqual(edgeList(prev), edgeList(whole)) || outline(prev) != outline(whole) {
			t.Errorf("%s: the streamed graph differs from the whole one", name)
		}
	}
}

func edgeList(g *flow.Graph) []string {
	var out []string
	for _, e := range g.Edges {
		out = append(out, e.ID)
	}
	return out
}

func tailOf(s string) string {
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return s[i+1:]
	}
	return s
}

// TestCorpusLayout lays out every sample with the tree layout, to catch
// crashes and overlaps.
func TestCorpusLayout(t *testing.T) {
	for f, src := range corpus(t) {
		g := Parse(src, true)
		lo := tree.Layout(g, measure.Fixed{}, layout.Options{})
		if len(lo.Nodes) != len(g.Nodes) || len(lo.Edges) != len(g.Edges) {
			t.Errorf("%s: %d boxes for %d nodes, %d routes for %d edges", filepath.Base(f), len(lo.Nodes), len(g.Nodes), len(lo.Edges), len(g.Edges))
		}
		for i, a := range lo.Nodes {
			if math.IsNaN(a.Rect.X) || a.Rect.X < 0 || a.Rect.Y < 0 || a.Rect.Right() > lo.Width || a.Rect.Bottom() > lo.Height {
				t.Errorf("%s: %s at %v, outside %vx%v", filepath.Base(f), a.ID, a.Rect, lo.Width, lo.Height)
			}
			for _, b := range lo.Nodes[i+1:] {
				if a.Rect.X < b.Rect.Right() && b.Rect.X < a.Rect.Right() && a.Rect.Y < b.Rect.Bottom() && b.Rect.Y < a.Rect.Bottom() {
					t.Errorf("%s: %s and %s overlap", filepath.Base(f), a.ID, b.ID)
				}
			}
		}
	}
}
