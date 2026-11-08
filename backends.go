package main

import (
	"log"

	"tcpmessenger/internal/core"
	"tcpmessenger/internal/store"
	"tcpmessenger/internal/tcpnet"
)

// openBackends returns the network layer and the store the app runs on:
// the in-memory fakes with a few demo peers when TCPMSG_FAKE=1, otherwise
// the real ones. A store that cannot open falls back to memory, so the app
// still starts (without a log on disk).
func openBackends() (core.Messenger, core.Store) {
	if fakeMode() {
		return fakeBackends()
	}
	st, err := store.Open(store.DefaultDir())
	if err != nil {
		log.Println("store:", err, "- keeping settings and the log in memory")
		return tcpnet.New(), core.NewMemStore()
	}
	return tcpnet.New(), st
}

// checkPassword reports whether pw is the password of the settings, which
// the store keeps hashed; a value not hashed (as the memory store keeps
// it) is compared as it is.
func checkPassword(saved, pw string) bool {
	if store.IsHashed(saved) {
		return store.CheckPassword(saved, pw)
	}
	return saved == pw
}
