package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"my-app/internal/core"
)

func open(t *testing.T, dir string) *Store {
	t.Helper()
	s, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestDefaultDir(t *testing.T) {
	if filepath.Base(DefaultDir()) != "TCP Messenger" {
		t.Fatal(DefaultDir())
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir)
	got, _ := s.LoadSettings()
	if got.Port != core.DefaultPort || len(got.AbsenceModes) != 4 {
		t.Fatalf("first run should give defaults: %+v", got)
	}
	got.Nickname, got.Group, got.SealByDefault = "Alice", "Dev", true
	got.QuoteOnReply = false
	got.BroadcastAddrs = []string{"10.0.0.255"}
	got.List.Second = core.SortByHost
	got.Password = "secret"
	if err := s.SaveSettings(got); err != nil {
		t.Fatal(err)
	}
	s.Close()
	if m, _ := filepath.Glob(filepath.Join(dir, ".settings*")); len(m) > 0 {
		t.Fatalf("temp files left: %v", m)
	}

	s2 := open(t, dir)
	back, _ := s2.LoadSettings()
	if back.Nickname != "Alice" || back.Group != "Dev" || !back.SealByDefault || back.QuoteOnReply ||
		len(back.BroadcastAddrs) != 1 || back.List.Second != core.SortByHost {
		t.Fatalf("round trip: %+v", back)
	}
	if back.Password == "secret" || !CheckPassword(back.Password, "secret") || CheckPassword(back.Password, "wrong") {
		t.Fatalf("password not hashed properly: %q", back.Password)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "settings.json"))
	if bytes.Contains(raw, []byte("secret")) {
		t.Fatal("plain password on disk")
	}
	// Saving the hash again keeps it.
	h := back.Password
	s2.SaveSettings(back)
	if b, _ := s2.LoadSettings(); b.Password != h {
		t.Fatal("hash rehashed")
	}
}

func TestSettingsDefaultsFill(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "settings.json"),
		[]byte(`{"Nickname":"Bob","Unknown":42,"List":{"ShowIP":true}}`), 0o600)
	s := open(t, dir)
	got, _ := s.LoadSettings()
	d := core.DefaultSettings()
	if got.Nickname != "Bob" || got.Port != d.Port || got.QuoteMark != d.QuoteMark ||
		len(got.AbsenceModes) != len(d.AbsenceModes) || !got.List.ShowIP || !got.List.ShowGroup || !got.LogEnabled {
		t.Fatalf("defaults not filled: %+v", got)
	}

	// A broken file falls back to defaults.
	dir2 := t.TempDir()
	os.WriteFile(filepath.Join(dir2, "settings.json"), []byte(`{not json`), 0o600)
	s2 := open(t, dir2)
	if got, _ := s2.LoadSettings(); got.Port != d.Port {
		t.Fatal("broken settings")
	}
}

func TestPassword(t *testing.T) {
	h := HashPassword("pw")
	if !CheckPassword(h, "pw") || CheckPassword(h, "pW") || HashPassword("pw") == h {
		t.Fatal("hash")
	}
	if HashPassword("") != "" || !CheckPassword("", "") || CheckPassword("", "x") || CheckPassword("garbage", "garbage") {
		t.Fatal("empty")
	}
}

var (
	alice = core.User{ID: "alice@pc1", Name: "Alice", Group: "Dev", Host: "pc1", Addr: "10.0.0.2:2425"}
	bob   = core.User{ID: "bob@pc2", Name: "Bob", Group: "Ops", Host: "pc2", Addr: "10.0.0.3:2425"}
)

func msg(i int, text string) core.Message {
	return core.Message{ID: fmt.Sprint("m", i), From: alice, To: []core.User{bob}, Text: text, Incoming: true,
		Time: time.Date(2026, 11, 2, 10, 0, i, 0, time.UTC)}
}

