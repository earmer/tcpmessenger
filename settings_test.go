package main

import (
	"errors"
	"strings"
	"testing"

	"tcpmessenger/internal/core"
	"tcpmessenger/internal/store"

	"github.com/egoist/mygo/ui"
)

func TestSettingsEveryCategoryBuilds(t *testing.T) {
	a, _ := auxTestApp(t)
	sv := newSettingsView(a)
	tt := ui.NewTester(sv.view, 800, 660)
	for i, name := range settingsCategories {
		if err := tt.Click(name); err != nil {
			t.Fatal(err)
		}
		if sv.cat != i {
			t.Errorf("clicked %q, category %d", name, sv.cat)
		}
	}
	tt.SetDark(true)
	tt.Frame()
}

func TestSettingsApplyAndCancel(t *testing.T) {
	a, st := auxTestApp(t)
	sv := newSettingsView(a)
	closed := 0
	sv.close = func() { closed++ }
	tt := ui.NewTester(sv.view, 800, 660)
	if err := tt.Click("User name"); err != nil {
		t.Fatal(err)
	}
	tt.Type("Kaga")
	if a.settings.Nickname != "Me" {
		t.Fatal("edited the app's settings before Apply")
	}
	if err := tt.Click("Apply"); err != nil {
		t.Fatal(err)
	}
	saved, _ := st.LoadSettings()
	if a.settings.Nickname != "MeKaga" || saved.Nickname != "MeKaga" {
		t.Errorf("after Apply: app %q, store %q", a.settings.Nickname, saved.Nickname)
	}
	if a.net.Self().Name != "MeKaga" {
		t.Errorf("network not told: %q", a.net.Self().Name)
	}

	tt.Click("Recv Window")
	tt.Click("NonPopup Receive")
	if !a.settings.PopupOnReceive || sv.s.PopupOnReceive {
		t.Errorf("non-popup: app %v, copy %v", a.settings.PopupOnReceive, sv.s.PopupOnReceive)
	}
	tt.Click("Cancel")
	if closed != 1 || !a.settings.PopupOnReceive || !sv.s.PopupOnReceive {
		t.Errorf("cancel: closed %d, app %v, copy %v", closed, a.settings.PopupOnReceive, sv.s.PopupOnReceive)
	}

	tt.Click("Send Window")
	tt.Click("Seal messages by default")
	tt.Click("OK")
	if closed != 2 || !a.settings.SealByDefault {
		t.Errorf("OK: closed %d, seal %v", closed, a.settings.SealByDefault)
	}
}

func TestSettingsPortSaysRestart(t *testing.T) {
	a, _ := auxTestApp(t)
	sv := newSettingsView(a)
	tt := ui.NewTester(sv.view, 800, 660)
	sv.port = 2426
	tt.Click("Apply")
	if a.settings.Port != 2426 || !strings.Contains(sv.status, "after restart") {
		t.Errorf("port %d, status %q", a.settings.Port, sv.status)
	}
	sv.port = 0
	if sv.apply() || a.settings.Port != 2426 {
		t.Error("accepted port 0")
	}
}

func TestSettingsBroadcastList(t *testing.T) {
	a, _ := auxTestApp(t)
	sv := newSettingsView(a)
	tt := ui.NewTester(sv.view, 800, 660)
	tt.Click("Broadcast address")
	tt.Type("192.168.20.255")
	tt.Click("Add")
	tt.Click("Broadcast address")
	tt.Type("10.0.0.255")
	tt.Key(0, ui.KeyEnter)
	if got := strings.Join(sv.s.BroadcastAddrs, ","); got != "192.168.20.255,10.0.0.255" {
		t.Fatalf("addresses %q", got)
	}
	if len(a.settings.BroadcastAddrs) != 0 {
		t.Fatal("app settings share the slice")
	}
	tt.Click("192.168.20.255")
	tt.Click("Remove")
	if got := strings.Join(sv.s.BroadcastAddrs, ","); got != "10.0.0.255" {
		t.Fatalf("after remove %q", got)
	}
}

func TestSettingsFoldersLoginPassword(t *testing.T) {
	a, _ := auxTestApp(t)
	sv := newSettingsView(a)
	var login []bool
	sv.setLogin = func(on bool) error { login = append(login, on); return nil }
	sv.pickDir = func(_, _ string, done func(string)) { done("/tmp/dl") }
	tt := ui.NewTester(sv.view, 800, 660)

	tt.Click("Auto FileDownload")
	tt.Click("Download folder…")
	if sv.s.DownloadDir != "/tmp/dl" {
		t.Errorf("download dir %q", sv.s.DownloadDir)
	}

	tt.Click("Settings/Misc")
	tt.Click("Start TCP Messenger at login")
	tt.Click("New password")
	tt.Type("secret")
	tt.Click("Confirm password")
	tt.Type("secret")
	tt.Click("Set Password")
	tt.Click("Apply")
	if len(login) != 1 || !login[0] || !a.settings.StartAtLogin {
		t.Errorf("login calls %v, setting %v", login, a.settings.StartAtLogin)
	}
	if !store.CheckPassword(a.settings.Password, "secret") {
		t.Errorf("password %q", a.settings.Password)
	}
	if a.settings.DownloadDir != "/tmp/dl" {
		t.Errorf("download dir not applied")
	}

	sv.setLogin = func(bool) error { return errors.New("not packaged") }
	tt.Click("Start TCP Messenger at login")
	tt.Click("Clear Password")
	tt.Click("Apply")
	if !a.settings.StartAtLogin || a.settings.Password != "" || !strings.Contains(sv.status, "not packaged") {
		t.Errorf("login %v, password %q, status %q", a.settings.StartAtLogin, a.settings.Password, sv.status)
	}
}

func TestSettingsLogAndCapture(t *testing.T) {
	a, _ := auxTestApp(t)
	sv := newSettingsView(a)
	tt := ui.NewTester(sv.view, 800, 660)
	tt.Click("Log Settings")
	tt.Click("Log images")
	tt.Click("Logging available")
	tt.Click("Image/Capture")
	sv.s.CaptureFormat = "jpg"
	tt.Frame()
	tt.Click("Apply")
	want := core.DefaultSettings()
	if a.settings.LogEnabled || a.settings.LogImages == want.LogImages || a.settings.CaptureFormat != "jpg" {
		t.Errorf("log %v images %v format %q", a.settings.LogEnabled, a.settings.LogImages, a.settings.CaptureFormat)
	}
}

func TestSettingsKeepsOtherWindowsEdits(t *testing.T) {
	a, _ := auxTestApp(t)
	sv := newSettingsView(a)
	a.settings.Absent = true
	a.settings.List.ShowIP = true
	sv.s.Nickname = "New"
	if !sv.apply() {
		t.Fatal(sv.status)
	}
	if !a.settings.Absent || !a.settings.List.ShowIP || a.settings.Nickname != "New" {
		t.Errorf("settings %+v", a.settings)
	}
}
