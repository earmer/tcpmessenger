package tcpnet

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"my-app/internal/core"
)

// freePort finds a port free for both TCP and UDP on loopback.
func freePort(t *testing.T) int {
	t.Helper()
	for i := 0; i < 50; i++ {
		ln, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		port := ln.Addr().(*net.TCPAddr).Port
		u, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
		ln.Close()
		if err == nil {
			u.Close()
			return port
		}
	}
	t.Fatal("no free port")
	return 0
}

type recorder struct {
	mu        sync.Mutex
	peers     []core.User
	received  []core.Message
	opened    []string
	delivered map[string]error
	changed   chan struct{}
}

func newRecorder() *recorder {
	return &recorder{delivered: map[string]error{}, changed: make(chan struct{}, 1)}
}

func (r *recorder) poke() {
	select {
	case r.changed <- struct{}{}:
	default:
	}
}

func (r *recorder) events() core.Events {
	return core.Events{
		PeersChanged: func(u []core.User) { r.mu.Lock(); r.peers = u; r.mu.Unlock(); r.poke() },
		Received:     func(m core.Message) { r.mu.Lock(); r.received = append(r.received, m); r.mu.Unlock(); r.poke() },
		Opened: func(from core.User, id string) {
			r.mu.Lock()
			r.opened = append(r.opened, from.ID+"|"+id)
			r.mu.Unlock()
			r.poke()
		},
		Delivered: func(to core.User, id string, err error) {
			r.mu.Lock()
			r.delivered[to.ID+"|"+id] = err
			r.mu.Unlock()
			r.poke()
		},
	}
}

// wait polls cond until it holds or the timeout passes.
func (r *recorder) wait(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		r.mu.Lock()
		ok := cond()
		r.mu.Unlock()
		if ok {
			return
		}
		select {
		case <-r.changed:
		case <-time.After(20 * time.Millisecond):
		case <-deadline:
			t.Fatalf("timed out waiting for %s", what)
		}
	}
}

type node struct {
	m    *Messenger
	rec  *recorder
	port int
	s    core.Settings
}

// startNodes starts n loopback nodes that know each other's addresses.
func startNodes(t *testing.T, n int, tweak func(i int, c *Config)) []*node {
	t.Helper()
	ports := make([]int, n)
	for i := range ports {
		ports[i] = freePort(t)
	}
	nodes := make([]*node, n)
	for i := range nodes {
		c := Config{ListenIP: "127.0.0.1", NoBroadcast: true, Logon: fmt.Sprintf("user%d", i), Host: "test",
			SendTimeout: 2 * time.Second, RetryDelay: 50 * time.Millisecond}
		if tweak != nil {
			tweak(i, &c)
		}
		s := core.DefaultSettings()
		s.Port = ports[i]
		s.Nickname = fmt.Sprintf("Node %c", 'A'+i)
		s.Group = "QA"
		for j, p := range ports {
			if j != i {
				s.BroadcastAddrs = append(s.BroadcastAddrs, fmt.Sprintf("127.0.0.1:%d", p))
			}
		}
		nd := &node{m: NewWithConfig(c), rec: newRecorder(), port: ports[i], s: s}
		if err := nd.m.Start(s, nd.rec.events()); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { nd.m.Stop() })
		nodes[i] = nd
	}
	return nodes
}

func peerByID(us []core.User, id string) (core.User, bool) {
	for _, u := range us {
		if u.ID == id {
			return u, true
		}
	}
	return core.User{}, false
}

func waitPeers(t *testing.T, nodes []*node, want int) {
	t.Helper()
	for _, nd := range nodes {
		nd.rec.wait(t, "discovery", func() bool { return len(nd.rec.peers) == want })
	}
}

func TestDiscovery(t *testing.T) {
	nodes := startNodes(t, 3, nil)
	waitPeers(t, nodes, 2)
	a := nodes[0]
	if self := a.m.Self(); self.ID != "user0@test" || self.Name != "Node A" || !self.Encrypt {
		t.Fatalf("self = %+v", self)
	}
	peers := a.m.Peers()
	if len(peers) != 2 || peers[0].Name != "Node B" || peers[1].Name != "Node C" {
		t.Fatalf("peers not sorted or wrong: %+v", peers)
	}
	b := peers[0]
	if b.ID != "user1@test" || b.Group != "QA" || b.Host != "test" || b.Logon != "user1" || !b.Encrypt ||
		b.Addr != fmt.Sprintf("127.0.0.1:%d", nodes[1].port) || b.LastSeen.IsZero() {
		t.Fatalf("peer B = %+v", b)
	}
}

func TestNameDefaultsToLogon(t *testing.T) {
	port := freePort(t)
	m := NewWithConfig(Config{ListenIP: "127.0.0.1", NoBroadcast: true, Logon: "alice", Host: "box"})
	s := core.DefaultSettings()
	s.Port = port
	if err := m.Start(s, core.Events{}); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	if u := m.Self(); u.Name != "alice" || u.ID != "alice@box" {
		t.Fatalf("self = %+v", u)
	}
}

