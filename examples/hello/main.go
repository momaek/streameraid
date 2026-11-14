// Command hello is the smallest MyGo app that draws a Mermaid diagram:
// one window, one mermaid.View.
//
//	go run ./hello
package main

import (
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/momaek/streameraid/mygo/mermaid"
)

const diagram = `flowchart LR
    A([开始]) --> B[读取配置]
    B --> C{配置有效?}
    C -->|是| D[启动服务]
    C -->|否| E[打印错误并退出]
    D --> F([运行中])
`

func view(c *ui.Context) {
	ui.Scroll(c).Fill().Padding(24).Children(func() {
		// The source is complete, so Streaming stays false: the diagram is
		// laid out once and scaled down to fit if it is too wide.
		mermaid.View(c, diagram, mermaid.Options{Labels: mermaid.ChineseLabels})
	})
}

func main() {
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{Title: "hello streameraid", Width: 760, Height: 320, Content: ui.View(view)})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
