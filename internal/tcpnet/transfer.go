package tcpnet

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"

	"tcpmessenger/internal/core"
)

const (
	frameMsg    = "msg"
	frameOpened = "opened"
	frameAck    = "ack"

	maxFrame = 256 << 20 // attachments travel inline
)

type frame struct {
	Type    string `json:"t"`
	From    string `json:"from,omitempty"`
	To      string `json:"to,omitempty"`
	SignKey []byte `json:"sk,omitempty"`
	Body    []byte `json:"body,omitempty"`
	Box     *box   `json:"box,omitempty"`
	Sig     []byte `json:"sig,omitempty"`
	ID      string `json:"id,omitempty"`
	OK      bool   `json:"ok,omitempty"`
	Err     string `json:"err,omitempty"`
}

type wireUser struct {
	ID    string `json:"id"`
	Name  string `json:"name,omitempty"`
	Group string `json:"group,omitempty"`
	Host  string `json:"host,omitempty"`
	Logon string `json:"logon,omitempty"`
	Port  int    `json:"port,omitempty"`
}

type wireMsg struct {
	ID          string            `json:"id"`
	From        wireUser          `json:"from"`
	To          []wireUser        `json:"to,omitempty"`
	Text        string            `json:"text,omitempty"`
	Sealed      bool              `json:"sealed,omitempty"`
	Locked      bool              `json:"locked,omitempty"`
	Attachments []core.Attachment `json:"att,omitempty"`
	Time        time.Time         `json:"time"`
	AutoReply   bool              `json:"auto,omitempty"`
	Quiet       bool              `json:"quiet,omitempty"`
}

func toWire(u core.User, port int) wireUser {
	return wireUser{ID: u.ID, Name: u.Name, Group: u.Group, Host: u.Host, Logon: u.Logon, Port: port}
}

func writeFrame(w io.Writer, f *frame) error {
	// Encode behind a reserved length prefix so that the frame is built
	// in one buffer and goes out in one write.
	var buf bytes.Buffer
	buf.Write([]byte{0, 0, 0, 0})
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(f); err != nil {
		return err
	}
	b := buf.Bytes()
	b = b[:len(b)-1] // Encode ends the value with a newline
	if len(b)-4 > maxFrame {
		return errors.New("tcpnet: message too large")
	}
	binary.BigEndian.PutUint32(b, uint32(len(b)-4))
	_, err := w.Write(b)
	return err
}

func readFrame(r io.Reader) (*frame, error) {
	var h [4]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(h[:])
	if n > maxFrame {
		return nil, errors.New("tcpnet: frame too large")
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(r, b); err != nil {
		return nil, err
	}
	var f frame
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	return &f, nil
}

// Send delivers m to each of m.To; outcomes come through Events.Delivered.
func (m *Messenger) Send(msg core.Message) (string, error) {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return "", ErrNotRunning
	}
	if len(msg.To) == 0 {
		m.mu.Unlock()
		return "", errors.New("tcpnet: no recipients")
	}
	if msg.ID == "" {
		msg.ID = randomID()
	}
	if msg.Time.IsZero() {
		msg.Time = time.Now()
	}
	w := wireMsg{
		ID: msg.ID, From: toWire(m.self, m.settings.Port), Text: msg.Text,
		Sealed: msg.Sealed, Locked: msg.Locked, Attachments: msg.Attachments, Time: msg.Time,
		AutoReply: msg.AutoReply, Quiet: msg.Quiet,
	}
	for _, u := range msg.To {
		w.To = append(w.To, toWire(u, 0))
	}
	ctx, delivered := m.ctx, m.ev.Delivered
	m.wg.Add(len(msg.To))
	m.mu.Unlock()

	body, err := json.Marshal(w)
	if err != nil {
		m.wg.Add(-len(msg.To))
		return "", err
	}
	for _, to := range msg.To {
		go func(to core.User) {
			defer m.wg.Done()
			err := m.deliverRetry(ctx, to, frameMsg, msg.ID, body)
			if delivered != nil {
				m.post(func() { delivered(to, msg.ID, err) })
			}
		}(to)
	}
	return msg.ID, nil
}