func TestSendAck(t *testing.T) {
	nodes := startNodes(t, 3, nil)
	waitPeers(t, nodes, 2)
	a, b, c := nodes[0], nodes[1], nodes[2]
	pb, _ := peerByID(a.m.Peers(), "user1@test")
	pc, _ := peerByID(a.m.Peers(), "user2@test")
	att := core.Attachment{Name: "pic.png", Size: 4, Data: []byte{1, 2, 3, 4}, IsImage: true}
	id, err := a.m.Send(core.Message{To: []core.User{pb, pc}, Text: "hello there", Attachments: []core.Attachment{att}})
	if err != nil || id == "" {
		t.Fatalf("Send = %q, %v", id, err)
	}
	for _, nd := range []*node{b, c} {
		nd.rec.wait(t, "receive", func() bool { return len(nd.rec.received) == 1 })
		got := nd.rec.received[0]
		if got.ID != id || got.Text != "hello there" || !got.Incoming || !got.Signed ||
			got.From.ID != "user0@test" || got.From.Name != "Node A" || len(got.To) != 2 {
			t.Fatalf("received %+v", got)
		}
		if len(got.Attachments) != 1 || !bytes.Equal(got.Attachments[0].Data, att.Data) || !got.Attachments[0].IsImage {
			t.Fatalf("attachments %+v", got.Attachments)
		}
	}
	a.rec.wait(t, "delivered", func() bool { return len(a.rec.delivered) == 2 })
	for k, err := range a.rec.delivered {
		if err != nil || !strings.HasSuffix(k, "|"+id) {
			t.Fatalf("delivered %s: %v", k, err)
		}
	}
}

func TestDeliveryFailure(t *testing.T) {
	nodes := startNodes(t, 1, nil)
	a := nodes[0]
	dead := core.User{ID: "ghost@nowhere", Addr: fmt.Sprintf("127.0.0.1:%d", freePort(t))}
	id, err := a.m.Send(core.Message{To: []core.User{dead}, Text: "anyone?"})
	if err != nil {
		t.Fatal(err)
	}
	a.rec.wait(t, "delivery failure", func() bool { _, ok := a.rec.delivered["ghost@nowhere|"+id]; return ok })
	if a.rec.delivered["ghost@nowhere|"+id] == nil {
		t.Fatal("expected an error")
	}
}

func TestSealedOpened(t *testing.T) {
	nodes := startNodes(t, 2, nil)
	waitPeers(t, nodes, 1)
	a, b := nodes[0], nodes[1]
	id, err := a.m.Send(core.Message{To: a.m.Peers(), Text: "secret", Sealed: true, Locked: true})
	if err != nil {
		t.Fatal(err)
	}
	b.rec.wait(t, "receive", func() bool { return len(b.rec.received) == 1 })
	got := b.rec.received[0]
	if !got.Sealed || !got.Locked || got.Text != "secret" {
		t.Fatalf("received %+v", got)
	}
	if err := b.m.NotifyOpened(got); err != nil {
		t.Fatal(err)
	}
	a.rec.wait(t, "opened", func() bool { return len(a.rec.opened) == 1 })
	if a.rec.opened[0] != "user1@test|"+id {
		t.Fatalf("opened = %v", a.rec.opened)
	}
}

func TestAbsenceChange(t *testing.T) {
	nodes := startNodes(t, 2, nil)
	waitPeers(t, nodes, 1)
	a, b := nodes[0], nodes[1]
	s := b.s
	s.Absent, s.AbsenceIndex, s.Nickname = true, 1, "Bob"
	b.m.UpdateStatus(s)
	a.rec.wait(t, "absence", func() bool {
		u, ok := peerByID(a.rec.peers, "user1@test")
		return ok && u.Absent && u.AbsenceTitle == "Lunch" && u.Name == "Bob"
	})
	if u := b.m.Self(); !u.Absent || u.AbsenceTitle != "Lunch" {
		t.Fatalf("self = %+v", u)
	}
	s.Absent = false
	b.m.UpdateStatus(s)
	a.rec.wait(t, "back", func() bool {
		u, ok := peerByID(a.rec.peers, "user1@test")
		return ok && !u.Absent && u.AbsenceTitle == ""
	})
}

func TestExit(t *testing.T) {
	nodes := startNodes(t, 3, nil)
	waitPeers(t, nodes, 2)
	if err := nodes[2].m.Stop(); err != nil {
		t.Fatal(err)
	}
	waitPeers(t, nodes[:2], 1)
	if _, ok := peerByID(nodes[0].m.Peers(), "user2@test"); ok {
		t.Fatal("C still listed")
	}
	if _, err := nodes[2].m.Send(core.Message{To: nodes[0].m.Peers(), Text: "x"}); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("Send after Stop = %v", err)
	}
}

