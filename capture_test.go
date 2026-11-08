package main

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"testing"

	"tcpmessenger/internal/core"

	"github.com/egoist/mygo/ui"
)

var (
	red   = capColors[0].rgba
	white = color.RGBA{255, 255, 255, 255}
)

func whiteImage(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Rect, image.NewUniform(white), image.Point{}, draw.Src)
	return img
}

func at(img image.Image, x, y int) color.RGBA {
	return color.RGBAModel.Convert(img.At(x, y)).(color.RGBA)
}

func TestShapeHit(t *testing.T) {
	line := capShape{Kind: toolLine, Width: 4, Pts: []fpt{{10, 10}, {100, 10}}}
	rect := capShape{Kind: toolRect, Width: 2, Pts: []fpt{{20, 20}, {80, 60}}}
	mask := capShape{Kind: toolMask, Pts: []fpt{{200, 200}, {250, 240}}}
	arrow := capShape{Kind: toolArrow, Width: 3, FontSize: 14, Pts: []fpt{{300, 10}, {320, 100}}, Text: "Look"}
	cases := []struct {
		s    capShape
		p    fpt
		want bool
	}{
		{line, fpt{50, 12}, true},
		{line, fpt{50, 20}, false},
		{line, fpt{103, 10}, true}, // round end within the tolerance
		{rect, fpt{20, 40}, true},  // left edge
		{rect, fpt{50, 40}, false}, // inside an outline is not on it
		{mask, fpt{225, 220}, true},
		{mask, fpt{260, 220}, false},
		{arrow, fpt{310, 55}, true},  // shaft
		{arrow, fpt{330, 108}, true}, // comment box
		{arrow, fpt{400, 300}, false},
	}
	for i, tc := range cases {
		if got := tc.s.hit(tc.p, 2, capMeasure); got != tc.want {
			t.Errorf("case %d: hit(%v) on %v = %v, want %v", i, tc.p, tc.s.Kind, got, tc.want)
		}
	}
	// The topmost shape wins.
	over := capShape{Kind: toolMask, Pts: []fpt{{0, 0}, {60, 30}}}
	if got := hitTest([]capShape{line, over}, fpt{50, 10}, 2, capMeasure); got != 1 {
		t.Errorf("hitTest = %d, want 1 (the mask on top)", got)
	}
	if got := hitTest([]capShape{line}, fpt{500, 500}, 2, capMeasure); got != -1 {
		t.Errorf("hitTest on nothing = %d", got)
	}
}

func TestDocUndoRedo(t *testing.T) {
	var d capDoc
	a := capShape{Kind: toolLine, Pts: []fpt{{0, 0}, {10, 10}}}
	b := capShape{Kind: toolRect, Pts: []fpt{{5, 5}, {20, 20}}}
	d.add(a)
	d.add(b)
	if len(d.shapes) != 2 || !d.canUndo() || d.canRedo() {
		t.Fatalf("after two adds: %d shapes", len(d.shapes))
	}
	d.replace(0, a.moved(5, 0))
	if d.shapes[0].Pts[0].X != 5 {
		t.Fatalf("replace did not move: %v", d.shapes[0].Pts)
	}
	d.undo()
	if d.shapes[0].Pts[0].X != 0 {
		t.Errorf("undo of the move: %v", d.shapes[0].Pts)
	}
	d.undo()
	if len(d.shapes) != 1 {
		t.Errorf("undo of the add: %d shapes", len(d.shapes))
	}
	d.redo()
	d.redo()
	if len(d.shapes) != 2 || d.shapes[0].Pts[0].X != 5 || d.canRedo() {
		t.Errorf("after redos: %+v", d.shapes)
	}
	d.undo()
	d.remove(1) // a new change drops what could be redone
	if d.canRedo() || len(d.shapes) != 1 {
		t.Errorf("after remove: redo %v, %d shapes", d.canRedo(), len(d.shapes))
	}
	d.undo()
	d.undo()
	d.undo()
	d.undo() // more undos than changes do nothing
	if len(d.shapes) != 0 || d.canUndo() {
		t.Errorf("all undone: %d shapes", len(d.shapes))
	}
}

