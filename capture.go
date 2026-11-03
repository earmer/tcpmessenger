package main

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	"math"
	"os"
	"strings"

	"my-app/internal/core"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// errCaptureCancelled is what captureScreen returns when the user leaves
// the range selection without picking one.
var errCaptureCancelled = errors.New("capture cancelled")

// readCaptureFile reads the PNG a capture tool wrote; none, or an empty
// one, means the user cancelled.
func readCaptureFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return nil, errCaptureCancelled
	}
	return data, nil
}

// openCapture opens the screen capture editor. done receives the edited image,
// or is not called when the user cancels.
//
// The image comes from a range of the screen the user picks (screencapture
// on macOS) or, when the clipboard holds an image, from the clipboard if the
// user prefers. Both block, so they run on a goroutine, and the editor opens
// back on the main thread.
func openCapture(a *tcpApp, done func(img core.Attachment)) {
	parent := a.main
	format := a.settings.CaptureFormat
	onMain := func(fn func()) {
		if parent != nil {
			parent.Update(fn)
		} else {
			mygo.App.WhenReady(fn)
		}
	}
	go func() {
		data, err := pickCaptureSource(parent)
		if errors.Is(err, errCaptureCancelled) {
			return
		}
		if err != nil {
			mygo.Dialog.Error("Screen capture", err.Error())
			return
		}
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			mygo.Dialog.Error("Screen capture", "The captured image cannot be read: "+err.Error())
			return
		}
		scale := 1.0
		if d := mygo.Screen.PrimaryDisplay(); d.ScaleFactor > 0 {
			scale = d.ScaleFactor
		}
		onMain(func() { openCaptureEditor(img, format, scale, done) })
	}()
}

// pickCaptureSource asks where the image comes from when the clipboard
// holds one, and returns it as PNG.
func pickCaptureSource(parent *mygo.Window) ([]byte, error) {
	if clip := mygo.Clipboard.ReadImage(); len(clip) > 0 {
		res, err := mygo.Dialog.Message(mygo.MessageOptions{
			Parent:       parent,
			Type:         mygo.MessageInfo,
			Title:        "TCPMsg Capture",
			Message:      "The clipboard holds an image.",
			Detail:       "Capture a range of the screen, or annotate the image on the clipboard?",
			Buttons:      []string{"Capture Screen", "Paste image from clipboard", "Cancel"},
			CancelButton: 2,
		})
		if err != nil {
			return nil, err
		}
		switch res.Button {
		case 0:
		case 1:
			return clip, nil
		default:
			return nil, errCaptureCancelled
		}
	}
	return captureScreen()
}

// openCaptureEditor opens the "TCPMsg Capture" window on img. scale is
// the display's pixels per DIP, so that a capture shows at its real size.
func openCaptureEditor(img image.Image, format string, scale float64, done func(core.Attachment)) {
	e := newCapEditor(img, format)
	var win *mygo.Window
	e.onSend = func(att core.Attachment) {
		if done != nil {
			done(att)
		}
		win.Close()
	}
	e.onCancel = func() { win.Close() }
	e.onCopy = func(png []byte) error { return mygo.Clipboard.WriteImage(png) }
	e.onSave = func(data []byte, ext string) {
		go func() {
			path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
				Parent:      win,
				Title:       "Save Capture",
				DefaultPath: "capture." + ext,
				Filters:     []mygo.FileFilter{{Name: strings.ToUpper(ext) + " image", Extensions: []string{ext}}},
			})
			if err != nil || path == "" {
				return
			}
			if err := os.WriteFile(path, data, 0o644); err != nil {
				mygo.Dialog.Error("Save Capture", err.Error())
				return
			}
			win.Update(func() { e.status = "Saved " + path })
		}()
	}
	b := img.Bounds()
	w := clampInt(int(float64(b.Dx())/scale)+48, 640, 1280)
	h := clampInt(int(float64(b.Dy())/scale)+140, 420, 900)
	win = mygo.NewWindow(mygo.WindowOptions{
		Title:     "TCPMsg Capture",
		Width:     w,
		Height:    h,
		MinWidth:  560,
		MinHeight: 360,
		Content:   ui.View(e.view),
	})
}

func clampInt(v, lo, hi int) int {
	return max(lo, min(hi, v))
}

