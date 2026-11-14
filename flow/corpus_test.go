package flow

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// corpus returns the flowcharts of testdata/corpus.
func corpus(t *testing.T) map[string]string {
	t.Helper()
	files, err := filepath.Glob("../testdata/corpus/*/*.mmd")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		if isFlowchart(src) {
			out[f] = src
		}
	}
	if len(out) == 0 {
		t.Fatal("no flowcharts in testdata/corpus")
	}
	return out
}

func isFlowchart(src string) bool {
	for _, l := range strings.Split(src, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "%%") || l == "---" || strings.Contains(l, ":") && !strings.Contains(l, "graph") && !strings.Contains(l, "flowchart") {
			continue
		}
		w, _ := firstWord(l)
		return w == "graph" || w == "flowchart" || w == "flowchart-elk" || strings.HasPrefix(w, "graph;") || strings.HasPrefix(w, "flowchart;")
	}
	return false
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

// TestCorpusStreaming streams every sample a character at a time and
// checks that nothing appears, goes and comes back, and that the last step
// matches the whole source.
func TestCorpusStreaming(t *testing.T) {
	for f, src := range corpus(t) {
		seen := map[string]string{}
		var prev *Graph
		for end := 0; end <= len(src); end++ {
			g := Parse(src[:end], false)
			d := Compare(prev, g)
			for _, id := range append(d.AddedNodes, d.AddedEdges...) {
				if seen[id] == "gone" {
					t.Errorf("%s: at %d (%q) %s came back", filepath.Base(f), end, tailOf(src[:end]), id)
				}
				seen[id] = "present"
			}
			for _, id := range append(d.RemovedNodes, d.RemovedEdges...) {
				seen[id] = "gone"
			}
			prev = g
		}
		whole := Parse(src, true)
		if !reflect.DeepEqual(prev.NodeOrder, whole.NodeOrder) || !reflect.DeepEqual(edgeList(prev), edgeList(whole)) {
			t.Errorf("%s: the streamed graph differs from the whole one", filepath.Base(f))
		}
	}
}
