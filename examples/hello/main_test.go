package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestView(t *testing.T) {
	tt := ui.NewTester(view, 760, 320)
	tt.Frame()
	tt.Advance(time.Second)
	for _, s := range []string{"开始", "配置有效?", "打印错误并退出"} {
		if !tt.HasText(s) {
			t.Errorf("no %q in %q", s, tt.Texts())
		}
	}
	snapshot(t, tt)
}