// capEditor is the state of a capture editor window.
type capEditor struct {
	base   image.Image
	bitmap *ui.Bitmap
	imgW   float64
	imgH   float64
	doc    capDoc

	tool   capTool
	color  int    // index in capColors
	format string // "PNG" or "JPG"
	status string

	// The drag in progress, in image pixels.
	dragging  bool
	draft     *capShape // the shape being drawn
	moveIdx   int       // the shape being moved, or -1
	dragStart fpt
	moveBy    fpt
	selected  int // the shape Move selected, or -1
	ptrX      float32
	ptrY      float32 // the pointer in the canvas, in DIPs

	// The comment of an arrow just drawn, being typed.
	commenting bool
	pending    capShape
	comment    string

	// The canvas as last painted: its size in DIPs and the display's
	// pixels per DIP.
	boxW, boxH float32
	devScale   float32

	onSend   func(core.Attachment)
	onCancel func()
	onCopy   func(png []byte) error
	onSave   func(data []byte, ext string)
}

func newCapEditor(img image.Image, format string) *capEditor {
	b := img.Bounds()
	f := "PNG"
	if normFormat(format) == "jpg" {
		f = "JPG"
	}
	return &capEditor{
		base:     img,
		bitmap:   ui.NewBitmap(img),
		imgW:     float64(b.Dx()),
		imgH:     float64(b.Dy()),
		tool:     toolArrow,
		format:   f,
		moveIdx:  -1,
		selected: -1,
		devScale: 1,
	}
}

// fit is where the image shows in the canvas: its scale (DIPs per image
// pixel) and its top-left corner. It shows a pixel per display pixel, or
// smaller to fit.
func (e *capEditor) fit() (s, ox, oy float64) {
	const margin = 8
	s = 1 / float64(max(e.devScale, 1))
	if e.boxW > 0 && e.boxH > 0 {
		s = math.Min(s, math.Min((float64(e.boxW)-2*margin)/e.imgW, (float64(e.boxH)-2*margin)/e.imgH))
	}
	if s <= 0 {
		s = 1
	}
	ox = math.Max(margin, (float64(e.boxW)-e.imgW*s)/2)
	oy = math.Max(margin, (float64(e.boxH)-e.imgH*s)/2)
	return s, ox, oy
}

// toImage maps a point of the canvas, in DIPs, to the image's pixels,
// kept inside the image.
func (e *capEditor) toImage(x, y float32) fpt {
	s, ox, oy := e.fit()
	p := fpt{(float64(x) - ox) / s, (float64(y) - oy) / s}
	p.X = math.Max(0, math.Min(e.imgW, p.X))
	p.Y = math.Max(0, math.Min(e.imgH, p.Y))
	return p
}

// strokeWidth is 3 DIPs as shown, in image pixels.
func (e *capEditor) strokeWidth() float64 {
	s, _, _ := e.fit()
	return math.Max(2, math.Round(3/s))
}

func (e *capEditor) fontSize() float64 {
	s, _, _ := e.fit()
	return math.Max(10, math.Round(14/s))
}

func (e *capEditor) setTool(t capTool) {
	e.finishComment()
	e.tool = t
	if t != toolMove {
		e.selected = -1
	}
}

func (e *capEditor) beginDrag(p fpt) {
	e.finishComment()
	e.dragging = true
	e.dragStart = p
	e.moveBy = fpt{}
	e.moveIdx = -1
	e.draft = nil
	if e.tool == toolMove {
		e.selected = hitTest(e.doc.shapes, p, 4/e.scale(), capMeasure)
		e.moveIdx = e.selected
		return
	}
	s := capShape{
		Kind:     e.tool,
		Color:    capColors[e.color].rgba,
		Width:    e.strokeWidth(),
		FontSize: e.fontSize(),
		Pts:      []fpt{p, p},
	}
	if e.tool == toolMask {
		s.Color = capColors[len(capColors)-1].rgba // always black
		s.Width = 0
	}
	if e.tool == toolPen {
		s.Pts = []fpt{p}
	}
	e.draft = &s
}

func (e *capEditor) scale() float64 {
	s, _, _ := e.fit()
	return s
}

func (e *capEditor) dragTo(p fpt) {
	if !e.dragging {
		return
	}
	if e.tool == toolMove {
		e.moveBy = fpt{p.X - e.dragStart.X, p.Y - e.dragStart.Y}
		return
	}
	if e.draft == nil {
		return
	}
	if e.draft.Kind == toolPen {
		last := e.draft.Pts[len(e.draft.Pts)-1]
		if math.Hypot(p.X-last.X, p.Y-last.Y) >= 1 {
			e.draft.Pts = append(e.draft.Pts, p)
		}
		return
	}
	e.draft.Pts[1] = p
}

