package main

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// helpWin is the open help window, nil while it is closed; helpTopic is
// the chapter it scrolls to next.
var (
	helpWin   *mygo.Window
	helpTopic string
)

// nativeWindows reports whether windows open: not in tests.
var nativeWindows = func() bool { return false }

// openHelp opens the help at the chapter of topic, or brings the open
// help forward and goes there.
func openHelp(topic string) {
	helpTopic = topic
	if helpWin != nil {
		helpWin.Focus()
		helpWin.Update(func() {})
		return
	}
	if !nativeWindows() {
		return
	}
	w := mygo.NewWindow(mygo.WindowOptions{
		Title:     appName + " Help",
		Width:     680,
		Height:    640,
		MinWidth:  420,
		MinHeight: 320,
		StateKey:  "help",
		Content:   ui.View(helpView),
	})
	helpWin = w
	w.OnClosed(func() {
		if helpWin == w {
			helpWin = nil
		}
	})
}

// helpChapter is a chapter of the help: its bar, then its paragraphs and
// sub-sections.
type helpChapter struct {
	id, title string
	body      []string
	subs      []helpSub
}

type helpSub struct {
	id, title string
	body      []string
}

var helpFeatures = []string{
	"Serverless messaging over the LAN: no server, no account.",
	"Auto member detection by UDP broadcast, also across segments.",
	"Sealed and password-locked messages, with an open notice to the sender.",
	"Files and screen captures travel inside the message.",
	"Searchable message log with stars, marks and notes.",
	"Absence modes that answer for you.",
}

var helpChapters = []helpChapter{
	{id: "send", title: "Send Message", body: []string{
		"The send window lists every member on the LAN. Choose one or more rows, write the message and press Send.",
		"Seal asks the receiver to open the message before reading it. Lock also asks for the receiver's password.",
		"Attach files with the clip button or by dropping them on the window. The capture button takes an area of the screen and attaches it as an image.",
	}},
	{id: "receive", title: "Receive Message", body: []string{
		"Each message opens its own window. A sealed one shows Open first; when you open it the sender is told.",
		"Reply opens a send window to the sender. With Quote checked, the message is quoted with the quote mark of the settings.",
	}},
	{id: "log", title: "Log Viewer", body: []string{
		"Every message sent and received is kept in the log. Search matches show in red; marked messages in green.",
		"Star a message to keep it in the Starred filter. A note shows under the message as a reference.",
		"Export writes the messages shown to a text file.",
	}},
	{id: "absence", title: "Absence Mode", body: []string{
		"While absent, your name shows with ! in other members' lists, and the message of the chosen mode is sent back to whoever messages you.",
		"Auto Absence switches to absent when the desktop is idle, and back when you return.",
	}},
	{id: "settings", title: "Settings", subs: []helpSub{
		{"settings/0", "Basic/LAN", []string{"Your user name and group, and the broadcast addresses that reach other segments. The port takes effect after restart."}},
		{"settings/1", "Master", []string{"Member Master is not available in TCP Messenger. Use broadcast addresses instead."}},
		{"settings/2", "Detail", []string{"How arriving messages notify you."}},
		{"settings/3", "Send Window", []string{"Sealing by default, quoting, and what Ctrl+Enter does."}},
		{"settings/4", "Recv Window", []string{"Whether received messages pop up, sound, and tell the sender when opened."}},
		{"settings/5", "TaskTray", []string{"TCP Messenger always shows its icon in the menu bar or the notification area."}},
		{"settings/6", "Image/Capture", []string{"The file format of screen captures."}},
		{"settings/7", "URL/File Link", []string{"How links in messages open."}},
		{"settings/8", "Log", []string{"Whether messages are logged, and what each entry records."}},
		{"settings/9", "Auto FileDownload", []string{"The folder received files are saved to."}},
		{"settings/10", "Remote Command", []string{"Remote commands are not available in TCP Messenger."}},
		{"settings/11", "RecvTrans", []string{"Message transfer is not available in TCP Messenger."}},
		{"settings/12", "Auto Update", []string{"Updates come with new releases of the app."}},
		{"settings/13", "Misc", []string{"Starting at login, the password for locked messages, and the idle time of Auto Absence."}},
	}},
}

// helpKeys are the keyboard table's rows.
var helpKeys = [][2]string{
	{"Cmd+N / Ctrl+N", "Send Message"},
	{"Cmd+Enter / Ctrl+Enter", "Send (when the setting is on)"},
	{"Cmd+L / Ctrl+L", "Log Viewer"},
	{"F5", "Refresh Members"},
	{"Alt+F, Alt+S, ...", "Open the window's menus"},
	{"Esc", "Close a received message"},
}

