// Command playground types Mermaid sources out a few characters at a time,
// as a model writes them, and draws each diagram as it grows.
//
//	go run ./examples/playground
//	go run ./examples/playground -snapshot out.png -at 0.6   # without a window
package main

import (
	"flag"
	"fmt"
	"image/png"
	"log"
	"os"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/momaek/streameraid/mygo/mermaid"
)

type sample struct{ name, src string }

var samples = []sample{
	{"登录流程", `flowchart TD
    A[用户访问页面] --> B{是否已登录?}
    B -->|是| C[显示主页]
    B -->|否| D[跳转登录页]
    D --> E[输入用户名和密码]
    E --> F{验证通过?}
    F -->|是| G[生成 Token]
    G --> C
    F -->|否| H[显示错误信息]
    H --> E
`},
	{"架构", `graph TB
    subgraph Client["客户端"]
        Web[Web 应用]
        Mobile[移动端 App]
    end
    subgraph Services["微服务"]
        Auth[认证服务]
        Order[订单服务]
        Pay[支付服务]
    end
    subgraph Data["数据层"]
        DB[(MySQL)]
        Cache[(Redis)]
        MQ[[Kafka]]
    end
    Web --> GW[API 网关]
    Mobile --> GW
    GW --> Auth & Order & Pay
    Order --> DB
    Order --> Cache
    Order --> MQ
    Pay --> MQ
    Auth --> Cache
`},
	{"CI/CD", `flowchart LR
    A[Push to GitHub] --> B[Run Tests]
    B --> C{Tests Pass?}
    C -->|Yes| D[Build Docker Image]
    C -->|No| E[Notify Developer]
    D --> F[Push to Registry]
    F --> G[Deploy to Staging]
    G --> H{Manual Approval}
    H -->|Approved| I[Deploy to Production]
    H -->|Rejected| E
`},
	{"时序图", `sequenceDiagram
    autonumber
    actor U as 用户
    participant F as 前端
    participant B as 后端
    participant D as 数据库

    U->>F: 输入用户名和密码
    F->>+B: POST /api/login
    B->>+D: 查询用户信息
    D-->>-B: 返回用户记录
    Note right of B: 校验密码哈希
    alt 密码正确
        B-->>F: 200 OK + Token
        F-->>U: 跳转到首页
    else 密码错误
        B-->>-F: 401 Unauthorized
        F-->>U: 显示错误提示
    end
`},
	{"Order (EN)", `flowchart TD
    A([Customer places order]) --> B{In stock?}
    B -->|No| C[Notify customer]
    B -->|Yes| D[Charge payment]
    D --> E{Paid?}
    E -->|No| C
    E -->|Yes| F[Create shipment]
    subgraph Fulfilment
        F --> G[[Pick and pack]]
        G --> H[(Update inventory)]
        G --> I[Hand to courier]
    end
`},
	{"Login (EN)", `sequenceDiagram
    autonumber
    actor U as User
    participant F as Frontend
    participant B as Backend
    participant D as Database

    U->>F: Enter name and password
    F->>+B: POST /api/login
    B->>+D: Look up the user
    D-->>-B: User record
    Note right of B: Check the password hash
    alt Password matches
        B-->>F: 200 OK + token
        F-->>U: Go to the home page
    else Wrong password
        B-->>-F: 401 Unauthorized
        F-->>U: Show an error
    end
`},
	{"Agent", `flowchart TD
    U[User request] --> P[Planner]
    P --> T{Need a tool?}
    T -- yes --> S[Select tool]
    S --> X[Execute tool]
    X --> O[Observe result]
    O --> P
    T -- no --> R[Compose final answer]
    R --> U2([Reply to user])
    classDef io fill:#e1f5fe,stroke:#0277bd
    class U,U2 io
`},
}

type app struct {
	sample  int
	start   time.Time
	speed   float64 // characters a second
	pos     int
	stopped bool
	bare    bool // show the source and the diagram without the controls
}

