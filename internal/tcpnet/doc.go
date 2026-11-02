// Package tcpnet is the network layer of TCP Messenger. It implements
// core.Messenger with its own small protocol, described here.
//
// # Ports
//
// Every node uses one port number (Settings.Port, default 2425) for both a
// UDP socket (presence) and a TCP listener (messages). If either is taken,
// Start fails with an error wrapping ErrPortInUse.
//
// # Presence (UDP)
//
// Presence packets are single UDP datagrams holding one JSON object:
//
//	{"app":"tcpmsg","v":1,"type":"entry","inst":"<random>",
//	 "id":"logon@host","name":"...","group":"...","host":"...","logon":"...",
//	 "absent":false,"absence":"","port":2425,
//	 "ek":"<base64 X25519 public key>","sk":"<base64 Ed25519 public key>"}
//
// Types:
//
//   - entry: sent at Start. Receivers add the sender and reply with a
//     unicast answer-entry so the newcomer learns everyone.
//   - answer-entry: the reply to entry or refresh.
//   - absence: a status change (nickname, group, absence). It is also sent
//     every AnnounceInterval (default 3 minutes) as a keep-alive.
//   - refresh: asks everyone to reply with answer-entry (Messenger.Refresh).
//   - exit: sent at Stop; receivers drop the sender.
//
// Broadcast packets go to 255.255.255.255, to the directed broadcast
// address of every local IPv4 interface, to each Settings.BroadcastAddrs
// entry ("host", "ip" or "ip:port") and, with Settings.DialUp, unicast to
// every known peer. Packets carrying our own instance token are dropped.
// A peer not heard from for PeerTimeout (default 10 minutes) is dropped.
// The peer's TCP address is the packet's source IP with the advertised port.
//
// # Messages (TCP)
//
// A sender opens one TCP connection per delivery and writes one frame; the
// receiver answers with one ack frame and the connection closes. A frame is
// a 4-byte big-endian length followed by that many bytes of JSON:
//
//	{"t":"msg"|"opened"|"ack", "from":"<sender id>", "to":"<recipient id>",
//	 "sk":"<sender Ed25519 key>", "body":"<plain payload>" | "box":{...},
//	 "sig":"<signature>", "id":"<message id>", "ok":true, "err":""}
//
// The payload of "msg" is a JSON message (id, from, to, text, sealed,
// locked, attachments with their data inline, time). The payload of
// "opened" names the ID of the sealed message the recipient opened; the
// original sender reports it through Events.Opened. An "ack" carries ok or
// an error string.
//
// A delivery that fails or gets no ack within the timeout is retried once;
// the outcome is reported by Events.Delivered. The receiver drops
// duplicates (same sender and message ID), acking them again.
//
// # Encryption and signing
//
// Each node has an X25519 key (encryption) and an Ed25519 key (signing),
// advertised in presence packets. The payload is always signed with
// Ed25519 over "tcpmsg-v1\x00" + type + "\x00" + from + "\x00" + to +
// "\x00" + plaintext. When the recipient advertised an X25519 key, the
// payload is sealed: the sender makes an ephemeral X25519 key, derives an
// AES-256 key with HKDF-SHA256 from the shared secret (info binds both
// public keys) and encrypts with AES-GCM, using type, from and to as
// additional data. Message.Signed is set when the signature verifies with
// the key the sender advertised in presence (or, for a sender not yet
// seen, with the key in the frame).
//
// Keys are kept in memory and, when Config.KeyFile is set (New sets it to
// os.UserConfigDir()/TCP Messenger/tcpnet-keys.json), stored there so the
// identity survives restarts.
package tcpnet
