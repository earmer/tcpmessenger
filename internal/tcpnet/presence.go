package tcpnet

import (
	"encoding/json"
	"net"
	"strconv"
	"time"

	"my-app/internal/core"
)

const (
	appMagic   = "tcpmsg"
	appVersion = 1
)

const (
	pktEntry       = "entry"
	pktExit        = "exit"
	pktAbsence     = "absence"
	pktAnswerEntry = "answer-entry"
	pktRefresh     = "refresh"
)

// packet is a presence datagram.
type packet struct {
	App      string `json:"app"`
	V        int    `json:"v"`
	Type     string `json:"type"`
	Instance string `json:"inst"`
	ID       string `json:"id"`
	Name     string `json:"name"`
	Group    string `json:"group,omitempty"`
	Host     string `json:"host"`
	Logon    string `json:"logon"`
	Absent   bool   `json:"absent,omitempty"`
	Absence  string `json:"absence,omitempty"`
	Port     int    `json:"port"`
	EncKey   []byte `json:"ek,omitempty"`
	SignKey  []byte `json:"sk,omitempty"`
}

// packetLocked encodes our presence. Caller holds mu.
func (m *Messenger) packetLocked(typ string) []byte {
	p := packet{
		App: appMagic, V: appVersion, Type: typ, Instance: m.instance,
		ID: m.self.ID, Name: m.self.Name, Group: m.self.Group,
		Host: m.self.Host, Logon: m.self.Logon,
		Absent: m.self.Absent, Absence: m.self.AbsenceTitle,
		Port: m.settings.Port, EncKey: m.keys.encPub(), SignKey: m.keys.signPub(),
	}
	b, _ := json.Marshal(p)
	return b
}

// broadcastDestsLocked lists where broadcasts go. Caller holds mu.
func (m *Messenger) broadcastDestsLocked() []*net.UDPAddr {
	port := m.settings.Port
	var out []*net.UDPAddr
	if !m.cfg.NoBroadcast {
		out = append(out, &net.UDPAddr{IP: net.IPv4bcast, Port: port})
		for _, ip := range directedBroadcasts() {
			out = append(out, &net.UDPAddr{IP: ip, Port: port})
		}
	}
	for _, a := range m.settings.BroadcastAddrs {
		if u := resolveDest(a, port); u != nil {
			out = append(out, u)
		}
	}
	if m.settings.DialUp {
		for _, p := range m.peers {
			out = append(out, p.udp)
		}
	}
	return out
}

func resolveDest(a string, port int) *net.UDPAddr {
	if a == "" {
		return nil
	}
	if _, _, err := net.SplitHostPort(a); err != nil {
		a = net.JoinHostPort(a, strconv.Itoa(port))
	}
	u, err := net.ResolveUDPAddr("udp4", a)
	if err != nil {
		return nil
	}
	return u
}

// directedBroadcasts returns the broadcast address of every IPv4 network
// on an interface that is up and can broadcast.
func directedBroadcasts() []net.IP {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []net.IP
	for _, ifc := range ifs {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagBroadcast == 0 {
			continue
		}
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			n, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := n.IP.To4()
			if ip4 == nil || len(n.Mask) != net.IPv4len {
				continue
			}
			b := make(net.IP, net.IPv4len)
			for i := range b {
				b[i] = ip4[i] | ^n.Mask[i]
			}
			out = append(out, b)
		}
	}
	return out
}

// sendPacket writes pkt to each distinct destination. Errors (no route,
// broadcast not permitted) are ignored: presence is best effort.
func (m *Messenger) sendPacket(udp *net.UDPConn, pkt []byte, dests []*net.UDPAddr) {
	done := map[string]bool{}
	for _, d := range dests {
		if d == nil {
			continue
		}
		k := d.String()
		if done[k] {
			continue
		}
		done[k] = true
		_, _ = udp.WriteToUDP(pkt, d)
	}
}

func (m *Messenger) readUDP(udp *net.UDPConn) {
	defer m.wg.Done()
	buf := make([]byte, 64*1024)
	for {
		n, src, err := udp.ReadFromUDP(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			m.mu.Lock()
			running := m.running
			m.mu.Unlock()
			if !running {
				return
			}
			// A transient error (e.g. ICMP unreachable on some systems).
			time.Sleep(10 * time.Millisecond)
			continue
		}
		m.handlePacket(buf[:n], src)
	}
}

func (m *Messenger) handlePacket(b []byte, src *net.UDPAddr) {
	var p packet
	if json.Unmarshal(b, &p) != nil || p.App != appMagic || p.ID == "" || p.Port <= 0 || p.Port > 65535 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.running || p.Instance == m.instance || p.ID == m.self.ID {
		return
	}
	switch p.Type {
	case pktExit:
		if old, ok := m.peers[p.ID]; ok && old.user.Addr == tcpAddr(src, p.Port) {
			delete(m.peers, p.ID)
			m.peersChangedLocked()
		}
		return
	case pktEntry, pktRefresh:
		m.sendPacket(m.udp, m.packetLocked(pktAnswerEntry), []*net.UDPAddr{src})
	case pktAnswerEntry, pktAbsence:
	default:
		return
	}
	m.upsertLocked(p, src)
}

func tcpAddr(src *net.UDPAddr, port int) string {
	return net.JoinHostPort(src.IP.String(), strconv.Itoa(port))
}

// upsertLocked records the sender of p. Caller holds mu.
func (m *Messenger) upsertLocked(p packet, src *net.UDPAddr) {
	u := core.User{
		ID: p.ID, Name: p.Name, Group: p.Group, Host: p.Host, Logon: p.Logon,
		Addr:   tcpAddr(src, p.Port),
		Absent: p.Absent, AbsenceTitle: p.Absence,
		Encrypt:  len(p.EncKey) == 32 && len(p.SignKey) == 32,
		LastSeen: time.Now(),
	}
	if len(p.SignKey) == 32 {
		u.Fingerprint = core.Fingerprint(p.SignKey)
	}
	if u.Name == "" {
		u.Name = u.Logon
	}
	old, ok := m.peers[p.ID]
	if ok {
		u.Priority = old.user.Priority
	}
	changed := !ok || !equalUser(old.user, u) || !sameKey(old.encKey, p.EncKey) || !sameKey(old.signKey, p.SignKey)
	m.peers[p.ID] = &peer{user: u, udp: src, encKey: p.EncKey, signKey: p.SignKey}
	if changed {
		m.peersChangedLocked()
	}
}
