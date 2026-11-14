package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestClickNode(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.view, 1000, 520)
	tt.Frame()
	tt.Advance(time.Second)
	if !tt.HasText("未选择服务") {
		t.Fatalf("texts %q", tt.Texts())
	}
	if err := tt.Click("支付服务"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.selected != "pay" || !tt.HasText("状态：延迟升高") {
		t.Errorf("selected %q, texts %q", a.selected, tt.Texts())
	}
	snapshot(t, tt)
}
