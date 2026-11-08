package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"tcpmessenger/internal/core"

	"github.com/egoist/mygo/ui"
)

func names(us []core.User) []string {
	var out []string
	for _, u := range us {
		out = append(out, u.Name)
	}
	return out
}

func TestSortUsers(t *testing.T) {
	us := []core.User{
		{ID: "1", Name: "bob", Group: "Tech", Host: "h3", Addr: "10.0.0.20:2425"},
		{ID: "2", Name: "Alice", Group: "Sales", Host: "h1", Addr: "10.0.0.3:2425"},
		{ID: "3", Name: "carol", Group: "Tech", Host: "h2", Addr: "10.0.0.100:2425"},
		{ID: "4", Name: "Dave", Group: "Sales", Host: "h4", Addr: "10.0.0.1:2425", Priority: 1},
	}
	cases := []struct {
		o    core.ListOptions
		want []string
	}{
		{core.ListOptions{SortByGroup: true, IgnoreCase: true}, []string{"Dave", "Alice", "bob", "carol"}},
		{core.ListOptions{SortByGroup: true, GroupReverse: true, IgnoreCase: true}, []string{"Dave", "bob", "carol", "Alice"}},
		{core.ListOptions{}, []string{"Dave", "Alice", "bob", "carol"}}, // case-sensitive: upper first
		{core.ListOptions{IgnoreCase: true, SecondReverse: true}, []string{"Dave", "carol", "bob", "Alice"}},
		{core.ListOptions{Second: core.SortByIP}, []string{"Dave", "Alice", "bob", "carol"}}, // numeric, not textual
		{core.ListOptions{Second: core.SortByHost}, []string{"Dave", "Alice", "carol", "bob"}},
	}
	for i, tc := range cases {
		got := slices.Clone(us)
		sendSortUsers(got, tc.o)
		if !slices.Equal(names(got), tc.want) {
			t.Errorf("case %d: %v, want %v", i, names(got), tc.want)
		}
	}
}

