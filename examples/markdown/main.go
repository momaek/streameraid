// Command markdown shows a Markdown document whose mermaid code blocks are
// drawn as diagrams. It takes one hook: markdown.Options.Code maps the
// language "mermaid" to mermaid.CodeBlock. Kinds of diagrams streameraid
// does not draw yet, as the pie chart at the end, still show as code.
//
//	go run ./markdown
//	go run ./markdown path/to/notes.md   # a file of your own
package main

import (
	"log"
	"os"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"github.com/momaek/streameraid/mygo/mermaid"
)

const doc = "# 订单服务设计说明\n\n" +
	"本文用两张图说明订单服务：先看整体架构，再看一次下单请求的时序。\n\n" +
	"## 架构\n\n" +
	"```mermaid\n" +
	"graph TB\n" +
	"    subgraph 接入层\n" +
	"        GW[API 网关]\n" +
	"    end\n" +
	"    subgraph 服务层\n" +
	"        Order[订单服务]\n" +
	"        Stock[库存服务]\n" +
	"        Pay[支付服务]\n" +
	"    end\n" +
	"    subgraph 存储\n" +
	"        DB[(MySQL)]\n" +
	"        MQ[[Kafka]]\n" +
	"    end\n" +
	"    GW --> Order\n" +
	"    Order --> Stock & Pay\n" +
	"    Order --> DB\n" +
	"    Pay -.->|支付结果| MQ\n" +
	"    MQ -.-> Order\n" +
	"```\n\n" +
	"## 下单时序\n\n" +
	"```mermaid\n" +
	"sequenceDiagram\n" +
	"    autonumber\n" +
	"    actor U as 用户\n" +
	"    participant O as 订单服务\n" +
	"    participant S as 库存服务\n" +
	"    participant P as 支付服务\n" +
	"    U->>+O: 提交订单\n" +
	"    O->>+S: 锁定库存\n" +
	"    S-->>-O: 锁定成功\n" +
	"    O->>P: 创建支付单\n" +
	"    O-->>-U: 返回支付链接\n" +
	"    Note over U,P: 用户在 15 分钟内完成支付\n" +
	"    P--)O: 支付回调\n" +
	"```\n\n" +
	"## 流量占比\n\n" +
	"饼图还不支持，会按代码显示：\n\n" +
	"```mermaid\n" +
	"pie title 渠道\n" +
	"    \"App\" : 62\n" +
	"    \"Web\" : 31\n" +
	"    \"小程序\" : 7\n" +
	"```\n"

type app struct {
	src string
	md  markdown.Options
}

func newApp(src string) *app {
	a := &app{src: src}
	// The one line an app needs: mermaid blocks go to streameraid,
	// every other language keeps MyGo's own code block.
	a.md.Code = map[string]func(*ui.Context, *markdown.CodeBlockView){
		"mermaid": mermaid.CodeBlockWith(mermaid.Options{Labels: mermaid.ChineseLabels}),
	}
	return a
}

func (a *app) view(c *ui.Context) {
	ui.Scroll(c).Fill().Padding(24).Children(func() {
		ui.Column(c).MaxWidth(760).AlignSelf(ui.Stretch).Children(func() {
			markdown.View(c, a.src, &a.md)
		})
	})
}

func main() {
	src := doc
	if len(os.Args) > 1 {
		b, err := os.ReadFile(os.Args[1])
		if err != nil {
			log.Fatal(err)
		}
		src = string(b)
	}
	a := newApp(src)
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{Title: "streameraid markdown", Width: 820, Height: 900, Content: ui.View(a.view)})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
