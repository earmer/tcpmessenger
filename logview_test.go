package main

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"my-app/internal/core"

	"github.com/egoist/mygo/ui"
)

// auxTestApp is a tcpApp over an in-memory store and a fake network.
func auxTestApp(t *testing.T) (*tcpApp, *core.MemStore) {
	t.Helper()
	st := core.NewMemStore()
	fm := core.NewFakeMessenger()
	s := core.DefaultSettings()
	s.Nickname = "Me"
	if err := fm.Start(s, core.Events{}); err != nil {
		t.Fatal(err)
	}
	return &tcpApp{net: fm, store: st, settings: s}, st
}

func lvFixture(t *testing.T) (*tcpApp, *core.MemStore) {
	a, st := auxTestApp(t)
	kaga := core.User{ID: "kaga@pc1", Name: "Kaga", Host: "pc1"}
	moto := core.User{ID: "moto@pc2", Name: "Motoyama", Host: "pc2"}
	base := time.Date(2026, 11, 2, 12, 59, 0, 0, time.Local)
	st.Append(core.Message{From: kaga, Incoming: true, Text: "The member list comes over TCP port 2425.", Time: base})
	st.Append(core.Message{From: core.User{ID: "me@localhost", Name: "Me"}, To: []core.User{moto}, Text: "So the router is no problem?", Time: base.Add(time.Minute)})
	st.Append(core.Message{From: kaga, Incoming: true, Text: "That's the plan.\nTwo clients per segment.", Time: base.Add(2 * time.Minute)})
	return a, st
}

func TestLogViewerListsNewestFirst(t *testing.T) {
	a, _ := lvFixture(t)
	lv := newLogViewer(a)
	if len(lv.entries) != 3 || lv.entries[0].Seq != 3 {
		t.Fatalf("entries %+v", lv.entries)
	}
	tt := ui.NewTester(lv.view, 900, 620)
	for _, s := range []string{"pos=0", "find=3", "total=3", "Kaga", "Motoyama"} {
		if !tt.HasText(s) {
			t.Errorf("missing %q in %q", s, tt.Texts())
		}
	}
}

func TestLogViewerSearchAndFilters(t *testing.T) {
	a, st := lvFixture(t)
	lv := newLogViewer(a)
	tt := ui.NewTester(lv.view, 900, 620)
	if err := tt.Click("Search"); err != nil {
		t.Fatal(err)
	}
	tt.Type("port")
	if len(lv.entries) != 1 || !tt.HasText("find=1") || !tt.HasText("total=3") {
		t.Fatalf("after search: %d entries, texts %q", len(lv.entries), tt.Texts())
	}
	tt.Key(0, ui.KeyEscape)
	lv.query = ""
	lv.refresh()
	tt.Frame()

	st.Update(core.LogEntry{Seq: 2, Starred: true})
	lv.refresh()
	if err := tt.Click("Starred"); err != nil {
		t.Fatal(err)
	}
	if len(lv.entries) != 1 || lv.entries[0].Seq != 2 {
		t.Fatalf("starred filter: %+v", lv.entries)
	}
	lv.starred = false
	lv.user = "Kaga"
	lv.refresh()
	if len(lv.entries) != 2 {
		t.Fatalf("user filter: %d entries", len(lv.entries))
	}
}