func (e *capEditor) endDrag() {
	if !e.dragging {
		return
	}
	e.dragging = false
	if e.tool == toolMove {
		if e.moveIdx >= 0 && (e.moveBy.X != 0 || e.moveBy.Y != 0) {
			e.doc.replace(e.moveIdx, e.doc.shapes[e.moveIdx].moved(e.moveBy.X, e.moveBy.Y))
		}
		e.moveIdx, e.moveBy = -1, fpt{}
		return
	}
	d := e.draft
	e.draft = nil
	if d == nil {
		return
	}
	if d.Kind == toolPen && len(d.Pts) == 1 {
		d.Pts = append(d.Pts, d.Pts[0].add(0.5, 0.5)) // a dot
	} else if d.empty() {
		return
	}
	if d.Kind == toolArrow {
		e.pending = *d
		e.comment = ""
		e.commenting = true
		return
	}
	e.doc.add(*d)
}

// finishComment adds the arrow waiting for its comment, with what was
// typed.
func (e *capEditor) finishComment() {
	if !e.commenting {
		return
	}
	e.commenting = false
	e.pending.Text = strings.TrimSpace(e.comment)
	e.comment = ""
	e.doc.add(e.pending)
}

func (e *capEditor) undo() {
	if e.commenting {
		e.commenting = false // drop the arrow not yet added
		return
	}
	e.doc.undo()
	e.selected = -1
}

func (e *capEditor) redo() {
	e.finishComment()
	e.doc.redo()
	e.selected = -1
}

// shownShapes are the shapes as they show now: with the one being moved
// where the drag has it, and those being drawn.
func (e *capEditor) shownShapes() []capShape {
	shapes := e.doc.shapes
	if e.dragging && e.moveIdx >= 0 && e.moveIdx < len(shapes) {
		shapes = append([]capShape(nil), shapes...)
		shapes[e.moveIdx] = shapes[e.moveIdx].moved(e.moveBy.X, e.moveBy.Y)
	}
	if e.commenting {
		shapes = append(shapes[:len(shapes):len(shapes)], e.pending)
	}
	if e.draft != nil {
		shapes = append(shapes[:len(shapes):len(shapes)], *e.draft)
	}
	return shapes
}

// encoded renders the image with its shapes in format ("png" or "jpg").
func (e *capEditor) encoded(format string) ([]byte, error) {
	e.finishComment()
	return encodeCapture(renderCapture(e.base, e.doc.shapes), format)
}

// attachment is the edited image as an attachment to send.
func (e *capEditor) attachment() (core.Attachment, error) {
	ext := normFormat(e.format)
	data, err := e.encoded(ext)
	if err != nil {
		return core.Attachment{}, err
	}
	return core.Attachment{Name: "capture." + ext, Size: int64(len(data)), Data: data, IsImage: true}, nil
}

// sizeLabel is what the size tooltip says while a shape is drawn.
func (e *capEditor) sizeLabel() string {
	if !e.dragging || e.draft == nil || len(e.draft.Pts) < 2 {
		return ""
	}
	a, b := e.draft.Pts[0], e.draft.Pts[len(e.draft.Pts)-1]
	if e.draft.Kind == toolPen {
		r := e.draft.bounds().inset(-e.draft.Width / 2)
		a, b = fpt{r.X0, r.Y0}, fpt{r.X1, r.Y1}
	}
	return fmt.Sprintf("%d x %d", int(math.Round(math.Abs(b.X-a.X))), int(math.Round(math.Abs(b.Y-a.Y))))
}

var (
	capFocus   = ui.Hex("#000000")
	capTipBack = ui.Hex("#ffffe1")
)

func uiColor(c interface{ RGBA() (r, g, b, a uint32) }) ui.Color {
	r, g, b, _ := c.RGBA()
	return ui.RGB(uint8(r>>8), uint8(g>>8), uint8(b>>8))
}

func (e *capEditor) view(c *ui.Context) {
	if !e.commenting {
		if c.Shortcut(ui.Cmd, ui.KeyZ) {
			e.undo()
		}
		if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyZ) {
			e.redo()
		}
		if e.tool == toolMove && e.selected >= 0 && (c.Shortcut(0, ui.KeyDelete) || c.Shortcut(0, ui.KeyBackspace)) {
			e.doc.remove(e.selected)
			e.selected = -1
		}
	}
	bpApply(c, groundChrome, false)
	ui.Column(c).Fill().Children(func() {
		e.toolbar(c)
		e.canvas(c)
		e.bottomRow(c)
	})
}

