package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"tcpmessenger/internal/core"
	"tcpmessenger/internal/store"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// logWin is the open log viewer, nil while it is closed; logView its state.
var (
	logWin  *mygo.Window
	logView *logViewer
)

// openLogViewer opens its window, or brings it forward when it is open.
func openLogViewer(a *tcpApp) {
	if logWin != nil {
		logView.refresh()
		logWin.Focus()
		return
	}
	lv := newLogViewer(a)
	w := mygo.NewWindow(mygo.WindowOptions{
		Title:     "TCPMsg LogViewer",
		Width:     860,
		Height:    620,
		MinWidth:  560,
		MinHeight: 360,
		StateKey:  "logviewer",
		Content:   ui.View(lv.view),
	})
	lv.bindWindow(w)
	logWin, logView = w, lv
	w.OnFocus(func() { lv.refresh() })
	w.OnClosed(func() {
		if logWin == w {
			logWin, logView = nil, nil
		}
	})
}

// logViewerChanged tells an open log viewer that the log changed (a message
// was received or sent), so that it reloads. Call it on the main thread.
func logViewerChanged() {
	if logWin != nil && logView != nil {
		logView.refresh()
		logWin.Update(func() {})
	}
}

const lvAllUsers = "All Users"

// logViewer is the state of the log viewer window.
type logViewer struct {
	a *tcpApp

	entries []core.LogEntry // shown, newest first
	total   int             // entries in the whole log

	query                 string
	starred, marked       bool
	withImages, withFiles bool
	user                  string // lvAllUsers or a label of userIDs
	userLabels            []string
	userIDs               map[string]string
	userByID              map[string]core.User
	selected              int
	list                  ui.ListState
	split                 float32
	note                  string
	noteSeq               int64
	confirmDelete         bool
	status                string
	bitmaps               map[string]*ui.Bitmap

	// saveFile asks where to save, then calls done with the path ("" when
	// canceled) on the main thread. reply opens a send window. Tests
	// replace them.
	saveFile func(name string, done func(path string))
	reply    func(to []core.User, quoted string)
}

func newLogViewer(a *tcpApp) *logViewer {
	lv := &logViewer{a: a, user: lvAllUsers, selected: -1, split: 340, bitmaps: map[string]*ui.Bitmap{}}
	lv.list.Selected = &lv.selected
	lv.list.Key = func(i int) any {
		if i < len(lv.entries) {
			return lv.entries[i].Seq
		}
		return i
	}
	lv.list.Label = func(i int) string {
		if i < len(lv.entries) {
			return lv.entries[i].From.Name
		}
		return ""
	}
	lv.saveFile = func(name string, done func(string)) { done("") }
	lv.reply = func(to []core.User, quoted string) { openSendWindow(a, to, quoted) }
	lv.refresh()
	return lv
}

// bindWindow makes the dialogs of lv attach to w.
func (lv *logViewer) bindWindow(w *mygo.Window) {
	lv.saveFile = func(name string, done func(string)) {
		go func() {
			p, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
				Parent:      w,
				Title:       "Export log",
				DefaultPath: name,
				Filters:     []mygo.FileFilter{{Name: "Text", Extensions: []string{"txt", "log"}}},
			})
			if err != nil {
				p = ""
			}
			w.Update(func() { done(p) })
		}()
	}
}