func TestLogPersistence(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir)
	img := []byte("\x89PNG fake image")
	m1 := msg(1, "hello world")
	m1.Attachments = []core.Attachment{{Name: "shot.png", Data: img, IsImage: true}, {Name: "doc.txt", Size: 3, Data: []byte("abc")}}
	seq1, err := s.Append(m1)
	if err != nil || seq1 != 1 {
		t.Fatal(seq1, err)
	}
	seq2, _ := s.Append(msg(2, "second"))
	seq3, _ := s.Append(msg(3, "third"))
	if err := s.Update(core.LogEntry{Seq: seq2, Starred: true, Note: "remember"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(seq3); err != nil {
		t.Fatal(err)
	}
	if s.Delete(99) == nil || s.Update(core.LogEntry{Seq: 99}) == nil {
		t.Fatal("missing seq should fail")
	}
	if _, err := os.Stat(filepath.Join(dir, "images", "1-shot.png")); err != nil {
		t.Fatal("image not saved:", err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "tcpmsg-log.jsonl"))
	if bytes.Contains(raw, []byte("PNG")) || bytes.Contains(raw, []byte("YWJj")) {
		t.Fatal("attachment content inlined in log")
	}
	s.Close()

	s = open(t, dir)
	all, _ := s.Query(core.LogQuery{})
	if len(all) != 2 || all[0].Seq != seq2 || all[1].Seq != seq1 {
		t.Fatalf("want newest first [2 1], got %v", seqs(all))
	}
	if !all[0].Starred || all[0].Note != "remember" || all[1].Starred {
		t.Fatalf("update lost: %+v", all[0])
	}
	a := all[1].Attachments
	if len(a) != 2 || !bytes.Equal(a[0].Data, img) || a[0].Size != int64(len(img)) || a[1].Data != nil || a[1].Size != 3 {
		t.Fatalf("attachments: %+v", a)
	}
	if p := s.ImagePath(seq1, 0); filepath.Base(p) != "1-shot.png" {
		t.Fatal(p)
	}
	if got, _ := s.Query(core.LogQuery{Starred: true}); len(got) != 1 || got[0].Seq != seq2 {
		t.Fatal("starred query")
	}
	if got, _ := s.Query(core.LogQuery{Text: "HELLO"}); len(got) != 1 || got[0].Seq != seq1 {
		t.Fatal("text query")
	}
	// Seq numbers are never reused, even after deleting the last entry.
	if seq, _ := s.Append(msg(4, "four")); seq != 4 {
		t.Fatal("seq reused:", seq)
	}
	// Deleting removes the image.
	s.Delete(seq1)
	if _, err := os.Stat(filepath.Join(dir, "images", "1-shot.png")); !os.IsNotExist(err) {
		t.Fatal("image not removed")
	}
	// Mutating a result does not change the store.
	got, _ := s.Query(core.LogQuery{})
	got[0].To[0].Name = "X"
	if again, _ := s.Query(core.LogQuery{}); again[0].To[0].Name != "Bob" {
		t.Fatal("aliasing")
	}
}

func seqs(es []core.LogEntry) []int64 {
	var out []int64
	for _, e := range es {
		out = append(out, e.Seq)
	}
	return out
}

func countLines(t *testing.T, p string) map[string]int {
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	n := map[string]int{}
	for _, l := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var r record
		json.Unmarshal([]byte(l), &r)
		n[r.T]++
	}
	return n
}

func TestCompaction(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir)
	for i := 1; i <= 3; i++ {
		s.Append(msg(i, "x"))
	}
	for i := 0; i < 5; i++ {
		s.Update(core.LogEntry{Seq: 1, Marked: i%2 == 0, Note: fmt.Sprint("n", i)})
	}
	s.Delete(3)
	s.Close()
	p := filepath.Join(dir, "tcpmsg-log.jsonl")
	if n := countLines(t, p); n["u"] != 5 || n["d"] != 1 {
		t.Fatalf("before: %v", n)
	}

	s = open(t, dir) // 6 changes > 2 live entries: compacts
	if n := countLines(t, p); n["u"] != 0 || n["d"] != 0 || n["e"] != 2 || n["h"] != 1 {
		t.Fatalf("after compaction: %v", n)
	}
	all, _ := s.Query(core.LogQuery{})
	if len(all) != 2 || !all[1].Marked || all[1].Note != "n4" {
		t.Fatalf("state lost in compaction: %+v", all)
	}
	if seq, _ := s.Append(msg(9, "y")); seq != 4 {
		t.Fatal("next seq lost in compaction:", seq)
	}
	s.Close()
	s = open(t, dir)
	if all, _ := s.Query(core.LogQuery{}); len(all) != 3 || all[0].Seq != 4 {
		t.Fatalf("after reopen: %v", seqs(all))
	}
}

