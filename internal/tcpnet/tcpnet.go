package tcpnet

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"my-app/internal/core"
)

// ErrPortInUse is wrapped by Start's error when the port is taken.
var ErrPortInUse = errors.New("port already in use")

// ErrNotRunning is returned by calls that need Start first.
var ErrNotRunning = errors.New("tcpnet: messenger is not running")

// Config tunes a messenger. The zero value is what New uses (except for
// KeyFile); tests use it to run several nodes on loopback.
type Config struct {
	// ListenIP is the address to bind; "" binds all interfaces.
	ListenIP string
	// NoBroadcast sends presence only to Settings.BroadcastAddrs (and
	// known peers), not to the broadcast addresses.
	NoBroadcast bool
	// Logon and Host override the account and machine names.
	Logon, Host string
	// AnnounceInterval is how often we re-announce (default 3m).
	AnnounceInterval time.Duration
	// PeerTimeout drops peers not heard from for this long (default 10m).
	PeerTimeout time.Duration
	// SendTimeout bounds one delivery attempt (default 15s, plus time for
	// large attachments).
	SendTimeout time.Duration
	// RetryDelay is the wait before the one retry (default 1s).
	RetryDelay time.Duration
	// KeyFile stores the keys; "" keeps them in memory only.
	KeyFile string
}

// New returns the messenger the app uses.
func New() core.Messenger {
	var c Config
	if dir, err := os.UserConfigDir(); err == nil {
		c.KeyFile = filepath.Join(dir, "TCP Messenger", "tcpnet-keys.json")
	}
	return NewWithConfig(c)
}

// NewWithConfig returns a messenger tuned by c.
func NewWithConfig(c Config) *Messenger {
	if c.AnnounceInterval <= 0 {
		c.AnnounceInterval = 3 * time.Minute
	}
	if c.PeerTimeout <= 0 {
		c.PeerTimeout = 10 * time.Minute
	}
	if c.SendTimeout <= 0 {
		c.SendTimeout = 15 * time.Second
	}
	if c.RetryDelay <= 0 {
		c.RetryDelay = time.Second
	}
	return &Messenger{cfg: c}
}

type peer struct {
	user    core.User
	udp     *net.UDPAddr
	encKey  []byte
	signKey []byte
}

// Messenger implements core.Messenger. It is safe for concurrent use.
type Messenger struct {
	cfg Config

	mu       sync.Mutex
	running  bool
	settings core.Settings
	ev       core.Events
	self     core.User
	keys     *keyPair
	instance string
	udp      *net.UDPConn
	ln       net.Listener
	peers    map[string]*peer
	conns    map[net.Conn]struct{}
	seen     map[string]time.Time
	ctx      context.Context
	cancel   context.CancelFunc
	disp     *dispatcher

	wg sync.WaitGroup
}

var _ core.Messenger = (*Messenger)(nil)

func randomID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func logonName() string {
	name := ""
	if u, err := user.Current(); err == nil {
		name = u.Username
	}
	if name == "" {
		name = os.Getenv("USER")
	}
	if name == "" {
		name = os.Getenv("USERNAME")
	}
	if i := strings.LastIndexByte(name, '\\'); i >= 0 {
		name = name[i+1:]
	}
	if name == "" {
		name = "user"
	}
	return name
}

func hostName() string {
	h, _ := os.Hostname()
	if i := strings.IndexByte(h, '.'); i > 0 {
		h = h[:i]
	}
	if h == "" {
		h = "localhost"
	}
	return h
}

func absenceTitle(s core.Settings) string {
	if !s.Absent {
		return ""
	}
	if s.AbsenceIndex >= 0 && s.AbsenceIndex < len(s.AbsenceModes) {
		return s.AbsenceModes[s.AbsenceIndex].Title
	}
	return "Absent"
}

