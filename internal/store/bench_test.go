package store

import (
	"fmt"
	"testing"
	"time"

	"tcpmessenger/internal/core"
)

func benchMsg(i int, imgs bool) core.Message {
	m := core.Message{
		ID:       fmt.Sprintf("id-%d", i),
		From:     core.User{ID: fmt.Sprintf("u%d", i%50), Name: fmt.Sprintf("User %d", i%50), Host: "host", Logon: "logon"},
		To:       []core.User{{ID: "me", Name: "Me", Host: "h"}},
		Text:     fmt.Sprintf("Message number %d with some body text to make the line realistic and searchable", i),
		Time:     time.Unix(1700000000+int64(i), 0),
		Incoming: i%2 == 0,
	}
	if imgs {
		m.Attachments = []core.Attachment{{Name: "p.png", IsImage: true, Data: make([]byte, 200<<10)}}
	}
	return m
}

func benchFill(b *testing.B, n int, imgEvery int) string {
	dir := b.TempDir()
	s, err := OpenStore(dir)
	if err != nil {
		b.Fatal(err)
	}
	for i := 0; i < n; i++ {
		if _, err := s.Append(benchMsg(i, imgEvery > 0 && i%imgEvery == 0)); err != nil {
			b.Fatal(err)
		}
	}
	s.Close()
	return dir
}

func BenchmarkOpen20k(b *testing.B) {
	dir := benchFill(b, 20000, 0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s, err := OpenStore(dir)
		if err != nil {
			b.Fatal(err)
		}
		s.Close()
	}
}

func BenchmarkQueryAll20k(b *testing.B) {
	dir := benchFill(b, 20000, 0)
	s, _ := OpenStore(dir)
	defer s.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Query(core.LogQuery{})
	}
}

func BenchmarkQueryText20k(b *testing.B) {
	dir := benchFill(b, 20000, 0)
	s, _ := OpenStore(dir)
	defer s.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Query(core.LogQuery{Text: "number 1999"})
	}
}

func BenchmarkQueryImages(b *testing.B) {
	dir := benchFill(b, 500, 2) // 250 images of 200 KiB
	s, _ := OpenStore(dir)
	defer s.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Query(core.LogQuery{})
	}
}

func BenchmarkAppend(b *testing.B) {
	s, _ := OpenStore(b.TempDir())
	defer s.Close()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		s.Append(benchMsg(i, false))
	}
}

func BenchmarkQueryImagesNoData(b *testing.B) {
	dir := benchFill(b, 500, 2)
	s, _ := OpenStore(dir)
	defer s.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Query(core.LogQuery{NoData: true})
	}
}

func TestQueryNoDataAndImage(t *testing.T) {
	s, _ := OpenStore(t.TempDir())
	defer s.Close()
	seq, err := s.Append(benchMsg(0, true))
	if err != nil {
		t.Fatal(err)
	}
	got, _ := s.Query(core.LogQuery{NoData: true})
	if len(got) != 1 || len(got[0].Attachments[0].Data) != 0 || got[0].Attachments[0].Size != 200<<10 {
		t.Fatalf("NoData should skip the content: %+v", got)
	}
	if got, _ = s.Query(core.LogQuery{}); len(got[0].Attachments[0].Data) != 200<<10 {
		t.Fatal("default query should include the content")
	}
	if len(s.Image(seq, 0)) != 200<<10 || s.Image(seq, 1) != nil || s.Image(99, 0) != nil {
		t.Fatal("Image")
	}
}