func TestTruncatedLine(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir)
	s.Append(msg(1, "a"))
	s.Close()
	p := filepath.Join(dir, "tcpmsg-log.jsonl")
	f, _ := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString(`{"t":"e","seq":2,"m":{"te`)
	f.Close()
	s = open(t, dir)
	if seq, _ := s.Append(msg(2, "b")); seq != 2 {
		t.Fatal(seq)
	}
	s.Close()
	s = open(t, dir)
	if all, _ := s.Query(core.LogQuery{}); len(all) != 2 {
		t.Fatalf("got %v", seqs(all))
	}
}

func TestLogPath(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir)
	s.Append(msg(1, "a"))
	set, _ := s.LoadSettings()
	set.LogFile = "other.jsonl"
	if err := s.SaveSettings(set); err != nil {
		t.Fatal(err)
	}
	if all, _ := s.Query(core.LogQuery{}); len(all) != 0 {
		t.Fatal("new log should be empty")
	}
	s.Append(msg(2, "b"))
	s.Close()
	s = open(t, dir)
	if s.LogPath() != filepath.Join(dir, "other.jsonl") {
		t.Fatal(s.LogPath())
	}
	if all, _ := s.Query(core.LogQuery{}); len(all) != 1 || all[0].Text != "b" {
		t.Fatal("log file setting ignored")
	}
	if err := s.SetLogPath(""); err != nil {
		t.Fatal(err)
	}
	if all, _ := s.Query(core.LogQuery{}); len(all) != 1 || all[0].Text != "a" {
		t.Fatal("default log")
	}
}

func TestConcurrent(t *testing.T) {
	s := open(t, t.TempDir())
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				seq, err := s.Append(msg(i, "c"))
				if err != nil {
					t.Error(err)
					return
				}
				s.Update(core.LogEntry{Seq: seq, Starred: true})
				s.Query(core.LogQuery{Starred: true})
				if i%5 == 0 {
					s.Delete(seq)
				}
				s.LoadSettings()
			}
		}(g)
	}
	wg.Wait()
	all, _ := s.Query(core.LogQuery{})
	if len(all) != 8*40 {
		t.Fatal(len(all))
	}
}

func TestQueryLarge(t *testing.T) {
	s := open(t, t.TempDir())
	for i := 0; i < 20000; i++ {
		s.Append(msg(i%60, fmt.Sprint("message ", i)))
	}
	start := time.Now()
	got, _ := s.Query(core.LogQuery{Text: "message 1999"})
	if len(got) != 12 { // 1999, 11999, 19990..19999: both words occur
		t.Fatal(len(got))
	}
	t.Logf("query over 20000 entries: %v", time.Since(start))
}

func TestExport(t *testing.T) {
	in := msg(5, "hi there")
	in.Sealed = true
	out := core.Message{To: []core.User{alice, bob}, Text: "reply\n", Time: time.Date(2026, 11, 2, 9, 5, 0, 0, time.UTC),
		Attachments: []core.Attachment{{Name: "a.png"}}}
	var buf bytes.Buffer
	if err := Export([]core.LogEntry{{Message: in, Seq: 1}, {Message: out, Seq: 2, Note: "n"}}, &buf); err != nil {
		t.Fatal(err)
	}
	want := `=====================================
 From: Alice (Dev/pc1/10.0.0.2)
  at Mon Nov 02 10:00:05 2026 (Sealed)
-------------------------------------
hi there

=====================================
 To: Alice (Dev/pc1/10.0.0.2)
 To: Bob (Ops/pc2/10.0.0.3)
  at Mon Nov 02 09:05:00 2026
  (files) a.png
  (note) n
-------------------------------------
reply

`
	if buf.String() != want {
		t.Fatalf("got:\n%s\nwant:\n%s", buf.String(), want)
	}
}