// applySettings fills the status fields of self. Caller holds mu.
func (m *Messenger) applySettings(s core.Settings) {
	m.settings = s
	m.self.Name = s.Nickname
	if m.self.Name == "" {
		m.self.Name = m.self.Logon
	}
	m.self.Group = s.Group
	m.self.Absent = s.Absent
	m.self.AbsenceTitle = absenceTitle(s)
}

func portInUse(err error) bool {
	return errors.Is(err, syscall.EADDRINUSE)
}

func localIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && !n.IP.IsLoopback() {
			if ip4 := n.IP.To4(); ip4 != nil && !ip4.IsLinkLocalUnicast() {
				return ip4.String()
			}
		}
	}
	return ""
}

// Start listens and announces the user.
func (m *Messenger) Start(s core.Settings, ev core.Events) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		return errors.New("tcpnet: already started")
	}
	port := s.Port
	if port <= 0 {
		port = core.DefaultPort
	}
	s.Port = port
	keys, err := loadOrCreateKeys(m.cfg.KeyFile)
	if err != nil {
		return fmt.Errorf("tcpnet: keys: %w", err)
	}
	hostPort := net.JoinHostPort(m.cfg.ListenIP, strconv.Itoa(port))
	ln, err := net.Listen("tcp4", hostPort)
	if err != nil {
		if portInUse(err) {
			return fmt.Errorf("tcpnet: TCP port %d is already in use; another TCP Messenger may be running (%w)", port, ErrPortInUse)
		}
		return fmt.Errorf("tcpnet: listen: %w", err)
	}
	uaddr, err := net.ResolveUDPAddr("udp4", hostPort)
	if err != nil {
		ln.Close()
		return fmt.Errorf("tcpnet: %w", err)
	}
	udp, err := net.ListenUDP("udp4", uaddr)
	if err != nil {
		ln.Close()
		if portInUse(err) {
			return fmt.Errorf("tcpnet: UDP port %d is already in use; another TCP Messenger may be running (%w)", port, ErrPortInUse)
		}
		return fmt.Errorf("tcpnet: listen: %w", err)
	}

	logon := m.cfg.Logon
	if logon == "" {
		logon = logonName()
	}
	host := m.cfg.Host
	if host == "" {
		host = hostName()
	}
	ip := m.cfg.ListenIP
	if ip == "" || ip == "0.0.0.0" {
		ip = localIP()
	}
	if ip == "" {
		ip = "127.0.0.1"
	}
	m.self = core.User{
		ID: logon + "@" + host, Host: host, Logon: logon,
		Addr:    net.JoinHostPort(ip, strconv.Itoa(port)),
		Encrypt: true,
	}
	m.applySettings(s)
	m.ev = ev
	m.keys = keys
	m.instance = randomID()
	m.udp, m.ln = udp, ln
	m.peers = map[string]*peer{}
	m.conns = map[net.Conn]struct{}{}
	m.seen = map[string]time.Time{}
	m.ctx, m.cancel = context.WithCancel(context.Background())
	m.disp = newDispatcher()
	m.running = true

	go m.disp.run()
	m.wg.Add(3)
	go m.readUDP(udp)
	go m.acceptLoop(ln)
	go m.ticker(m.ctx)

	pkt := m.packetLocked(pktEntry)
	dests := m.broadcastDestsLocked()
	m.sendPacket(udp, pkt, dests)
	return nil
}

// Stop announces the exit and closes the sockets.
func (m *Messenger) Stop() error {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return nil
	}
	m.running = false
	pkt := m.packetLocked(pktExit)
	dests := m.broadcastDestsLocked()
	for _, p := range m.peers {
		dests = append(dests, p.udp)
	}
	udp, ln, disp := m.udp, m.ln, m.disp
	m.sendPacket(udp, pkt, dests)
	m.cancel()
	udp.Close()
	ln.Close()
	for c := range m.conns {
		c.Close()
	}
	m.mu.Unlock()
	m.wg.Wait()
	disp.close()
	return nil
}

