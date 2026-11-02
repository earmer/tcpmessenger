package main

import (
	"my-app/internal/core"

	"github.com/egoist/mygo"
)

// tcpApp is the state every window of TCP Messenger shares. Its fields are
// read and written on the main thread only: network callbacks hop there
// with Window.Update (on a.main) before touching it.
type tcpApp struct {
	net      core.Messenger
	store    core.Store
	settings core.Settings
	peers    []core.User

	// main is the send window opened at startup (owned by sendwin.go).
	main *mygo.Window

	// Added for the send, receive and tray windows (main.go, sendwin.go,
	// recvwin.go, tray.go).
	desk     deskAPI                      // the desktop: windows, dialogs, notifications (faked in tests)
	mainSend *sendWin                     // the state of the main send window
	sendWins []*sendWin                   // every open send window, the main one included
	recvWins []*recvWin                   // every open receive window
	queued   []core.Message               // received without a popup, not shown yet
	pending  map[string]*pendingSend      // sent messages awaiting their deliveries, by ID
	failures map[string][]deliveryFailure // deliveries reported before Send returned
	tray     *mygo.Tray
	menuSig  string // what the tray and app menus were last built from
	quitting bool
}

// saveSettings stores a.settings and tells the network layer of changes
// that others see (nickname, group, absence).
func (a *tcpApp) saveSettings() error {
	a.net.UpdateStatus(a.settings)
	return a.store.SaveSettings(a.settings)
}
