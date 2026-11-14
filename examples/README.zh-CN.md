# 示例

几个小的 MyGo 应用，每个演示一种 streameraid 的用法。它们是单独的 Go module，这样核心包不必依赖 MyGo。在本目录下运行：

```sh
go run ./hello        # 最小的应用：一个窗口，一个 mermaid.View
go run ./markdown     # Markdown 文档里的 mermaid 代码块画成图
go run ./llm-stream   # 聊天：模型的回复流式输出，图边写边画
go run ./node-click   # OnNodeClick 打开节点详情，OnExport 保存 SVG
go run ./editor       # 实时编辑器：左边输入，右边出图
```

| 示例 | 演示什么 |
| --- | --- |
| [hello](hello/main.go) | 用 `mermaid.View` 画一段完整的源码。 |
| [markdown](markdown/main.go) | 通过 `markdown.Options.Code` 挂上 `mermaid.CodeBlockWith`。还不支持的图（饼图）照常显示为代码。`go run ./markdown notes.md` 可以打开自己的文件。 |
| [llm-stream](llm-stream/main.go) | 接入流式输出的模型：goroutine 读取输出，用 `Window.Update` 把每块文本交给窗口；回复输出中用 `markdown.Stream`，完成后用 `markdown.View`；“停止”按钮取消请求。模型是假的：把 `fakeModel` 换成你的客户端的流式调用即可。 |
| [node-click](node-click/main.go) | 用 `OnNodeClick` 让图成为应用的一部分，用 `OnExport` 通过保存对话框导出 SVG。 |
| [editor](editor/main.go) | 输入时设 `Streaming`，停下后取消，图先保持不动、再重新排好；可以试 `FontSize`、`NoFit`、`NoToolbar` 和 `Labels`；用核心包的 `streameraid.RenderSVG` 保存。 |

更大的演示（录制 README 动图的 playground 和一个聊天示例）在 [`../mygo/examples`](../mygo/examples)。

每个示例都有一个不开窗口就能画出来的测试。想看看画出来的样子：

```sh
mkdir -p /tmp/shots && EXAMPLES_SNAPSHOTS=/tmp/shots go test ./...
```