func TestRasterize(t *testing.T) {
	base := whiteImage(200, 150)
	shapes := []capShape{
		{Kind: toolLine, Color: red, Width: 5, Pts: []fpt{{10, 20}, {110, 20}}},
		{Kind: toolRect, Color: red, Width: 3, Pts: []fpt{{20, 50}, {80, 100}}},
		{Kind: toolMask, Color: color.RGBA{0, 0, 0, 255}, Pts: []fpt{{120, 50}, {180, 90}}},
		{Kind: toolArrow, Color: red, Width: 3, FontSize: 14, Pts: []fpt{{150, 140}, {100, 110}}, Text: "Hi"},
	}
	out := renderCapture(base, shapes)
	if out.Rect != base.Rect {
		t.Fatalf("size %v", out.Rect)
	}
	if base.At(10, 20) != white {
		t.Fatal("rendering changed the base image")
	}
	check := func(what string, x, y int, want color.RGBA) {
		t.Helper()
		if got := at(out, x, y); got != want {
			t.Errorf("%s: pixel (%d,%d) = %v, want %v", what, x, y, got, want)
		}
	}
	check("line middle", 60, 20, red)
	check("line edge", 60, 18, red)
	check("off the line", 60, 26, white)
	check("past the line's end", 118, 20, white)
	check("rect left edge", 20, 75, red)
	check("rect bottom edge", 50, 100, red)
	check("inside the rect", 50, 75, white)
	check("mask", 150, 70, color.RGBA{0, 0, 0, 255})
	check("past the mask", 185, 70, white)
	check("arrow shaft", 135, 131, red)
	check("next to the arrow head", 150, 130, white)
	// The arrow head is filled just behind its tip.
	if got := at(out, 146, 137); got.G > 100 || got.R < 150 {
		t.Errorf("arrow head pixel = %v", got)
	}
	// The comment box is white, framed in red, with red text in it.
	box, _ := shapes[3].commentBox(capMeasure)
	if c := at(out, int(box.X0)+5, int(box.Y0)); c.R < 150 || c.G > 100 {
		t.Errorf("comment frame pixel = %v", c)
	}
	reds := 0
	for y := int(box.Y0) + 3; y < int(box.Y1)-3; y++ {
		for x := int(box.X0) + 3; x < int(box.X1)-3; x++ {
			if c := at(out, x, y); c.R > 150 && c.G < 120 {
				reds++
			}
		}
	}
	if reds < 10 {
		t.Errorf("comment text: %d red pixels in the box", reds)
	}
}

func TestPenRaster(t *testing.T) {
	pen := capShape{Kind: toolPen, Color: capColors[1].rgba, Width: 3, Pts: []fpt{{10, 10}, {20, 10}, {20, 30}}}
	out := renderCapture(whiteImage(40, 40), []capShape{pen})
	if got := at(out, 20, 20); got != capColors[1].rgba {
		t.Errorf("pen pixel = %v", got)
	}
	if got := at(out, 30, 30); got != white {
		t.Errorf("off the pen = %v", got)
	}
}

func TestEncodeCapture(t *testing.T) {
	img := renderCapture(whiteImage(64, 48), []capShape{{Kind: toolMask, Pts: []fpt{{0, 0}, {32, 48}}, Color: color.RGBA{0, 0, 0, 255}}})
	for _, f := range []string{"png", "PNG", "jpg", "JPG", "jpeg"} {
		data, err := encodeCapture(img, f)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		var dec image.Image
		if normFormat(f) == "png" {
			dec, err = png.Decode(bytes.NewReader(data))
		} else {
			dec, err = jpeg.Decode(bytes.NewReader(data))
		}
		if err != nil {
			t.Fatalf("%s: decode: %v", f, err)
		}
		if dec.Bounds().Dx() != 64 || dec.Bounds().Dy() != 48 {
			t.Errorf("%s: size %v", f, dec.Bounds())
		}
		if c := at(dec, 10, 10); c.R > 30 {
			t.Errorf("%s: masked pixel %v", f, c)
		}
		if c := at(dec, 50, 10); c.R < 220 {
			t.Errorf("%s: white pixel %v", f, c)
		}
	}
	if _, err := encodeCapture(img, "tiff"); err == nil {
		t.Error("tiff encoded")
	}
}

