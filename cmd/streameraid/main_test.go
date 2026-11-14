package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const flowSrc = "flowchart TD\n    A[Start] --> B[End]\n"

func runWith(t *testing.T, stdin string, args ...string) (status int, stdout, stderr string) {
	t.Helper()
	var out, errb bytes.Buffer
	status = run(args, strings.NewReader(stdin), &out, &errb)
	return status, out.String(), errb.String()
}

func TestRenderStdin(t *testing.T) {
	status, out, errs := runWith(t, flowSrc, "render")
	if status != 0 || errs != "" {
		t.Fatalf("status %d, stderr %q", status, errs)
	}
	if !strings.HasPrefix(out, "<svg") || !strings.Contains(out, "Start") {
		t.Errorf("output %.80q", out)
	}
}

func TestRenderFileAndFlagsInAnyOrder(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.mmd")
	if err := os.WriteFile(in, []byte(flowSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	for i, args := range [][]string{
		{"render", "-o", filepath.Join(dir, "a.svg"), "-dark", in},
		{"render", in, "-o", filepath.Join(dir, "b.svg"), "-dark"},
		{"render", "-dark", in, "-o", filepath.Join(dir, "c.svg"), "-bg", "#000"},
	} {
		status, out, errs := runWith(t, "", args...)
		if status != 0 || out != "" || errs != "" {
			t.Errorf("%d: status %d, stdout %.40q, stderr %q", i, status, out, errs)
		}
	}
	a, _ := os.ReadFile(filepath.Join(dir, "a.svg"))
	b, _ := os.ReadFile(filepath.Join(dir, "b.svg"))
	c, _ := os.ReadFile(filepath.Join(dir, "c.svg"))
	if len(a) == 0 || !bytes.Equal(a, b) {
		t.Errorf("flags before and after the file gave different files (%d and %d bytes)", len(a), len(b))
	}
	if !bytes.Contains(c, []byte(`fill="#000"`)) {
		t.Errorf("-bg after the file was not applied")
	}
}

func TestRenderSequence(t *testing.T) {
	status, out, _ := runWith(t, "sequenceDiagram\n    A->>B: hello\n", "render", "-")
	if status != 0 || !strings.Contains(out, "hello") {
		t.Errorf("status %d, output %.80q", status, out)
	}
}

func TestProblemsAreReported(t *testing.T) {
	status, out, errs := runWith(t, "flowchart TD\n    A --> B\n    A -->> ???\n", "render")
	if status != 0 || !strings.HasPrefix(out, "<svg") {
		t.Errorf("status %d: the rest of the diagram should still be drawn", status)
	}
	if !strings.Contains(errs, "line 3") {
		t.Errorf("stderr %q", errs)
	}
}

func TestErrors(t *testing.T) {
	for _, c := range []struct {
		stdin  string
		args   []string
		status int
		want   string
	}{
		{"", nil, 2, "usage"},
		{"", []string{"draw"}, 2, "unknown command"},
		{"", []string{"render", "a.mmd", "b.mmd"}, 2, "one input file"},
		{"", []string{"render", "-nope"}, 2, "usage"},
		{"pie\n\"a\": 1\n", []string{"render"}, 1, "unsupported"},
		{"", []string{"render", "/no/such/file.mmd"}, 1, "no such file"},
		{"", []string{"help"}, 0, "usage"},
	} {
		status, _, errs := runWith(t, c.stdin, c.args...)
		if status != c.status || !strings.Contains(errs, c.want) {
			t.Errorf("%v: status %d, stderr %q; want %d and %q", c.args, status, errs, c.status, c.want)
		}
	}
}
