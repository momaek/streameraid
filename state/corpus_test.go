package state

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

// corpus returns the state diagrams of testdata/corpus.
func corpus(t *testing.T) map[string]string {
	t.Helper()
	files, err := filepath.Glob("../testdata/corpus/*/state-*.mmd")
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
		t.Fatal("no state diagrams in testdata/corpus")
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
		if g := Parse(src, true); len(g.Problems) > 0 || len(g.Nodes) == 0 {
			t.Errorf("%s: %d nodes, problems %+v", filepath.Base(f), len(g.Nodes), g.Problems)
		}
	}
	if n < 8 {
		t.Errorf("only %d AI samples", n)
	}
}

// TestCorpusDocs checks that the samples of Mermaid's documentation parse
// without problems.
func TestCorpusDocs(t *testing.T) {
	for f, src := range corpus(t) {
		if !strings.Contains(f, "/mermaid-docs/") {
			continue
		}
		if g := Parse(src, true); len(g.Problems) > 0 || len(g.Nodes) == 0 {
			t.Errorf("%s: %d nodes, problems %+v", filepath.Base(f), len(g.Nodes), g.Problems)
		}
	}
}

// TestCorpusStreaming streams every sample a byte at a time and checks
// that nothing appears, goes and comes back, that no problems show on the
// way, and that the last step matches the whole source.
func TestCorpusStreaming(t *testing.T) {
	for f, src := range corpus(t) {
		seen := map[string]string{}
		var prev *flow.Graph
		for end := 0; end <= len(src); end++ {
			g := Parse(src[:end], false)
			d := flow.Compare(prev, g)
			added := append(append(d.AddedNodes, d.AddedEdges...), d.AddedSubgraphs...)
			removed := append(append(d.RemovedNodes, d.RemovedEdges...), d.RemovedSubgraphs...)
			for _, id := range added {
				if seen[id] == "gone" {
					t.Errorf("%s: at %d (%q) %s came back", filepath.Base(f), end, tailOf(src[:end]), id)
				}
				seen[id] = "present"
			}
			for _, id := range removed {
				seen[id] = "gone"
			}
			if len(g.Problems) > 0 {
				t.Errorf("%s: at %d (%q) problems %+v", filepath.Base(f), end, tailOf(src[:end]), g.Problems)
			}
			prev = g
		}
		whole := Parse(src, true)
		if !reflect.DeepEqual(prev.NodeOrder, whole.NodeOrder) || !reflect.DeepEqual(edgeList(prev), edgeList(whole)) ||
			!reflect.DeepEqual(prev.SubgraphOrder, whole.SubgraphOrder) {
			t.Errorf("%s: the streamed graph differs from the whole one", filepath.Base(f))
		}
	}
}

// TestCorpusLayout lays out every sample, whole and streamed, to catch
// crashes.
func TestCorpusLayout(t *testing.T) {
	for f, src := range corpus(t) {
		g := Parse(src, true)
		l := sugiyama.Layout(g, measure.Fixed{}, layout.Options{})
		if l == nil {
			t.Errorf("%s: no layout", filepath.Base(f))
		}
		for end := 0; end <= len(src); end += 7 {
			sugiyama.Layout(Parse(src[:end], false), measure.Fixed{}, layout.Options{})
		}
	}
}

func tailOf(s string) string {
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return s[i+1:]
	}
	return s
}
