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

// applyAuxTheme gives a dialog the Bluepane dialog theme on the face
// ground, following the desktop's dark mode, and returns it.
func applyAuxTheme(c *ui.Context) *ui.Theme {
	bpApply(c, groundFace, true)
	return c.Theme()
}

// auxGroup is a group box of a pane (bp-group).
func auxGroup(c *ui.Context, legend string, fn func()) ui.Element {
	return bpGroup(c, legend, fn)
}

// auxLabeled is a dialog row: a label and controls. The width is that of
// the original layout; in a group the labels line up by themselves.
func auxLabeled(c *ui.Context, label string, width float32, fn func()) {
	bpRow(c, label, fn)
}

// auxOff* show controls the app does not implement: built from values of
// their own, disabled.
func auxOffCheck(c *ui.Context, label string, on bool) {
	ui.Checkbox(c, &on, label).Disabled(true).Tooltip("Not available in TCP Messenger")
}

func auxOffSelect(c *ui.Context, label string, options ...string) {
	v := options[0]
	ui.Select(c, &v, options).Label(label).Disabled(true)
}

func auxOffInput(c *ui.Context, label, value string, width float32) {
	bpDlgField(c, ui.TextInput(c, &value).Label(label).Width(width).Disabled(true))
}

// auxNote is a hint under a control, as an example of what to type.
func auxNote(c *ui.Context, s string) {
	ui.Text(c, s).FontSize(13).TextColor(bpOf(c).muted).Wrap()
}

// auxInput is a dialog text field.
func auxInput(c *ui.Context, v *string, label string) ui.Element {
	return bpDlgField(c, ui.TextInput(c, v).Label(label))
}

// settingsCategories are the panes of the Settings window, in order.
var settingsCategories = []string{
	"Basic/LAN Settings", "Master Settings", "Detail Settings", "Send Window",
	"Recv Window", "TaskTray", "Image/Capture", "URL/File Link", "Log Settings",
	"Auto FileDownload", "Remote Command", "RecvTrans (Slack/etc)", "Auto Update",
	"Settings/Misc",
}

// settingsLeads are the explanations that lead each pane.
var settingsLeads = []string{
	"Your name and group as other members see them, and how the LAN is reached.",
	"With a member master, users in every segment appear in the list.",
	"How arriving messages notify you.",
	"How the send window behaves.",
	"How received messages open.",
	"The icon in the menu bar or the notification area.",
	"Images in messages and screen captures.",
	"How links and file paths in messages open.",
	"What the message log keeps.",
	"Where received files are saved.",
	"Commands that other members may send to this PC.",
	"Forwarding received messages to Slack or a web hook.",
	"Checking for new versions.",
	"Startup, the password for locked messages, and other options.",
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
	ui.Row(c).Fill().Padding(sp3).Gap(sp5).AlignItems(ui.Stretch).Children(func() {
		ui.Column(c).Width(190).Shrink(0).Gap(sp5).Children(func() {
			bpCatList(c, &sv.cats, len(settingsCategories), func(i int) string { return settingsCategories[i] }).
				Height(280).Label("Categories")
			ui.Column(c).Gap(sp3).Padding(0, sp6).Children(func() {
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
				if bpDlgButton(c, "Help", false).Margin(sp5-sp3, 0, 0, 0).Clicked() {
					openHelp(fmt.Sprintf("settings/%d", sv.cat))
				}
			})
			if sv.status != "" {
				bpInfoBox(c, sv.status).Grow(0).FontSize(13)
			}
		})
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(sp1, sp1, sp3, 0).Children(func() {
				sv.pane(c, t)
			})
		})
	})
}

