package core

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode"
)

// FakeMessenger is an in-memory Messenger for tests and for running the
// windows without a network. Deliver simulates an incoming message.
type FakeMessenger struct {
	mu    sync.Mutex
	self  User
	peers []User
	ev    Events
	Sent  []Message
	n     int
}

// NewFakeMessenger returns a messenger that knows the given peers.
func NewFakeMessenger(peers ...User) *FakeMessenger {
	return &FakeMessenger{peers: peers}
}

func (f *FakeMessenger) Start(s Settings, ev Events) error {
	f.mu.Lock()
	f.ev = ev
	f.self = User{ID: "me@localhost", Name: s.Nickname, Group: s.Group, Host: "localhost", Logon: "me", Addr: "127.0.0.1:2425"}
	f.mu.Unlock()
	if ev.PeersChanged != nil {
		ev.PeersChanged(f.Peers())
	}
	return nil
}

func (f *FakeMessenger) Stop() error { return nil }

func (f *FakeMessenger) Self() User {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.self
}

func (f *FakeMessenger) Peers() []User {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]User(nil), f.peers...)
}

func (f *FakeMessenger) Refresh() {
	if f.ev.PeersChanged != nil {
		f.ev.PeersChanged(f.Peers())
	}
}

func (f *FakeMessenger) Send(m Message) (string, error) {
	f.mu.Lock()
	f.n++
	m.ID = fmt.Sprintf("fake-%d", f.n)
	m.From = f.self
	m.Time = time.Now()
	f.Sent = append(f.Sent, m)
	ev := f.ev
	f.mu.Unlock()
	if ev.Delivered != nil {
		for _, u := range m.To {
			ev.Delivered(u, m.ID, nil)
		}
	}
	return m.ID, nil
}

func (f *FakeMessenger) NotifyOpened(m Message) error { return nil }

func (f *FakeMessenger) UpdateStatus(s Settings) {
	f.mu.Lock()
	f.self.Name, f.self.Group, f.self.Absent = s.Nickname, s.Group, s.Absent
	f.mu.Unlock()
}

// Deliver simulates a message arriving from a peer.
func (f *FakeMessenger) Deliver(m Message) {
	m.Incoming = true
	if m.Time.IsZero() {
		m.Time = time.Now()
	}
	if f.ev.Received != nil {
		f.ev.Received(m)
	}
}

// MemStore is an in-memory Store.
type MemStore struct {
	mu       sync.Mutex
	settings Settings
	log      []LogEntry
	seq      int64
}

// NewMemStore returns a store with the default settings.
func NewMemStore() *MemStore { return &MemStore{settings: DefaultSettings()} }

func (s *MemStore) LoadSettings() (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.settings, nil
}

func (s *MemStore) SaveSettings(v Settings) error {
	s.mu.Lock()
	s.settings = v
	s.mu.Unlock()
	return nil
}

func (s *MemStore) Append(m Message) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	s.log = append(s.log, LogEntry{Message: m, Seq: s.seq})
	return s.seq, nil
}

func (s *MemStore) Query(q LogQuery) ([]LogEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []LogEntry
	for _, e := range s.log {
		if MatchLog(e, q) {
			out = append(out, e)
		}
	}
	return out, nil
}

func (s *MemStore) Update(e LogEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.log {
		if s.log[i].Seq == e.Seq {
			s.log[i].Starred, s.log[i].Marked, s.log[i].Note = e.Starred, e.Marked, e.Note
			return nil
		}
	}
	return fmt.Errorf("no log entry %d", e.Seq)
}

func (s *MemStore) Delete(seq int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.log {
		if s.log[i].Seq == seq {
			s.log = append(s.log[:i], s.log[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("no log entry %d", seq)
}

func (s *MemStore) Dir() string { return "" }

// MatchLog reports whether e passes q. Stores share it.
func MatchLog(e LogEntry, q LogQuery) bool {
	if q.Starred && !e.Starred || q.Marked && !e.Marked {
		return false
	}
	if q.UserID != "" && e.From.ID != q.UserID {
		found := false
		for _, u := range e.To {
			found = found || u.ID == q.UserID
		}
		if !found {
			return false
		}
	}
	if !q.Since.IsZero() && e.Time.Before(q.Since) || !q.Until.IsZero() && e.Time.After(q.Until) {
		return false
	}
	if q.Text != "" && !matchText(e, q.Text) {
		return false
	}
	return true
}

// matchText reports whether every space-separated word of t occurs, in any
// order and ignoring case, in the text, note or sender name of e.
func matchText(e LogEntry, t string) bool {
	// Walk the words in place: this runs for every entry of the log.
	for t = strings.TrimLeftFunc(t, unicode.IsSpace); t != ""; {
		end := strings.IndexFunc(t, unicode.IsSpace)
		if end < 0 {
			end = len(t)
		}
		if !matchWord(e, t[:end]) {
			return false
		}
		t = strings.TrimLeftFunc(t[end:], unicode.IsSpace)
	}
	return true
}

// matchWord reports whether the word w, which has no spaces, occurs in the
// text, note or sender name of e.
func matchWord(e LogEntry, t string) bool {
	if ascii(t) {
		// Search the three fields in place instead of building and
		// lowercasing their concatenation for every entry. A field with
		// non-ASCII text goes the exact way below.
		lt := strings.ToLower(t)
		found, exact := false, true
		for _, f := range [...]string{e.Text, e.Note, e.From.Name} {
			if !ascii(f) {
				exact = false
				break
			}
			found = found || containsFoldASCII(f, lt)
		}
		if exact {
			return found
		}
	}
	t = strings.ToLower(t)
	return strings.Contains(strings.ToLower(e.Text+"\n"+e.Note+"\n"+e.From.Name), t)
}

func ascii(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// containsFoldASCII reports whether the ASCII string s contains lt, an
// ASCII string already in lower case, ignoring the case of s.
func containsFoldASCII(s, lt string) bool {
	n := len(lt)
	if n == 0 {
		return true
	}
	c0 := lt[0]
	for i := 0; i+n <= len(s); i++ {
		if lower(s[i]) != c0 {
			continue
		}
		j := 1
		for j < n && lower(s[i+j]) == lt[j] {
			j++
		}
		if j == n {
			return true
		}
	}
	return false
}

func lower(c byte) byte {
	if 'A' <= c && c <= 'Z' {
		return c + 'a' - 'A'
	}
	return c
}