// refresh queries the store again with the current filters.
func (lv *logViewer) refresh() {
	if lv.a == nil || lv.a.store == nil {
		return
	}
	all, err := lv.a.store.Query(core.LogQuery{NoData: true})
	if err != nil {
		lv.status = "Error: " + err.Error()
		return
	}
	lv.total = len(all)
	lv.collectUsers(all)

	q := core.LogQuery{Text: strings.TrimSpace(lv.query), Starred: lv.starred, Marked: lv.marked, NoData: true}
	if lv.user != lvAllUsers {
		q.UserID = lv.userIDs[lv.user]
	}
	got := all
	if q.Text != "" || q.Starred || q.Marked || q.UserID != "" {
		got, err = lv.a.store.Query(q)
		if err != nil {
			lv.status = "Error: " + err.Error()
			return
		}
	}
	out := got
	if lv.withImages || lv.withFiles {
		out = got[:0:0]
		for _, e := range got {
			if lv.withImages && !lvHasAttachment(e, true) || lv.withFiles && !lvHasAttachment(e, false) {
				continue
			}
			out = append(out, e)
		}
	}
	if !sort.SliceIsSorted(out, func(i, j int) bool { return out[i].Seq > out[j].Seq }) {
		sort.SliceStable(out, func(i, j int) bool { return out[i].Seq > out[j].Seq })
	}

	var keep int64 = -1
	if cur, ok := lv.current(); ok {
		keep = cur.Seq
	}
	lv.entries = out
	lv.selected = -1
	for i, e := range out {
		if e.Seq == keep {
			lv.selected = i
		}
	}
	lv.syncNote()
}

func lvHasAttachment(e core.LogEntry, image bool) bool {
	for _, at := range e.Attachments {
		if at.IsImage == image {
			return true
		}
	}
	return false
}

// collectUsers builds the user filter's options from the log and the peers.
func (lv *logViewer) collectUsers(all []core.LogEntry) {
	byID := map[string]core.User{}
	add := func(u core.User) {
		if u.ID == "" {
			return
		}
		if _, ok := byID[u.ID]; !ok || u.Name != "" {
			byID[u.ID] = u
		}
	}
	for _, e := range all {
		if e.Incoming {
			add(e.From)
		}
		for _, u := range e.To {
			add(u)
		}
	}
	for _, u := range lv.a.peers {
		add(u)
	}
	names := map[string]int{}
	for _, u := range byID {
		names[lvUserName(u)]++
	}
	lv.userIDs = map[string]string{}
	lv.userByID = byID
	labels := []string{}
	for id, u := range byID {
		l := lvUserName(u)
		if names[l] > 1 {
			l += " (" + u.Host + ")"
		}
		if _, dup := lv.userIDs[l]; dup {
			l += " " + id
		}
		lv.userIDs[l] = id
		labels = append(labels, l)
	}
	sort.Strings(labels)
	lv.userLabels = append([]string{lvAllUsers}, labels...)
	if _, ok := lv.userIDs[lv.user]; !ok {
		lv.user = lvAllUsers
	}
}

func lvUserName(u core.User) string {
	switch {
	case u.Name != "":
		return u.Name
	case u.Logon != "":
		return u.Logon
	}
	return u.ID
}

func (lv *logViewer) current() (core.LogEntry, bool) {
	if lv.selected >= 0 && lv.selected < len(lv.entries) {
		return lv.entries[lv.selected], true
	}
	return core.LogEntry{}, false
}

// syncNote loads the note of the chosen entry into the note editor when
// the choice changed.
func (lv *logViewer) syncNote() {
	e, ok := lv.current()
	if !ok {
		lv.note, lv.noteSeq = "", 0
		return
	}
	if e.Seq != lv.noteSeq {
		lv.note, lv.noteSeq = e.Note, e.Seq
	}
}

// update saves the star, mark and note of entry i.
func (lv *logViewer) update(i int, fn func(e *core.LogEntry)) {
	if i < 0 || i >= len(lv.entries) {
		return
	}
	e := lv.entries[i]
	fn(&e)
	if err := lv.a.store.Update(e); err != nil {
		lv.status = "Error: " + err.Error()
		return
	}
	lv.entries[i] = e
	lv.status = ""
	if lv.starred && !e.Starred || lv.marked && !e.Marked {
		lv.refresh()
	}
}

func (lv *logViewer) deleteCurrent() {
	e, ok := lv.current()
	if !ok {
		return
	}
	if err := lv.a.store.Delete(e.Seq); err != nil {
		lv.status = "Error: " + err.Error()
		return
	}
	i := lv.selected
	lv.refresh()
	if i >= len(lv.entries) {
		i = len(lv.entries) - 1
	}
	lv.selected = i
	lv.syncNote()
	lv.status = "Deleted."
}

