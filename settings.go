package main

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"my-app/internal/core"
	"my-app/internal/store"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// settingsWin is the open Settings window, nil while it is closed.
var settingsWin *mygo.Window

// openSettings opens its window, or brings it forward when it is open.
func openSettings(a *tcpApp) {
	if settingsWin != nil {
		settingsWin.Focus()
		return
	}
	sv := newSettingsView(a)
	w := mygo.NewWindow(mygo.WindowOptions{
		Title:     "TCP Messenger Settings",
		Width:     780,
		Height:    640,
		MinWidth:  640,
		MinHeight: 480,
		StateKey:  "settings",
		Content:   ui.View(sv.view),
	})
	sv.bindWindow(w)
	settingsWin = w
	w.OnClosed(func() {
		if settingsWin == w {
			settingsWin = nil
		}
	})
}

// The log viewer's highlight colors, from the Bluepane tokens.
var (
	lvMarkColor = bpLight.mark
	lvStarColor = bpLight.star
)

// applyAuxTheme gives a secondary window the Bluepane dialog theme on the
// face ground, following the desktop's dark mode, and returns it.
func applyAuxTheme(c *ui.Context) *ui.Theme {
	bpApply(c, groundFace, true)
	return c.Theme()
}

// auxStatusBar is the status strip along the bottom of a window.
func auxStatusBar(c *ui.Context, t *ui.Theme, fn func()) {
	bpStatusBar(c, fn)
}

// auxGroup is a framed group of controls under a legend, as a fieldset.
func auxGroup(c *ui.Context, legend string, fn func()) ui.Element {
	return bpGroup(c, legend, fn)
}

// labeled is a row of a label of a fixed width and a control.
func auxLabeled(c *ui.Context, label string, width float32, fn func()) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).Width(width)
		fn()
	})
}

// off shows a control the app does not implement: built from a value of
// its own, disabled.
func auxOffCheck(c *ui.Context, label string, on bool) {
	ui.Checkbox(c, &on, label).Disabled(true).Tooltip("Not available in TCP Messenger")
}

func auxOffSelect(c *ui.Context, label string, options ...string) {
	v := options[0]
	ui.Select(c, &v, options).Label(label).Disabled(true)
}

func auxOffInput(c *ui.Context, label, value string, width float32) {
	ui.TextInput(c, &value).Label(label).Width(width).Disabled(true)
}

func auxNote(c *ui.Context, s string) {
	ui.Text(c, s).FontSize(12).TextColor(c.Theme().TextMuted).Wrap()
}

// settingsCategories are the panes of the Settings window, in order.
var settingsCategories = []string{
	"Basic/LAN Settings", "Master Settings", "Detail Settings", "Send Window",
	"Recv Window", "TaskTray", "Image/Capture", "URL/File Link", "Log Settings",
	"Auto FileDownload", "Remote Command", "RecvTrans (Slack/etc)", "Auto Update",
	"Settings/Misc",
}

// settingsView is the state of the Settings window: a copy of the
// settings, which Apply and OK give back to the app.
type settingsView struct {
	a    *tcpApp
	s    core.Settings
	orig core.Settings // as last applied

	cat      int
	cats     ui.ListState
	port     float64
	minutes  float64
	newAddr  string
	addrSel  int
	addrs    ui.ListState
	pw1, pw2 string
	pwMsg    string
	status   string
	trans    string

	// close closes the window; pickDir and pickFile ask for a folder or a
	// file to save to, and call done on the main thread ("" when canceled);
	// setLogin turns starting at login on or off. Tests replace them.
	close    func()
	pickDir  func(title, current string, done func(string))
	pickFile func(title, current string, done func(string))
	setLogin func(on bool) error
}

func settingsClone(s core.Settings) core.Settings {
	s.BroadcastAddrs = slices.Clone(s.BroadcastAddrs)
	s.AbsenceModes = slices.Clone(s.AbsenceModes)
	return s
}

func newSettingsView(a *tcpApp) *settingsView {
	sv := &settingsView{a: a, addrSel: -1, trans: "slack"}
	sv.load()
	sv.cats.Selected = &sv.cat
	sv.cats.Label = func(i int) string { return settingsCategories[i] }
	sv.addrs.Selected = &sv.addrSel
	sv.close = func() {}
	none := func(_, _ string, done func(string)) { done("") }
	sv.pickDir, sv.pickFile = none, none
	sv.setLogin = func(bool) error { return nil }
	return sv
}

