package main

import (
	"testing"

	"tcpmessenger/internal/core"

	"github.com/egoist/mygo/ui"
)

func TestQuietMessageWaitsInTray(t *testing.T) {
	a, n, d := newTestApp(t)
	a.settings.PlaySound = true
	n.Deliver(core.Message{ID: "q1", From: kaga, Text: "fyi", Quiet: true})
	if len(a.recvWins) != 0 || len(a.queued) != 1 {
		t.Fatalf("wins %d queued %d", len(a.recvWins), len(a.queued))
	}
	if len(d.notes) != 0 || d.beeps != 0 {
		t.Errorf("a quiet message made noise: %q %d", d.notes, d.beeps)
	}
	if log, _ := a.store.Query(core.LogQuery{}); len(log) != 1 {
		t.Error("quiet message not logged")
	}
	if a.unreadCount() != 1 {
		t.Errorf("unread %d", a.unreadCount())
	}
}

func TestSendQuiet(t *testing.T) {
	a, n, _ := newTestApp(t)
	sw := openSendWindow(a, nil, "")
	tt := ui.NewTester(sw.view, 520, 430)
	tt.Click("Kaga")
	tt.Click("Quiet")
	tt.Click("Message")
	tt.Type("later")
	tt.Click("Send")
	if len(n.Sent) != 1 || !n.Sent[0].Quiet {
		t.Fatalf("sent %+v", n.Sent)
	}
}

func TestFingerprint(t *testing.T) {
	k1, k2 := []byte("key-one"), []byte("key-two")
	f1, f2 := core.Fingerprint(k1), core.Fingerprint(k2)
	if f1 == f2 || f1 != core.Fingerprint(k1) {
		t.Errorf("fingerprints %q %q", f1, f2)
	}
	if core.Identicon(f1) != core.Identicon(f1) {
		t.Error("identicon is not stable")
	}
	g := core.Identicon(f1)
	for y := range g {
		if g[y][0] != g[y][4] || g[y][1] != g[y][3] {
			t.Error("identicon is not mirrored")
		}
	}
}

func TestKeyChangeWarning(t *testing.T) {
	a, n, _ := newTestApp(t)
	k := kaga
	k.Fingerprint = "Cobalt-Falcon-24"
	n.Deliver(core.Message{ID: "k1", From: k, Text: "first"})
	if a.settings.TrustedKeys[k.ID] != "Cobalt-Falcon-24" {
		t.Fatalf("not trusted on first use: %v", a.settings.TrustedKeys)
	}
	if a.recvWins[0].m.From.KeyChanged {
		t.Error("first key flagged")
	}
	k.Fingerprint = "Amber-Otter-07"
	n.Deliver(core.Message{ID: "k2", From: k, Text: "second"})
	rw := a.recvWins[1]
	if !rw.m.From.KeyChanged {
		t.Fatal("changed key not flagged")
	}
	tt := ui.NewTester(rw.view, 420, 380)
	if !tt.HasText("Trust new key") {
		t.Fatalf("no warning: %q", tt.Texts())
	}
	tt.Click("Trust new key")
	if rw.m.From.KeyChanged || a.settings.TrustedKeys[k.ID] != "Amber-Otter-07" {
		t.Error("key not trusted after the click")
	}
}

func TestPinCard(t *testing.T) {
	a, n, _ := newTestApp(t)
	n.Deliver(core.Message{ID: "p1", From: kaga, Text: "review the PR"})
	rw := a.recvWins[0]
	tt := ui.NewTester(rw.view, 420, 380)
	tt.Click("Pin")
	if !rw.pinned {
		t.Fatal("not pinned")
	}
	tt.Frame()
	if tt.HasText("Reply") || !tt.HasText("review the PR") {
		t.Errorf("pinned card shows %q", tt.Texts())
	}
	tt.Click("Done")
	if len(a.recvWins) != 0 {
		t.Error("Done did not close the card")
	}
}