func (lv *logViewer) export() {
	entries := append([]core.LogEntry(nil), lv.entries...)
	lv.saveFile("tcpmsg-log.txt", func(path string) {
		if path == "" {
			return
		}
		f, err := os.Create(path)
		if err == nil {
			err = store.Export(entries, f)
			if cerr := f.Close(); err == nil {
				err = cerr
			}
		}
		if err != nil {
			lv.status = "Export failed: " + err.Error()
			return
		}
		lv.status = fmt.Sprintf("Exported %d messages to %s", len(entries), path)
	})
}

// quote prefixes each line of text with the quote mark of the settings.
func lvQuote(s core.Settings, text string) string {
	if !s.QuoteOnReply || text == "" {
		return ""
	}
	mark := s.QuoteMark
	if mark == "" {
		mark = "> "
	}
	lines := strings.Split(strings.TrimRight(strings.ReplaceAll(text, "\r\n", "\n"), "\n"), "\n")
	for i, l := range lines {
		lines[i] = mark + l
	}
	return strings.Join(lines, "\n") + "\n"
}

// counterpart is the other side of a logged message: the sender of one
// received, the recipients of one sent.
func lvCounterpart(e core.LogEntry) []core.User {
	if e.Incoming {
		return []core.User{e.From}
	}
	return e.To
}

func lvWho(e core.LogEntry) string {
	names := make([]string, 0, len(e.To))
	for _, u := range e.To {
		names = append(names, lvUserName(u))
	}
	if e.Incoming {
		return lvUserName(e.From)
	}
	return strings.Join(names, ", ")
}

func lvTime(t time.Time, now time.Time) string {
	if t.Year() == now.Year() && t.YearDay() == now.YearDay() {
		return t.Format("15:04")
	}
	if t.Year() == now.Year() {
		return t.Format("01/02 15:04")
	}
	return t.Format("2006/01/02 15:04")
}

func lvPreview(text string) string {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n"))
	lines := strings.SplitN(text, "\n", 3)
	if len(lines) > 2 {
		lines = append(lines[:2], "…")
	}
	return strings.Join(lines, "\n")
}

func (lv *logViewer) bitmap(e core.LogEntry, i int) *ui.Bitmap {
	key := fmt.Sprintf("%d/%d", e.Seq, i)
	if b, ok := lv.bitmaps[key]; ok {
		return b
	}
	data := e.Attachments[i].Data
	if len(data) == 0 {
		if l, ok := lv.a.store.(core.ImageLoader); ok {
			data = l.Image(e.Seq, i)
		}
	}
	b, err := ui.DecodeBitmap(data)
	if err != nil {
		b = nil
	}
	lv.bitmaps[key] = b
	return b
}

func (lv *logViewer) view(c *ui.Context) {
	k := bpApply(c, groundChrome, false)
	t := c.Theme()
	ui.Column(c).Fill().Children(func() {
		lv.menubar(c)
		lv.toolbar(c, t)
		ui.Split(c, &lv.split, func() { lv.listPane(c, t) }, func() { lv.detailPane(c, t) }).Grow(1)
		bpStatusBar(c, func() {
			if lv.status != "" {
				ui.Text(c, lv.status).Grow(1).MaxLines(1).TextColor(k.muted)
			}
			pos := 0
			if lv.selected >= 0 {
				pos = lv.selected + 1
			}
			ui.Textf(c, "pos=%d", pos)
			ui.Textf(c, "find=%d", len(lv.entries))
			ui.Textf(c, "total=%d", lv.total)
		})
	})
	switch ui.AlertDialog(c, &lv.confirmDelete, "Delete this message?", "It is removed from the log. You can't undo this.", "Cancel", "Delete") {
	case 1:
		lv.deleteCurrent()
	}
}

