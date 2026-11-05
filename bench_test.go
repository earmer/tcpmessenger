package main

import (
	"fmt"
	"testing"
	"time"

	"my-app/internal/core"
	"my-app/internal/store"

	"github.com/egoist/mygo/ui"
)

func benchLogApp(n int) *tcpApp {
	st := core.NewMemStore()
	fm := core.NewFakeMessenger()
	s := core.DefaultSettings()
	s.Nickname = "Me"
	fm.Start(s, core.Events{})
	base := time.Date(2026, 11, 2, 12, 0, 0, 0, time.Local)
	for i := 0; i < n; i++ {
		u := core.User{ID: fmt.Sprintf("u%d@pc", i%80), Name: fmt.Sprintf("User %d", i%80), Host: "pc"}
		st.Append(core.Message{From: u, To: []core.User{{ID: "me@localhost", Name: "Me"}}, Incoming: i%2 == 0,
			Text: fmt.Sprintf("Message %d about the router and the plan.\nSecond line here.", i), Time: base.Add(time.Duration(i) * time.Minute)})
	}
	return &tcpApp{net: fm, store: st, settings: s}
}

func benchRealStoreApp(b *testing.B, n int) *tcpApp {
	a := benchLogApp(0)
	st, err := store.OpenStore(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { st.Close() })
	base := time.Date(2026, 11, 2, 12, 0, 0, 0, time.Local)
	for i := 0; i < n; i++ {
		u := core.User{ID: fmt.Sprintf("u%d@pc", i%80), Name: fmt.Sprintf("User %d", i%80), Host: "pc"}
		st.Append(core.Message{From: u, To: []core.User{{ID: "me@localhost", Name: "Me"}}, Incoming: i%2 == 0,
			Text: fmt.Sprintf("Message %d about the router and the plan.\nSecond line here.", i), Time: base.Add(time.Duration(i) * time.Minute)})
	}
	a.store = st
	return a
}

func BenchmarkLogViewerRefreshStore20k(b *testing.B) {
	lv := newLogViewer(benchRealStoreApp(b, 20000))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lv.refresh()
	}
}

func BenchmarkLogViewerRefresh20k(b *testing.B) {
	lv := newLogViewer(benchLogApp(20000))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lv.refresh()
	}
}

func BenchmarkLogViewerRefreshFiltered20k(b *testing.B) {
	lv := newLogViewer(benchLogApp(20000))
	lv.query = "message 1999"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lv.refresh()
	}
}

func BenchmarkLogViewerFrame20k(b *testing.B) {
	lv := newLogViewer(benchLogApp(20000))
	lv.query = "router"
	tt := ui.NewTester(lv.view, 900, 620)
	tt.Frame()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tt.Frame()
	}
}