func TestLogViewerStarMarkNoteDelete(t *testing.T) {
	a, st := lvFixture(t)
	lv := newLogViewer(a)
	tt := ui.NewTester(lv.view, 900, 620)
	if err := tt.Click("Star 3"); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.Query(core.LogQuery{Starred: true}); len(got) != 1 || got[0].Seq != 3 {
		t.Fatalf("starred after click: %+v", got)
	}

	lv.selected = 0
	tt.Frame()
	if !tt.HasText("pos=1") {
		t.Errorf("texts %q", tt.Texts())
	}
	if err := tt.Click("Mark"); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.Query(core.LogQuery{Marked: true}); len(got) != 1 || got[0].Seq != 3 {
		t.Fatalf("marked after click: %+v", got)
	}
	if err := tt.Click("Marked"); err != nil { // the filter
		t.Fatal(err)
	}
	if len(lv.entries) != 1 || !tt.HasText("find=1") {
		t.Fatalf("marked filter: %d entries", len(lv.entries))
	}
	if err := tt.Click("Marked"); err != nil {
		t.Fatal(err)
	}
	if len(lv.entries) != 3 || lv.selected != 0 {
		t.Fatalf("filter off: %d entries, selected %d", len(lv.entries), lv.selected)
	}

	lv.note = "Ref: release note"
	tt.Frame()
	if err := tt.Click("Save Note"); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.Query(core.LogQuery{Text: "release note"}); len(got) != 1 {
		t.Fatalf("note not saved: %+v", got)
	}

	if err := tt.Click("Copy"); err != nil {
		t.Fatal(err)
	}
	if tt.Clipboard() != lv.entries[0].Text {
		t.Errorf("clipboard %q", tt.Clipboard())
	}

	if err := tt.Click("Delete"); err != nil {
		t.Fatal(err)
	}
	if !lv.confirmDelete {
		t.Fatal("no confirmation")
	}
	tt.Key(0, ui.KeyEnter) // the alert's default button, Delete
	if got, _ := st.Query(core.LogQuery{}); len(got) != 2 {
		t.Fatalf("after delete %d entries", len(got))
	}
	if lv.total != 2 {
		t.Errorf("total %d", lv.total)
	}
}

func TestLogViewerReplyQuotes(t *testing.T) {
	a, _ := lvFixture(t)
	lv := newLogViewer(a)
	var to []core.User
	var quoted string
	lv.reply = func(u []core.User, q string) { to, quoted = u, q }
	lv.selected = 0
	tt := ui.NewTester(lv.view, 900, 620)
	if err := tt.Click("Reply"); err != nil {
		t.Fatal(err)
	}
	if len(to) != 1 || to[0].ID != "kaga@pc1" {
		t.Errorf("reply to %+v", to)
	}
	if quoted != "> That's the plan.\n> Two clients per segment.\n" {
		t.Errorf("quoted %q", quoted)
	}
}

func TestLogViewerExport(t *testing.T) {
	a, _ := lvFixture(t)
	lv := newLogViewer(a)
	path := filepath.Join(t.TempDir(), "log.txt")
	lv.saveFile = func(name string, done func(string)) { done(path) }
	tt := ui.NewTester(lv.view, 900, 620)
	if err := tt.Click("Export…"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "Two clients per segment.") || strings.Count(string(b), "=====\n") != 3 {
		t.Errorf("export:\n%s", b)
	}
	if !strings.Contains(lv.status, "Exported 3") {
		t.Errorf("status %q", lv.status)
	}
}

func TestLogViewerShowsImages(t *testing.T) {
	a, st := auxTestApp(t)
	img := ui.Render(func(c *ui.Context) { ui.Box(c).Size(4, 4).Background(ui.RGB(255, 0, 0)) }, 4, 4, 1)
	data := encodePNGForTest(t, img)
	st.Append(core.Message{From: core.User{ID: "x@y", Name: "X"}, Incoming: true, Text: "see", Time: time.Now(),
		Attachments: []core.Attachment{{Name: "shot.png", Data: data, Size: int64(len(data)), IsImage: true}, {Name: "a.zip", Size: 10}}})
	lv := newLogViewer(a)
	lv.selected = 0
	tt := ui.NewTester(lv.view, 900, 620)
	if !tt.HasText("[File] a.zip (10 bytes)") {
		t.Errorf("texts %q", tt.Texts())
	}
	if _, ok := tt.Find("shot.png"); !ok {
		t.Error("image not shown")
	}
	lv.withFiles = true
	lv.refresh()
	if len(lv.entries) != 1 {
		t.Errorf("files filter %d", len(lv.entries))
	}
}

func encodePNGForTest(t *testing.T, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