func (sv *settingsView) load() {
	sv.s = settingsClone(sv.a.settings)
	sv.orig = settingsClone(sv.a.settings)
	sv.port = float64(sv.s.Port)
	if sv.port == 0 {
		sv.port = core.DefaultPort
	}
	sv.minutes = float64(sv.s.AutoAbsenceMinutes)
}

func (sv *settingsView) bindWindow(w *mygo.Window) {
	sv.close = w.Close
	sv.pickDir = func(title, current string, done func(string)) {
		go func() {
			paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: w, Title: title, DefaultPath: current, Directory: true, CreateDirectories: true})
			p := ""
			if err == nil && len(paths) > 0 {
				p = paths[0]
			}
			w.Update(func() { done(p) })
		}()
	}
	sv.pickFile = func(title, current string, done func(string)) {
		go func() {
			p, err := mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: w, Title: title, DefaultPath: current, CreateDirectories: true})
			if err != nil {
				p = ""
			}
			w.Update(func() { done(p) })
		}()
	}
	sv.setLogin = mygo.App.SetOpenAtLogin
	sv.s.StartAtLogin = sv.s.StartAtLogin || mygo.App.OpenAtLogin()
}

// apply gives the edited settings to the app and saves them.
func (sv *settingsView) apply() bool {
	s := settingsClone(sv.s)
	s.Nickname = strings.TrimSpace(s.Nickname)
	s.Group = strings.TrimSpace(s.Group)
	s.Port = int(sv.port)
	// Other windows edit these while this one is open: keep theirs.
	cur := sv.a.settings
	s.Absent, s.AbsenceIndex = cur.Absent, cur.AbsenceIndex
	s.AbsenceModes, s.List = slices.Clone(cur.AbsenceModes), cur.List
	if s.Port <= 0 || s.Port > 65535 {
		sv.status = "The port must be between 1 and 65535."
		return false
	}
	if s.AutoAbsenceMinutes = int(sv.minutes); s.AutoAbsenceMinutes < 1 {
		s.AutoAbsenceMinutes = 1
	}
	if s.QuoteMark == "" {
		s.QuoteMark = ">"
	}
	var msgs []string
	if s.StartAtLogin != sv.orig.StartAtLogin {
		if err := sv.setLogin(s.StartAtLogin); err != nil {
			msgs = append(msgs, "Start at login: "+err.Error())
			s.StartAtLogin = sv.orig.StartAtLogin
		}
	}
	sv.a.settings = s
	if err := sv.a.saveSettings(); err != nil {
		sv.status = "Could not save the settings: " + err.Error()
		return false
	}
	syncMenus(sv.a)
	if s.Port != sv.orig.Port {
		msgs = append(msgs, "The port change takes effect after restart.")
	}
	sv.orig = settingsClone(s)
	sv.s = settingsClone(s)
	if len(msgs) == 0 {
		msgs = append(msgs, "Settings saved.")
	}
	sv.status = strings.Join(msgs, " ")
	if sv.a.main != nil {
		sv.a.main.Update(func() {})
	}
	return true
}

func (sv *settingsView) view(c *ui.Context) {
	t := applyAuxTheme(c)
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).Grow(1).Gap(12).Padding(12).AlignItems(ui.Stretch).Children(func() {
			ui.Column(c).Width(190).Gap(8).Children(func() {
				ui.List(c, &sv.cats, len(settingsCategories), func(i int) {
					ui.Text(c, settingsCategories[i]).Padding(1, 4).FontSize(14)
				}).Grow(1).Background(t.Surface).Border(1, t.Border).Label("Categories")
				ui.Column(c).Gap(8).Padding(8, 24, 0, 24).Children(func() {
					if bpDlgButton(c, "OK", true).Clicked() && sv.apply() {
						sv.close()
					}
					if bpDlgButton(c, "Apply", false).Clicked() {
						sv.apply()
					}
					if bpDlgButton(c, "Cancel", false).Clicked() {
						sv.load()
						sv.close()
					}
					bpDlgButton(c, "Help", false).Margin(8, 0, 0, 0).Disabled(true)
				})
			})
			ui.Scroll(c).Grow(1).Gap(12).Padding(0, 8, 0, 0).Children(func() {
				ui.Text(c, settingsCategories[sv.cat]).FontSize(16).Bold()
				sv.pane(c, t)
			})
		})
		auxStatusBar(c, t, func() {
			s := sv.status
			if s == "" {
				s = "Changes are kept until OK or Apply."
			}
			ui.Text(c, s).MaxLines(1).TextColor(t.TextMuted)
		})
	})
}