func (lv *logViewer) menubar(c *ui.Context) {
	e, ok := lv.current()
	filter := func(m *ui.Menu, on *bool, label string) {
		if m.Item(label).Checked(*on).Chosen() {
			*on = !*on
			lv.refresh()
		}
	}
	bpMenubar(c,
		bpMenu{"File(F)", func(m *ui.Menu) {
			if m.Item("Export…").Chosen() {
				lv.export()
			}
		}},
		bpMenu{"Edit(E)", func(m *ui.Menu) {
			if m.Item("Copy").Disabled(!ok).Chosen() {
				c.WriteClipboard(e.Text)
				lv.status = "Copied."
			}
			if m.Item("Reply").Disabled(!ok).Chosen() {
				lv.reply(lvCounterpart(e), lvQuote(lv.a.settings, e.Text))
			}
			m.Separator()
			if m.Item("Delete…").Disabled(!ok).Chosen() {
				lv.confirmDelete = true
			}
		}},
		bpMenu{"View(V)", func(m *ui.Menu) {
			filter(m, &lv.starred, "Starred Only")
			filter(m, &lv.marked, "Marked Only")
			filter(m, &lv.withImages, "With Images")
			filter(m, &lv.withFiles, "With Files")
		}},
		bpMenu{"Help(H)", func(m *ui.Menu) {
			if m.Item(appName + " Help").Chosen() {
				openHelp("log")
			}
		}},
	)
}

func (lv *logViewer) toolbar(c *ui.Context, t *ui.Theme) {
	bpToolbar(c, func() {
		changed := false
		changed = bpToggleWith(c, &lv.starred, "Starred", func() {
			bpStar(c, lv.starred)
			ui.Text(c, "Starred").SingleLine()
		}).Tooltip("Favorites only").Changed() || changed
		changed = bpToggle(c, &lv.marked, "Marked").Tooltip("Marked messages").Changed() || changed
		changed = bpToggle(c, &lv.withImages, "Images").Tooltip("Messages with images").Changed() || changed
		changed = bpToggle(c, &lv.withFiles, "Files").Tooltip("Messages with attachments").Changed() || changed
		bpSep(c)
		changed = ui.Select(c, &lv.user, lv.userLabels).Label("User filter").Width(150).Changed() || changed
		ui.Spacer(c)
		changed = bpField(c, ui.SearchField(c, &lv.query).Label("Search").Width(180)).Changed() || changed
		bpSep(c)
		if bpButton(c, "Export…").Tooltip("Export the messages shown to a text file").Clicked() {
			lv.export()
		}
		if changed {
			lv.refresh()
		}
	})
}

func (lv *logViewer) listPane(c *ui.Context, t *ui.Theme) {
	now := time.Now()
	p := bpOf(c)
	ui.List(c, &lv.list, len(lv.entries), func(i int) {
		e := lv.entries[i]
		ui.Column(c).Children(func() {
			// The head: time and direction, who, and the tools, on the
			// grey bar.
			ui.Row(c).Gap(sp3).Padding(sp1, sp3).AlignItems(ui.Center).Background(p.bar).TextColor(p.ink).Children(func() {
				arrow := "⇨"
				if e.Incoming {
					arrow = "⇦"
				}
				ui.Text(c, lvTime(e.Time, now)+" "+arrow).SingleLine()
				bpChip(c, lvWho(e)).Shrink(1)
				ui.Spacer(c)
				if len(e.Attachments) > 0 {
					ui.Textf(c, "files=%d", len(e.Attachments)).FontSize(12).Font(bpMonoFont())
				}
				idx := i
				label := "Star"
				if e.Starred {
					label = "Unstar"
				}
				ui.ButtonBase(c).Label(fmt.Sprintf("Star %d", e.Seq)).Tooltip(label).TextColor(p.muted).OnClick(func() {
					lv.update(idx, func(e *core.LogEntry) { e.Starred = !e.Starred })
				}).Children(func() { bpStar(c, e.Starred) })
				ui.ButtonBase(c).Label(fmt.Sprintf("More %d", e.Seq)).TextColor(p.ink).PopoverMenuButton(func(m *ui.Menu) {
					if m.Item("Mark").Checked(e.Marked).Chosen() {
						lv.update(idx, func(e *core.LogEntry) { e.Marked = !e.Marked })
					}
					if m.Item("Reply").Chosen() {
						lv.reply(lvCounterpart(e), lvQuote(lv.a.settings, e.Text))
					}
					if m.Item("Copy").Chosen() {
						c.WriteClipboard(e.Text)
						lv.status = "Copied."
					}
				}).Children(func() { ui.Icon(c, iconMore).Size(14, 14) })
			})
			// The body.
			ui.Column(c).Padding(sp3, sp3, sp4, sp3).Gap(sp1).Children(func() {
				ui.RichText(c, bpHighlight(c, lvPreview(e.Text), lv.query, e.Marked)...).MaxLines(3)
				if e.Note != "" {
					bpNote(c, "Ref.: "+e.Note).MaxLines(1).Margin(sp3, 0, 0, 0)
				}
			})
		})
	}).Fill().Background(p.chrome).Children(func() {
		if len(lv.entries) == 0 {
			ui.Text(c, "No messages").TextColor(t.TextMuted).Padding(sp4)
		}
	})
	lv.syncNote()
}

