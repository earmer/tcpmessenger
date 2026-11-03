package main

import (
	"log"
	"os"
	"path/filepath"
	"strings"

	"my-app/internal/core"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// recvWin is a Receive Message window, one per incoming message.
type recvWin struct {
	a      *tcpApp
	win    *mygo.Window
	m      core.Message
	closed bool

	opened   bool // the text shows: not sealed, or opened
	asking   bool // a locked message asks for the password
	password string
	wrong    bool
	quote    bool
	status   string // the outcome of the last save

	images []*ui.Bitmap // per attachment, nil for files
}

// handleReceived takes a message that came in: it logs it, then opens its
// window or keeps it for later, and tells the user.
func (a *tcpApp) handleReceived(m core.Message) {
	m.Incoming = true
	if a.settings.LogEnabled {
		if _, err := a.store.Append(m); err != nil {
			log.Println("log:", err)
		}
		logViewerChanged()
	}
	s := a.settings
	// While absent, the absence message goes back to the sender, as IP
	// Messenger does. Auto replies are not answered, so two absent users
	// don't reply to each other forever.
	if s.Absent && !m.AutoReply && s.AbsenceIndex >= 0 && s.AbsenceIndex < len(s.AbsenceModes) {
		if _, err := a.net.Send(core.Message{To: []core.User{m.From}, Text: s.AbsenceModes[s.AbsenceIndex].Message, AutoReply: true}); err != nil {
			log.Println("absence reply:", err)
		}
	}
	if s.PopupOnReceive && !(s.NoPopupWhenAbsent && s.Absent) {
		openReceiveWindow(a, m)
	} else {
		a.queued = append(a.queued, m)
	}
	if s.NotifyWithToast {
		body := m.Text
		if m.Sealed {
			body = "Sealed message"
		}
		a.desk.notify("Message from "+displayName(m.From), body, a.openQueued)
	}
	if s.PlaySound {
		a.desk.beep()
	}
	syncMenus(a)
}

// handleOpened tells the user that a peer opened a sealed message we sent.
func (a *tcpApp) handleOpened(from core.User, id string) {
	a.desk.notify(appName, displayName(from)+" opened your message", nil)
}

// openQueued opens the windows of the messages received without one.
func (a *tcpApp) openQueued() {
	q := a.queued
	a.queued = nil
	for _, m := range q {
		openReceiveWindow(a, m)
	}
	syncMenus(a)
}

// unreadCount is what the tray shows: messages without a window, and
// sealed ones not opened yet.
func (a *tcpApp) unreadCount() int {
	n := len(a.queued)
	for _, rw := range a.recvWins {
		if !rw.opened {
			n++
		}
	}
	return n
}

// openReceiveWindow opens the window of an incoming message.
func openReceiveWindow(a *tcpApp, m core.Message) *recvWin {
	rw := &recvWin{a: a, m: m, opened: !m.Sealed || m.Opened, quote: a.settings.QuoteOnReply}
	rw.images = make([]*ui.Bitmap, len(m.Attachments))
	for i, at := range m.Attachments {
		if at.IsImage {
			if b, err := ui.DecodeBitmap(at.Data); err == nil {
				rw.images[i] = b
			}
		}
	}
	rw.win = a.desk.newWindow(mygo.WindowOptions{
		Title:     "Receive Message",
		Width:     420,
		Height:    380,
		MinWidth:  320,
		MinHeight: 260,
		Content:   ui.View(rw.view),
	})
	a.recvWins = append(a.recvWins, rw)
	if rw.win != nil {
		rw.win.OnClosed(rw.onClosed)
		rw.win.Focus()
	}
	return rw
}

func (rw *recvWin) onClosed() {
	rw.closed = true
	a := rw.a
	for i, w := range a.recvWins {
		if w == rw {
			a.recvWins = append(a.recvWins[:i], a.recvWins[i+1:]...)
			break
		}
	}
	syncMenus(a)
}

func (rw *recvWin) close() {
	if rw.win != nil {
		rw.win.Close()
		return
	}
	rw.onClosed()
}

// open shows a sealed message, asking first for the password of a locked
// one, and tells the sender it was opened.
func (rw *recvWin) open() {
	a := rw.a
	if rw.m.Locked && a.settings.Password != "" && !rw.asking {
		rw.asking = true
		return
	}
	if rw.asking {
		if !checkPassword(a.settings.Password, rw.password) {
			rw.wrong = true
			rw.password = ""
			return
		}
		rw.asking, rw.wrong, rw.password = false, false, ""
	}
	rw.opened = true
	rw.m.Opened = true
	if a.settings.OpenCheck {
		m := rw.m
		a.desk.background(func() {
			if err := a.net.NotifyOpened(m); err != nil {
				log.Println("open notice:", err)
			}
		})
	}
	syncMenus(a)
}

// reply opens a send window to the sender, quoting the text when asked.
func (rw *recvWin) reply() {
	text := ""
	if rw.quote && rw.opened {
		text = recvQuote(rw.m.Text, rw.a.settings.QuoteMark)
	}
	openSendWindow(rw.a, []core.User{rw.m.From}, text)
}

// recvQuote puts mark before every line of text, and a blank line after
// it for the reply.
func recvQuote(text, mark string) string {
	text = strings.TrimRight(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		lines[i] = mark + l
	}
	return strings.Join(lines, "\n") + "\n\n"
}

// saveAttachment asks where to save attachment i, in the download
// directory by default.
func (rw *recvWin) saveAttachment(i int) {
	at := rw.m.Attachments[i]
	dir := rw.a.settings.DownloadDir
	if dir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			dir = filepath.Join(home, "Downloads")
		}
	}
	rw.a.desk.saveFile(rw.win, filepath.Join(dir, filepath.Base(at.Name)), func(path string) {
		if err := os.WriteFile(path, at.Data, 0o644); err != nil {
			rw.status = "Could not save " + at.Name + ": " + err.Error()
			return
		}
		rw.status = "Saved " + filepath.Base(path)
	})
}

