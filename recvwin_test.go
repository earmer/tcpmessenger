package main

import (
	"os"
	"path/filepath"
	"testing"

	"my-app/internal/core"

	"github.com/egoist/mygo/ui"
)

func TestReceivePlain(t *testing.T) {
	a, n, d := newTestApp(t)
	a.settings.PlaySound = true
	n.Deliver(core.Message{ID: "m1", From: kaga, Text: "Hello there"})
	if len(a.recvWins) != 1 {
		t.Fatalf("%d receive windows", len(a.recvWins))
	}
	rw := a.recvWins[0]
	tt := ui.NewTester(rw.view, 420, 380)
	for _, s := range []string{"Kaga", "Tech/H24", "Hello there"} {
		if !tt.HasText(s) {
			t.Errorf("no %q in %q", s, tt.Texts())
		}
	}
	if len(d.notes) != 1 || d.beeps != 1 {
		t.Errorf("notes %q beeps %d", d.notes, d.beeps)
	}
	if log, _ := a.store.Query(core.LogQuery{}); len(log) != 1 || !log[0].Incoming {
		t.Errorf("log %+v", log)
	}
	tt.Click("Close")
	if len(a.recvWins) != 0 {
		t.Error("window not closed")
	}
}

func TestReceiveSealedOpen(t *testing.T) {
	a, n, _ := newTestApp(t)
	n.Deliver(core.Message{ID: "m2", From: kaga, Text: "Secret plan", Sealed: true})
	rw := a.recvWins[0]
	tt := ui.NewTester(rw.view, 420, 380)
	if tt.HasText("Secret plan") || !tt.HasText("Unopened") {
		t.Fatalf("sealed text shows: %q", tt.Texts())
	}
	if a.unreadCount() != 1 {
		t.Errorf("unread %d", a.unreadCount())
	}
	if err := tt.Click("Open"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Secret plan") {
		t.Fatalf("not opened: %q", tt.Texts())
	}
	if len(n.opened) != 1 || n.opened[0].ID != "m2" {
		t.Errorf("open notices %v", n.opened)
	}
	if a.unreadCount() != 0 {
		t.Errorf("unread %d after opening", a.unreadCount())
	}
}

func TestReceiveLockedAsksPassword(t *testing.T) {
	a, n, _ := newTestApp(t)
	a.settings.Password = "pw"
	a.settings.OpenCheck = false
	n.Deliver(core.Message{ID: "m3", From: kaga, Text: "Locked text", Sealed: true, Locked: true})
	rw := a.recvWins[0]
	tt := ui.NewTester(rw.view, 420, 380)
	tt.Click("Open")
	if !tt.HasText("This message is locked. Enter your password.") {
		t.Fatalf("no password prompt: %q", tt.Texts())
	}
	tt.Click("Password")
	tt.Type("bad")
	tt.Click("OK")
	if !tt.HasText("Wrong password") || tt.HasText("Locked text") {
		t.Fatalf("wrong password opened it: %q", tt.Texts())
	}
	tt.Click("Password")
	tt.Type("pw")
	tt.Click("OK")
	if !tt.HasText("Locked text") {
		t.Fatalf("right password did not open it: %q", tt.Texts())
	}
	if len(n.opened) != 0 {
		t.Error("open notice sent with OpenCheck off")
	}
}

func TestReplyQuotes(t *testing.T) {
	a, _, _ := newTestApp(t)
	openSendWindow(a, nil, "")
	a.handleReceived(core.Message{ID: "m4", From: kaga, Text: "line one\nline two\n"})
	rw := a.recvWins[0]
	tt := ui.NewTester(rw.view, 420, 380)
	if !rw.quote {
		t.Error("quote is not on by default")
	}
	tt.Click("Reply")
	if len(a.sendWins) != 2 {
		t.Fatalf("%d send windows", len(a.sendWins))
	}
	sw := a.sendWins[1]
	if sw.text != "> line one\n> line two\n\n" || !sw.chosen.Has(kaga.ID) {
		t.Errorf("reply text %q, chosen kaga %v", sw.text, sw.chosen.Has(kaga.ID))
	}
	// Without the quote.
	tt.Click("Quote")
	tt.Click("Reply")
	if got := a.sendWins[2].text; got != "" {
		t.Errorf("unquoted reply text %q", got)
	}
}

func TestNoPopupWhenAbsent(t *testing.T) {
	a, n, d := newTestApp(t)
	a.settings.NoPopupWhenAbsent = true
	a.settings.Absent = true
	n.Deliver(core.Message{ID: "m5", From: kaga, Text: "While you were out"})
	if len(a.recvWins) != 0 || len(a.queued) != 1 {
		t.Fatalf("windows %d queued %d", len(a.recvWins), len(a.queued))
	}
	if a.unreadCount() != 1 || len(d.notes) != 1 {
		t.Errorf("unread %d notes %q", a.unreadCount(), d.notes)
	}
	a.openQueued()
	if len(a.recvWins) != 1 || len(a.queued) != 0 {
		t.Errorf("windows %d queued %d after opening", len(a.recvWins), len(a.queued))
	}
}

func TestSaveAttachment(t *testing.T) {
	a, _, d := newTestApp(t)
	a.settings.DownloadDir = "/downloads"
	d.savePath = filepath.Join(t.TempDir(), "report.txt")
	a.handleReceived(core.Message{ID: "m6", From: kaga, Text: "See attached", Attachments: []core.Attachment{{Name: "report.txt", Size: 4, Data: []byte("data")}}})
	rw := a.recvWins[0]
	tt := ui.NewTester(rw.view, 420, 380)
	if err := tt.Click("Save report.txt"); err != nil {
		t.Fatal(err)
	}
	if len(d.asked) != 1 || d.asked[0] != filepath.Join("/downloads", "report.txt") {
		t.Errorf("asked %q", d.asked)
	}
	if b, err := os.ReadFile(d.savePath); err != nil || string(b) != "data" {
		t.Errorf("saved %q, %v", b, err)
	}
	if !tt.HasText("Saved report.txt") {
		t.Errorf("texts %q", tt.Texts())
	}
}

func TestRecvQuote(t *testing.T) {
	if got := recvQuote("a\r\nb", "| "); got != "| a\n| b\n\n" {
		t.Errorf("%q", got)
	}
	if got := recvQuote("", "> "); got != "" {
		t.Errorf("%q", got)
	}
}

func TestAbsenceAutoReply(t *testing.T) {
	a, n, _ := newTestApp(t)
	a.settings.Absent, a.settings.AbsenceIndex = true, 1
	before := len(n.Sent)
	a.handleReceived(core.Message{From: kaga, Text: "hi"})
	if len(n.Sent) != before+1 || !n.Sent[before].AutoReply || n.Sent[before].Text != a.settings.AbsenceModes[1].Message {
		t.Fatalf("no absence reply: %+v", n.Sent[before:])
	}
	a.handleReceived(core.Message{From: kaga, Text: "away too", AutoReply: true})
	if len(n.Sent) != before+1 {
		t.Fatal("answered an auto reply")
	}
}