// helpView draws the help after the bundled manual's stylesheet: azure
// ground, serif text, every heading on a blue chapter bar.
func helpView(c *ui.Context) {
	p := bpApply(c, func(p *bpPalette) ui.Color { return p.helpGround }, false)
	topic := helpTopic
	helpTopic = ""
	ui.Scroll(c).Fill().Children(func() {
		ui.Column(c).Padding(sp3).Font(bpSerifFont()).FontSize(15.2).TextColor(p.ink).Children(func() {
			ui.Row(c).Justify(ui.Center).Margin(0, 0, sp4, 0).Children(func() {
				ui.Text(c, appName).Bold().FontSize(32).Padding(0, sp1).
					Background(p.helpChapter).TextColor(p.helpTitleInk)
			})
			ui.Text(c, "v"+appVersion()+"  LastUpdate: 2026/11/03").TextAlign(ui.End)
			ui.Text(c, "Serverless LAN Messenger").Bold().FontSize(22.8).TextAlign(ui.Center).
				Margin(4, 10, 8, 10).Padding(5).Background(p.helpNotice).Border(3, p.helpHeadlineEdge)
			ui.Column(c).Margin(sp3).Padding(sp3).Gap(sp2).Background(p.helpFeature).
				Border(3, p.helpHeadlineEdge).TextColor(p.helpFeatureInk).Children(func() {
				for i, f := range helpFeatures {
					ui.Textf(c.Key(i), "%d. %s", i+1, f).Bold()
				}
			})
			for _, ch := range helpChapters {
				bar := helpBar(c.Key(ch.id), ch.title, 22.8, 10, p).Margin(20, 0, 10, 0)
				if topic == ch.id {
					bar.ScrollIntoView()
				}
				ui.Column(c.Key(ch.id+"/body")).Margin(0, 0, sp4, sp4).Gap(sp3).Children(func() {
					for i, b := range ch.body {
						ui.Text(c.Key(i), b).Wrap()
					}
					for _, s := range ch.subs {
						sb := helpBar(c.Key(s.id), s.title, 17.8, 5, p).Margin(sp2, 0, 0, 0)
						if topic == s.id {
							sb.ScrollIntoView()
						}
						for i, b := range s.body {
							ui.Text(c.Key(s.id+string(rune('a'+i))), b).Wrap().Margin(0, 0, 0, 10)
						}
					}
				})
			}
			helpBar(c.Key("keys"), "Keyboard", 22.8, 10, p).Margin(20, 0, 10, 0)
			helpTable(c, p)
			helpBar(c.Key("license"), "License", 22.8, 10, p).Margin(20, 0, 10, 0)
			ui.Text(c, "TCP Messenger is freeware. Use it, copy it and pass it on freely; it comes with no warranty.").Wrap().
				Padding(sp3).Background(p.helpLicense).Border(3, p.helpLicenseEdge)
		})
	})
}

// helpBar is a chapter (h2) or sub-section (h3) bar.
func helpBar(c *ui.Context, title string, size, pad float32, p *bpPalette) ui.Element {
	return ui.Text(c, title).Bold().FontSize(size).PaddingStart(pad).
		Background(p.helpChapter).TextColor(p.inverse)
}

// helpTable is the keyboard table: help-cell rows under a help-head row
// in a 4px help-edge frame.
func helpTable(c *ui.Context, p *bpPalette) {
	ui.Column(c).Margin(0, 0, 0, sp4).Border(4, p.helpEdge).Background(p.helpCell).Padding(sp1).Gap(sp1).
		FontSize(14.4).Children(func() {
		row := func(k int, a, b string, head bool) {
			ui.Row(c.Key(k)).Gap(sp1).Children(func() {
				for j, s := range []string{a, b} {
					t := ui.Text(c.Key(j), s).Padding(1, sp1).Border(1, p.helpEdge)
					if j == 0 {
						t.Width(200).Shrink(0)
					} else {
						t.Grow(1)
					}
					if head {
						t.Bold().Background(p.helpHead).TextAlign(ui.Center)
					}
				}
			})
		}
		row(-1, "Key", "Does", true)
		for i, k := range helpKeys {
			row(i, k[0], k[1], false)
		}
	})
}

// appVersion is the app's version, or that of mygo.json in tests.
func appVersion() string {
	if nativeWindows() {
		if v := mygo.App.Version(); v != "" {
			return v
		}
	}
	return "0.2.0"
}
