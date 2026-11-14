package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// TestReply shows a reply half-way through its mermaid block, then
// complete.
func TestReply(t *testing.T) {
	a := newChat(fakeModel)
	reply := &message{id: 2}
	a.messages = []*message{{id: 1, mine: true, text: suggestions[0], done: true}, reply}
	full := replies[0]
	reply.text = full[:strings.Index(full, "I -->|否|")]
	tt := ui.NewTester(a.view, 860, 900)
	tt.Frame()
	tt.Advance(2 * time.Second)
	tt.Frame()
	if !tt.HasText("锁定库存") || !tt.HasText("停止") {
		t.Errorf("streaming: texts %q", tt.Texts())
	}
	snapshot(t, tt)

	reply.text, reply.done = full, true
	tt.Frame()
	tt.Advance(2 * time.Second)
	tt.Frame()
	if !tt.HasText("取消订单并释放库存") || !tt.HasText("发送") {
		t.Errorf("done: texts %q", tt.Texts())
	}
}

func TestFakeModel(t *testing.T) {
	var b strings.Builder
	if err := fakeModel(context.Background(), "OAuth 时序", func(s string) { b.WriteString(s) }); err != nil {
		t.Fatal(err)
	}
	if b.String() != replies[1] {
		t.Errorf("the chunks do not add up to the reply")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := fakeModel(ctx, "x", func(string) {}); err == nil {
		t.Errorf("a canceled stream should fail")
	}
}