func TestEditorDrag(t *testing.T) {
	e := newCapEditor(whiteImage(300, 200), "png")
	e.boxW, e.boxH = 316, 216 // the image at 1:1 with 8 DIPs around it

	e.tool = toolRect
	e.beginDrag(fpt{10, 10})
	e.dragTo(fpt{60, 40})
	if got := e.sizeLabel(); got != "50 x 30" {
		t.Errorf("size label %q", got)
	}
	e.endDrag()
	if len(e.doc.shapes) != 1 || e.doc.shapes[0].Kind != toolRect {
		t.Fatalf("shapes %+v", e.doc.shapes)
	}

	// A click without a drag draws nothing.
	e.beginDrag(fpt{100, 100})
	e.endDrag()
	if len(e.doc.shapes) != 1 {
		t.Errorf("a click drew %d shapes", len(e.doc.shapes))
	}

	// An arrow waits for its comment.
	e.tool = toolArrow
	e.beginDrag(fpt{200, 50})
	e.dragTo(fpt{150, 120})
	e.endDrag()
	if !e.commenting || len(e.doc.shapes) != 1 {
		t.Fatalf("arrow: commenting %v, %d shapes", e.commenting, len(e.doc.shapes))
	}
	e.comment = " Bug here "
	e.setTool(toolMove)
	if e.commenting || len(e.doc.shapes) != 2 || e.doc.shapes[1].Text != "Bug here" {
		t.Fatalf("comment: %+v", e.doc.shapes)
	}

	// Move the rectangle by its edge.
	e.beginDrag(fpt{10, 25})
	if e.selected != 0 {
		t.Fatalf("selected %d", e.selected)
	}
	e.dragTo(fpt{30, 35})
	e.endDrag()
	if p := e.doc.shapes[0].Pts[0]; p != (fpt{30, 20}) {
		t.Errorf("moved to %v", p)
	}
	e.undo()
	if p := e.doc.shapes[0].Pts[0]; p != (fpt{10, 10}) {
		t.Errorf("undone to %v", p)
	}

	// The mask is black whatever the colour.
	e.tool, e.color = toolMask, 0
	e.beginDrag(fpt{100, 100})
	e.dragTo(fpt{120, 110})
	e.endDrag()
	if c := e.doc.shapes[len(e.doc.shapes)-1].Color; c != (color.RGBA{0, 0, 0, 255}) {
		t.Errorf("mask colour %v", c)
	}

	att, err := e.attachment()
	if err != nil {
		t.Fatal(err)
	}
	if att.Name != "capture.png" || !att.IsImage || att.Size != int64(len(att.Data)) {
		t.Errorf("attachment %q image %v size %d", att.Name, att.IsImage, att.Size)
	}
	img, err := png.Decode(bytes.NewReader(att.Data))
	if err != nil {
		t.Fatal(err)
	}
	if c := at(img, 110, 105); c != (color.RGBA{0, 0, 0, 255}) {
		t.Errorf("masked pixel in the attachment %v", c)
	}
}

