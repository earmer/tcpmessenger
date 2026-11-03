package main

import (
	"fmt"
	"log"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"my-app/internal/core"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// sendWin is a Send Message window: the main window, or one opened to
// reply. Its fields are used on the main thread only.
type sendWin struct {
	a      *tcpApp
	win    *mygo.Window
	isMain bool
	closed bool

	list   ui.ListState
	chosen ui.Selection[string] // user IDs
	group  string               // the group filter, or sendAllGroups
	extra  []core.User          // recipients preselected but not among the peers
	users  []core.User          // the rows of the last frame, sorted

	text   string
	seal   bool
	lock   bool
	attach []core.Attachment

	// A failed delivery, offered for a retry.
	failOpen bool
	failMsg  core.Message
	failTo   []core.User
	failErr  string
}

// pendingSend is a sent message whose deliveries have not all reported.
type pendingSend struct {
	msg  core.Message
	from *sendWin
	left int
}

// deliveryFailure is the outcome of a delivery reported before Send
// returned its ID: err is nil when it was delivered.
type deliveryFailure struct {
	to  core.User
	err error
}

// openSendWindow opens a Send Message window with the users of to chosen
// and text in its message field. The first one opened is the main window.
func openSendWindow(a *tcpApp, to []core.User, text string) *sendWin {
	sw := newSendWin(a, to, text)
	if a.mainSend == nil {
		sw.isMain = true
		a.mainSend = sw
	}
	opts := mygo.WindowOptions{
		Title:     "Send Message",
		Width:     460,
		Height:    430,
		MinWidth:  360,
		MinHeight: 320,
		Content:   ui.View(sw.view),
	}
	if sw.isMain {
		opts.StateKey = "main"
	}
	sw.win = a.desk.newWindow(opts)
	a.sendWins = append(a.sendWins, sw)
	if sw.win != nil {
		if sw.isMain {
			a.main = sw.win
			// The main window hides to the tray rather than closing, so
			// the app keeps receiving.
			sw.win.OnClose(func(e *mygo.CloseEvent) {
				if !a.quitting && a.tray != nil {
					e.PreventDefault()
					sw.win.Hide()
				}
			})
		}
		sw.win.OnClosed(sw.onClosed)
		sw.win.Focus()
	}
	return sw
}

func newSendWin(a *tcpApp, to []core.User, text string) *sendWin {
	sw := &sendWin{a: a, text: text, seal: a.settings.SealByDefault, group: sendAllGroups}
	for _, u := range to {
		sw.chosen.Add(u.ID)
		if !hasUser(a.peers, u.ID) {
			sw.extra = append(sw.extra, u)
		}
	}
	return sw
}

func hasUser(us []core.User, id string) bool {
	for _, u := range us {
		if u.ID == id {
			return true
		}
	}
	return false
}

// onClosed forgets the window once it closed.
func (sw *sendWin) onClosed() {
	sw.closed = true
	a := sw.a
	for i, w := range a.sendWins {
		if w == sw {
			a.sendWins = append(a.sendWins[:i], a.sendWins[i+1:]...)
			break
		}
	}
	if a.mainSend == sw {
		a.mainSend = nil
		a.main = nil
	}
}

// close closes the window, or forgets it when it has none (tests).
func (sw *sendWin) close() {
	if sw.win != nil {
		sw.win.Close()
		return
	}
	sw.onClosed()
}

// sendListUsers returns the users the list shows: the peers and the
// preselected recipients, without hidden ones, sorted by o.
func sendListUsers(peers, extra []core.User, o core.ListOptions) []core.User {
	us := make([]core.User, 0, len(peers)+len(extra))
	for _, u := range peers {
		if u.Priority >= 0 {
			us = append(us, u)
		}
	}
	for _, u := range extra {
		if !hasUser(us, u.ID) {
			us = append(us, u)
		}
	}
	sendSortUsers(us, o)
	return us
}

// sendSortUsers sorts the user list as the Sort/Display settings say:
// higher priority first, then by group when SortByGroup, then by the
// second key, each optionally reversed, strings optionally ignoring case.
func sendSortUsers(us []core.User, o core.ListOptions) {
	str := func(a, b string) int {
		if o.IgnoreCase {
			a, b = strings.ToLower(a), strings.ToLower(b)
		}
		return strings.Compare(a, b)
	}
	sort.SliceStable(us, func(i, j int) bool {
		a, b := us[i], us[j]
		if a.Priority != b.Priority {
			return a.Priority > b.Priority
		}
		if o.SortByGroup {
			if d := str(a.Group, b.Group); d != 0 {
				if o.GroupReverse {
					d = -d
				}
				return d < 0
			}
		}
		var d int
		switch o.Second {
		case core.SortByIP:
			d = compareIP(userIP(a), userIP(b))
		case core.SortByHost:
			d = str(a.Host, b.Host)
		default:
			d = str(a.Name, b.Name)
		}
		if d == 0 {
			d = strings.Compare(a.ID, b.ID)
		}
		if o.SecondReverse {
			d = -d
		}
		return d < 0
	})
}

// userIP is the IP part of a user's address.
func userIP(u core.User) string {
	if h, _, err := net.SplitHostPort(u.Addr); err == nil {
		return h
	}
	return u.Addr
}

// compareIP orders addresses numerically, those that do not parse last.
func compareIP(a, b string) int {
	x, errA := netip.ParseAddr(a)
	y, errB := netip.ParseAddr(b)
	switch {
	case errA != nil && errB != nil:
		return strings.Compare(a, b)
	case errA != nil:
		return 1
	case errB != nil:
		return -1
	}
	return x.Compare(y)
}

// sendColumn is a column of the user list.
type sendColumn struct {
	col  bpCol
	cell func(u core.User) string
}

func sendColumns(o core.ListOptions) []sendColumn {
	cols := []sendColumn{{bpCol{Title: "User"}, nil}}
	if o.ShowGroup {
		cols = append(cols, sendColumn{bpCol{Title: "Group", Width: 80}, func(u core.User) string { return u.Group }})
	}
	if o.ShowHost {
		cols = append(cols, sendColumn{bpCol{Title: "Host", Width: 70}, func(u core.User) string { return u.Host }})
	}
	if o.ShowLogon {
		cols = append(cols, sendColumn{bpCol{Title: "Logon", Width: 70}, func(u core.User) string { return u.Logon }})
	}
	if o.ShowIP {
		cols = append(cols, sendColumn{bpCol{Title: "IP", Width: 100}, userIP})
	}
	if o.ShowPriority {
		cols = append(cols, sendColumn{bpCol{Title: "Priority", Width: 56, Align: ui.End}, func(u core.User) string { return fmt.Sprint(u.Priority) }})
	}
	return cols
}

const sendAllGroups = "All Groups"

// sendGroups are the groups of the users, sorted, for the group filter.
func sendGroups(us []core.User) []string {
	var out []string
	for _, u := range us {
		if u.Group != "" && !hasString(out, u.Group) {
			out = append(out, u.Group)
		}
	}
	sort.Strings(out)
	return out
}

func hasString(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

// sendFilterGroup keeps the users of group, all of them for sendAllGroups.
func sendFilterGroup(us []core.User, group string) []core.User {
	if group == sendAllGroups || group == "" {
		return us
	}
	var out []core.User
	for _, u := range us {
		if u.Group == group {
			out = append(out, u)
		}
	}
	return out
}

// selectedUsers are the chosen users the list shows, in its order.
func (sw *sendWin) selectedUsers() []core.User {
	var out []core.User
	for _, u := range sw.users {
		if sw.chosen.Has(u.ID) {
			out = append(out, u)
		}
	}
	return out
}

func (sw *sendWin) view(c *ui.Context) {
	a := sw.a
	k := bpApply(c, groundChrome, false)
	t := c.Theme()
	if sw.isMain {
		syncMenus(a)
	}
	all := sendListUsers(a.peers, sw.extra, a.settings.List)
	groups := sendGroups(all)
	if sw.group != sendAllGroups && !hasString(groups, sw.group) {
		sw.group = sendAllGroups
	}
	sw.users = sendFilterGroup(all, sw.group)
	users := sw.users
	selected := sw.selectedUsers()
	attach := func() { a.desk.openFiles(sw.win, "Attach Files", sw.attachFiles) }
	capture := func() { openCapture(a, func(img core.Attachment) { sw.attach = append(sw.attach, img) }) }

	ui.Column(c).Fill().Children(func() {
		bpMenubar(c,
			bpMenu{"File(F)", func(m *ui.Menu) {
				if m.Item("Attach Files…").Chosen() {
					attach()
				}
				if m.Item("Screen Capture…").Chosen() {
					capture()
				}
				m.Separator()
				if m.Item("Close").Chosen() {
					sw.close()
				}
			}},
			bpMenu{"Settings(S)", func(m *ui.Menu) {
				if m.Item("Settings…").Chosen() {
					openSettings(a)
				}
				if m.Item("Sort/Display…").Chosen() {
					openSortDisplay(a)
				}
				if m.Item("Absence…").Chosen() {
					openAbsence(a)
				}
			}},
			bpMenu{"Window(W)", func(m *ui.Menu) {
				if m.Item("Log Viewer").Chosen() {
					openLogViewer(a)
				}
				if m.Item("Refresh Members").Chosen() {
					a.net.Refresh()
				}
			}},
			bpMenu{"Help(H)", func(m *ui.Menu) {
				if m.Item(appName + " Help").Chosen() {
					openHelp("send")
				}
				if m.Item("About " + appName).Chosen() {
					showAbout()
				}
			}},
		)

		bpToolbar(c, func() {
			if bpIconButton(c, iconRefresh, "Refresh member list").Clicked() {
				a.net.Refresh()
			}
			bpSep(c)
			if bpIconButton(c, iconAttach, "Attach files").Clicked() {
				attach()
			}
			if bpIconButton(c, iconCapture, "Screen capture").Clicked() {
				capture()
			}
			bpSep(c)
			ui.Select(c, &sw.group, append([]string{sendAllGroups}, groups...)).Label("Group filter").Width(140).FontSize(13)
			ui.Spacer(c)
			bpLabel(c, fmt.Sprintf("Users %d", len(users)))
		})

		// The user list.
		ui.Column(c).Grow(1).MinHeight(110).Padding(sp3).Children(func() {
			cols := sendColumns(a.settings.List)
			bcols := make([]bpCol, len(cols))
			for i := range cols {
				bcols[i] = cols[i].col
			}
			sw.list.Key = func(i int) any { return users[i].ID }
			sw.list.Label = func(i int) string { return users[i].Name }
			sw.list.Selection = &sw.chosen
			bpList(c, &sw.list, bcols, len(users), func(row, col int) {
				u := users[row]
				if col == 0 {
					ui.Row(c).Gap(sp2).Children(func() {
						ui.Text(c, displayName(u)).SingleLine()
						if u.Absent {
							ui.Text(c, "!").Bold().TextColor(k.alertInk).Tooltip(absentTip(u))
						}
					})
					return
				}
				ui.Text(c, cols[col].cell(u)).SingleLine()
			}).Grow(1).Label("Users").Children(func() {
				if len(users) == 0 {
					ui.Text(c, "No one is online").TextColor(t.TextMuted).Padding(sp4)
				}
			})
		})

		// The message.
		ui.Column(c).Padding(0, sp3, sp3, sp3).Gap(sp2).Children(func() {
			bpField(c, ui.TextArea(c, &sw.text).Label("Message").Height(100).Font(bpMonoFont()).FontSize(13)).
				OnTextKey(func(key ui.TextKey) bool {
					if key.Key == ui.KeyEnter && key.Mods == ui.Cmd && a.settings.CtrlEnterSends {
						sw.text = key.Text()
						if sw.send() {
							key.SetText(sw.text)
						}
						return true
					}
					return false
				})
			if len(sw.attach) > 0 {
				ui.Row(c).Wrap().Gap(sp2).Children(func() {
					for i, at := range sw.attach {
						bpBevelOut(ui.Row(c.Key(i)).Gap(sp2).Padding(sp1, sp1, sp1, sp3).AlignItems(ui.Center).
							Background(k.face).Border(1, k.fieldEdge), k).Children(func() {
							ui.Textf(c, "%s (%s)", at.Name, sizeText(at.Size)).FontSize(12).Bold().SingleLine().MaxWidth(200)
							ui.ButtonBase(c).Label("Remove "+at.Name).Padding(0, sp1).TextColor(k.muted).OnClick(func() {
								sw.attach = append(sw.attach[:i:i], sw.attach[i+1:]...)
							}).Children(func() { ui.Text(c, "✕").FontSize(11) })
						})
					}
				})
			}
		})

		// The actions.
		ui.Row(c).Padding(0, sp3, sp3, sp3).Gap(sp3).AlignItems(ui.Center).Children(func() {
			if bpButton(c, "Send").Grow(1).Disabled(len(selected) == 0).Clicked() {
				sw.send()
			}
			if ui.Checkbox(c, &sw.seal, "Seal").Changed() && !sw.seal {
				sw.lock = false
			}
			if ui.Checkbox(c, &sw.lock, "Lock").Tooltip("The receiver enters a password to open it").Changed() && sw.lock {
				sw.seal = true
			}
		})

		bpStatusBar(c, func() {
			ui.Text(c, encryptionText(selected))
			ui.Textf(c, "sel=%d", len(selected))
			ui.Textf(c, "total=%d", len(all))
			ui.Textf(c, "port=%d", a.settings.Port)
		})
	})

	failMsg := "Could not deliver the message to " + userNames(sw.failTo) + "."
	if sw.failErr != "" {
		failMsg += "\n" + sw.failErr
	}
	if ui.AlertDialog(c, &sw.failOpen, "Message not delivered", failMsg, "Cancel", "Retry") == 1 {
		sw.retry()
	} else if !sw.failOpen {
		sw.failTo, sw.failErr = nil, ""
	}
}

func displayName(u core.User) string {
	switch {
	case u.Name != "":
		return u.Name
	case u.Logon != "":
		return u.Logon
	}
	return u.ID
}

func absentTip(u core.User) string {
	if u.AbsenceTitle != "" {
		return "Absent: " + u.AbsenceTitle
	}
	return "Absent"
}

func userNames(us []core.User) string {
	names := make([]string, len(us))
	for i, u := range us {
		names[i] = displayName(u)
	}
	return strings.Join(names, ", ")
}

// encryptionText says how a message to the users goes: encrypted and
// signed when every one of them has a key.
func encryptionText(us []core.User) string {
	if len(us) == 0 {
		return "RSA2/Signed+"
	}
	for _, u := range us {
		if !u.Encrypt {
			return "Plain (not encrypted)"
		}
	}
	return "RSA2/Signed+"
}

func sizeText(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}

// attachFiles reads the files chosen and adds them as attachments.
func (sw *sendWin) attachFiles(paths []string) {
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			log.Println("attach:", err)
			continue
		}
		ext := strings.ToLower(filepath.Ext(p))
		img := ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".gif"
		sw.attach = append(sw.attach, core.Attachment{Name: filepath.Base(p), Size: int64(len(data)), Data: data, IsImage: img})
	}
}

