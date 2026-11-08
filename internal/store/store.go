// Package store keeps TCP Messenger's settings and message log on disk.
//
// The settings live in settings.json, rewritten atomically on every save.
// The log is a JSON-lines file (tcpmsg-log.jsonl by default) that is only
// ever appended to: a message is one "e" record, and later star, mark,
// note and delete changes are small "u" and "d" records folded in on load.
// When the change records outnumber the messages, Open rewrites the file
// with the folded state. Images attached to logged messages are saved
// under images/ and referenced by path. The whole log is indexed in
// memory, so queries never touch the log file.
package store

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"tcpmessenger/internal/core"
)

const (
	settingsName = "settings.json"
	logName      = "tcpmsg-log.jsonl"
	imagesDir    = "images"
)

// DefaultDir is where the store keeps its files when no other directory is
// given: "TCP Messenger" under the user's configuration directory.
func DefaultDir() string {
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		if home, err := os.UserHomeDir(); err == nil {
			base = home
		} else {
			base = os.TempDir()
		}
	}
	return filepath.Join(base, "TCP Messenger")
}

// Store is the on-disk core.Store. It is safe for concurrent use.
type Store struct {
	dir string

	mu       sync.RWMutex
	settings core.Settings

	logPath string
	f       *os.File // the log, opened for appending
	entries []*entry // by Seq ascending; nil once deleted
	bySeq   map[int64]int
	live    int
	changes int // update and delete records since the last compaction
	next    int64
}

// entry is a log entry in memory. Image attachments have no Data; their
// content is read from paths on demand.
type entry struct {
	core.LogEntry
	paths []string // per attachment, relative to dir; "" when not saved
}

var _ core.Store = (*Store)(nil)

// Open opens the store in dir, creating dir when needed. It loads the
// settings and indexes the log, compacting the log file when it has
// collected more change records than messages.
func Open(dir string) (core.Store, error) {
	s, err := OpenStore(dir)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// OpenStore is Open returning the concrete type, for SetLogPath and Close.
func OpenStore(dir string) (*Store, error) {
	if dir == "" {
		dir = DefaultDir()
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	s := &Store{dir: dir}
	set, err := s.readSettings()
	if err != nil {
		return nil, err
	}
	s.settings = set
	if err := s.openLog(s.resolve(set.LogFile)); err != nil {
		return nil, err
	}
	return s, nil
}

// Dir is the directory the store keeps its files in.
func (s *Store) Dir() string { return s.dir }

// LogPath is the log file currently in use.
func (s *Store) LogPath() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.logPath
}

// Close closes the log file. The store must not be used afterwards.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.f == nil {
		return nil
	}
	err := s.f.Close()
	s.f = nil
	return err
}

func (s *Store) resolve(p string) string {
	if p == "" {
		return filepath.Join(s.dir, logName)
	}
	if strings.HasPrefix(p, "~"+string(filepath.Separator)) || p == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, p[1:])
		}
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(s.dir, p)
	}
	return filepath.Clean(p)
}

// ---- settings ----

func (s *Store) readSettings() (core.Settings, error) {
	set := core.DefaultSettings()
	p := filepath.Join(s.dir, settingsName)
	data, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return set, nil
	}
	if err != nil {
		return set, err
	}
	if err := json.Unmarshal(data, &set); err != nil {
		// Keep the broken file for the user and start from defaults
		// rather than refusing to start.
		os.Rename(p, p+".bad")
		return core.DefaultSettings(), nil
	}
	return set, nil
}

// LoadSettings returns the saved settings, or the defaults on a first run.
// Password is the hash; compare with CheckPassword.
func (s *Store) LoadSettings() (core.Settings, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return copySettings(s.settings), nil
}

// SaveSettings writes v atomically. A Password that is not yet hashed is
// hashed first. When LogFile changed, the log moves to the new file.
func (s *Store) SaveSettings(v core.Settings) error {
	v = copySettings(v)
	if v.Password != "" && !IsHashed(v.Password) {
		v.Password = HashPassword(v.Password)
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := writeAtomic(filepath.Join(s.dir, settingsName), append(data, '\n')); err != nil {
		return err
	}
	s.settings = v
	if p := s.resolve(v.LogFile); p != s.logPath {
		return s.switchLog(p)
	}
	return nil
}

func copySettings(v core.Settings) core.Settings {
	v.BroadcastAddrs = append([]string(nil), v.BroadcastAddrs...)
	v.AbsenceModes = append([]core.AbsenceMode(nil), v.AbsenceModes...)
	return v
}

// writeAtomic writes data to a temporary file next to p and renames it
// over p, so p is never seen half written.
func writeAtomic(p string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(p), "."+filepath.Base(p)+".tmp*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	_, err = tmp.Write(data)
	if err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(name, p)
	}
	if err != nil {
		os.Remove(name)
	}
	return err
}

