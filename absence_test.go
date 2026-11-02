package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestAbsenceSetAndAbsent(t *testing.T) {
	a, st := auxTestApp(t)
	av := newAbsenceView(a)
	closed := false
	av.close = func() { closed = true }
	tt := ui.NewTester(av.view, 540, 520)
	if !tt.HasText("Not Absent") || !tt.HasText("1: Meeting") {
		t.Fatalf("texts %q", tt.Texts())
	}
	tt.Click("2: Lunch")
	if av.sel != 1 {
		t.Fatalf("selected %d", av.sel)
	}
	tt.Click("Set & Absent")
	saved, _ := st.LoadSettings()
	if !a.settings.Absent || a.settings.AbsenceIndex != 1 || !saved.Absent || !a.net.Self().Absent {
		t.Errorf("absent %v index %d saved %v", a.settings.Absent, a.settings.AbsenceIndex, saved.Absent)
	}
	if !tt.HasText("Absent: Lunch") {
		t.Errorf("status texts %q", tt.Texts())
	}
	tt.Click("Not Absent")
	if a.settings.Absent {
		t.Error("still absent")
	}
	if closed {
		t.Error("closed by Set & Absent")
	}
}

func TestAbsenceEditModes(t *testing.T) {
	a, _ := auxTestApp(t)
	av := newAbsenceView(a)
	closed := false
	av.close = func() { closed = true }
	tt := ui.NewTester(av.view, 540, 520)
	tt.Click("Add")
	if len(av.modes) != 5 || av.sel != 4 {
		t.Fatalf("modes %d sel %d", len(av.modes), av.sel)
	}
	tt.Click("Title")
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("Training")
	tt.Click("Absence message")
	tt.Type("At a training course.")
	if !tt.HasText("5: Training") {
		t.Errorf("rename not shown: %q", tt.Texts())
	}
	if len(a.settings.AbsenceModes) != 4 {
		t.Fatal("edited the app's modes before OK")
	}
	tt.Click("1: Meeting")
	tt.Click("Delete")
	tt.Click("Switch to [Away] when the desktop is idle")
	tt.Click("OK")
	m := a.settings.AbsenceModes
	if !closed || len(m) != 4 || m[0].Title != "Lunch" || m[3].Message != "At a training course." || !a.settings.AutoAbsence {
		t.Errorf("closed %v modes %+v auto %v", closed, m, a.settings.AutoAbsence)
	}

	av2 := newAbsenceView(a)
	av2.close = func() {}
	tt2 := ui.NewTester(av2.view, 540, 520)
	tt2.Click("Add")
	tt2.Click("Cancel")
	if len(a.settings.AbsenceModes) != 4 {
		t.Error("cancel kept the new mode")
	}
}

func TestAutoAbsenceStep(t *testing.T) {
	a, _ := auxTestApp(t)
	w := &autoAbsence{a: a}
	a.settings.AutoAbsenceMinutes = 10
	if w.step(time.Hour) {
		t.Error("switched with auto absence off")
	}
	a.settings.AutoAbsence = true
	if w.step(9*time.Minute) || a.settings.Absent {
		t.Error("switched before the idle time")
	}
	if !w.step(10*time.Minute) || !a.settings.Absent {
		t.Error("not absent after the idle time")
	}
	if !w.step(time.Second) || a.settings.Absent {
		t.Error("still absent after the user came back")
	}
	// Absence the user set stays.
	a.settings.Absent = true
	if w.step(time.Second) || !a.settings.Absent {
		t.Error("cleared the user's absence")
	}
}
