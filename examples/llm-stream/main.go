// Command llm-stream is a chat whose replies stream in from a model, with
// mermaid blocks drawn while the model is still writing them.
//
// It shows how to wire a real model in: a goroutine reads the stream and
// hands each chunk to the window with Window.Update; the reply shows with
// markdown.Stream while it comes and markdown.View once it is done;
// mermaid.CodeBlock tells from the block whether it is still open. The
// model here is a fake one, fakeModel: replace it with your client's
// streaming call.
//
//	go run ./llm-stream
package main

import (
	"context"
	"log"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"github.com/momaek/streameraid/mygo/mermaid"
)

// Model streams a reply to prompt, calling emit with each chunk of text as
// it arrives, from any goroutine. It returns once the reply is complete or
// ctx is canceled.
type Model func(ctx context.Context, prompt string, emit func(chunk string)) error

type message struct {
	id   int
	mine bool
	text string
	done bool
	err  error
}

type chat struct {
	win      *mygo.Window
	model    Model
	messages []*message
	input    string
	list     ui.ListState
	md       markdown.Options
	next     int
	cancel   context.CancelFunc
}

func newChat(m Model) *chat {
	a := &chat{model: m}
	a.md.Code = map[string]func(*ui.Context, *markdown.CodeBlockView){
		"mermaid": mermaid.CodeBlockWith(mermaid.Options{Labels: mermaid.ChineseLabels}),
	}
	return a
}

func (a *chat) streaming() bool {
	return len(a.messages) > 0 && !a.messages[len(a.messages)-1].done
}

func (a *chat) view(c *ui.Context) {
	th := c.Theme()
	a.list.Key = func(i int) any { return a.messages[i].id }
	a.list.FollowEnd = true // keep the end in view as the reply grows
	ui.Column(c).Fill().Children(func() {
		ui.List(c, &a.list, len(a.messages), func(i int) {
			a.message(c, a.messages[i])
		}).Grow(1).Padding(16, 20).Gap(16).Justify(ui.End).Children(func() {
			if len(a.messages) == 0 {
				ui.Column(c).Gap(8).Margin(ui.Auto).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "试试这些问题：").TextColor(th.TextMuted)
					for _, q := range suggestions {
						if ui.Button(c, q).Clicked() {
							a.input = q
							a.send()
						}
					}
				})
			}
		})
		ui.Row(c).Gap(8).Padding(12, 20).BorderWidth(1, 0, 0, 0).BorderColor(th.Border).Children(func() {
			in := ui.TextInput(c, &a.input).Placeholder("问点什么，比如“画一下 OAuth 登录的时序”").Grow(1).AutoFocus()
			empty := strings.TrimSpace(a.input) == ""
			if a.streaming() {
				if ui.Button(c, "停止").Clicked() && a.cancel != nil {
					a.cancel()
				}
			} else if (ui.PrimaryButton(c, "发送").Disabled(empty).Clicked() || in.Submitted()) && !empty {
				a.send()
			}
		})
	})
}

func (a *chat) message(c *ui.Context, m *message) {
	th := c.Theme()
	if m.mine {
		ui.Row(c).Children(func() {
			ui.Spacer(c)
			ui.Text(c, m.text).Selectable().Padding(8, 12).Radius(14).Background(th.Surface).MaxWidth(520)
		})
		return
	}
	ui.Column(c).MaxWidth(760).AlignSelf(ui.Stretch).Gap(6).Children(func() {
		// Stream while the reply comes, View once it is done, in the same
		// place: the parse carries on, and the open mermaid block, once its
		// fence closes, is laid out once more and fitted to the width.
		if m.done {
			markdown.View(c, m.text, &a.md)
		} else {
			markdown.Stream(c, m.text, &a.md)
		}
		if m.err != nil {
			ui.Text(c, "出错了："+m.err.Error()).TextColor(th.Danger)
		}
	})
}

// send sends the input and streams the model's reply in.
func (a *chat) send() {
	prompt := strings.TrimSpace(a.input)
	a.input = ""
	a.next++
	a.messages = append(a.messages, &message{id: a.next, mine: true, text: prompt, done: true})
	a.next++
	reply := &message{id: a.next}
	a.messages = append(a.messages, reply)

	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	win := a.win
	go func() {
		defer cancel()
		err := a.model(ctx, prompt, func(chunk string) {
			// The UI's state is the main thread's: hand it each chunk.
			win.Update(func() { reply.text += chunk })
		})
		win.Update(func() {
			reply.done = true
			if err != nil && ctx.Err() == nil {
				reply.err = err
			}
		})
	}()
}

var suggestions = []string{
	"画一下用户下单到支付的流程",
	"OAuth 授权码模式的时序是怎样的？",
}

// fakeModel answers with canned replies, in chunks of a few bytes at the
// uneven pace of a model.
func fakeModel(ctx context.Context, prompt string, emit func(string)) error {
	reply := replies[0]
	if strings.Contains(prompt, "时序") || strings.Contains(strings.ToLower(prompt), "oauth") {
		reply = replies[1]
	}
	for i := 0; i < len(reply); {
		n := min(len(reply)-i, 2+rand.IntN(6))
		for i+n < len(reply) && reply[i+n]&0xC0 == 0x80 { // not inside a UTF-8 sequence
			n++
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(10+rand.IntN(30)) * time.Millisecond):
		}
		emit(reply[i : i+n])
		i += n
	}
	return nil
}

var replies = []string{
	"好的，下面是一个典型的下单与支付流程：\n\n" +
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
		"```\n\n" +
		"要点：**先锁库存再下单**，避免超卖；支付结果以支付平台的回调为准。\n",
	"OAuth 2.0 授权码模式大致如下：\n\n" +
		"```mermaid\n" +
		"sequenceDiagram\n" +
		"    autonumber\n" +
		"    actor U as 用户\n" +
		"    participant C as 客户端\n" +
		"    participant A as 授权服务器\n" +
		"    participant R as 资源服务器\n" +
		"    U->>C: 点击“用 GitHub 登录”\n" +
		"    C->>A: 重定向到授权页\n" +
		"    A->>U: 请求授权\n" +
		"    U->>A: 同意\n" +
		"    A-->>C: 回调，带上授权码\n" +
		"    C->>+A: 用授权码换令牌\n" +
		"    A-->>-C: access_token\n" +
		"    loop 每次调用接口\n" +
		"        C->>+R: 带令牌请求\n" +
		"        R-->>-C: 数据\n" +
		"    end\n" +
		"```\n\n" +
		"授权码只经过浏览器一次，令牌由客户端在后端换取，不会暴露给浏览器。\n",
}

func main() {
	a := newChat(fakeModel)
	mygo.App.WhenReady(func() {
		a.win = mygo.NewWindow(mygo.WindowOptions{Title: "streameraid llm-stream", Width: 860, Height: 860, Content: ui.View(a.view)})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