// ---- log records ----

type attRec struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	IsImage bool   `json:"img,omitempty"`
	// Path is where an image was saved, relative to the store directory.
	Path string `json:"path,omitempty"`
}

type msgRec struct {
	ID          string      `json:"id,omitempty"`
	From        core.User   `json:"from"`
	To          []core.User `json:"to,omitempty"`
	Text        string      `json:"text"`
	Sealed      bool        `json:"sealed,omitempty"`
	Locked      bool        `json:"locked,omitempty"`
	Attachments []attRec    `json:"att,omitempty"`
	Time        time.Time   `json:"time"`
	Incoming    bool        `json:"in,omitempty"`
	Opened      bool        `json:"opened,omitempty"`
	Signed      bool        `json:"signed,omitempty"`
}

// record is one line of the log file.
type record struct {
	// T is "e" for an entry, "u" for a star/mark/note update, "d" for a
	// delete and "h" for the header a compaction writes.
	T       string  `json:"t"`
	Seq     int64   `json:"seq,omitempty"`
	Next    int64   `json:"next,omitempty"` // "h": the next sequence number
	M       *msgRec `json:"m,omitempty"`
	Starred bool    `json:"star,omitempty"`
	Marked  bool    `json:"mark,omitempty"`
	Note    string  `json:"note,omitempty"`
}

func entryRecord(e *entry) record {
	m := e.Message
	r := &msgRec{ID: m.ID, From: m.From, To: m.To, Text: m.Text, Sealed: m.Sealed, Locked: m.Locked,
		Time: m.Time, Incoming: m.Incoming, Opened: m.Opened, Signed: m.Signed}
	for i, a := range m.Attachments {
		r.Attachments = append(r.Attachments, attRec{Name: a.Name, Size: a.Size, IsImage: a.IsImage, Path: e.paths[i]})
	}
	return record{T: "e", Seq: e.Seq, M: r, Starred: e.Starred, Marked: e.Marked, Note: e.Note}
}

func recordEntry(r record) *entry {
	m := r.M
	e := &entry{LogEntry: core.LogEntry{
		Message: core.Message{ID: m.ID, From: m.From, To: m.To, Text: m.Text, Sealed: m.Sealed, Locked: m.Locked,
			Time: m.Time, Incoming: m.Incoming, Opened: m.Opened, Signed: m.Signed},
		Seq: r.Seq, Starred: r.Starred, Marked: r.Marked, Note: r.Note,
	}}
	for _, a := range m.Attachments {
		e.Attachments = append(e.Attachments, core.Attachment{Name: a.Name, Size: a.Size, IsImage: a.IsImage})
		e.paths = append(e.paths, a.Path)
	}
	return e
}

// ---- loading, switching and compacting the log ----

// openLog indexes the log at p and opens it for appending. s.mu must be
// held or s not yet shared.
func (s *Store) openLog(p string) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	s.logPath = p
	s.entries, s.bySeq, s.live, s.changes, s.next = nil, map[int64]int{}, 0, 0, 1
	if err := s.load(); err != nil {
		return err
	}
	if s.changes > 0 && s.changes > s.live {
		if err := s.compact(); err != nil {
			return err
		}
	}
	if s.f == nil {
		f, err := os.OpenFile(p, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
		if err != nil {
			return err
		}
		s.f = f
		// Terminate a last line cut short by a crash so the next record
		// starts on its own line.
		if st, err := f.Stat(); err == nil && st.Size() > 0 {
			if r, err := os.Open(p); err == nil {
				b := make([]byte, 1)
				if _, err := r.ReadAt(b, st.Size()-1); err == nil && b[0] != '\n' {
					f.Write([]byte{'\n'})
				}
				r.Close()
			}
		}
	}
	return nil
}

// SetLogPath moves the log to the file p (relative paths are inside the
// store directory, "" is the default file). The old file is left as it is
// and p is indexed afresh, so pointing at an existing log opens it.
func (s *Store) SetLogPath(p string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.switchLog(s.resolve(p))
}