// Self is the user as others see us.
func (m *Messenger) Self() core.User {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.self
}

// Peers returns the known users sorted by name, not including Self.
func (m *Messenger) Peers() []core.User {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.peerListLocked()
}

func (m *Messenger) peerListLocked() []core.User {
	out := make([]core.User, 0, len(m.peers))
	for _, p := range m.peers {
		out = append(out, p.user)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := strings.ToLower(out[i].Name), strings.ToLower(out[j].Name)
		if a != b {
			return a < b
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// peersChangedLocked queues a PeersChanged event. Caller holds mu.
func (m *Messenger) peersChangedLocked() {
	if f := m.ev.PeersChanged; f != nil {
		list := m.peerListLocked()
		m.disp.post(func() { f(list) })
	}
}

// Refresh asks everyone to announce again.
func (m *Messenger) Refresh() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.running {
		return
	}
	m.sendPacket(m.udp, m.packetLocked(pktRefresh), m.broadcastDestsLocked())
}

// UpdateStatus re-announces after the nickname, group or absence changed.
func (m *Messenger) UpdateStatus(s core.Settings) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s.Port = m.settings.Port // the port only changes on restart
	m.applySettings(s)
	if !m.running {
		return
	}
	m.sendPacket(m.udp, m.packetLocked(pktAbsence), m.broadcastDestsLocked())
}

// ticker re-announces and expires peers.
func (m *Messenger) ticker(ctx context.Context) {
	defer m.wg.Done()
	check := m.cfg.PeerTimeout / 4
	if check > m.cfg.AnnounceInterval {
		check = m.cfg.AnnounceInterval
	}
	if check < 10*time.Millisecond {
		check = 10 * time.Millisecond
	}
	t := time.NewTicker(check)
	defer t.Stop()
	last := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			m.mu.Lock()
			if !m.running {
				m.mu.Unlock()
				return
			}
			if now.Sub(last) >= m.cfg.AnnounceInterval {
				last = now
				m.sendPacket(m.udp, m.packetLocked(pktAbsence), m.broadcastDestsLocked())
			}
			changed := false
			for id, p := range m.peers {
				if now.Sub(p.user.LastSeen) > m.cfg.PeerTimeout {
					delete(m.peers, id)
					changed = true
				}
			}
			for k, t := range m.seen {
				if now.Sub(t) > 10*time.Minute {
					delete(m.seen, k)
				}
			}
			if changed {
				m.peersChangedLocked()
			}
			m.mu.Unlock()
		}
	}
}

// dispatcher runs event callbacks in order on one goroutine, so callbacks
// never run under the messenger's lock and may call back into it.
type dispatcher struct {
	mu     sync.Mutex
	q      []func()
	wake   chan struct{}
	closed bool
}

func newDispatcher() *dispatcher { return &dispatcher{wake: make(chan struct{}, 1)} }

func (d *dispatcher) post(f func()) {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return
	}
	d.q = append(d.q, f)
	d.mu.Unlock()
	select {
	case d.wake <- struct{}{}:
	default:
	}
}

// close lets the queued callbacks run, then ends run.
func (d *dispatcher) close() {
	d.mu.Lock()
	d.closed = true
	d.mu.Unlock()
	select {
	case d.wake <- struct{}{}:
	default:
	}
}

func (d *dispatcher) run() {
	for {
		d.mu.Lock()
		if len(d.q) == 0 {
			closed := d.closed
			d.mu.Unlock()
			if closed {
				return
			}
			<-d.wake
			continue
		}
		f := d.q[0]
		d.q[0] = nil
		d.q = d.q[1:]
		d.mu.Unlock()
		f()
	}
}

func equalUser(a, b core.User) bool {
	a.LastSeen, b.LastSeen = time.Time{}, time.Time{}
	return a == b
}

func sameKey(a, b []byte) bool { return bytes.Equal(a, b) }