func TestEditorView(t *testing.T) {
	e := newCapEditor(whiteImage(400, 300), "jpg")
	var sent []core.Attachment
	var copied []byte
	cancelled := false
	e.onSend = func(a core.Attachment) { sent = append(sent, a) }
	e.onCopy = func(png []byte) error { copied = png; return nil }
	e.onCancel = func() { cancelled = true }

	tt := ui.NewTester(e.view, 1000, 600)
	for _, s := range []string{"Pen", "Line", "Rect", "Arrow+Comment", "Mask", "Move", "Undo", "Redo",
		"Save as...", "Copy", "Paste to Send", "Cancel", "Tool: Arrow+Comment"} {
		if _, ok := tt.Find(s); !ok {
			t.Errorf("%q not shown; texts %q", s, tt.Texts())
		}
	}
	for _, col := range capColors {
		if _, ok := tt.Find(col.name); !ok {
			t.Errorf("swatch %q not shown", col.name)
		}
	}
	if e.format != "JPG" {
		t.Errorf("format %q, want JPG from the settings", e.format)
	}

	if err := tt.Click("Rect"); err != nil {
		t.Fatal(err)
	}
	if e.tool != toolRect || !tt.HasText("Tool: Rect") {
		t.Errorf("tool %v after clicking Rect", e.tool)
	}
	if err := tt.Click("Blue"); err != nil {
		t.Fatal(err)
	}
	if e.color != 1 {
		t.Errorf("colour %d after clicking Blue", e.color)
	}

	// Draw a rectangle across the middle of the canvas.
	tt.Press(400, 250)
	tt.Move(450, 280)
	tt.Move(500, 300)
	if e.sizeLabel() == "" {
		t.Error("no size label while dragging")
	}
	tt.Release(500, 300)
	tt.Frame()
	if len(e.doc.shapes) != 1 || e.doc.shapes[0].Kind != toolRect || e.doc.shapes[0].Color != capColors[1].rgba {
		t.Fatalf("shapes after a drag: %+v", e.doc.shapes)
	}
	r := rectOf(e.doc.shapes[0].Pts[0], e.doc.shapes[0].Pts[1])
	if r.X1-r.X0 < 90 || r.X1-r.X0 > 110 || r.Y1-r.Y0 < 40 || r.Y1-r.Y0 > 60 {
		t.Errorf("rect %v for a 100×50 drag", r)
	}

	if err := tt.Click("Undo"); err != nil {
		t.Fatal(err)
	}
	if len(e.doc.shapes) != 0 {
		t.Errorf("%d shapes after Undo", len(e.doc.shapes))
	}
	if err := tt.Click("Redo"); err != nil {
		t.Fatal(err)
	}
	if len(e.doc.shapes) != 1 {
		t.Errorf("%d shapes after Redo", len(e.doc.shapes))
	}

	if err := tt.Click("Copy"); err != nil {
		t.Fatal(err)
	}
	if _, err := png.Decode(bytes.NewReader(copied)); err != nil {
		t.Errorf("copied no PNG: %v", err)
	}
	if err := tt.Click("Paste to Send"); err != nil {
		t.Fatal(err)
	}
	if len(sent) != 1 || sent[0].Name != "capture.jpg" || !sent[0].IsImage {
		t.Fatalf("sent %+v", sent)
	}
	if _, err := jpeg.Decode(bytes.NewReader(sent[0].Data)); err != nil {
		t.Errorf("sent no JPEG: %v", err)
	}
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	if !cancelled {
		t.Error("Cancel did not cancel")
	}
}

func TestEditorComment(t *testing.T) {
	e := newCapEditor(whiteImage(400, 300), "png")
	tt := ui.NewTester(e.view, 1000, 600)
	tt.Press(400, 200)
	tt.Move(450, 260)
	tt.Release(450, 260)
	tt.Frame()
	if !e.commenting {
		t.Fatalf("no comment box after an arrow; shapes %+v", e.doc.shapes)
	}
	if _, ok := tt.Find("Comment"); !ok {
		t.Fatal("comment input not shown")
	}
	tt.Type("Typo")
	tt.Key(0, ui.KeyEnter)
	if e.commenting || len(e.doc.shapes) != 1 || e.doc.shapes[0].Text != "Typo" {
		t.Errorf("after Enter: commenting %v, shapes %+v", e.commenting, e.doc.shapes)
	}
}
