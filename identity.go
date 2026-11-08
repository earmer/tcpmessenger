package main

import (
	"log"

	"tcpmessenger/internal/core"

	"github.com/egoist/mygo/ui"
)

// checkKey trusts the key of u on first use and flags u when the key
// differs from the one trusted for its ID.
func (a *tcpApp) checkKey(u core.User) core.User {
	u.KeyChanged = false
	if u.Fingerprint == "" || u.ID == "" {
		return u
	}
	old, ok := a.settings.TrustedKeys[u.ID]
	switch {
	case !ok:
		a.setTrusted(u.ID, u.Fingerprint)
	case old != u.Fingerprint:
		u.KeyChanged = true
	}
	return u
}

// checkKeys is checkKey over a list of peers.
func (a *tcpApp) checkKeys(us []core.User) []core.User {
	out := make([]core.User, len(us))
	for i, u := range us {
		out[i] = a.checkKey(u)
	}
	return out
}

// trustKey accepts the current key of u as the one to trust from now on.
func (a *tcpApp) trustKey(u core.User) {
	a.setTrusted(u.ID, u.Fingerprint)
	for i := range a.peers {
		if a.peers[i].ID == u.ID {
			a.peers[i].KeyChanged = false
		}
	}
	for _, rw := range a.recvWins {
		if rw.m.From.ID == u.ID {
			rw.m.From.KeyChanged = false
		}
	}
}

// setTrusted records the fingerprint of id. It saves the settings file
// without announcing again: nobody else sees this.
func (a *tcpApp) setTrusted(id, fp string) {
	if a.settings.TrustedKeys == nil {
		a.settings.TrustedKeys = map[string]string{}
	}
	a.settings.TrustedKeys[id] = fp
	if err := a.store.SaveSettings(a.settings); err != nil {
		log.Println("trusted keys:", err)
	}
}

// bpIdenticon draws the 5x5 pattern of a fingerprint in the ink colour.
func bpIdenticon(c *ui.Context, fp string, p *bpPalette) {
	const cell = 3
	g := core.Identicon(fp)
	ui.Column(c).Border(1, p.fieldEdge).Background(p.field).Padding(1).Children(func() {
		for y := range g {
			ui.Row(c.Key(y)).Children(func() {
				for x := range g[y] {
					col := p.field
					if g[y][x] {
						col = p.ink
					}
					ui.Box(c.Key(x)).Size(cell, cell).Background(col)
				}
			})
		}
	})
}
