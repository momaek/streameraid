package streameraid

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/momaek/streameraid/measure"
)

var update = flag.Bool("update", false, "rewrite testdata/golden")

// TestGolden renders the AI-style samples with fixed-width text, so that
// the results depend on no font, and compares them with testdata/golden.
// Run with -update after a change meant to change them, and look at the
// diff.
func TestGolden(t *testing.T) {
	files, _ := filepath.Glob("testdata/corpus/ai/*.mmd")
	if len(files) == 0 {
		t.Fatal("no samples")
	}
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		res, err := Render(string(src), Options{Measurer: measure.Fixed{}})
		if err != nil {
			t.Errorf("%s: %v", f, err)
			continue
		}
		golden := filepath.Join("testdata/golden", strings.TrimSuffix(filepath.Base(f), ".mmd")+".svg")
		if *update {
			if err := os.WriteFile(golden, []byte(res.SVG), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(golden)
		if err != nil {
			t.Errorf("%s: %v (run go test -run TestGolden -update)", golden, err)
			continue
		}
		if string(want) != res.SVG {
			t.Errorf("%s differs from %s (run go test -run TestGolden -update to accept)", f, golden)
		}
	}
}

func TestDetect(t *testing.T) {
	for src, want := range map[string]Kind{
		"flowchart TD\nA":                    Flowchart,
		"graph LR;A-->B":                     Flowchart,
		"%%{init: {}}%%\n\ngraph TD\nA":      Flowchart,
		"---\ntitle: x\n---\nflowchart LR\n": Flowchart,
		"sequenceDiagram\nA->>B: hi":         Sequence,
		"pie\n\"a\": 1":                      Pie,
		"stateDiagram-v2\n[*] --> A":         State,
		"gantt\ntitle x":                     "gantt",
		"":                                   "",
	} {
		if got := Detect(src); got != want {
			t.Errorf("Detect(%q) = %q, want %q", src, got, want)
		}
	}
}

func TestUnsupported(t *testing.T) {
	_, err := RenderSVG("gantt\n  title x", Options{})
	if !errors.Is(err, ErrUnsupported) {
		t.Errorf("err %v", err)
	}
}