func TestSendWindowSelectAndSend(t *testing.T) {
	a, n, _ := newTestApp(t)
	sw := openSendWindow(a, nil, "")
	tt := ui.NewTester(sw.view, 460, 430)

	for _, s := range []string{"Kaga", "Oda", "tanabe", "!", "Users 3", "sel=0", "port=2425"} {
		if !tt.HasText(s) {
			t.Errorf("no %q in %q", s, tt.Texts())
		}
	}
	if tt.HasText("Ghost") {
		t.Error("a hidden user shows")
	}
	// Send does nothing with no one chosen.
	tt.Click("Send")
	if len(n.Sent) != 0 {
		t.Fatal("sent with no one chosen")
	}

	if err := tt.Click("Kaga"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ClickWith(ui.Cmd, "tanabe"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("sel=2") {
		t.Fatalf("texts %q after choosing two", tt.Texts())
	}
	tt.Click("Message")
	tt.Type("Lunch at noon?")
	if err := tt.Click("Send"); err != nil {
		t.Fatal(err)
	}
	if len(n.Sent) != 1 {
		t.Fatalf("sent %d messages", len(n.Sent))
	}
	m := n.Sent[0]
	if m.Text != "Lunch at noon?" || len(m.To) != 2 || m.Sealed {
		t.Errorf("sent %+v", m)
	}
	if sw.text != "" {
		t.Errorf("text %q left after sending", sw.text)
	}
	log, _ := a.store.Query(core.LogQuery{})
	if len(log) != 1 || log[0].ID == "" || log[0].Text != "Lunch at noon?" {
		t.Errorf("log %+v", log)
	}
	if len(a.pending) != 0 {
		t.Errorf("pending %v after every delivery", a.pending)
	}
}

func TestSendSealLockAndCmdEnter(t *testing.T) {
	a, n, _ := newTestApp(t)
	a.settings.SealByDefault = true
	a.settings.LogEnabled = false
	sw := openSendWindow(a, nil, "")
	tt := ui.NewTester(sw.view, 460, 430)
	if !sw.seal {
		t.Error("seal is not on by default")
	}
	tt.Click("Lock")
	if !sw.lock || !sw.seal {
		t.Error("lock does not seal")
	}
	tt.Click("Kaga")
	tt.Click("Message")
	tt.Type("secret")
	tt.Key(ui.Cmd, ui.KeyEnter)
	if len(n.Sent) != 1 || !n.Sent[0].Sealed || !n.Sent[0].Locked || n.Sent[0].Text != "secret" {
		t.Fatalf("sent %+v", n.Sent)
	}
	if log, _ := a.store.Query(core.LogQuery{}); len(log) != 0 {
		t.Error("logged with the log off")
	}
}

func TestSendFailureRetry(t *testing.T) {
	a, n, _ := newTestApp(t)
	n.fail[tanabe.ID] = errUnreachable
	sw := openSendWindow(a, nil, "")
	tt := ui.NewTester(sw.view, 460, 430)
	tt.Click("Kaga")
	tt.ClickWith(ui.Cmd, "tanabe")
	tt.Click("Message")
	tt.Type("hi")
	tt.Click("Send")
	tt.Frame()
	if !sw.failOpen || !tt.HasText("Message not delivered") {
		t.Fatalf("no failure alert: %q", tt.Texts())
	}
	if len(sw.failTo) != 1 || sw.failTo[0].ID != tanabe.ID {
		t.Fatalf("failed %v", sw.failTo)
	}
	delete(n.fail, tanabe.ID)
	if err := tt.Click("Retry"); err != nil {
		t.Fatal(err)
	}
	if len(n.Sent) != 2 || len(n.Sent[1].To) != 1 || n.Sent[1].To[0].ID != tanabe.ID || n.Sent[1].Text != "hi" {
		t.Fatalf("retry sent %+v", n.Sent)
	}
	if sw.failOpen {
		t.Error("alert still open after a good retry")
	}
}

func TestReplyWindowClosesAfterSend(t *testing.T) {
	a, n, _ := newTestApp(t)
	openSendWindow(a, nil, "")
	stranger := core.User{ID: "x@far", Name: "Stranger", Host: "far", Addr: "10.9.9.9:2425"}
	sw := openSendWindow(a, []core.User{stranger}, "> hello\n\n")
	if sw.isMain {
		t.Fatal("reply window is the main one")
	}
	tt := ui.NewTester(sw.view, 460, 430)
	if !tt.HasText("Stranger") || !tt.HasText("sel=1") {
		t.Fatalf("preselected recipient missing: %q", tt.Texts())
	}
	tt.Click("Send")
	if len(n.Sent) != 1 || n.Sent[0].To[0].ID != stranger.ID || n.Sent[0].Text != "> hello\n\n" {
		t.Fatalf("sent %+v", n.Sent)
	}
	if len(a.sendWins) != 1 || a.sendWins[0] != a.mainSend {
		t.Error("reply window did not close after sending")
	}
}

func TestAttachFiles(t *testing.T) {
	a, n, d := newTestApp(t)
	p := filepath.Join(t.TempDir(), "notes.txt")
	os.WriteFile(p, []byte("hello"), 0o644)
	d.openPaths = []string{p}
	sw := openSendWindow(a, nil, "")
	tt := ui.NewTester(sw.view, 460, 430)
	tt.Click("Attach files")
	if !tt.HasText("notes.txt (5 B)") {
		t.Fatalf("no chip: %q", tt.Texts())
	}
	tt.Click("Remove notes.txt")
	if len(sw.attach) != 0 {
		t.Fatal("chip not removed")
	}
	tt.Click("Attach files")
	tt.Click("Kaga")
	tt.Click("Send")
	if len(n.Sent) != 1 || len(n.Sent[0].Attachments) != 1 || string(n.Sent[0].Attachments[0].Data) != "hello" {
		t.Fatalf("sent %+v", n.Sent)
	}
}

func TestListColumnsAndSortSettings(t *testing.T) {
	a, _, _ := newTestApp(t)
	a.settings.List = core.ListOptions{ShowIP: true, ShowLogon: true, Second: core.SortByName, SecondReverse: true, IgnoreCase: true}
	sw := openSendWindow(a, nil, "")
	tt := ui.NewTester(sw.view, 600, 430)
	for _, s := range []string{"IP", "Logon", "192.168.1.24"} {
		if !tt.HasText(s) {
			t.Errorf("no %q in %q", s, tt.Texts())
		}
	}
	for _, col := range sendColumns(a.settings.List) {
		if col.col.Title == "Group" {
			t.Error("group column shows")
		}
	}
	if got := names(sw.users); !slices.Equal(got, []string{"tanabe", "Oda", "Kaga"}) {
		t.Errorf("order %v", got)
	}
}
