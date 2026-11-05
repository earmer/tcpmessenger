package core

import (
	"strings"
	"testing"
)

func TestMatchLogTextMatchesSlowPath(t *testing.T) {
	slow := func(e LogEntry, q string) bool {
		return strings.Contains(strings.ToLower(e.Text+"\n"+e.Note+"\n"+e.From.Name), strings.ToLower(q))
	}
	entries := []LogEntry{
		{Text: "Hello World", Note: "todo", From: User{Name: "Alice"}},
		{Text: "héllo wörld", Note: "", From: User{Name: "Bob"}},
		{Text: "abc", Note: "def", From: User{Name: "ghi"}},
		{Text: "Kelvin K", From: User{Name: "x"}},
	}
	for _, e := range entries {
		for _, q := range []string{"hello", "WORLD", "o w", "ALICE", "todo", "c\nd", "k", "ö", "zzz", "f\ng", "def\nghi", "l"} {
			if got := MatchLog(e, LogQuery{Text: q}); got != slow(e, q) {
				t.Errorf("%q in %+v: got %v", q, e, got)
			}
		}
	}
}
