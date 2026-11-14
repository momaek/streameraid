// Command editor is a live Mermaid editor: the source on the left, the
// diagram on the right, redrawn as you type.
//
// While the user types, the view is told the source is still streaming,
// so the diagram holds still and grows; once they pause, it is laid out
// once more and fitted. The controls on the left try the view's options,
// and "保存 SVG" renders the source with the core package,
// streameraid.RenderSVG, which needs no window.
//
//	go run ./editor
package main

import (
	"log"
	"os"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/momaek/streameraid"
	"github.com/momaek/streameraid/mygo/mermaid"
)

// pause is how long after the last key the source counts as complete.
const pause = 700 * time.Millisecond

var templates = []struct{ name, src string }{
	{"流程图", `flowchart TD
    A[收到请求] --> B{命中缓存?}
    B -->|是| C[返回缓存]
    B -->|否| D[查询数据库]
    D --> E[写入缓存]
    E --> C
`},
	{"时序图", `sequenceDiagram
    participant C as 客户端
    participant S as 服务端
    C->>+S: GET /items
    S-->>-C: 200 OK
    Note over C,S: 连接保持
`},
	{"子图", `flowchart LR
    subgraph 构建
        A[拉取代码] --> B[编译] --> C[单元测试]
    end
    subgraph 发布
        D[打镜像] --> E[灰度] --> F[全量]
    end
    C --> D
`},
}

type app struct {
	win      *mygo.Window
	src      string
	template int
	lastEdit time.Time

	fontSize  float64
	noFit     bool
	noToolbar bool
	english   bool
	status    string
}

func newApp() *app {
	return &app{src: templates[0].src, fontSize: 14}
}

func (a *app) view(c *ui.Context) {
	th := c.Theme()
	now := c.Now()
	// Typing: the source is "streaming" until the user pauses. Ask for a
	// frame when the pause is over, to draw it complete.
	streaming := !a.lastEdit.IsZero() && now.Sub(a.lastEdit) < pause
	if streaming {
		c.After(pause - now.Sub(a.lastEdit))
	}
	labels := mermaid.ChineseLabels
	if a.english {
		labels = mermaid.Labels{}
	}
	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		ui.Column(c).Width(380).Padding(14).Gap(10).Background(th.Surface).Children(func() {
			prev := a.template
			ui.Segmented(c, &a.template, names()...).Label("模板")
			if a.template != prev {
				a.src, a.lastEdit = templates[a.template].src, time.Time{}
			}
			if ui.TextArea(c, &a.src).Font("monospace").Grow(1).AlignSelf(ui.Stretch).Changed() {
				a.lastEdit = now
			}
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "字号")
				ui.Slider(c, &a.fontSize, 10, 22).Grow(1).Label("字号")
			})
			ui.Checkbox(c, &a.noFit, "不缩放（NoFit）")
			ui.Checkbox(c, &a.noToolbar, "隐藏工具栏（NoToolbar）")
			ui.Checkbox(c, &a.english, "英文界面文字（Labels）")
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				if ui.Button(c, "保存 SVG").Clicked() {
					a.save(th.Dark)
				}
				state := "已完成"
				if streaming {
					state = "输入中…"
				}
				ui.Text(c, state).TextColor(th.TextMuted)
			})
			if a.status != "" {
				ui.Text(c, a.status).TextColor(th.TextMuted)
			}
		})
		ui.Scroll(c).Grow(1).Padding(20).Children(func() {
			mermaid.View(c, a.src, mermaid.Options{
				Streaming: streaming,
				FontSize:  float32(a.fontSize),
				NoFit:     a.noFit,
				NoToolbar: a.noToolbar,
				Labels:    labels,
			})
		})
	})
}

func names() []string {
	var s []string
	for _, t := range templates {
		s = append(s, t.name)
	}
	return s
}

// save renders the source with the core package and writes it where the
// user chooses.
func (a *app) save(dark bool) {
	svg, err := streameraid.RenderSVG(a.src, streameraid.Options{Dark: dark, FontSize: a.fontSize})
	if err != nil {
		a.status = "无法导出：" + err.Error()
		return
	}
	go func() {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{Title: "保存 SVG", DefaultPath: "diagram.svg"})
		if err != nil || path == "" {
			return
		}
		status := "已保存到 " + path
		if err := os.WriteFile(path, []byte(svg), 0o644); err != nil {
			status = "保存失败：" + err.Error()
		}
		a.win.Update(func() { a.status = status })
	}()
}

func main() {
	a := newApp()
	mygo.App.WhenReady(func() {
		a.win = mygo.NewWindow(mygo.WindowOptions{Title: "streameraid editor", Width: 1100, Height: 720, Content: ui.View(a.view)})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