func (lv *logViewer) detailPane(c *ui.Context, t *ui.Theme) {
	e, ok := lv.current()
	p := bpOf(c)
	ui.Column(c).Fill().Padding(sp3).Gap(sp3).Background(p.chrome).Children(func() {
		if !ok {
			ui.Text(c, "Choose a message to read it.").TextColor(t.TextMuted)
			return
		}
		i := lv.selected
		ui.Row(c).Gap(sp3).AlignItems(ui.Center).Children(func() {
			dir := "To"
			if e.Incoming {
				dir = "From"
			}
			bpLabel(c, dir)
			bpChip(c, lvWho(e)).Shrink(1)
			ui.Spacer(c)
			ui.Text(c, e.Time.Format("2006/01/02 (Mon) 15:04:05")).Font(bpMonoFont()).TextColor(t.TextMuted).SingleLine()
		})
		ui.Row(c).Gap(sp3).AlignItems(ui.Center).Children(func() {
			starred, marked := e.Starred, e.Marked
			if ui.Checkbox(c, &starred, "Favorite").Changed() {
				lv.update(i, func(e *core.LogEntry) { e.Starred = starred })
			}
			if ui.Checkbox(c, &marked, "Mark").Changed() {
				lv.update(i, func(e *core.LogEntry) { e.Marked = marked })
			}
			for _, f := range []struct {
				on   bool
				name string
			}{{e.Sealed, "sealed"}, {e.Locked, "locked"}, {e.Signed, "signed"}} {
				if f.on {
					ui.Text(c.Key(f.name), f.name).Font(bpMonoFont()).TextColor(t.TextMuted)
				}
			}
			ui.Spacer(c)
			if bpButton(c, "Reply").Clicked() {
				lv.reply(lvCounterpart(e), lvQuote(lv.a.settings, e.Text))
			}
			if bpButton(c, "Copy").Clicked() {
				c.WriteClipboard(e.Text)
				lv.status = "Copied."
			}
			if bpButton(c, "Delete").Clicked() {
				lv.confirmDelete = true
			}
		})
		bpField(c, ui.Scroll(c).Grow(1)).Children(func() {
			ui.Column(c).Padding(sp3).Gap(sp3).Children(func() {
				ui.RichText(c, bpHighlight(c, e.Text, lv.query, e.Marked)...).Selectable().Wrap()
				for ai, at := range e.Attachments {
					if at.IsImage {
						if b := lv.bitmap(e, ai); b != nil {
							ui.Image(c, b).MaxWidth(480).MaxHeight(360).Fit(ui.Contain).Label(at.Name)
							continue
						}
					}
					ui.Text(c, fmt.Sprintf("[File] %s (%d bytes)", at.Name, at.Size)).TextColor(t.TextMuted)
				}
				if e.Note != "" {
					bpNote(c, "Ref.: "+e.Note)
				}
			})
		})
		bpLabel(c, "Note")
		bpField(c, ui.TextArea(c, &lv.note).Lines(2, 4).Placeholder("Add a note").Label("Note"))
		ui.Row(c).Justify(ui.End).Children(func() {
			if bpButton(c, "Save Note").Disabled(lv.note == e.Note).Clicked() {
				note := lv.note
				lv.update(i, func(e *core.LogEntry) { e.Note = note })
				lv.status = "Note saved."
			}
		})
	})
}
