package core

import (
	"strings"
	"testing"
)

func TestMatchLogTextMatchesSlowPath(t *testing.T) {
	slow := func(e LogEntry, q string) bool {
		hay := strings.ToLower(e.Text + "\n" + e.Note + "\n" + e.From.Name)
		for _, w := range strings.Fields(strings.ToLower(q)) {
			if !strings.Contains(hay, w) {
				return false
			}
		}
		return true
	}
	entries := []LogEntry{
		{Text: "Hello World", Note: "todo", From: User{Name: "Alice"}},
		{Text: "héllo wörld", Note: "", From: User{Name: "Bob"}},
		{Text: "abc", Note: "def", From: User{Name: "ghi"}},
		{Text: "Kelvin K", From: User{Name: "x"}},
	}
	for _, e := range entries {
		for _, q := range []string{"hello", "WORLD", "o w", "world hello", "hello  alice", "alice todo", "hello nope", " ", "ALICE", "todo", "c\nd", "k", "ö", "zzz", "f\ng", "def\nghi", "l"} {
			if got := MatchLog(e, LogQuery{Text: q}); got != slow(e, q) {
				t.Errorf("%q in %+v: got %v", q, e, got)
			}
		}
	}
}

func TestMatchLogWordsAnyOrderAcrossFields(t *testing.T) {
	e := LogEntry{Text: "the build on port 2425", Note: "Release", From: User{Name: "Kaga"}}
	for q, want := range map[string]bool{
		"port build": true, "kaga release": true, "BUILD  2425": true,
		"build nope": false, "port 2426": false, "": true,
	} {
		if got := MatchLog(e, LogQuery{Text: q}); got != want {
			t.Errorf("%q: got %v, want %v", q, got, want)
		}
	}
}
