package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestHighlightEveryWord(t *testing.T) {
	var spans []ui.Span
	ui.NewTester(func(c *ui.Context) {
		bpApply(c, groundChrome, false)
		spans = bpHighlight(c, "The Build on port 2425", "port  build", false)
	}, 200, 50).Frame()
	var hits []string
	for _, s := range spans {
		if s.Background != (ui.Color{}) {
			hits = append(hits, s.Text)
		}
	}
	if len(hits) != 2 || hits[0] != "Build" || hits[1] != "port" {
		t.Errorf("hits %q in %+v", hits, spans)
	}
}