// send sends the message to the chosen users and reports whether it went.
func (sw *sendWin) send() bool {
	a := sw.a
	to := sw.selectedUsers()
	if len(to) == 0 {
		return false
	}
	m := core.Message{
		To:          to,
		Text:        sw.text,
		Sealed:      sw.seal || sw.lock,
		Locked:      sw.lock,
		Attachments: sw.attach,
	}
	if !a.sendMessage(m, sw) {
		return false
	}
	sw.text, sw.attach = "", nil
	switch {
	case !sw.isMain:
		sw.close()
	case a.settings.HideAfterSend && sw.win != nil:
		sw.win.Hide()
	}
	return true
}

// retry sends the failed message again to those it did not reach.
func (sw *sendWin) retry() {
	m := sw.failMsg
	m.To = sw.failTo
	m.ID = ""
	sw.failTo, sw.failErr = nil, ""
	sw.a.sendMessage(m, sw)
}

// sendMessage hands m to the network layer, logs it, and keeps it until
// its deliveries report.
func (a *tcpApp) sendMessage(m core.Message, from *sendWin) bool {
	id, err := a.net.Send(m)
	if err != nil {
		from.showFailure(m, m.To, err)
		return false
	}
	m.ID, m.From, m.Time = id, a.net.Self(), time.Now()
	if a.settings.LogEnabled {
		if _, err := a.store.Append(m); err != nil {
			log.Println("log:", err)
		}
		logViewerChanged()
	}
	p := &pendingSend{msg: m, from: from, left: len(m.To)}
	a.pending[id] = p
	// Deliveries the network layer reported before Send returned.
	early := a.failures[id]
	delete(a.failures, id)
	for _, f := range early {
		a.deliveryDone(p, id, f.to, f.err)
	}
	return true
}

