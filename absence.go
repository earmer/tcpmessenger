package main

import (
	"fmt"
	"slices"
	"sync"
	"time"

	"tcpmessenger/internal/core"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// absenceWin is the open Absence Mode Setting window, nil while closed.
var absenceWin *mygo.Window

// openAbsence opens its window, or brings it forward when it is open.
func openAbsence(a *tcpApp) {
	if absenceWin != nil {
		absenceWin.Focus()
		return
	}
	av := newAbsenceView(a)
	w := mygo.NewWindow(mygo.WindowOptions{
		Title:           "Absence Mode Setting",
		Width:           540,
		Height:          520,
		MinWidth:        460,
		MinHeight:       440,
		DisableMaximize: true,
		StateKey:        "absence",
		Content:         ui.View(av.view),
	})
	av.close = w.Close
	absenceWin = w
	w.OnClosed(func() {
		if absenceWin == w {
			absenceWin = nil
		}
	})
}

// absenceView edits a copy of the absence modes; OK and Set & Absent give
// it back to the app.
type absenceView struct {
	a       *tcpApp
	modes   []core.AbsenceMode
	sel     int
	list    ui.ListState
	auto    bool
	minutes float64
	status  string
	close   func()
}

func newAbsenceView(a *tcpApp) *absenceView {
	av := &absenceView{a: a, close: func() {}}
	av.modes = slices.Clone(a.settings.AbsenceModes)
	if len(av.modes) == 0 {
		av.modes = slices.Clone(core.DefaultSettings().AbsenceModes)
	}
	av.sel = min(max(a.settings.AbsenceIndex, 0), len(av.modes)-1)
	av.auto = a.settings.AutoAbsence
	av.minutes = float64(a.settings.AutoAbsenceMinutes)
	if av.minutes < 1 {
		av.minutes = 10
	}
	av.list.Selected = &av.sel
	av.list.Label = func(i int) string { return av.modes[i].Title }
	return av
}

// commit gives the edited modes and auto absence settings to the app.
func (av *absenceView) commit() {
	s := &av.a.settings
	s.AbsenceModes = slices.Clone(av.modes)
	s.AutoAbsence = av.auto
	s.AutoAbsenceMinutes = max(int(av.minutes), 1)
	if s.AbsenceIndex >= len(s.AbsenceModes) {
		s.AbsenceIndex = len(s.AbsenceModes) - 1
	}
}

func (av *absenceView) save() bool {
	if err := av.a.saveSettings(); err != nil {
		av.status = "Could not save: " + err.Error()
		return false
	}
	syncMenus(av.a)
	if av.a.main != nil {
		av.a.main.Update(func() {})
	}
	return true
}

func (av *absenceView) setAbsent(on bool) {
	av.commit()
	av.a.settings.Absent = on
	if on {
		av.a.settings.AbsenceIndex = av.sel
	}
	av.save()
}

func (av *absenceView) statusLine() string {
	if av.status != "" {
		return av.status
	}
	s := av.a.settings
	if s.Absent && s.AbsenceIndex >= 0 && s.AbsenceIndex < len(s.AbsenceModes) {
		return "Absent: " + s.AbsenceModes[s.AbsenceIndex].Title
	}
	if s.Absent {
		return "Absent"
	}
	return "Not Absent"
}

func (av *absenceView) view(c *ui.Context) {
	applyAuxTheme(c)
	ui.Column(c).Fill().Padding(sp3).Gap(sp4).Children(func() {
		bpGroup(c, "Absence Mode", func() {
			bpLead(c, `Absent users show "!" in other members' lists. Their absence message is sent back to whoever messages them.`, "absence")
			auxLabeled(c, "Current state", 110, func() { bpReadonly(c, av.statusLine(), 0).Grow(1) })
			ui.Row(c).Gap(sp4).AlignItems(ui.Stretch).Children(func() {
				ui.Column(c).Width(170).Shrink(0).Gap(sp2).Children(func() {
					bpCatList(c, &av.list, len(av.modes), func(i int) string {
						return fmt.Sprintf("%d: %s", i+1, av.modes[i].Title)
					}).Height(150).Label("Absence modes")
					ui.Row(c).Gap(sp2).Children(func() {
						if bpMiniButton(c, "Add").Grow(1).Clicked() {
							av.modes = append(av.modes, core.AbsenceMode{Title: fmt.Sprintf("Mode %d", len(av.modes)+1)})
							av.sel = len(av.modes) - 1
						}
						if bpMiniButton(c, "Delete").Grow(1).Disabled(len(av.modes) <= 1).Clicked() {
							av.deleteMode()
						}
					})
				})
				ui.Column(c).Grow(1).Gap(sp2).Children(func() {
					if av.sel < 0 || av.sel >= len(av.modes) {
						return
					}
					m := &av.modes[av.sel]
					auxLabeled(c, "Title", 0, func() {
						auxInput(c, &m.Title, "Title").Placeholder("Mode name").Grow(1)
					})
					auxLabeled(c, "Message", 0, func() {
						bpDlgField(c, ui.TextArea(c, &m.Message).Lines(4, 6).Label("Absence message").Placeholder("Sent back to those who message you")).Grow(1)
					})
				})
			})
		})
		bpGroup(c, "Auto Absence (Experimental)", func() {
			ui.Checkbox(c, &av.auto, "Switch to [Away] when the desktop is idle")
			bpIndent(c, func() {
				auxLabeled(c, "Idle time (min)", 120, func() {
					ui.NumberInput(c, &av.minutes, 1, 600, 1).Label("Idle time").Width(110).Disabled(!av.auto)
				})
			})
		})
		ui.Spacer(c)
		ui.Row(c).Gap(sp3).Children(func() {
			if av.a.settings.Absent {
				if bpDlgButton(c, "Not Absent", false).Clicked() {
					av.status = ""
					av.setAbsent(false)
				}
			}
			ui.Spacer(c)
			if bpDlgButton(c, "Set & Absent", false).Clicked() {
				av.status = ""
				av.setAbsent(true)
			}
			if bpDlgButton(c, "OK", true).Clicked() {
				av.commit()
				if av.save() {
					av.close()
				}
			}
			if bpDlgButton(c, "Cancel", false).Clicked() {
				av.close()
			}
		})
	})
}

func (av *absenceView) deleteMode() {
	if len(av.modes) <= 1 || av.sel < 0 || av.sel >= len(av.modes) {
		return
	}
	av.modes = slices.Delete(av.modes, av.sel, av.sel+1)
	// Keep the absent mode pointing at the same mode.
	s := &av.a.settings
	if s.AbsenceIndex > av.sel {
		s.AbsenceIndex--
	}
	av.sel = min(av.sel, len(av.modes)-1)
}

// autoAbsence switches to absent when the desktop has been idle for the
// minutes the settings say, and back when the user returns, unless the
// user set the absence by hand.
type autoAbsence struct {
	a       *tcpApp
	setByUs bool
}

// step applies one reading of the idle time. It reports whether the
// settings changed.
func (w *autoAbsence) step(idle time.Duration) bool {
	s := &w.a.settings
	limit := time.Duration(max(s.AutoAbsenceMinutes, 1)) * time.Minute
	switch {
	case s.AutoAbsence && !s.Absent && idle >= limit:
		s.Absent, w.setByUs = true, true
		if s.AbsenceIndex < 0 || s.AbsenceIndex >= len(s.AbsenceModes) {
			s.AbsenceIndex = 0
		}
		return true
	case s.Absent && w.setByUs && idle < limit:
		s.Absent, w.setByUs = false, false
		return true
	case !s.Absent:
		w.setByUs = false
	}
	return false
}

var autoAbsenceOnce sync.Once

// startAutoAbsence watches the desktop's idle time (mygo.Power.IdleTime)
// and switches the user to absent and back as the Auto Absence setting
// says. Call it once on the main thread after a.main is set: the watcher
// changes the settings on the main thread through a.main.Update.
func startAutoAbsence(a *tcpApp) {
	win := a.main
	if win == nil {
		return
	}
	autoAbsenceOnce.Do(func() {
		w := &autoAbsence{a: a}
		go func() {
			tick := time.NewTicker(15 * time.Second)
			defer tick.Stop()
			for range tick.C {
				idle := mygo.Power.IdleTime()
				win.Update(func() {
					if w.step(idle) {
						a.saveSettings()
						syncMenus(a)
					}
				})
			}
		}()
	})
}
