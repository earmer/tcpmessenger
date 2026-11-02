# TCP Messenger — shared brief for every subagent

We are building **TCP Messenger**, a working LAN messenger modelled on IP Messenger
(rebranded: "TCP Messenger" / "TCPMsg"). It is a mygo **native UI** app (Go only,
package `github.com/egoist/mygo/ui`, no webview) at
`/Users/earmercarey/tcpmessenger/my-app` (module name `my-app`, mygo v0.7.4, Go 1.27).

## References
- mygo docs (cloned): `/private/tmp/claude-501/-Users-earmercarey-tcpmessenger/a127bc5f-17ae-4220-87d7-6f3a20c47508/scratchpad/mygo-src/docs/`
  (start at `README.md`, `ui/README.md`, `ui/views.md`, `windows.md`, `menus.md`, `native.md`; there
  is a page per widget in `docs/ui/`). Examples in `.../mygo-src/examples/` (e.g. `gallery`, `todo`,
  `native`). The Go API: `go doc -all github.com/egoist/mygo/ui` inside the module. Read the docs
  and check signatures with `go doc` — do not guess APIs.
- Design mockups of every window (HTML; read for layout, labels, controls):
  `/private/tmp/claude-501/-Users-earmercarey-tcpmessenger/a127bc5f-17ae-4220-87d7-6f3a20c47508/scratchpad/canvas/project/*.dc.html`
  Bluepane colours: `.../canvas/project/ds/bluepane/tokens.css` — apply them through the ui theme / element colours where practical
  (classic desktop look: grey face, white fields, blue selection).
- Feature catalogue: `/Users/earmercarey/tcpmessenger/IP Messenger feature catalogue.md`;
  help file (HTML) under `/Users/earmercarey/tcpmessenger/ipmsghelps/` (e.g. `ipmsghlp_eng.htm`).

## Shared contract (do not change without need; if you must, keep it backward compatible and say so)
- `internal/core/core.go`: types `User`, `Message`, `Attachment`, `Settings`, `LogEntry`, `LogQuery`,
  `Events`, interfaces `Messenger` and `Store`, `DefaultSettings()`.
- `internal/core/fake.go`: `FakeMessenger`, `MemStore`, `MatchLog` — use them in tests.
- `app.go`: `type tcpApp` (shared state, main thread only) and `saveSettings()`.
- Window entry points in package main, each in its own file:
  `openLogViewer(a *tcpApp)` logview.go, `openSettings(a *tcpApp)` settings.go,
  `openAbsence(a *tcpApp)` absence.go, `openSortDisplay(a *tcpApp)` sortdisplay.go,
  `openCapture(a *tcpApp, done func(core.Attachment))` capture.go. They are stubs now; their owner replaces them.
- Real backends (being written in parallel):
  `my-app/internal/tcpnet`: `func New() core.Messenger`;
  `my-app/internal/store`: `func DefaultDir() string`, `func Open(dir string) (core.Store, error)`.

## Rules
- Several agents edit this module at the same time. **Only create/edit the files you own.** If the
  build fails because of someone else's file, it is mid-edit: wait a minute and retry, or verify your
  own package in isolation. Never revert or rewrite other agents' files. Do not run `git` commits.
- Threading: view functions and tcpApp fields run on the main thread; network callbacks must hop
  with `Window.Update(func(){...})`.
- Add tests (`ui.NewTester` for views, see docs/ui/testing.md) for your part and make `go vet` pass.
- Don't launch the GUI app (no `go tool mygo dev`) — the integrator does that.
- Finish with a short report: files written, what works, what is missing, any contract changes.
