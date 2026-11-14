package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestTemplates(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.view, 1100, 720)
	tt.Frame()
	tt.Advance(time.Second)
	if !tt.HasText("命中缓存?") {
		t.Fatalf("texts %q", tt.Texts())
	}
	if err := tt.Click("时序图"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Advance(time.Second)
	if !tt.HasText("GET /items") {
		t.Errorf("after choosing the sequence template: texts %q", tt.Texts())
	}
	snapshot(t, tt)
}

// TestTyping checks that an edit counts as streaming until the pause.
func TestTyping(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.view, 1100, 720)
	tt.Frame()
	a.src += "    C --> F[记录日志]\n"
	a.lastEdit = tt.Now()
	tt.Frame()
	if !tt.HasText("输入中…") {
		t.Errorf("right after an edit: texts %q", tt.Texts())
	}
	tt.Advance(time.Second)
	tt.Frame()
	if !tt.HasText("已完成") || !tt.HasText("记录日志") {
		t.Errorf("after the pause: texts %q", tt.Texts())
	}
}
