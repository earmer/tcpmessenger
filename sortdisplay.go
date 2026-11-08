package main

import (
	"tcpmessenger/internal/core"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// sortDisplayWin is the open Sort/Display window, nil while closed.
var sortDisplayWin *mygo.Window

// openSortDisplay opens its window, or brings it forward when it is open.
func openSortDisplay(a *tcpApp) {
	if sortDisplayWin != nil {
		sortDisplayWin.Focus()
		return
	}
	sd := &sortDisplayView{a: a, close: func() {}}
	w := mygo.NewWindow(mygo.WindowOptions{
		Title:           "SendWindow Sort/Display Option",
		Width:           380,
		Height:          470,
		MinWidth:        340,
		MinHeight:       420,
		DisableMaximize: true,
		StateKey:        "sortdisplay",
		Content:         ui.View(sd.view),
	})
	sd.close = w.Close
	sortDisplayWin = w
	w.OnClosed(func() {
		if sortDisplayWin == w {
			sortDisplayWin = nil
		}
	})
}

// sortDisplayView edits a.settings.List in place: every change is saved
// and shown in the send window at once.
type sortDisplayView struct {
	a      *tcpApp
	status string
	close  func()
}

func (sd *sortDisplayView) changed() {
	if err := sd.a.saveSettings(); err != nil {
		sd.status = "Could not save: " + err.Error()
	} else {
		sd.status = ""
	}
	if sd.a.main != nil {
		sd.a.main.Update(func() {})
	}
}

func (sd *sortDisplayView) view(c *ui.Context) {
	t := applyAuxTheme(c)
	l := &sd.a.settings.List
	ch := false
	check := func(v *bool, label string) {
		if ui.Checkbox(c, v, label).Changed() {
			ch = true
		}
	}
	ui.Column(c).Fill().Padding(sp3).Gap(sp4).Children(func() {
		auxGroup(c, "User List Display Options", func() {
			ui.Grid(c).Columns(3).Gap(sp4).Children(func() {
				check(&l.ShowGroup, "Group")
				check(&l.ShowHost, "Host")
				check(&l.ShowPriority, "DispPriority")
				check(&l.ShowLogon, "Logon")
				check(&l.ShowIP, "IP addr")
				check(&l.GridLines, "GridLines")
			})
		})
		auxGroup(c, "User Sort Settings", func() {
			auxGroup(c, "First Sort Key", func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					check(&l.SortByGroup, "Group name")
					ui.Spacer(c)
					if ui.Checkbox(c, &l.GroupReverse, "Reverse").Label("Reverse group order").Disabled(!l.SortByGroup).Changed() {
						ch = true
					}
				})
			})
			auxGroup(c, "Second Sort Key", func() {
				ui.Row(c).Gap(8).AlignItems(ui.End).Children(func() {
					second := l.Second
					bpRadios(c, func() {
						ui.RadioGroup(c, func() {
							ui.Radio(c, &second, core.SortByName, "User name")
							ui.Radio(c, &second, core.SortByIP, "IP address")
							ui.Radio(c, &second, core.SortByHost, "Host name")
						}).Gap(sp4).Label("Second sort key")
					})
					if second != l.Second {
						l.Second = second
						ch = true
					}
					ui.Spacer(c)
					if ui.Checkbox(c, &l.SecondReverse, "Reverse").Label("Reverse second order").Changed() {
						ch = true
					}
				})
			})
			check(&l.IgnoreCase, "Ignore case (ASCII)")
		})
		ui.Spacer(c)
		ui.Row(c).Justify(ui.End).Children(func() {
			if bpDlgButton(c, "OK", true).Clicked() {
				sd.close()
			}
		})
		if sd.status != "" {
			ui.Text(c, sd.status).TextColor(t.Danger)
		}
	})
	if ch {
		sd.changed()
	}
}
