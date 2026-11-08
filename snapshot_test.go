package main

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"tcpmessenger/internal/core"

	"github.com/egoist/mygo/ui"
)

// TestSnapshots renders every window to a PNG in $TCPMSG_SNAPSHOTS, to look
// at the windows without launching the app. It is skipped otherwise.
func TestSnapshots(t *testing.T) {
	dir := os.Getenv("TCPMSG_SNAPSHOTS")
	if dir == "" {
		t.Skip("set TCPMSG_SNAPSHOTS to a directory")
	}
	a, _, _ := newTestApp(t)
	msg := core.Message{ID: "m1", From: kaga, Text: "The build on port 2425 is green.\nCan you check the release notes?", Time: time.Now(), Incoming: true, Signed: true}
	a.store.Append(msg)
	a.store.Append(core.Message{ID: "m2", From: a.net.Self(), To: []core.User{oda}, Text: "Lunch at noon?", Time: time.Now()})
	lv := newLogViewer(a)
	lv.refresh()
	shots := []struct {
		name string
		view func(c *ui.Context)
		w, h int
	}{
		{"send", newSendWin(a, []core.User{kaga}, "Hello").view, 520, 460},
		{"receive-sealed", (&recvWin{a: a, m: core.Message{From: kaga, Sealed: true, Time: time.Now()}}).view, 480, 400},
		{"receive", (&recvWin{a: a, m: msg, opened: true, quote: true}).view, 480, 400},
		{"logview", lv.view, 760, 560},
		{"settings", newSettingsView(a).view, 760, 600},
		{"absence", newAbsenceView(a).view, 540, 560},
		{"help", helpView, 680, 900},
		{"sort", (&sortDisplayView{a: a, close: func() {}}).view, 460, 480},
	}
	for _, s := range shots {
		for _, dark := range []bool{false, true} {
			tt := ui.NewTester(s.view, s.w, s.h)
			tt.SetDark(dark)
			tt.Frame()
			tt.Frame()
			name := s.name
			if dark {
				name += "-dark"
			}
			f, err := os.Create(filepath.Join(dir, name+".png"))
			if err != nil {
				t.Fatal(err)
			}
			png.Encode(f, tt.Image())
			f.Close()
		}
	}
}