// handleDelivered takes the outcome of a delivery.
func (a *tcpApp) handleDelivered(to core.User, id string, err error) {
	p := a.pending[id]
	if p == nil {
		// Send has not returned yet: sendMessage takes it then.
		a.failures[id] = append(a.failures[id], deliveryFailure{to, err})
		return
	}
	a.deliveryDone(p, id, to, err)
}

func (a *tcpApp) deliveryDone(p *pendingSend, id string, to core.User, err error) {
	p.left--
	if p.left <= 0 {
		delete(a.pending, id)
	}
	if err == nil {
		return
	}
	sw := p.from
	if sw == nil || sw.closed {
		if a.mainSend == nil {
			a.showMain()
		}
		sw = a.mainSend
	}
	if sw != nil {
		sw.showFailure(p.msg, []core.User{to}, err)
	}
}

// showFailure offers to send m again to the users it did not reach.
func (sw *sendWin) showFailure(m core.Message, to []core.User, err error) {
	if !sw.failOpen || sw.failMsg.ID != m.ID {
		sw.failTo, sw.failErr = nil, ""
	}
	sw.failMsg = m
	for _, u := range to {
		if !hasUser(sw.failTo, u.ID) {
			sw.failTo = append(sw.failTo, u)
		}
	}
	if err != nil {
		sw.failErr = err.Error()
	}
	sw.failOpen = true
	if sw.win != nil {
		sw.win.Show()
		sw.win.Focus()
	}
}