func (sv *settingsView) pane(c *ui.Context, t *ui.Theme) {
	switch sv.cat {
	case 0:
		sv.basic(c)
	case 1:
		sv.master(c)
	case 2:
		sv.detail(c)
	case 3:
		sv.send(c)
	case 4:
		sv.recv(c)
	case 5:
		sv.tray(c)
	case 6:
		sv.image(c)
	case 7:
		sv.url(c)
	case 8:
		sv.logPane(c)
	case 9:
		sv.download(c)
	case 10:
		sv.remote(c)
	case 11:
		sv.recvTrans(c)
	case 12:
		sv.update(c)
	case 13:
		sv.misc(c)
	}
}

func (sv *settingsView) groups() []string {
	seen := map[string]bool{}
	var out []string
	for _, u := range sv.a.peers {
		if u.Group != "" && !seen[u.Group] {
			seen[u.Group] = true
			out = append(out, u.Group)
		}
	}
	sort.Strings(out)
	return out
}

func (sv *settingsView) basic(c *ui.Context) {
	ui.Row(c).Gap(12).Children(func() {
		ui.Field(c, "User name", func() {
			ui.TextInput(c, &sv.s.Nickname).Placeholder("Shown to others")
		}).Grow(1)
		ui.Field(c, "Group name", func() {
			ui.Autocomplete(c, &sv.s.Group, sv.groups())
		}).Grow(1)
	})
	auxGroup(c, "Broadcast Setup for different segments", func() {
		ui.Row(c).Gap(8).AlignItems(ui.Start).Children(func() {
			ui.Column(c).Gap(4).Grow(1).Children(func() {
				if ui.TextInput(c, &sv.newAddr).Label("Broadcast address").Placeholder("192.168.0.255").Submitted() {
					sv.addAddr()
				}
				auxNote(c, "Ex) 192.168.0.255 (or FQDN)")
				auxOffCheck(c, "Unicast required", false)
			})
			ui.Column(c).Gap(4).Children(func() {
				if bpMiniButton(c, ">>").Label("Add").Clicked() {
					sv.addAddr()
				}
				if bpMiniButton(c, "<<").Label("Remove").Disabled(sv.addrSel < 0).Clicked() {
					sv.removeAddr()
				}
			})
			ui.List(c, &sv.addrs, len(sv.s.BroadcastAddrs), func(i int) {
				ui.Text(c, sv.s.BroadcastAddrs[i]).Padding(3, 6)
			}).Width(180).Height(96).Background(c.Theme().Surface).Border(1, c.Theme().Border).Label("Broadcast addresses")
		})
		bpInfo(c, "If Master Mode is set, this setting is unnecessary").TextAlign(ui.Center)
	})
	auxGroup(c, "Network", func() {
		auxLabeled(c, "Port", 160, func() {
			ui.NumberInput(c, &sv.port, 1, 65535, 1).Label("Port").Width(150)
			auxNote(c, "(takes effect after restart)")
		})
		ui.Checkbox(c, &sv.s.DialUp, "Dial-up connection (announce to the addresses above only)")
		auxLabeled(c, "Local segment broadcast", 160, func() {
			auxOffSelect(c, "Local segment broadcast", "Directed broadcast", "Use both (Direct/Limited)")
		})
		auxLabeled(c, "IPv4/IPv6 Mode", 160, func() { auxOffSelect(c, "IP mode", "IPv4 mode", "IPv6 mode", "IPv4/IPv6") })
		auxLabeled(c, "IPv6 multicast", 160, func() {
			auxOffSelect(c, "IPv6 multicast", "SiteLocal only", "LinkLocal only", "Site/LinkLocal dual")
		})
	})
	auxGroup(c, "Language", func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			auxOffSelect(c, "Language", "System/Auto", "English", "Japanese")
			auxNote(c, "(need to restart)")
		})
	})
}

func (sv *settingsView) addAddr() {
	a := strings.TrimSpace(sv.newAddr)
	if a == "" || slices.Contains(sv.s.BroadcastAddrs, a) {
		return
	}
	sv.s.BroadcastAddrs = append(sv.s.BroadcastAddrs, a)
	sv.newAddr = ""
}

