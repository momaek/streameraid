package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestDocument(t *testing.T) {
	a := newApp(doc)
	tt := ui.NewTester(a.view, 820, 1400)
	tt.Frame()
	tt.Advance(time.Second)
	// Labels of the diagrams are drawn as text; the Gantt chart stays code.
	for _, s := range []string{"订单服务设计说明", "API 网关", "锁定库存", "支付回调", "渠道", "62%", "gantt"} {
		if !tt.HasText(s) {
			t.Errorf("no %q in %q", s, tt.Texts())
		}
	}
	snapshot(t, tt)
}