func (s *Store) switchLog(p string) error {
	if p == s.logPath {
		return nil
	}
	if s.f != nil {
		s.f.Close()
		s.f = nil
	}
	return s.openLog(p)
}

func (s *Store) load() error {
	f, err := os.Open(s.logPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	br := bufio.NewReaderSize(f, 64<<10)
	for {
		line, err := br.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			var r record
			// A line that does not parse (say, cut short by a crash)
			// is skipped.
			if json.Unmarshal(line, &r) == nil {
				s.apply(r)
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// apply folds one record into the index.
func (s *Store) apply(r record) {
	switch r.T {
	case "h":
		if r.Next > s.next {
			s.next = r.Next
		}
	case "e":
		if r.M == nil || r.Seq <= 0 {
			return
		}
		if _, dup := s.bySeq[r.Seq]; dup {
			return
		}
		s.insert(recordEntry(r))
	case "u":
		s.changes++
		if i, ok := s.bySeq[r.Seq]; ok {
			e := s.entries[i]
			e.Starred, e.Marked, e.Note = r.Starred, r.Marked, r.Note
		}
	case "d":
		s.changes++
		s.remove(r.Seq)
	}
}

func (s *Store) insert(e *entry) {
	if n := len(s.entries); n > 0 && s.entries[n-1] != nil && e.Seq < s.entries[n-1].Seq {
		// Out of order: should not happen with this package's files,
		// but keep the slice sorted if it does.
		i := len(s.entries)
		for i > 0 && (s.entries[i-1] == nil || s.entries[i-1].Seq > e.Seq) {
			i--
		}
		s.entries = append(s.entries, nil)
		copy(s.entries[i+1:], s.entries[i:])
		s.entries[i] = e
		s.reindex()
	} else {
		s.bySeq[e.Seq] = len(s.entries)
		s.entries = append(s.entries, e)
	}
	s.live++
	if e.Seq >= s.next {
		s.next = e.Seq + 1
	}
}

func (s *Store) reindex() {
	s.bySeq = make(map[int64]int, len(s.entries))
	for i, e := range s.entries {
		if e != nil {
			s.bySeq[e.Seq] = i
		}
	}
}

func (s *Store) remove(seq int64) *entry {
	i, ok := s.bySeq[seq]
	if !ok {
		return nil
	}
	e := s.entries[i]
	s.entries[i] = nil
	delete(s.bySeq, seq)
	s.live--
	// Drop the holes once they are most of the slice.
	if len(s.entries) > 64 && s.live < len(s.entries)/2 {
		kept := s.entries[:0]
		for _, x := range s.entries {
			if x != nil {
				kept = append(kept, x)
			}
		}
		clear(s.entries[len(kept):])
		s.entries = kept
		s.reindex()
	}
	return e
}

// compact rewrites the log with only the live entries in their folded
// state. s.mu must be held.
func (s *Store) compact() error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.Encode(record{T: "h", Next: s.next})
	for _, e := range s.entries {
		if e != nil {
			if err := enc.Encode(entryRecord(e)); err != nil {
				return err
			}
		}
	}
	if s.f != nil {
		s.f.Close()
		s.f = nil
	}
	if err := writeAtomic(s.logPath, buf.Bytes()); err != nil {
		return err
	}
	s.changes = 0
	return nil
}

// Compact rewrites the log file now, dropping change records and deleted
// entries.
func (s *Store) Compact() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.compact(); err != nil {
		return err
	}
	f, err := os.OpenFile(s.logPath, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	s.f = f
	return nil
}

func (s *Store) write(r record) error {
	if s.f == nil {
		return errors.New("store: closed")
	}
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	_, err = s.f.Write(append(data, '\n'))
	return err
}

// ---- the core.Store log methods ----

// Append records m in the log and returns its sequence number. Image
// attachments are saved under images/ (unless Settings.LogImages is off);
// the content of other attachments is not kept, only their names and sizes.
func (s *Store) Append(m core.Message) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	seq := s.next
	e := &entry{LogEntry: core.LogEntry{Message: m, Seq: seq}}
	e.To = append([]core.User(nil), m.To...)
	e.Attachments = make([]core.Attachment, len(m.Attachments))
	e.paths = make([]string, len(m.Attachments))
	var saved []string
	for i, a := range m.Attachments {
		e.Attachments[i] = core.Attachment{Name: a.Name, Size: a.Size, IsImage: a.IsImage}
		if a.Size == 0 {
			e.Attachments[i].Size = int64(len(a.Data))
		}
		if !a.IsImage || len(a.Data) == 0 || !s.settings.LogImages {
			continue
		}
		rel, err := s.saveImage(seq, a)
		if err != nil {
			for _, p := range saved {
				os.Remove(filepath.Join(s.dir, p))
			}
			return 0, err
		}
		saved = append(saved, rel)
		e.paths[i] = rel
	}
	if len(m.Attachments) == 0 {
		e.Attachments, e.paths = nil, nil
	}
	if err := s.write(entryRecord(e)); err != nil {
		for _, p := range saved {
			os.Remove(filepath.Join(s.dir, p))
		}
		return 0, err
	}
	s.insert(e)
	return seq, nil
}

func (s *Store) saveImage(seq int64, a core.Attachment) (string, error) {
	dir := filepath.Join(s.dir, imagesDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	name := safeName(a.Name)
	for i := 0; ; i++ {
		base := fmt.Sprintf("%d-%s", seq, name)
		if i > 0 {
			base = fmt.Sprintf("%d-%d-%s", seq, i, name)
		}
		f, err := os.OpenFile(filepath.Join(dir, base), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		_, err = f.Write(a.Data)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			os.Remove(f.Name())
			return "", err
		}
		return filepath.Join(imagesDir, base), nil
	}
}

func safeName(n string) string {
	n = filepath.Base(strings.ReplaceAll(n, "\\", "/"))
	n = strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`/\:*?"<>|`, r) {
			return '_'
		}
		return r
	}, n)
	if n == "" || n == "." || n == ".." {
		n = "image"
	}
	return n
}