func (sv *settingsView) removeAddr() {
	i := sv.addrSel
	if i < 0 || i >= len(sv.s.BroadcastAddrs) {
		return
	}
	sv.newAddr = sv.s.BroadcastAddrs[i]
	sv.s.BroadcastAddrs = slices.Delete(sv.s.BroadcastAddrs, i, i+1)
	sv.addrSel = -1
}

func (sv *settingsView) master(c *ui.Context) {
	auxGroup(c, "Member Master Settings", func() {
		auxNote(c, "This function is to detect members over routers. If one PC becomes the Member Master and one or more PCs use it in each segment, users in the entire LAN can be detected.")
		mode := 0
		ui.RadioGroup(c, func() {
			ui.Radio(c, &mode, 0, "Not use")
			ui.Radio(c, &mode, 1, "Use Member Master (& cooperate with the master)")
			ui.Radio(c, &mode, 2, "Become Member Master")
		}).Disabled(true).Label("Member Master")
		auxLabeled(c, "Master IP address", 170, func() { auxOffInput(c, "Master IP address", "", 160) })
		auxLabeled(c, "Refresh Interval (min)", 170, func() { auxOffInput(c, "Refresh interval", "3", 60) })
		auxNote(c, "Member Master is not available in TCP Messenger: use the broadcast addresses of the Basic page to reach other segments.")
	})
}

func (sv *settingsView) detail(c *ui.Context) {
	auxGroup(c, "Balloon/Toast Notification", func() {
		ui.Checkbox(c, &sv.s.NotifyWithToast, "Notify with a system notification when a message arrives")
		auxLabeled(c, "Received duration (sec)", 170, func() { auxOffInput(c, "Received duration", "10", 60) })
		auxLabeled(c, "Opened duration (sec)", 170, func() { auxOffInput(c, "Opened duration", "3", 60) })
		auxOffCheck(c, "Do not display user information", false)
	})
	auxGroup(c, "Send/Recv/LogView HotKey", func() {
		auxOffCheck(c, "Use Hotkey (Ctrl+Alt+S/R/E)", false)
	})
	auxOffCheck(c, "Color emoji support on send/receive Dialog", false)
	auxOffCheck(c, "Change Send/Reply -> Fire/Intercept", false)
	auxLabeled(c, "Icon", 80, func() { auxOffInput(c, "Icon file", "", 260) })
	auxLabeled(c, "RevIcon", 80, func() { auxOffInput(c, "Receive icon file", "", 260) })
}

func (sv *settingsView) send(c *ui.Context) {
	auxGroup(c, "DelaySend", func() {
		auxOffCheck(c, "Use Send Queue when a receiver is stopped", false)
		auxNote(c, "Inactive users are shown as gray. The message is sent when the receiver becomes active.")
	})
	auxGroup(c, "Send Window", func() {
		ui.Checkbox(c, &sv.s.SealByDefault, "Seal messages by default")
		ui.Checkbox(c, &sv.s.QuoteOnReply, "Quote the message when replying")
		ui.Checkbox(c, &sv.s.CtrlEnterSends, "Ctrl+Enter sends (Enter breaks the line)")
		ui.Checkbox(c, &sv.s.HideAfterSend, "Hide the send window after sending")
		auxOffCheck(c, "1 Click Open SendDlg", false)
		auxOffCheck(c, "Prevent missend", false)
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Quoted Prefix")
			ui.TextInput(c, &sv.s.QuoteMark).Label("Quoted prefix").Width(60)
			ui.Spacer(c)
			if bpDlgButton(c, "Sort/Display…", false).Clicked() {
				openSortDisplay(sv.a)
			}
		})
	})
}

func (sv *settingsView) recv(c *ui.Context) {
	auxGroup(c, "Receive Window", func() {
		nonPopup := !sv.s.PopupOnReceive
		if ui.Checkbox(c, &nonPopup, "NonPopup Receive").Changed() {
			sv.s.PopupOnReceive = !nonPopup
		}
		ui.Checkbox(c, &sv.s.NoPopupWhenAbsent, "Don't pop up received messages while absent")
		ui.Checkbox(c, &sv.s.OpenCheck, "Open check (tell the sender when a sealed message is opened)")
		noSound := !sv.s.PlaySound
		if ui.Checkbox(c, &noSound, "No recv sound").Changed() {
			sv.s.PlaySound = !noSound
		}
		auxOffCheck(c, "Open LogViewer when trayicon is clicked at recv.", false)
		auxOffCheck(c, "Don't close RecvDlg when replying", false)
		auxOffCheck(c, "Restore unopened msgs after restart", false)
		auxLabeled(c, "Sound", 80, func() { auxOffInput(c, "Sound file", "System sound", 220) })
	})
}

