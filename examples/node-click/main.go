// Command node-click makes a diagram part of the app: clicking a node of
// the flowchart opens its details beside it (Options.OnNodeClick), and
// the toolbar's SVG button saves the diagram to a file of the user's
// choice (Options.OnExport).
//
//	go run ./node-click
package main

import (
	"log"
	"os"
	"strconv"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/momaek/streameraid/mygo/mermaid"
)

const topology = `flowchart LR
    gw[API 网关] --> auth[认证服务]
    gw --> order[订单服务]
    order --> stock[库存服务]
    order --> pay[支付服务]
    order --> db[(订单库)]
    pay -.-> mq[[消息队列]]
    mq -.-> notify[通知服务]
    classDef hot fill:#fde2e1,stroke:#d93025
    class pay hot
`

type service struct {
	name, owner, status string
	qps                 int
}

// services are the details of the nodes, by node ID.
var services = map[string]service{
	"gw":     {"API 网关", "基础架构组", "正常", 5200},
	"auth":   {"认证服务", "账号组", "正常", 1800},
	"order":  {"订单服务", "交易组", "正常", 950},
	"stock":  {"库存服务", "交易组", "正常", 900},
	"pay":    {"支付服务", "支付组", "延迟升高", 420},
	"db":     {"订单库", "DBA", "正常", 3100},
	"mq":     {"消息队列", "基础架构组", "正常", 2600},
	"notify": {"通知服务", "增长组", "正常", 380},
}

type app struct {
	win      *mygo.Window
	selected string // the ID of the node clicked last
	status   string // what became of the last export
	opt      mermaid.Options
}

func newApp() *app {
	a := &app{}
	a.opt = mermaid.Options{
		Labels: mermaid.ChineseLabels,
		// Called on the main thread, during a frame: change the state and
		// the next frame shows it.
		OnNodeClick: func(id string) { a.selected = id },
		OnExport:    a.export,
	}
	return a
}

// export asks where to save svg and writes it there. The dialog waits for
// the user, so it runs on a goroutine of its own, never on the main thread.
func (a *app) export(svg string) {
	go func() {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Title:       "导出 SVG",
			DefaultPath: "topology.svg",
			Filters:     []mygo.FileFilter{{Name: "SVG", Extensions: []string{"svg"}}},
		})
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

func (a *app) view(c *ui.Context) {
	th := c.Theme()
	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		ui.Column(c).Grow(1).Padding(20).Gap(10).Children(func() {
			ui.Text(c, "点击节点查看服务详情；鼠标移到图上，右上角的 SVG 按钮可以导出。").TextColor(th.TextMuted)
			mermaid.View(c, topology, a.opt)
			if a.status != "" {
				ui.Text(c, a.status).TextColor(th.TextMuted)
			}
		})
		ui.Column(c).Width(240).Padding(20).Gap(8).Background(th.Surface).Children(func() {
			s, ok := services[a.selected]
			if !ok {
				ui.Text(c, "未选择服务").TextColor(th.TextMuted)
				return
			}
			ui.Text(c, s.name).FontSize(18).Bold()
			ui.Text(c, "节点 ID："+a.selected).Font("monospace").TextColor(th.TextMuted)
			ui.Text(c, "负责团队："+s.owner)
			ui.Text(c, "状态："+s.status)
			ui.Text(c, "QPS："+strconv.Itoa(s.qps))
			if ui.Button(c, "关闭").Clicked() {
				a.selected = ""
			}
		})
	})
}

func main() {
	a := newApp()
	mygo.App.WhenReady(func() {
		a.win = mygo.NewWindow(mygo.WindowOptions{Title: "streameraid node-click", Width: 1000, Height: 520, Content: ui.View(a.view)})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
