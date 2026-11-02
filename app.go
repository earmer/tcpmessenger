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
}

// saveSettings stores a.settings and tells the network layer of changes
// that others see (nickname, group, absence).
func (a *tcpApp) saveSettings() error {
	a.net.UpdateStatus(a.settings)
	return a.store.SaveSettings(a.settings)
}