func (rw *recvWin) view(c *ui.Context) {
	a := rw.a
	k := bpApply(c, groundChrome, false)
	t := c.Theme()
	m := rw.m
	ui.Column(c).Fill().Children(func() {
		// Who sent it.
		bpToolbar(c, func() {
			bpChip(c, displayName(m.From)).Selectable()
			if g := strings.Trim(m.From.Group+"/"+m.From.Host, "/"); g != "" {
				ui.Text(c, g).SingleLine().TextColor(k.muted).Shrink(1)
			}
			ui.Spacer(c)
			bpSep(c)
			if bpIconButton(c, iconLog, "Log viewer").Clicked() {
				openLogViewer(a)
			}
		})

		// The message.
		bpField(c, ui.Column(c).Grow(1).Margin(sp3)).Children(func() {
			switch {
			case rw.asking:
				ui.Column(c).Grow(1).Center().Gap(sp3).Padding(sp4).Children(func() {
					ui.Text(c, "This message is locked. Enter your password.")
					in := bpField(c, ui.TextInput(c, &rw.password).Password().Label("Password").Placeholder("Password").Width(200).AutoFocus())
					if rw.wrong {
						ui.Text(c, "Wrong password").TextColor(k.alertInk)
					}
					ui.Row(c).Gap(sp3).Children(func() {
						if bpButton(c, "OK").Clicked() || in.Submitted() {
							rw.open()
						}
						if bpButton(c, "Cancel").Clicked() {
							rw.asking, rw.wrong, rw.password = false, false, ""
						}
					})
				})
			case !rw.opened:
				ui.Column(c).Grow(1).Center().Children(func() {
					if bpButtonBase(c, "Open").MinWidth(200).Padding(sp3, sp4).Children(func() {
						ui.Column(c).AlignItems(ui.Center).Gap(sp1).Children(func() {
							ui.Text(c, "Unopened").Bold()
							ui.Text(c, "(Click to open)").FontSize(12).TextColor(k.muted)
						})
					}).Clicked() {
						rw.open()
					}
				})
			default:
				ui.Scroll(c).Grow(1).Children(func() {
					ui.Column(c).Padding(sp2, sp3).Gap(sp3).Selectable().Children(func() {
						ui.Text(c, m.Text)
						for i, at := range m.Attachments {
							if b := rw.images[i]; b != nil {
								w, h := b.Size()
								fw, fh := float32(w), float32(h)
								if fw > 340 {
									fw, fh = 340, fh*340/fw
								}
								ui.Image(c.Key(i), b).Size(fw, fh).Fit(ui.Contain).Label(at.Name)
							}
						}
					})
					for i, at := range m.Attachments {
						if rw.images[i] != nil {
							continue
						}
						ui.Row(c.Key(-1-i)).Padding(sp1, sp3).Gap(sp3).AlignItems(ui.Center).Children(func() {
							bpChip(c, at.Name).Shrink(1)
							ui.Text(c, sizeText(at.Size)).SingleLine().TextColor(k.muted).Grow(1)
							if bpButton(c, "Save…").Label("Save " + at.Name).Clicked() {
								rw.saveAttachment(i)
							}
						})
					}
					if rw.status != "" {
						ui.Text(c, rw.status).FontSize(12).TextColor(t.TextMuted).Padding(sp1, sp3)
					}
				})
			}
		})

		// The actions.
		ui.Row(c).Padding(0, sp3, sp3, sp3).Gap(sp3).AlignItems(ui.Center).Children(func() {
			if bpButton(c, "Reply").Grow(1).Clicked() {
				rw.reply()
			}
			ui.Checkbox(c, &rw.quote, "Quote")
			if bpButton(c, "Close").Clicked() {
				rw.close()
			}
		})

		bpStatusBar(c, func() {
			if m.Sealed {
				if rw.opened {
					ui.Text(c, "sealed/opened")
				} else {
					ui.Text(c, "sealed")
				}
			}
			if m.Locked {
				ui.Text(c, "locked")
			}
			if m.Signed {
				ui.Text(c, "signed")
			}
			if n := len(m.Attachments); n > 0 {
				ui.Textf(c, "files=%d", n)
			}
			ui.Text(c, longTime(m.Time))
		})
	})
	if c.Shortcut(0, ui.KeyEscape) {
		rw.close()
	}
}
