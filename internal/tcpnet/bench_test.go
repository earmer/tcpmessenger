package tcpnet

import (
	"bytes"
	"io"
	"testing"
)

func benchBody(n int) []byte { return bytes.Repeat([]byte("abcdefgh"), n/8) }

func BenchmarkSignedBytes16M(b *testing.B) {
	body := benchBody(16 << 20)
	b.ReportAllocs()
	b.SetBytes(int64(len(body)))
	for i := 0; i < b.N; i++ {
		signedBytes("msg", "from", "to", body)
	}
}

func BenchmarkWriteFrame16M(b *testing.B) {
	k, _ := newKeyPair()
	body := benchBody(16 << 20)
	bx, _ := seal(k.encPub(), body, nil)
	f := &frame{Type: "msg", From: "a", To: "b", Box: bx}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := writeFrame(io.Discard, f); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReadFrame16M(b *testing.B) {
	k, _ := newKeyPair()
	body := benchBody(16 << 20)
	bx, _ := seal(k.encPub(), body, nil)
	var buf bytes.Buffer
	writeFrame(&buf, &frame{Type: "msg", From: "a", To: "b", Box: bx})
	raw := buf.Bytes()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := readFrame(bytes.NewReader(raw)); err != nil {
			b.Fatal(err)
		}
	}
}