func (e *capEditor) toolbar(c *ui.Context) {
	k := bpOf(c)
	bpToolbar(c, func() {
		ui.Toolbar(c, func() {
			ui.Row(c).Gap(sp2).Children(func() {
				for _, t := range capTools {
					on := e.tool == t.tool
					if bpToggle(c.Key("tool-"+t.label), &on, t.label).Changed() {
						e.setTool(t.tool)
					}
				}
			})
			bpSep(c)
			for i, col := range capColors {
				sw := ui.ButtonBase(c.Key("color-"+col.name)).Size(18, 18).Background(uiColor(col.rgba)).Label(col.name).Tooltip(col.name)
				if i == e.color {
					bpBevelIn(sw.Border(2, k.focus), k)
				} else {
					sw.Border(1, k.fieldEdge)
				}
				if sw.Clicked() {
					e.color = i
				}
			}
			bpSep(c)
			if bpButton(c, "Undo").Disabled(!e.doc.canUndo() && !e.commenting).Tooltip("Undo (Cmd+Z)").Clicked() {
				e.undo()
			}
			if bpButton(c, "Redo").Disabled(!e.doc.canRedo()).Tooltip("Redo (Shift+Cmd+Z)").Clicked() {
				e.redo()
			}
		}).Label("Capture tools").Grow(1)
	})
}

func (e *capEditor) canvas(c *ui.Context) {
	box := ui.Box(c).Grow(1).FillWidth().Clip().Background(bpOf(c).desk)
	if e.tool == toolMove {
		box.Cursor(ui.CursorMove)
	} else {
		box.Cursor(ui.CursorCrosshair)
	}
	x, y, _ := box.PointerPosition()
	if box.Pressed() {
		e.ptrX, e.ptrY = x, y
		if !e.dragging {
			e.beginDrag(e.toImage(x, y))
		} else {
			e.dragTo(e.toImage(x, y))
		}
	} else if e.dragging {
		e.ptrX, e.ptrY = x, y
		e.dragTo(e.toImage(x, y))
		e.endDrag()
	}
	shapes := e.shownShapes()
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		e.boxW, e.boxH, e.devScale = r.W, r.H, p.Scale()
		s, ox, oy := e.fit()
		img := ui.Rect{X: r.X + float32(ox), Y: r.Y + float32(oy), W: float32(e.imgW * s), H: float32(e.imgH * s)}
		p.Shadow(img, 0, 2, 3, 8, 0, ui.RGBA(0, 0, 0, 0.4))
		p.Image(e.bitmap, img, ui.FillBox)
		p.Clip(img, 0, func() {
			for _, sh := range shapes {
				paintShape(p, sh, r.X+float32(ox), r.Y+float32(oy), s)
			}
		})
		if e.selected >= 0 && e.selected < len(shapes) {
			b := shapes[e.selected].bounds()
			if cb, ok := shapes[e.selected].commentBox(capMeasure); ok {
				b = b.union(cb)
			}
			p.StrokeDashed(ui.Rect{
				X: r.X + float32(ox+b.X0*s) - 3, Y: r.Y + float32(oy+b.Y0*s) - 3,
				W: float32((b.X1-b.X0)*s) + 6, H: float32((b.Y1-b.Y0)*s) + 6,
			}, ui.Hex("#3d3dd6"), 0, 1)
		}
	})
	label := e.sizeLabel()
	box.DrawOver(func(p *ui.Painter, r ui.Rect) {
		if label == "" {
			return
		}
		span := ui.Span{Text: label, Size: 11, Color: capFocus}
		w, h := p.MeasureText(0, span)
		tx, ty := r.X+e.ptrX+14, r.Y+e.ptrY+16
		tx = min(tx, r.X+r.W-w-10)
		ty = min(ty, r.Y+r.H-h-8)
		tip := ui.Rect{X: tx, Y: ty, W: w + 8, H: h + 4}
		p.Fill(tip, capTipBack, 0)
		p.Stroke(tip, capFocus, 0, 1)
		p.RichText(tx+4, ty+2, 0, span)
	})
	box.Children(func() {
		if !e.commenting || len(e.pending.Pts) < 2 {
			return
		}
		s, ox, oy := e.fit()
		t := e.pending.Pts[1]
		in := ui.TextInput(c.Key("comment"), &e.comment).Placeholder("Comment, then Enter").Label("Comment").
			Absolute().Left(float32(ox + t.X*s)).Top(float32(oy + t.Y*s)).Width(200).AutoFocus()
		if in.Submitted() || in.Shortcut(0, ui.KeyEscape) {
			e.finishComment()
		}
	})
}