func TestRefresh(t *testing.T) {
	nodes := startNodes(t, 2, nil)
	waitPeers(t, nodes, 1)
	a := nodes[0]
	a.m.mu.Lock()
	a.m.peers = map[string]*peer{}
	a.m.mu.Unlock()
	a.m.Refresh()
	a.rec.wait(t, "refresh", func() bool { return len(a.m.Peers()) == 1 })
}

func TestExpiry(t *testing.T) {
	nodes := startNodes(t, 2, func(i int, c *Config) {
		if i == 0 {
			c.PeerTimeout = 300 * time.Millisecond
			c.AnnounceInterval = time.Hour
		} else {
			c.AnnounceInterval = time.Hour
		}
	})
	a := nodes[0]
	a.rec.wait(t, "discovery", func() bool { return len(a.rec.peers) == 1 })
	a.rec.wait(t, "expiry", func() bool { return len(a.rec.peers) == 0 })
}

func TestPortInUse(t *testing.T) {
	nodes := startNodes(t, 1, nil)
	m := NewWithConfig(Config{ListenIP: "127.0.0.1", NoBroadcast: true})
	s := core.DefaultSettings()
	s.Port = nodes[0].port
	err := m.Start(s, core.Events{})
	if !errors.Is(err, ErrPortInUse) || !strings.Contains(err.Error(), "already in use") {
		t.Fatalf("Start = %v", err)
	}
}

func TestEncryptedOnWire(t *testing.T) {
	nodes := startNodes(t, 2, nil)
	waitPeers(t, nodes, 1)
	a := nodes[0]
	// Capture what a sends by pointing it at a fake receiver that has B's
	// identity and keys but records the raw frame.
	b := nodes[1]
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	a.m.mu.Lock()
	a.m.peers["user1@test"].user.Addr = ln.Addr().String()
	a.m.mu.Unlock()
	raw := make(chan *frame, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		f, err := readFrame(c)
		if err != nil {
			return
		}
		raw <- f
		writeFrame(c, &frame{Type: frameAck, ID: f.ID, OK: true})
	}()
	pb, _ := peerByID(a.m.Peers(), "user1@test")
	if _, err := a.m.Send(core.Message{To: []core.User{pb}, Text: "top secret text"}); err != nil {
		t.Fatal(err)
	}
	var f *frame
	select {
	case f = <-raw:
	case <-time.After(5 * time.Second):
		t.Fatal("no frame")
	}
	if f.Box == nil || f.Body != nil || bytes.Contains(f.Box.Data, []byte("top secret")) {
		t.Fatalf("frame not encrypted: %+v", f)
	}
	// B's real key decrypts it and the signature checks out.
	if err := b.m.receive(f, &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)}); err != nil {
		t.Fatal(err)
	}
	b.rec.wait(t, "receive", func() bool { return len(b.rec.received) == 1 })
	if got := b.rec.received[0]; got.Text != "top secret text" || !got.Signed {
		t.Fatalf("received %+v", got)
	}
	// A tampered frame is refused.
	f2 := *f
	bx := *f.Box
	bx.Data = append([]byte(nil), bx.Data...)
	bx.Data[0] ^= 1
	f2.Box = &bx
	if err := b.m.receive(&f2, nil); err == nil {
		t.Fatal("tampered frame accepted")
	}
}

func TestCryptoRoundTrip(t *testing.T) {
	k, err := newKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	other, _ := newKeyPair()
	aad := aadFor("msg", "a", "b")
	bx, err := seal(k.encPub(), []byte("hello"), aad)
	if err != nil {
		t.Fatal(err)
	}
	if p, err := open(k.enc, bx, aad); err != nil || string(p) != "hello" {
		t.Fatalf("open = %q, %v", p, err)
	}
	if _, err := open(other.enc, bx, aad); err == nil {
		t.Fatal("wrong key opened the box")
	}
	if _, err := open(k.enc, bx, aadFor("msg", "a", "c")); err == nil {
		t.Fatal("wrong aad accepted")
	}
	msg := signedBytes("msg", "a", "b", []byte("hello"))
	sig := signWith(k, msg)
	if !verify(k.signPub(), msg, sig) || verify(other.signPub(), msg, sig) {
		t.Fatal("signature check wrong")
	}
}

func TestKeyFilePersists(t *testing.T) {
	path := t.TempDir() + "/keys.json"
	k1, err := loadOrCreateKeys(path)
	if err != nil {
		t.Fatal(err)
	}
	k2, err := loadOrCreateKeys(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(k1.encPub(), k2.encPub()) || !bytes.Equal(k1.signPub(), k2.signPub()) {
		t.Fatal("keys not reloaded")
	}
}

// TestBroadcastStart checks that a node bound to all interfaces with real
// broadcasting starts and stops cleanly.
func TestBroadcastStart(t *testing.T) {
	m := NewWithConfig(Config{Logon: "bcast", Host: "test"})
	s := core.DefaultSettings()
	s.Port = freePort(t)
	if err := m.Start(s, core.Events{}); err != nil {
		t.Fatal(err)
	}
	m.Refresh()
	if err := m.Stop(); err != nil {
		t.Fatal(err)
	}
}