func (a *app) restart(now time.Time) {
	a.start, a.pos, a.stopped = now, 0, false
}

func (a *app) view(c *ui.Context) {
	now := c.Now()
	if a.start.IsZero() {
		a.restart(now)
	}
	src := samples[a.sample].src
	if !a.stopped {
		a.pos = min(len(src), int(now.Sub(a.start).Seconds()*a.speed))
		for a.pos < len(src) && a.pos > 0 && !utf8.RuneStart(src[a.pos]) {
			a.pos--
		}
		if a.pos < len(src) {
			c.After(time.Second / 60)
		}
	}
	shown := src[:a.pos]
	streaming := a.pos < len(src)
	th := c.Theme()
	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		side := float32(380)
		if a.bare {
			side = 330
		}
		ui.Column(c).Width(side).Padding(14).Gap(10).Background(th.Surface).Children(func() {
			if a.bare {
				// Recording: the source and the diagram alone.
				ui.Scroll(c).Grow(1).AlignSelf(ui.Stretch).Children(func() {
					ui.Text(c, shown).Font("monospace").FontSize(12)
				})
				return
			}
			ui.Row(c).Gap(6).Wrap().Children(func() {
				for i, s := range samples {
					if button(c, s.name, i == a.sample).Clicked() {
						a.sample = i
						a.restart(now)
					}
				}
			})
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				if ui.Button(c, "重播").Clicked() {
					a.restart(now)
				}
				if ui.Button(c, "直接显示全部").Clicked() {
					a.pos, a.stopped = len(src), true
				}
				for _, sp := range []float64{20, 60, 200} {
					if button(c, fmt.Sprintf("%.0f 字/秒", sp), a.speed == sp).Clicked() {
						a.speed = sp
						a.restart(now)
					}
				}
			})
			state := "完成"
			if streaming {
				state = fmt.Sprintf("输出中 %d/%d", a.pos, len(src))
			}
			ui.Text(c, state).TextColor(th.TextMuted)
			ui.Scroll(c).Grow(1).AlignSelf(ui.Stretch).Children(func() {
				ui.Text(c, shown).Font("monospace").Selectable()
			})
		})
		ui.Scroll(c).Grow(1).Padding(20).Children(func() {
			mermaid.View(c, shown, mermaid.Options{Streaming: streaming, Labels: mermaid.ChineseLabels, NoToolbar: a.bare})
		})
	})
}

// button is a push button, primary when selected.
func button(c *ui.Context, label string, selected bool) ui.Element {
	if selected {
		return ui.PrimaryButton(c, label)
	}
	return ui.Button(c, label)
}

func main() {
	snapshot := flag.String("snapshot", "", "draw one frame into this PNG instead of opening a window")
	at := flag.Float64("at", 1, "with -snapshot: how far into the source, from 0 to 1")
	which := flag.Int("sample", 0, "the sample to start with")
	dark := flag.Bool("dark", false, "with -snapshot or -gif: draw in the dark theme")
	gifOut := flag.String("gif", "", "play the sample without a window and write it as an animated GIF")
	width := flag.Int("width", 860, "with -gif: the width of the picture")
	height := flag.Int("height", 520, "with -gif: the height of the picture")
	flag.Parse()
	a := &app{speed: 60, sample: *which % len(samples)}
	if *gifOut != "" {
		a.speed = 40
		if err := record(a, *gifOut, *width, *height, *dark); err != nil {
			log.Fatal(err)
		}
		return
	}
	if *snapshot != "" {
		src := samples[a.sample].src
		a.stopped = true
		a.start = time.Now()
		a.pos = int(float64(len(src)) * *at)
		for a.pos < len(src) && a.pos > 0 && !utf8.RuneStart(src[a.pos]) {
			a.pos--
		}
		tt := ui.NewTester(a.view, 1100, 720)
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
		mygo.NewWindow(mygo.WindowOptions{
			Title: "streameraid playground", Width: 1100, Height: 720,
			Content: ui.View(a.view),
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
