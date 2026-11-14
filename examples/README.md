# Examples

Small MyGo apps, each showing one way to use streameraid. They are a Go module of their own, so that the core keeps no dependency on MyGo. Run them from this directory:

```sh
go run ./hello        # the smallest app: one window, one mermaid.View
go run ./markdown     # a Markdown document whose mermaid blocks are drawn
go run ./llm-stream   # a chat: replies stream in from a model, diagrams draw as they are written
go run ./node-click   # OnNodeClick opens a node's details; OnExport saves the SVG
go run ./editor       # a live editor: type on the left, the diagram follows on the right
```

| Example | What it shows |
| --- | --- |
| [hello](hello/main.go) | `mermaid.View` with a complete source. |
| [markdown](markdown/main.go) | `markdown.Options.Code` with `mermaid.CodeBlockWith`. A diagram streameraid does not draw yet (a pie chart) stays code. `go run ./markdown notes.md` opens a file of your own. |
| [llm-stream](llm-stream/main.go) | Wiring a streaming model in: a goroutine hands chunks to the window with `Window.Update`; `markdown.Stream` while the reply comes, `markdown.View` once done; a Stop button cancels the stream. The model is a fake one: replace `fakeModel` with your client's streaming call. |
| [node-click](node-click/main.go) | `OnNodeClick` to make the diagram part of the app, and `OnExport` to save the SVG through a save dialog. |
| [editor](editor/main.go) | `Streaming` while the user types and false once they pause, so the diagram holds still and then settles; `FontSize`, `NoFit`, `NoToolbar` and `Labels` to try; saving with the core `streameraid.RenderSVG`. |

The bigger demos, the playground that records the GIFs of the README and a chat, are in [`../mygo/examples`](../mygo/examples).

Each example has a test that draws it without a window. To look at what they draw:

```sh
mkdir -p /tmp/shots && EXAMPLES_SNAPSHOTS=/tmp/shots go test ./...
```