func (sv *settingsView) tray(c *ui.Context) {
	auxGroup(c, "TaskTray", func() {
		auxOffCheck(c, "Confirm TCPMsgIcon in TaskTray at Starting", true)
		auxNote(c, "TCP Messenger always shows its icon in the menu bar or the notification area.")
	})
}

func (sv *settingsView) image(c *ui.Context) {
	auxGroup(c, "Embedded image in a message", func() {
		auxNote(c, "(To insert an image, paste it in the send window)")
		auxLabeled(c, "Show recv image", 140, func() {
			auxOffSelect(c, "Image display", "Always show directly", "Show directly if priority user", "Always show via Image button")
		})
	})
	auxGroup(c, "Display (area specified) Capture", func() {
		format := strings.ToUpper(sv.s.CaptureFormat)
		if format != "JPG" {
			format = "PNG"
		}
		auxLabeled(c, "Capture format", 140, func() {
			if ui.Select(c, &format, []string{"PNG", "JPG"}).Label("Capture format").Width(100).Changed() {
				sv.s.CaptureFormat = strings.ToLower(format)
			}
		})
		auxOffCheck(c, "Minimize Send Message Dialog as default", true)
		auxOffCheck(c, "Clipboard check as default", true)
		auxOffCheck(c, "Save check as default", false)
	})
}

func (sv *settingsView) url(c *ui.Context) {
	auxGroup(c, "Link behavior", func() {
		auxOffCheck(c, "One click open (default: double click is needed)", false)
		auxOffCheck(c, "Link targets are only URL (exclude files)", false)
		auxNote(c, "Permitted file extensions for clicking a file link:")
		exts := "txt log ini pdf doc docx ppt pptx xls xlsx csv html htm xml jpg jpeg png gif"
		ui.TextArea(c, &exts).Lines(3, 5).Disabled(true).Label("Permitted file extensions")
	})
}

func (sv *settingsView) logPane(c *ui.Context) {
	auxGroup(c, "Logging", func() {
		ui.Checkbox(c, &sv.s.LogEnabled, "Logging available")
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if bpDlgButton(c, "LogFile…", false).Disabled(!sv.s.LogEnabled).Clicked() {
				sv.pickFile("Log file", sv.s.LogFile, func(p string) {
					if p != "" {
						sv.s.LogFile = p
					}
				})
			}
			ui.TextInput(c, &sv.s.LogFile).Label("Log file").Placeholder("Default (in the settings folder)").Grow(1).Disabled(!sv.s.LogEnabled)
		})
		ui.Checkbox(c, &sv.s.LogLogonName, "Log the logon name").Disabled(!sv.s.LogEnabled)
		ui.Checkbox(c, &sv.s.LogIPAddr, "Log the IP address").Disabled(!sv.s.LogEnabled)
		ui.Checkbox(c, &sv.s.LogImages, "Log images").Disabled(!sv.s.LogEnabled)
		dir := ""
		if sv.a.store != nil {
			dir = sv.a.store.Dir()
		}
		if dir != "" {
			auxNote(c, "The log and its images are kept in "+dir+".")
		}
	})
}

func (sv *settingsView) download(c *ui.Context) {
	auxGroup(c, "Auto Save", func() {
		auxOffCheck(c, "Enable to download attached files automatically", false)
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if bpDlgButton(c, "Download folder…", false).Clicked() {
				sv.pickDir("Download folder", sv.s.DownloadDir, func(p string) {
					if p != "" {
						sv.s.DownloadDir = p
					}
				})
			}
			if bpDlgButton(c, "Open", false).Disabled(sv.s.DownloadDir == "").Clicked() {
				c.OpenURL("file://" + sv.s.DownloadDir)
			}
		})
		ui.TextInput(c, &sv.s.DownloadDir).Label("Download folder").Placeholder("Downloads").FillWidth()
		auxNote(c, "Received files are saved here.")
		auxLabeled(c, "Download size limitation (MB)", 200, func() { auxOffInput(c, "Size limit", "1000", 80) })
		auxLabeled(c, "Grace sec of starting auto downloading", 200, func() { auxOffInput(c, "Grace seconds", "120", 80) })
	})
}