// paneGroup is the first group of a pane: legend after the category, the
// explanation and "?" first.
func (sv *settingsView) paneGroup(c *ui.Context, fn func()) {
	bpGroup(c, settingsCategories[sv.cat], func() {
		bpLead(c, settingsLeads[sv.cat], fmt.Sprintf("settings/%d", sv.cat))
		fn()
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
	sv.paneGroup(c, func() {
		auxLabeled(c, "User name", 120, func() {
			auxInput(c, &sv.s.Nickname, "User name").Placeholder("Shown to others").Width(220)
		})
		auxLabeled(c, "Group name", 120, func() {
			bpDlgField(c, ui.Autocomplete(c, &sv.s.Group, sv.groups()).Label("Group name").Width(220))
		})
		auxLabeled(c, "This PC's address", 120, func() {
			addr := ""
			if sv.a.net != nil {
				addr = sv.a.net.Self().Addr
			}
			if addr == "" {
				addr = "(not started)"
			}
			bpReadonly(c, addr, 220)
		})
	})
	bpGroup(c, "Broadcast Setup for Different Segments", func() {
		ui.Row(c).Gap(sp3).AlignItems(ui.Start).Children(func() {
			ui.Column(c).Gap(sp2).Grow(1).Children(func() {
				if auxInput(c, &sv.newAddr, "Broadcast address").Placeholder("192.168.0.255").Submitted() {
					sv.addAddr()
				}
				auxNote(c, "Ex) 192.168.0.255 (or FQDN)")
				auxOffCheck(c, "Unicast required", false)
			})
			ui.Column(c).Gap(sp2).Children(func() {
				if bpMiniButton(c, ">>").Label("Add").Width(32).Clicked() {
					sv.addAddr()
				}
				if bpMiniButton(c, "<<").Label("Remove").Width(32).Disabled(sv.addrSel < 0).Clicked() {
					sv.removeAddr()
				}
			})
			bpCatList(c, &sv.addrs, len(sv.s.BroadcastAddrs), func(i int) string { return sv.s.BroadcastAddrs[i] }).
				Width(180).Height(96).Label("Broadcast addresses")
		})
		bpInfoBox(c, "With Member Master set, these addresses are not needed.")
	})
	bpGroup(c, "Network", func() {
		auxLabeled(c, "Port", 180, func() {
			ui.NumberInput(c, &sv.port, 1, 65535, 1).Label("Port").Width(110)
			auxNote(c, "(need restart)")
		})
		ui.Checkbox(c, &sv.s.DialUp, "Dial-up connection (announce to the addresses above only)")
		auxLabeled(c, "Local segment broadcast", 180, func() {
			auxOffSelect(c, "Local segment broadcast", "Directed broadcast", "Use both (Direct/Limited)")
		})
		auxLabeled(c, "IPv4/IPv6 mode", 180, func() { auxOffSelect(c, "IP mode", "IPv4 mode", "IPv6 mode", "IPv4/IPv6") })
		auxLabeled(c, "IPv6 multicast", 180, func() {
			auxOffSelect(c, "IPv6 multicast", "SiteLocal only", "LinkLocal only", "Site/LinkLocal dual")
		})
	})
	bpGroup(c, "Language", func() {
		ui.Row(c).Gap(sp3).AlignItems(ui.Center).Children(func() {
			auxOffSelect(c, "Language", "System/Auto", "English", "Japanese")
			auxNote(c, "(need restart)")
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
	sv.paneGroup(c, func() {
		mode := 0
		bpRadios(c, func() {
			ui.RadioGroup(c, func() {
				ui.Radio(c, &mode, 0, "Do not use")
				ui.Radio(c, &mode, 1, "Use a member master (and cooperate with it)")
				bpIndent(c, func() {
					auxLabeled(c, "Master IP address", 0, func() { auxOffInput(c, "Master IP address", "", 200) })
				})
				ui.Radio(c, &mode, 2, "Become the member master")
			}).Gap(sp4).Disabled(true).Label("Member Master")
		})
		bpInfoBox(c, "Only one PC on the whole LAN may be the member master, and it needs a fixed IP address.\n\nMember Master is not available in TCP Messenger: use the broadcast addresses of the Basic/LAN page to reach other segments.")
		bpIndent(c, func() {
			auxLabeled(c, "Refresh interval (min)", 200, func() { auxOffInput(c, "Refresh interval", "3", 56) })
		})
	})
}

func (sv *settingsView) detail(c *ui.Context) {
	sv.paneGroup(c, func() {
		ui.Checkbox(c, &sv.s.NotifyWithToast, "Notify with a system notification when a message arrives")
		bpIndent(c, func() {
			auxLabeled(c, "Received duration (sec)", 200, func() { auxOffInput(c, "Received duration", "10", 56) })
			auxLabeled(c, "Opened duration (sec)", 200, func() { auxOffInput(c, "Opened duration", "3", 56) })
			auxOffCheck(c, "Do not display user information", false)
		})
	})
	bpGroup(c, "Send/Recv/LogView HotKey", func() {
		auxOffCheck(c, "Use hotkeys (Ctrl+Alt+S/R/E)", false)
	})
	bpGroup(c, "Appearance", func() {
		auxOffCheck(c, "Change Send/Reply to Fire/Intercept", false)
		auxLabeled(c, "Icon", 80, func() { auxOffInput(c, "Icon file", "", 260) })
		auxLabeled(c, "RevIcon", 80, func() { auxOffInput(c, "Receive icon file", "", 260) })
	})
}

func (sv *settingsView) send(c *ui.Context) {
	sv.paneGroup(c, func() {
		ui.Checkbox(c, &sv.s.SealByDefault, "Seal messages by default")
		ui.Checkbox(c, &sv.s.QuoteOnReply, "Quote the message when replying")
		bpIndent(c, func() {
			auxLabeled(c, "Quoted prefix", 0, func() {
				auxInput(c, &sv.s.QuoteMark, "Quoted prefix").Width(56).Disabled(!sv.s.QuoteOnReply)
			})
		})
		ui.Checkbox(c, &sv.s.CtrlEnterSends, "Ctrl+Enter sends (Enter breaks the line)")
		ui.Checkbox(c, &sv.s.HideAfterSend, "Hide the send window after sending")
		auxOffCheck(c, "1 click opens the send window", false)
		auxOffCheck(c, "Prevent missend", false)
		ui.Row(c).Children(func() {
			if bpDlgButton(c, "Sort/Display…", false).Clicked() {
				openSortDisplay(sv.a)
			}
		})
	})
	bpGroup(c, "DelaySend", func() {
		auxOffCheck(c, "Use the send queue when a receiver is stopped", false)
		bpInfoBox(c, "Inactive users are shown in grey. The message is sent when the receiver becomes active.")
	})
}

func (sv *settingsView) recv(c *ui.Context) {
	sv.paneGroup(c, func() {
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
		bpIndent(c, func() {
			auxLabeled(c, "Sound", 80, func() { auxOffInput(c, "Sound file", "System sound", 220) })
		})
		auxOffCheck(c, "Open the log viewer when the tray icon is clicked", false)
		auxOffCheck(c, "Don't close the receive window when replying", false)
		auxOffCheck(c, "Restore unopened messages after restart", false)
	})
}

func (sv *settingsView) tray(c *ui.Context) {
	sv.paneGroup(c, func() {
		auxOffCheck(c, "Confirm the icon in the task tray at starting", true)
		bpInfoBox(c, "TCP Messenger always shows its icon in the menu bar or the notification area.")
	})
}

func (sv *settingsView) image(c *ui.Context) {
	sv.paneGroup(c, func() {
		auxLabeled(c, "Show recv image", 140, func() {
			auxOffSelect(c, "Image display", "Always show directly", "Show directly if priority user", "Always show via Image button")
		})
		auxNote(c, "(To insert an image, paste it in the send window)")
	})
	bpGroup(c, "Display (Area Specified) Capture", func() {
		format := strings.ToUpper(sv.s.CaptureFormat)
		if format != "JPG" {
			format = "PNG"
		}
		auxLabeled(c, "Capture format", 140, func() {
			if ui.Select(c, &format, []string{"PNG", "JPG"}).Label("Capture format").Width(100).Changed() {
				sv.s.CaptureFormat = strings.ToLower(format)
			}
		})
		auxOffCheck(c, "Minimize the send window as default", true)
		auxOffCheck(c, "Clipboard check as default", true)
		auxOffCheck(c, "Save check as default", false)
	})
}

func (sv *settingsView) url(c *ui.Context) {
	sv.paneGroup(c, func() {
		auxOffCheck(c, "One click opens (default: double click)", false)
		auxOffCheck(c, "Link targets are only URLs (exclude files)", false)
		bpLabel(c, "Permitted file extensions").FontSize(14).FontWeight(400)
		exts := "txt log ini pdf doc docx ppt pptx xls xlsx csv html htm xml jpg jpeg png gif"
		bpDlgField(c, ui.TextArea(c, &exts).Lines(3, 5).Disabled(true).Label("Permitted file extensions"))
	})
}

func (sv *settingsView) logPane(c *ui.Context) {
	sv.paneGroup(c, func() {
		on := sv.s.LogEnabled
		ui.Checkbox(c, &sv.s.LogEnabled, "Logging available")
		bpIndent(c, func() {
			ui.Row(c).Gap(sp3).AlignItems(ui.Center).Children(func() {
				if bpDlgButton(c, "Log File…", false).Disabled(!on).Clicked() {
					sv.pickFile("Log file", sv.s.LogFile, func(p string) {
						if p != "" {
							sv.s.LogFile = p
						}
					})
				}
				auxInput(c, &sv.s.LogFile, "Log file").Placeholder("Default (in the settings folder)").Grow(1).Disabled(!on)
			})
			ui.Checkbox(c, &sv.s.LogLogonName, "Log the logon name").Disabled(!on)
			ui.Checkbox(c, &sv.s.LogIPAddr, "Log the IP address").Disabled(!on)
			ui.Checkbox(c, &sv.s.LogImages, "Log images").Disabled(!on)
		})
		if dir := settingsDirLabel(sv.a); dir != "memory" {
			auxLabeled(c, "Log folder", 100, func() { bpReadonly(c, dir, 0).Grow(1) })
		}
	})
}

func (sv *settingsView) download(c *ui.Context) {
	sv.paneGroup(c, func() {
		auxOffCheck(c, "Download attached files automatically", false)
		ui.Row(c).Gap(sp3).AlignItems(ui.Center).Children(func() {
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
		auxInput(c, &sv.s.DownloadDir, "Download folder").Placeholder("Downloads").FillWidth()
		auxLabeled(c, "Download size limit (MB)", 240, func() { auxOffInput(c, "Size limit", "1000", 56) })
		auxLabeled(c, "Grace before auto download (sec)", 240, func() { auxOffInput(c, "Grace seconds", "120", 56) })
	})
}

func (sv *settingsView) remote(c *ui.Context) {
	sv.paneGroup(c, func() {
		for _, what := range []string{"reboot the PC", "shut down the PC", "quit TCP Messenger"} {
			auxOffCheck(c, "Allow a message to "+what, false)
		}
		bpInfoBox(c, "Remote commands are not available in TCP Messenger.")
	})
}

func (sv *settingsView) recvTrans(c *ui.Context) {
	sv.paneGroup(c, func() {
		auxLabeled(c, "Transfer mode", 120, func() {
			auxOffSelect(c, "Transfer mode", "No Transfer", "Transfer (at LockScreen)", "Transfer (always)")
		})
		bpRadios(c, func() {
			ui.RadioGroup(c, func() {
				ui.Radio(c, &sv.trans, "slack", "For Slack")
				ui.Radio(c, &sv.trans, "general", "For General")
			}).Row().Gap(sp5).Disabled(true).Label("Transfer target")
		})
		bpIndent(c, func() {
			if sv.trans == "slack" {
				auxLabeled(c, "#channel/@username", 160, func() { auxOffInput(c, "Channel or username", "@username", 200) })
			} else {
				auxLabeled(c, "URL", 160, func() { auxOffInput(c, "URL", "", 200) })
			}
		})
		bpInfoBox(c, "Message transfer is not available in TCP Messenger.")
	})
}

func (sv *settingsView) update(c *ui.Context) {
	sv.paneGroup(c, func() {
		auxOffCheck(c, "Regularly check the latest version", false)
		bpIndent(c, func() {
			auxOffCheck(c, "Check the latest version, but do not install", false)
		})
		ui.Row(c).Children(func() { bpDlgButton(c, "Check Update", false).Disabled(true) })
	})
}

func (sv *settingsView) misc(c *ui.Context) {
	sv.paneGroup(c, func() {
		ui.Checkbox(c, &sv.s.StartAtLogin, "Start TCP Messenger at login")
		ui.Checkbox(c, &sv.s.AutoAbsence, "Change to [Away] when the desktop is idle")
		bpIndent(c, func() {
			auxLabeled(c, "Idle time (min)", 120, func() {
				ui.NumberInput(c, &sv.minutes, 1, 600, 1).Label("Idle time").Width(110).Disabled(!sv.s.AutoAbsence)
			})
		})
		auxOffCheck(c, "Reduce tray icon actions on receiving", false)
		auxLabeled(c, "Other-segment UDP rate limit", 220, func() { auxOffInput(c, "UDP rate limit", "0", 56) })
	})
	bpGroup(c, "Password", func() {
		state := "No password is set."
		if sv.s.Password != "" {
			state = "A password is set: it is asked before opening locked messages."
		}
		bpInfo(c, state)
		auxLabeled(c, "New password", 140, func() { auxInput(c, &sv.pw1, "New password").Password().Width(180) })
		auxLabeled(c, "Confirm password", 140, func() { auxInput(c, &sv.pw2, "Confirm password").Password().Width(180) })
		ui.Row(c).Gap(sp3).AlignItems(ui.Center).Children(func() {
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
	bpGroup(c, "Export Settings", func() {
		ui.Row(c).Children(func() { bpDlgButton(c, "Export to File…", false).Disabled(true) })
		auxLabeled(c, "Settings folder", 120, func() { bpReadonly(c, settingsDirLabel(sv.a), 0).Grow(1) })
	})
}

func settingsDirLabel(a *tcpApp) string {
	if a.store != nil && a.store.Dir() != "" {
		return a.store.Dir()
	}
	return "memory"
}
