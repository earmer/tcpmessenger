package main

import (
	"errors"
	"testing"

	"tcpmessenger/internal/core"
)

// testNet is the fake messenger with failing deliveries and a record of
// the open notices sent.
type testNet struct {
	*core.FakeMessenger
	fail   map[string]error // deliveries to these user IDs fail
	opened []core.Message
}

func (n *testNet) Start(s core.Settings, ev core.Events) error {
	d := ev.Delivered
	ev.Delivered = func(to core.User, id string, err error) {
		if e := n.fail[to.ID]; e != nil {
			err = e
		}
		d(to, id, err)
	}
	return n.FakeMessenger.Start(s, ev)
}

func (n *testNet) NotifyOpened(m core.Message) error {
	n.opened = append(n.opened, m)
	return nil
}

var (
	kaga   = core.User{ID: "kaga@H24", Name: "Kaga", Group: "Tech", Host: "H24", Logon: "kaga", Addr: "192.168.1.24:2425", Encrypt: true}
	oda    = core.User{ID: "oda@H22", Name: "Oda", Group: "Tech", Host: "H22", Logon: "oda", Addr: "192.168.1.22:2425", Absent: true}
	tanabe = core.User{ID: "tanabe@S07", Name: "tanabe", Group: "Sales", Host: "S07", Logon: "tanabe", Addr: "192.168.2.7:2425", Encrypt: true}
	hidden = core.User{ID: "ghost@X", Name: "Ghost", Group: "Tech", Host: "X", Addr: "10.0.0.9:2425", Priority: -1}
)

// newTestApp is the app over the fakes, started, with a desk that opens no
// windows.
func newTestApp(t *testing.T) (*tcpApp, *testNet, *headlessDesk) {
	t.Helper()
	n := &testNet{FakeMessenger: core.NewFakeMessenger(kaga, oda, tanabe, hidden), fail: map[string]error{}}
	a := newAppWith(n, core.NewMemStore())
	if err := startMessenger(a); err != nil {
		t.Fatal(err)
	}
	return a, n, a.desk.(*headlessDesk)
}

func TestStartLoadsPeers(t *testing.T) {
	a, n, _ := newTestApp(t)
	if len(a.peers) != 4 {
		t.Fatalf("peers %v", a.peers)
	}
	n.Refresh()
	if len(a.peers) != 4 {
		t.Fatalf("peers after refresh %v", a.peers)
	}
	if a.settings.Port != core.DefaultPort || a.settings.Nickname == "" {
		t.Errorf("settings %+v", a.settings)
	}
}

func TestOpenedNotifies(t *testing.T) {
	a, _, d := newTestApp(t)
	a.handleOpened(kaga, "x")
	if len(d.notes) != 1 || d.notes[0] != appName+": Kaga opened your message" {
		t.Errorf("notes %q", d.notes)
	}
}

func TestShowMainReopens(t *testing.T) {
	a, _, _ := newTestApp(t)
	sw := openSendWindow(a, nil, "")
	if a.mainSend != sw || !sw.isMain {
		t.Fatal("first send window is not the main one")
	}
	sw.close()
	if a.mainSend != nil || len(a.sendWins) != 0 {
		t.Fatal("closed main window still known")
	}
	a.showMain()
	if a.mainSend == nil || len(a.sendWins) != 1 {
		t.Fatal("showMain did not open a main window")
	}
}

var errUnreachable = errors.New("connection refused")