// NotifyOpened tells the sender of the sealed message m that we opened it.
// It waits for the sender's ack.
func (m *Messenger) NotifyOpened(msg core.Message) error {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return ErrNotRunning
	}
	w := wireMsg{ID: msg.ID, From: toWire(m.self, m.settings.Port), Time: time.Now()}
	ctx := m.ctx
	m.wg.Add(1)
	m.mu.Unlock()
	defer m.wg.Done()
	body, err := json.Marshal(w)
	if err != nil {
		return err
	}
	return m.deliverRetry(ctx, msg.From, frameOpened, msg.ID, body)
}

func (m *Messenger) post(f func()) {
	m.mu.Lock()
	d := m.disp
	m.mu.Unlock()
	d.post(f)
}

func (m *Messenger) deliverRetry(ctx context.Context, to core.User, typ, id string, body []byte) error {
	err := m.deliver(ctx, to, typ, id, body)
	if err == nil || ctx.Err() != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return err
	case <-time.After(m.cfg.RetryDelay):
	}
	return m.deliver(ctx, to, typ, id, body)
}

// deliver makes one attempt to send a frame to to and read its ack.
func (m *Messenger) deliver(ctx context.Context, to core.User, typ, id string, body []byte) error {
	m.mu.Lock()
	addr, encKey := to.Addr, []byte(nil)
	if p, ok := m.peers[to.ID]; ok {
		addr, encKey = p.user.Addr, p.encKey
	}
	self := m.self.ID
	keys := m.keys
	m.mu.Unlock()
	if addr == "" {
		return fmt.Errorf("tcpnet: no address for %s", to.ID)
	}

	f := &frame{Type: typ, From: self, To: to.ID, ID: id, SignKey: keys.signPub()}
	f.Sig = signWith(keys, signedBytes(typ, self, to.ID, body))
	if len(encKey) == 32 {
		b, err := seal(encKey, body, aadFor(typ, self, to.ID))
		if err != nil {
			return fmt.Errorf("tcpnet: encrypt: %w", err)
		}
		f.Box = b
	} else {
		f.Body = body
	}

	timeout := m.cfg.SendTimeout + time.Duration(len(body)/(256<<10))*time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp4", addr)
	if err != nil {
		return fmt.Errorf("tcpnet: cannot reach %s: %w", addr, err)
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	deadline, _ := ctx.Deadline()
	_ = conn.SetDeadline(deadline)
	if err := writeFrame(conn, f); err != nil {
		return fmt.Errorf("tcpnet: send to %s: %w", addr, err)
	}
	ack, err := readFrame(conn)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("tcpnet: no answer from %s", addr)
		}
		return fmt.Errorf("tcpnet: no ack from %s: %w", addr, err)
	}
	if ack.Type != frameAck || ack.ID != id {
		return errors.New("tcpnet: bad ack")
	}
	if !ack.OK {
		return fmt.Errorf("tcpnet: refused by %s: %s", to.ID, ack.Err)
	}
	return nil
}

func (m *Messenger) acceptLoop(ln net.Listener) {
	defer m.wg.Done()
	for {
		c, err := ln.Accept()
		if err != nil {
			m.mu.Lock()
			running := m.running
			m.mu.Unlock()
			if !running {
				return
			}
			time.Sleep(10 * time.Millisecond)
			continue
		}
		m.mu.Lock()
		if !m.running {
			m.mu.Unlock()
			c.Close()
			return
		}
		m.conns[c] = struct{}{}
		m.wg.Add(1)
		m.mu.Unlock()
		go func() {
			defer m.wg.Done()
			m.handleConn(c)
			m.mu.Lock()
			delete(m.conns, c)
			m.mu.Unlock()
			c.Close()
		}()
	}
}