func (sv *settingsView) remote(c *ui.Context) {
	auxNote(c, "Reboot/Shutdown PC/TCPMsg by a remote msg. When the following msg is received, it will be executed.")
	for _, what := range []string{"reboot PC", "shutdown PC", "terminate TCPMsg"} {
		auxGroup(c, "Remote "+what, func() {
			auxOffCheck(c, "Allow to "+what+" by a message", false)
		})
	}
	auxNote(c, "Remote commands are not available in TCP Messenger.")
}

func (sv *settingsView) recvTrans(c *ui.Context) {
	auxGroup(c, "Receive Msg Transfer for Slack or etc", func() {
		auxLabeled(c, "Transfer Mode", 120, func() {
			auxOffSelect(c, "Transfer mode", "No Transfer", "Transfer (at LockScreen)", "Transfer (always)")
		})
		ui.RadioGroup(c, func() {
			ui.Radio(c, &sv.trans, "slack", "For Slack")
			ui.Radio(c, &sv.trans, "general", "For General")
		}).Row().Gap(16).Disabled(true).Label("Transfer target")
		if sv.trans == "slack" {
			auxLabeled(c, "#channel/@username", 140, func() { auxOffInput(c, "Channel or username", "@username", 200) })
		} else {
			auxLabeled(c, "URL", 140, func() { auxOffInput(c, "URL", "", 200) })
		}
		auxNote(c, "Message transfer is not available in TCP Messenger.")
	})
}

func (sv *settingsView) update(c *ui.Context) {
	auxGroup(c, "Auto update", func() {
		auxOffCheck(c, "Regularly check the latest version", false)
		auxOffCheck(c, "Check the latest version, but no install", false)
		bpDlgButton(c, "Check Update", false).Disabled(true)
	})
}

func (sv *settingsView) misc(c *ui.Context) {
	auxGroup(c, "Startup", func() {
		ui.Checkbox(c, &sv.s.StartAtLogin, "Start TCP Messenger at login")
	})
	auxGroup(c, "Password", func() {
		state := "No password is set."
		if sv.s.Password != "" {
			state = "A password is set: it is asked before opening locked messages."
		}
		auxNote(c, state)
		ui.Form(c, func() {
			ui.Field(c, "New password", func() { ui.TextInput(c, &sv.pw1).Password().Width(180) })
			ui.Field(c, "Confirm password", func() { ui.TextInput(c, &sv.pw2).Password().Width(180) })
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if bpDlgButton(c, "Set Password", false).Disabled(sv.pw1 == "").Clicked() {
				if sv.pw1 != sv.pw2 {
					sv.pwMsg = "The passwords differ."
				} else {
					sv.s.Password = store.HashPassword(sv.pw1)
					sv.pw1, sv.pw2 = "", ""
					sv.pwMsg = "Password set (saved with OK or Apply)."
				}
			}
			if bpDlgButton(c, "Clear Password", false).Disabled(sv.s.Password == "").Clicked() {
				sv.s.Password = ""
				sv.pwMsg = "Password cleared (saved with OK or Apply)."
			}
			if sv.pwMsg != "" {
				auxNote(c, sv.pwMsg)
			}
		})
	})
	auxGroup(c, "Experimental", func() {
		ui.Checkbox(c, &sv.s.AutoAbsence, "Change to [Away] when desktop is inactive")
		auxLabeled(c, "Idle time (min)", 120, func() {
			ui.NumberInput(c, &sv.minutes, 1, 600, 1).Label("Idle time").Width(100).Disabled(!sv.s.AutoAbsence)
		})
		auxOffCheck(c, "Reduce task tray action at receiving", false)
		auxLabeled(c, "Other-segment UDP rate limit", 200, func() { auxOffInput(c, "UDP rate limit", "0", 60) })
	})
	auxGroup(c, "Export Settings", func() {
		bpDlgButton(c, "Export settings to .reg file", false).Disabled(true)
		auxNote(c, fmt.Sprintf("Settings are kept in %s.", settingsDirLabel(sv.a)))
	})
}

func settingsDirLabel(a *tcpApp) string {
	if a.store != nil && a.store.Dir() != "" {
		return a.store.Dir()
	}
	return "memory"
}
