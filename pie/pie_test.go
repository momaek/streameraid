package pie

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	c := Parse("pie showData title Key elements\n    \"Calcium\" : 42.96\n    'Potassium' : 50.05\n    \"Iron\" :  5\n", true)
	if !c.HasHeader || !c.ShowData || c.Title != "Key elements" || len(c.Problems) > 0 {
		t.Fatalf("%+v", c)
	}
	if len(c.Slices) != 3 || c.Slices[1].Label != "Potassium" || c.Slices[2].Value != 5 || c.Slices[2].ID != "s2" {
		t.Fatalf("%+v", c.Slices)
	}
	c = Parse("pie\ntitle Pets\n\"Dogs\": 386\n\"Cats\": -2\n\"Rats\": many\nnonsense\n", true)
	if c.Title != "Pets" || len(c.Slices) != 1 || len(c.Problems) != 3 {
		t.Fatalf("%+v", c)
	}
}

func TestStreaming(t *testing.T) {
	src := "pie title Pets\n\"Dogs\" : 386\n\"Cats\" : 85\n\"Rats\" : 15\n"
	for end := 0; end < len(src); end++ {
		c := Parse(src[:end], false)
		if len(c.Problems) > 0 {
			t.Fatalf("at %d: %+v", end, c.Problems)
		}
		for _, s := range c.Slices {
			if want := Parse(src, true).Slices[len(c.Slices)-1]; s.ID == want.ID && s.Value != want.Value {
				t.Fatalf("at %d: slice %s shows %v before its value is complete", end, s.ID, s.Value)
			}
		}
	}
}

func TestCorpus(t *testing.T) {
	files, _ := filepath.Glob("../testdata/corpus/*/pie-*.mmd")
	if len(files) == 0 {
		t.Fatal("no pie charts in testdata/corpus")
	}
	for _, f := range files {
		b, _ := os.ReadFile(f)
		c := Parse(string(b), true)
		if len(c.Problems) > 0 || len(c.Slices) == 0 {
			t.Errorf("%s: %+v", filepath.Base(f), c)
		}
		for end := range len(b) {
			if c := Parse(string(b[:end]), false); len(c.Problems) > 0 && !strings.Contains(c.Problems[0].Msg, "header") {
				t.Errorf("%s: at %d %+v", filepath.Base(f), end, c.Problems)
				break
			}
		}
	}
}

func FuzzParse(f *testing.F) {
	f.Add("pie showData title x\n\"a\" : 1\n'b' : 2.5\n")
	f.Add("---\ntitle: t\n---\npie\n\"a\":")
	f.Fuzz(func(t *testing.T, src string) {
		Parse(src, false)
		Parse(src, true)
	})
}
