// Package core holds the types and interfaces that the network layer, the
// storage layer and the windows of TCP Messenger share. It has no
// dependencies on mygo, so every other package can import it.
package core

import "time"

// DefaultPort is the port TCP Messenger listens on, for both the UDP
// presence broadcasts and the TCP message connections.
const DefaultPort = 2425

// User is a member of the LAN that announced itself.
type User struct {
	// ID identifies the user across restarts: "<logon>@<host>".
	ID           string
	Name         string // nickname shown in lists
	Group        string
	Host         string // machine name
	Logon        string // operating system account name
	Addr         string // "ip:port" to send messages to
	Priority     int    // display priority, 0 is normal, -1 hidden
	Absent       bool
	AbsenceTitle string
	// Encrypt is true when the peer advertised a public key.
	Encrypt bool
	// Fingerprint names the key the peer signs with, as three words
	// ("Cobalt-Falcon-24"); empty when it advertised none.
	Fingerprint string
	// KeyChanged is set by the app when the fingerprint differs from the
	// one trusted for this ID.
	KeyChanged bool
	// LastSeen is when the last presence packet came in.
	LastSeen time.Time
}

// Attachment is a file or an image sent with a message.
type Attachment struct {
	Name string
	Size int64
	// Data holds the content. Attachments travel inside the message.
	Data []byte
	// IsImage marks pasted or captured images shown inline.
	IsImage bool
}

// Message is one message, sent or received.
type Message struct {
	ID          string // unique, chosen by the sender
	From        User
	To          []User
	Text        string
	Sealed      bool // the receiver must open it before reading
	Locked      bool // sealed with a password (the receiver's setting)
	Attachments []Attachment
	Time        time.Time
	Incoming    bool
	// Opened is set on incoming sealed messages once opened.
	Opened bool
	// Signed is true when the message carried a valid signature.
	Signed bool
	// AutoReply marks the absence message sent back automatically; it is
	// never answered in turn.
	AutoReply bool
	// Quiet asks the receiver not to pop the message up: it waits in the
	// tray, like a sticky note.
	Quiet bool
}

// AbsenceMode is one of the absence modes the user defined.
type AbsenceMode struct {
	Title   string
	Message string
}

// SortKey is the second key of the user list's sort.
type SortKey int

const (
	SortByName SortKey = iota
	SortByIP
	SortByHost
)

// ListOptions are the columns and sort order of the send window's list.
type ListOptions struct {
	ShowGroup, ShowHost, ShowPriority, ShowLogon, ShowIP, GridLines bool
	SortByGroup, GroupReverse                                       bool
	Second                                                          SortKey
	SecondReverse, IgnoreCase                                       bool
}

// Settings are everything the Settings window edits. The store saves them
// as JSON.
type Settings struct {
	// Basic
	Nickname       string
	Group          string
	Port           int
	BroadcastAddrs []string // extra unicast or broadcast addresses
	DialUp         bool

	// Send
	SealByDefault  bool
	QuoteOnReply   bool
	QuoteMark      string
	CtrlEnterSends bool
	HideAfterSend  bool

	// Receive
	PopupOnReceive    bool
	NoPopupWhenAbsent bool
	PlaySound         bool
	OpenCheck         bool // notify the sender when a sealed message is opened
	NotifyWithToast   bool

	// Absence
	Absent             bool
	AbsenceIndex       int
	AbsenceModes       []AbsenceMode
	AutoAbsence        bool
	AutoAbsenceMinutes int

	// Log
	LogEnabled   bool
	LogFile      string
	LogLogonName bool
	LogIPAddr    bool
	LogImages    bool

	// Send window
	List ListOptions

	// Download
	DownloadDir string

	// Misc
	StartAtLogin  bool
	Password      string // asked before opening locked messages, hashed
	CaptureFormat string // "png" or "jpg"

	// TrustedKeys maps a user ID to the fingerprint first seen for it.
	TrustedKeys map[string]string
}

// DefaultSettings returns the settings of a first run.
func DefaultSettings() Settings {
	return Settings{
		Port:            DefaultPort,
		QuoteOnReply:    true,
		QuoteMark:       "> ",
		CtrlEnterSends:  true,
		PopupOnReceive:  true,
		OpenCheck:       true,
		NotifyWithToast: true,
		AbsenceModes: []AbsenceMode{
			{"Meeting", "I am in a meeting. I will reply when I return."},
			{"Lunch", "Out for lunch. Back in about an hour."},
			{"Out of office", "I am out of the office today."},
			{"Remote", "Working remotely. Messages reach me with a delay."},
		},
		AutoAbsenceMinutes: 10,
		LogEnabled:         true,
		LogImages:          true,
		List: ListOptions{
			ShowGroup: true, ShowHost: true, GridLines: true,
			SortByGroup: true, Second: SortByName,
		},
		CaptureFormat: "png",
	}
}

// LogEntry is a message recorded in the log, with what the log viewer adds.
type LogEntry struct {
	Message
	Seq     int64 // position in the log, from 1
	Starred bool
	Marked  bool
	Note    string
}

// LogQuery filters the log viewer's list. Zero fields match everything.
type LogQuery struct {
	Text    string
	UserID  string
	Starred bool
	Marked  bool
	Since   time.Time
	Until   time.Time
	// NoData leaves out the content of images the store keeps in files;
	// fetch one with ImageLoader when it is needed.
	NoData bool
}

// ImageLoader is implemented by stores that keep image attachments in
// files: Image returns the content of attachment i of entry seq, or nil
// when there is none.
type ImageLoader interface {
	Image(seq int64, i int) []byte
}

// Events are what the network layer reports. Callbacks run on the network
// layer's goroutines: the windows hop to the main thread themselves.
type Events struct {
	PeersChanged func(users []User)
	Received     func(m Message)
	// Opened reports that a peer opened a sealed message we sent.
	Opened func(from User, messageID string)
	// Delivered reports the outcome of a send to one recipient.
	Delivered func(to User, messageID string, err error)
}

// Messenger is the network layer.
type Messenger interface {
	// Start listens and announces the user. It returns once listening.
	Start(s Settings, ev Events) error
	// Stop announces the exit and closes the sockets.
	Stop() error
	// Self is the user as others see us.
	Self() User
	// Peers returns the users currently known, not including Self.
	Peers() []User
	// Refresh broadcasts a request for everyone to announce again.
	Refresh()
	// Send delivers m to each of m.To and returns its ID at once; the
	// outcome of each delivery comes through Events.Delivered.
	Send(m Message) (string, error)
	// NotifyOpened tells the sender of a sealed message that we opened it.
	NotifyOpened(m Message) error
	// UpdateStatus re-announces after the nickname, group or absence
	// changed.
	UpdateStatus(s Settings)
}

// Store keeps the settings and the log on disk.
type Store interface {
	LoadSettings() (Settings, error)
	SaveSettings(s Settings) error
	// Append records m in the log and returns its sequence number.
	Append(m Message) (int64, error)
	Query(q LogQuery) ([]LogEntry, error)
	// Update saves the star, mark and note of e.
	Update(e LogEntry) error
	Delete(seq int64) error
	// Dir is the directory the store keeps its files in.
	Dir() string
}