// paintShape draws a shape on the canvas, whose image's top-left corner
// is at (x0, y0) and shows s DIPs a pixel.
func paintShape(p *ui.Painter, sh capShape, x0, y0 float32, s float64) {
	pt := func(q fpt) (float32, float32) { return x0 + float32(q.X*s), y0 + float32(q.Y*s) }
	col := uiColor(sh.Color)
	w := float32(sh.Width * s)
	if len(sh.Pts) == 0 {
		return
	}
	first, last := sh.Pts[0], sh.Pts[len(sh.Pts)-1]
	rect := func() ui.Rect {
		r := rectOf(first, last)
		ax, ay := pt(fpt{r.X0, r.Y0})
		return ui.Rect{X: ax, Y: ay, W: float32((r.X1 - r.X0) * s), H: float32((r.Y1 - r.Y0) * s)}
	}
	switch sh.Kind {
	case toolMask:
		p.Fill(rect(), col, 0)
	case toolRect:
		p.Stroke(rect(), col, 0, w)
	case toolArrow:
		tip, l, r, neck := arrowGeometry(first, last, sh.Width)
		var shaft ui.Path
		shaft.MoveTo(pt(last)).LineTo(pt(neck))
		p.StrokePath(&shaft, w, col)
		var head ui.Path
		head.MoveTo(pt(tip)).LineTo(pt(l)).LineTo(pt(r)).Close()
		p.FillPath(&head, col)
		if box, ok := sh.commentBox(capMeasure); ok {
			bx, by := pt(fpt{box.X0, box.Y0})
			br := ui.Rect{X: bx, Y: by, W: float32((box.X1 - box.X0) * s), H: float32((box.Y1 - box.Y0) * s)}
			p.Fill(br, ui.RGB(255, 255, 255), 0)
			p.Stroke(br, col, 0, max(1, w/2))
			pad := float32(sh.FontSize * 0.3 * s)
			for i, line := range strings.Split(sh.Text, "\n") {
				p.Text(bx+pad, by+pad+float32(i)*float32(sh.FontSize*1.2*s), line, float32(sh.FontSize*s), col)
			}
		}
	default:
		var path ui.Path
		path.MoveTo(pt(first))
		if len(sh.Pts) == 1 {
			path.LineTo(pt(first.add(0.5, 0.5)))
		}
		for _, q := range sh.Pts[1:] {
			path.LineTo(pt(q))
		}
		p.StrokePath(&path, w, col)
	}
}

func (e *capEditor) bottomRow(c *ui.Context) {
	k := bpOf(c)
	ui.Row(c).Padding(sp2, sp3).Gap(sp3).AlignItems(ui.Center).Background(k.face).
		BorderWidth(1, 0, 0, 0).BorderColor(k.cellEdge).Children(func() {
		ui.Text(c, "Tool: "+e.tool.String()).Width(110).Font(bpMonoFont()).TextColor(k.muted)
		ui.Text(c, e.status).Font(bpMonoFont()).TextColor(k.muted).Grow(1).MaxLines(1)
		ui.Select(c.Key("format"), &e.format, []string{"PNG", "JPG"}).Label("Save format").Width(80)
		if bpButton(c, "Save as...").Clicked() {
			ext := normFormat(e.format)
			if data, err := e.encoded(ext); err != nil {
				e.status = err.Error()
			} else if e.onSave != nil {
				e.onSave(data, ext)
			}
		}
		if bpButton(c, "Copy").Clicked() {
			data, err := e.encoded("png") // the clipboard takes PNG
			if err == nil && e.onCopy != nil {
				err = e.onCopy(data)
			}
			if err != nil {
				e.status = "Copy failed: " + err.Error()
			} else {
				e.status = "Copied to the clipboard"
			}
		}
		if bpButton(c, "Paste to Send").Clicked() {
			if att, err := e.attachment(); err != nil {
				e.status = err.Error()
			} else if e.onSend != nil {
				e.onSend(att)
			}
		}
		if bpButton(c, "Cancel").Clicked() {
			if e.onCancel != nil {
				e.onCancel()
			}
		}
	})
}
