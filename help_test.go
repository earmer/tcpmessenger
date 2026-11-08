package main

import (
	"encoding/json"
	"os"
	"testing"
)

// TestAppVersion checks the shown version matches mygo.json, the single
// source of truth, so the two cannot drift apart again.
func TestAppVersion(t *testing.T) {
	raw, err := os.ReadFile("mygo.json")
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Version == "" {
		t.Fatal("mygo.json has no version")
	}
	if got := appVersion(); got != cfg.Version {
		t.Errorf("appVersion() = %q, mygo.json version = %q", got, cfg.Version)
	}
}
