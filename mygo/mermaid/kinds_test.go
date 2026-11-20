package mermaid

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

// kindSamples are a reply with a diagram of each kind drawn besides
// flowcharts and sequence diagrams, and texts each must show.
var kindSamples = []struct {
	name, src string
	texts     []string
}{
	{"state", "stateDiagram-v2\n    [*] --> Idle\n    Idle --> Active : power on\n    state Active {\n        [*] --> NumLockOff\n        NumLockOff --> NumLockOn : press\n        --\n        [*] --> CapsOff\n    }\n    note right of Idle : waiting\n    Active --> [*]\n",
		[]string{"Idle", "Active", "power on", "NumLockOff", "NumLockOn", "press", "CapsOff", "waiting"}},
	{"class", "classDiagram\n    class Animal {\n        <<abstract>>\n        +String name\n        +makeSound() void\n    }\n    class Owner\n    Animal <|-- Dog\n    Owner \"1\" o-- \"*\" Animal : owns\n",
		[]string{"«abstract»", "Animal", "+String name", "+makeSound() : void", "Dog", "Owner", "owns", "1", "*"}},
	{"er", "erDiagram\n    CUSTOMER ||--o{ ORDER : places\n    CUSTOMER {\n        int id PK\n        string name\n    }\n    ORDER {\n        int id PK\n        int customer_id FK\n    }\n",
		[]string{"CUSTOMER", "ORDER", "places", "int", "id", "PK", "customer_id", "FK"}},
	{"mindmap", "mindmap\n  root((学习 Go))\n    基础\n      语法\n      类型\n    并发\n      goroutine\n    工程\n      测试\n",
		[]string{"学习 Go", "基础", "语法", "类型", "并发", "goroutine", "工程", "测试"}},
	{"pie", "pie showData title Pets adopted\n    \"Dogs\" : 386\n    \"Cats\" : 85\n    \"Rats\" : 15\n",
		[]string{"Pets adopted", "Dogs", "Cats", "Rats", "386 · 79.4%", "15 · 3.1%"}},
}

// TestKinds draws a diagram of each kind in the Markdown view, in light and
// dark, and streams it in a few characters at a time.
func TestKinds(t *testing.T) {
	for _, k := range kindSamples {
		reply := "Here:\n\n```mermaid\n" + k.src + "```\n"
		for _, dark := range []bool{false, true} {
			md := hooked()
			tt := ui.NewTester(func(c *ui.Context) {
				ui.Scroll(c).Fill().Padding(16).Children(func() { markdown.View(c, reply, md) })
			}, 640, 720)
			tt.SetDark(dark)
			tt.Frame()
			tt.Advance(time.Second)
			for _, s := range k.texts {
				if !tt.HasText(s) {
					t.Errorf("%s dark=%v: text %q missing; texts %q", k.name, dark, s, tt.Texts())
				}
			}
			name := k.name + "-light"
			if dark {
				name = k.name + "-dark"
			}
			snapshot(t, tt, name)
		}
		src := ""
		md := hooked()
		tt := ui.NewTester(func(c *ui.Context) {
			ui.Scroll(c).Fill().Padding(16).Children(func() { markdown.View(c, src, md) })
		}, 640, 720)
		for i := 0; i <= len(reply); i += 4 {
			src = reply[:i]
			tt.Frame()
			tt.Advance(30 * time.Millisecond)
		}
		src = reply
		tt.Frame()
		tt.Advance(time.Second)
		if !tt.HasText(k.texts[len(k.texts)-1]) {
			t.Errorf("%s: streamed in, texts at the end %q", k.name, tt.Texts())
		}
	}
}
