package main

import (
	"testing"

	"tcpmessenger/internal/core"

	"github.com/egoist/mygo/ui"
)

func TestSortDisplayEditsLive(t *testing.T) {
	a, st := auxTestApp(t)
	sd := &sortDisplayView{a: a}
	closed := false
	sd.close = func() { closed = true }
	tt := ui.NewTester(sd.view, 380, 470)
	tt.Click("IP addr")
	tt.Click("Group")
	tt.Click("Host name")
	tt.Click("Reverse second order")
	tt.Click("Ignore case (ASCII)")
	l := a.settings.List
	if !l.ShowIP || l.ShowGroup || l.Second != core.SortByHost || !l.SecondReverse || !l.IgnoreCase {
		t.Errorf("list options %+v", l)
	}
	saved, _ := st.LoadSettings()
	if saved.List != l {
		t.Errorf("saved %+v, want %+v", saved.List, l)
	}
	tt.Click("Group name")
	if a.settings.List.SortByGroup {
		t.Error("group sort still on")
	}
	tt.Click("OK")
	if !closed {
		t.Error("OK did not close")
	}
}
