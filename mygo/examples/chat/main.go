// Command chat shows a model's reply streaming into MyGo's Markdown view,
// with mermaid blocks drawn as diagrams while they are written.
//
//	go run ./examples/chat
//	go run ./examples/chat -snapshot out.png -at 0.7   # without a window
package main

import (
	"flag"
	"image/png"
	"log"
	"os"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"github.com/momaek/streameraid/mygo/mermaid"
)

const question = "能画一下用户下单到支付完成的流程吗？"

const answer = "当然可以。下面是一个典型的下单与支付流程：\n\n" +
	"```mermaid\n" +
	"flowchart TD\n" +
	"    A([用户提交订单]) --> B{库存充足?}\n" +
	"    B -->|否| C[提示库存不足]\n" +
	"    B -->|是| D[锁定库存]\n" +
	"    D --> E[创建订单<br/>状态: 待支付]\n" +
	"    E --> F[跳转支付页]\n" +
	"    F --> G{支付成功?}\n" +
	"    G -->|是| H[更新订单状态]\n" +
	"    G -->|否| I{超时 15 分钟?}\n" +
	"    I -->|否| F\n" +
	"    I -->|是| J[取消订单并释放库存]\n" +
	"    subgraph 异步处理\n" +
	"        H --> K[[发送 MQ 消息]]\n" +
	"        K --> L[(写入账务库)]\n" +
	"        K --> M[通知用户]\n" +
	"    end\n" +
	"```\n\n" +
	"几点说明：\n\n" +
	"1. **库存先锁定再下单**，避免超卖；\n" +
	"2. 支付结果以**支付平台回调**为准，前端跳转只作提示；\n" +
	"3. 订单状态变更之后的工作放进消息队列异步处理。\n"

type app struct {
	start time.Time
	pos   int
	md    *markdown.Options
	fixed bool
}

func (a *app) view(c *ui.Context) {
	now := c.Now()
	if a.start.IsZero() {
		a.start = now
	}
	if !a.fixed {
		// About 40 characters a second, as a model writes.
		a.pos = min(len(answer), int(now.Sub(a.start).Seconds()*40))
		for a.pos < len(answer) && a.pos > 0 && !utf8.RuneStart(answer[a.pos]) {
			a.pos--
		}
		if a.pos < len(answer) {
			c.After(time.Second / 30)
		}
	}
	th := c.Theme()
	ui.Column(c).Fill().Children(func() {
		ui.Scroll(c).Grow(1).AlignSelf(ui.Stretch).Padding(20).Children(func() {
			ui.Column(c).Gap(16).MaxWidth(720).AlignSelf(ui.Stretch).Children(func() {
				ui.Row(c).Children(func() {
					ui.Spacer(c)
					ui.Column(c).Padding(10, 14).Radius(12).Background(th.Surface).Children(func() {
						ui.Text(c, question)
					})
				})
				markdown.View(c, answer[:a.pos], a.md)
			})
		})
		ui.Row(c).Padding(12).Gap(8).Children(func() {
			if ui.Button(c, "重新生成").Clicked() {
				a.start, a.pos, a.fixed = now, 0, false
			}
		})
	})
}

func main() {
	snapshot := flag.String("snapshot", "", "draw one frame into this PNG instead of opening a window")
	at := flag.Float64("at", 1, "with -snapshot: how far into the reply, from 0 to 1")
	dark := flag.Bool("dark", false, "with -snapshot: draw in the dark theme")
	flag.Parse()
	a := &app{md: &markdown.Options{Code: map[string]func(*ui.Context, *markdown.CodeBlockView){
		"mermaid": mermaid.CodeBlockWith(mermaid.Options{Labels: mermaid.ChineseLabels}),
	}}}
	if *snapshot != "" {
		a.fixed = true
		a.pos = int(float64(len(answer)) * *at)
		for a.pos < len(answer) && a.pos > 0 && !utf8.RuneStart(answer[a.pos]) {
			a.pos--
		}
		tt := ui.NewTester(a.view, 800, 900)
		tt.SetDark(*dark)
		tt.Frame()
		tt.Advance(time.Second) // let what fades in finish
		f, err := os.Create(*snapshot)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, tt.Image()); err != nil {
			log.Fatal(err)
		}
		return
	}
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{Title: "streameraid chat", Width: 800, Height: 900, Content: ui.View(a.view)})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
