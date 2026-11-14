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
	// Labels of both diagrams are drawn as text; the pie chart stays code.
	for _, s := range []string{"订单服务设计说明", "API 网关", "锁定库存", "支付回调", "pie title 渠道"} {
		if !tt.HasText(s) {
			t.Errorf("no %q in %q", s, tt.Texts())
		}
	}
	snapshot(t, tt)
}
