package tcpnet

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// keyPair is a node's identity: X25519 for encryption, Ed25519 for signing.
type keyPair struct {
	enc  *ecdh.PrivateKey
	sign ed25519.PrivateKey
}

func newKeyPair() (*keyPair, error) {
	enc, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	_, sign, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return &keyPair{enc: enc, sign: sign}, nil
}

func (k *keyPair) encPub() []byte  { return k.enc.PublicKey().Bytes() }
func (k *keyPair) signPub() []byte { return []byte(k.sign.Public().(ed25519.PublicKey)) }

type keyFile struct {
	Enc  []byte `json:"x25519"`
	Sign []byte `json:"ed25519_seed"`
}

// loadOrCreateKeys reads the keys from path, creating and saving new ones
// when the file is missing or broken. path "" keeps the keys in memory.
// A save failure is not fatal: the keys then last for this run.
func loadOrCreateKeys(path string) (*keyPair, error) {
	if path != "" {
		if b, err := os.ReadFile(path); err == nil {
			var f keyFile
			if json.Unmarshal(b, &f) == nil && len(f.Sign) == ed25519.SeedSize {
				if enc, err := ecdh.X25519().NewPrivateKey(f.Enc); err == nil {
					return &keyPair{enc: enc, sign: ed25519.NewKeyFromSeed(f.Sign)}, nil
				}
			}
		}
	}
	k, err := newKeyPair()
	if err != nil {
		return nil, err
	}
	if path != "" {
		b, _ := json.Marshal(keyFile{Enc: k.enc.Bytes(), Sign: k.sign.Seed()})
		if os.MkdirAll(filepath.Dir(path), 0o700) == nil {
			_ = os.WriteFile(path, b, 0o600)
		}
	}
	return k, nil
}

// box is a payload sealed for one recipient.
type box struct {
	Eph   []byte `json:"eph"`
	Nonce []byte `json:"n"`
	Data  []byte `json:"d"`
}

func boxKey(shared, ephPub, recipPub []byte) ([]byte, error) {
	info := "tcpmsg-v1 box " + string(ephPub) + string(recipPub)
	return hkdf.Key(sha256.New, shared, nil, info, 32)
}

func gcm(key []byte) (cipher.AEAD, error) {
	b, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(b)
}

// seal encrypts plain for the X25519 public key recipPub.
func seal(recipPub, plain, aad []byte) (*box, error) {
	pub, err := ecdh.X25519().NewPublicKey(recipPub)
	if err != nil {
		return nil, err
	}
	eph, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	shared, err := eph.ECDH(pub)
	if err != nil {
		return nil, err
	}
	ephPub := eph.PublicKey().Bytes()
	key, err := boxKey(shared, ephPub, recipPub)
	if err != nil {
		return nil, err
	}
	a, err := gcm(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, a.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return &box{Eph: ephPub, Nonce: nonce, Data: a.Seal(nil, nonce, plain, aad)}, nil
}

var errDecrypt = errors.New("tcpnet: cannot decrypt message")

// open decrypts b with priv.
func open(priv *ecdh.PrivateKey, b *box, aad []byte) ([]byte, error) {
	pub, err := ecdh.X25519().NewPublicKey(b.Eph)
	if err != nil {
		return nil, errDecrypt
	}
	shared, err := priv.ECDH(pub)
	if err != nil {
		return nil, errDecrypt
	}
	key, err := boxKey(shared, b.Eph, priv.PublicKey().Bytes())
	if err != nil {
		return nil, errDecrypt
	}
	a, err := gcm(key)
	if err != nil || len(b.Nonce) != a.NonceSize() {
		return nil, errDecrypt
	}
	plain, err := a.Open(nil, b.Nonce, b.Data, aad)
	if err != nil {
		return nil, errDecrypt
	}
	return plain, nil
}

// signedBytes is what a frame's signature covers.
func signedBytes(typ, from, to string, plain []byte) []byte {
	var b bytes.Buffer
	b.WriteString("tcpmsg-v1\x00")
	b.WriteString(typ)
	b.WriteByte(0)
	b.WriteString(from)
	b.WriteByte(0)
	b.WriteString(to)
	b.WriteByte(0)
	b.Write(plain)
	return b.Bytes()
}

func aadFor(typ, from, to string) []byte {
	return []byte(typ + "\x00" + from + "\x00" + to)
}

func verify(pub, msg, sig []byte) bool {
	return len(pub) == ed25519.PublicKeySize && len(sig) == ed25519.SignatureSize &&
		ed25519.Verify(ed25519.PublicKey(pub), msg, sig)
}

func signWith(k *keyPair, msg []byte) []byte { return ed25519.Sign(k.sign, msg) }