func (m *Messenger) handleConn(c net.Conn) {
	_ = c.SetDeadline(time.Now().Add(m.cfg.SendTimeout + 2*time.Minute))
	f, err := readFrame(c)
	if err != nil {
		return
	}
	ack := &frame{Type: frameAck, ID: f.ID}
	if err := m.receive(f, c.RemoteAddr()); err != nil {
		ack.Err = err.Error()
	} else {
		ack.OK = true
	}
	_ = writeFrame(c, ack)
}

// receive decrypts, verifies and reports one incoming frame.
func (m *Messenger) receive(f *frame, remote net.Addr) error {
	m.mu.Lock()
	keys, selfID := m.keys, m.self.ID
	known, haveKnown := m.peers[f.From]
	var knownCopy peer
	if haveKnown {
		knownCopy = *known
	}
	m.mu.Unlock()

	if f.To != selfID {
		return fmt.Errorf("not addressed to %s", selfID)
	}
	var plain []byte
	if f.Box != nil {
		p, err := open(keys.enc, f.Box, aadFor(f.Type, f.From, f.To))
		if err != nil {
			return err
		}
		plain = p
	} else {
		plain = f.Body
	}
	signKey := f.SignKey
	if haveKnown && len(knownCopy.signKey) > 0 {
		signKey = knownCopy.signKey
	}
	signed := verify(signKey, signedBytes(f.Type, f.From, f.To, plain), f.Sig)

	var w wireMsg
	if err := json.Unmarshal(plain, &w); err != nil {
		return errors.New("malformed message")
	}
	if w.From.ID != f.From || w.ID != f.ID || w.ID == "" {
		return errors.New("inconsistent message")
	}

	from := knownCopy.user
	if !haveKnown {
		ip := ""
		if ta, ok := remote.(*net.TCPAddr); ok {
			ip = ta.IP.String()
		}
		from = core.User{
			ID: w.From.ID, Name: w.From.Name, Group: w.From.Group,
			Host: w.From.Host, Logon: w.From.Logon,
		}
		if from.Name == "" {
			from.Name = from.Logon
		}
		if ip != "" && w.From.Port > 0 {
			from.Addr = net.JoinHostPort(ip, strconv.Itoa(w.From.Port))
		}
	}

	switch f.Type {
	case frameOpened:
		m.mu.Lock()
		cb := m.ev.Opened
		m.mu.Unlock()
		if cb != nil {
			m.post(func() { cb(from, w.ID) })
		}
		return nil
	case frameMsg:
	default:
		return fmt.Errorf("unknown frame type %q", f.Type)
	}

	key := w.From.ID + "\x00" + w.ID
	m.mu.Lock()
	_, dup := m.seen[key]
	if !dup {
		m.seen[key] = time.Now()
	}
	cb := m.ev.Received
	self := m.self
	m.mu.Unlock()
	if dup {
		return nil
	}
	msg := core.Message{
		ID: w.ID, From: from, Text: w.Text, Sealed: w.Sealed, Locked: w.Locked,
		Attachments: w.Attachments, Time: w.Time, Incoming: true, Signed: signed,
		AutoReply: w.AutoReply, Quiet: w.Quiet,
	}
	if len(signKey) == 32 {
		msg.From.Fingerprint = core.Fingerprint(signKey)
	}
	for _, u := range w.To {
		if u.ID == self.ID {
			msg.To = append(msg.To, self)
		} else {
			msg.To = append(msg.To, core.User{ID: u.ID, Name: u.Name, Group: u.Group, Host: u.Host, Logon: u.Logon})
		}
	}
	if len(msg.To) == 0 {
		msg.To = []core.User{self}
	}
	if cb != nil {
		m.post(func() { cb(msg) })
	}
	return nil
}