// Query returns the entries matching q, newest first. Saved images are
// read back into Attachment.Data.
func (s *Store) Query(q core.LogQuery) ([]core.LogEntry, error) {
	s.mu.RLock()
	var hits []*entry
	for i := len(s.entries) - 1; i >= 0; i-- {
		if e := s.entries[i]; e != nil && core.MatchLog(e.LogEntry, q) {
			hits = append(hits, e)
		}
	}
	out := make([]core.LogEntry, len(hits))
	for i, e := range hits {
		out[i] = s.export(e, !q.NoData)
	}
	s.mu.RUnlock()
	return out, nil
}

// export copies e for a caller, reading saved images when withData is set.
func (s *Store) export(e *entry, withData bool) core.LogEntry {
	le := e.LogEntry
	le.To = append([]core.User(nil), e.To...)
	if len(e.Attachments) > 0 {
		le.Attachments = append([]core.Attachment(nil), e.Attachments...)
		for i, p := range e.paths {
			if withData && p != "" {
				if data, err := os.ReadFile(filepath.Join(s.dir, p)); err == nil {
					le.Attachments[i].Data = data
				}
			}
		}
	}
	return le
}

// Image returns the content of the saved image attachment i of entry seq,
// or nil when it has none or the file cannot be read.
func (s *Store) Image(seq int64, i int) []byte {
	p := s.ImagePath(seq, i)
	if p == "" {
		return nil
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	return data
}

// ImagePath returns the file an image attachment of entry seq was saved
// to, or "" when there is none.
func (s *Store) ImagePath(seq int64, attachment int) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	i, ok := s.bySeq[seq]
	if !ok || attachment < 0 || attachment >= len(s.entries[i].paths) || s.entries[i].paths[attachment] == "" {
		return ""
	}
	return filepath.Join(s.dir, s.entries[i].paths[attachment])
}

// Update saves the star, mark and note of e.
func (s *Store) Update(e core.LogEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i, ok := s.bySeq[e.Seq]
	if !ok {
		return fmt.Errorf("store: no log entry %d", e.Seq)
	}
	if err := s.write(record{T: "u", Seq: e.Seq, Starred: e.Starred, Marked: e.Marked, Note: e.Note}); err != nil {
		return err
	}
	x := s.entries[i]
	x.Starred, x.Marked, x.Note = e.Starred, e.Marked, e.Note
	s.changes++
	return nil
}

// Delete removes entry seq from the log, and its saved images.
func (s *Store) Delete(seq int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.bySeq[seq]; !ok {
		return fmt.Errorf("store: no log entry %d", seq)
	}
	if err := s.write(record{T: "d", Seq: seq}); err != nil {
		return err
	}
	e := s.remove(seq)
	s.changes++
	for _, p := range e.paths {
		if p != "" {
			os.Remove(filepath.Join(s.dir, p))
		}
	}
	return nil
}
