package main

import (
	"image/color"
	"math"
)

// The shape model of the capture editor: annotations drawn over a captured
// image, in the image's pixels, with an undo history. It knows nothing of
// windows, so tests exercise it directly.

// capTool is a tool of the capture editor's toolbar.
type capTool int

const (
	toolPen capTool = iota
	toolLine
	toolRect
	toolArrow // an arrow, then a comment box at its tail
	toolMask  // a filled black box hiding what is under it
	toolMove  // select and move shapes
)

var capTools = []struct {
	tool  capTool
	label string
}{
	{toolPen, "Pen"},
	{toolLine, "Line"},
	{toolRect, "Rect"},
	{toolArrow, "Arrow+Comment"},
	{toolMask, "Mask"},
	{toolMove, "Move"},
}

func (t capTool) String() string {
	for _, ct := range capTools {
		if ct.tool == t {
			return ct.label
		}
	}
	return "?"
}

// capColors are the swatches of the toolbar (the mockup's colours).
var capColors = []struct {
	name string
	rgba color.RGBA
}{
	{"Red", color.RGBA{0xc4, 0x10, 0x10, 0xff}},
	{"Blue", color.RGBA{0x00, 0x00, 0xee, 0xff}},
	{"Green", color.RGBA{0x2f, 0x7a, 0x22, 0xff}},
	{"Yellow", color.RGBA{0xe6, 0xb8, 0x00, 0xff}},
	{"Black", color.RGBA{0x00, 0x00, 0x00, 0xff}},
}

// fpt is a point in image pixels.
type fpt struct{ X, Y float64 }

func (p fpt) add(dx, dy float64) fpt { return fpt{p.X + dx, p.Y + dy} }

// frect is an axis-aligned rectangle in image pixels.
type frect struct{ X0, Y0, X1, Y1 float64 }

func rectOf(a, b fpt) frect {
	return frect{math.Min(a.X, b.X), math.Min(a.Y, b.Y), math.Max(a.X, b.X), math.Max(a.Y, b.Y)}
}

func (r frect) contains(p fpt) bool {
	return p.X >= r.X0 && p.X <= r.X1 && p.Y >= r.Y0 && p.Y <= r.Y1
}

func (r frect) union(o frect) frect {
	return frect{math.Min(r.X0, o.X0), math.Min(r.Y0, o.Y0), math.Max(r.X1, o.X1), math.Max(r.Y1, o.Y1)}
}

func (r frect) inset(d float64) frect { return frect{r.X0 - d, r.Y0 - d, r.X1 + d, r.Y1 + d} }

// capShape is one annotation.
//
//   - Pen: Pts is the polyline drawn.
//   - Line: Pts[0] to Pts[1].
//   - Rect, Mask: Pts[0] and Pts[1] are opposite corners.
//   - Arrow: the head is at Pts[0] (where the drag began, the thing pointed
//     at) and the tail at Pts[1], where the comment Text sits.
type capShape struct {
	Kind     capTool
	Color    color.RGBA
	Width    float64 // stroke width in pixels
	FontSize float64 // comment size in pixels (Arrow)
	Pts      []fpt
	Text     string
}

func (s capShape) clone() capShape {
	s.Pts = append([]fpt(nil), s.Pts...)
	return s
}

// moved returns s moved by (dx, dy).
func (s capShape) moved(dx, dy float64) capShape {
	s = s.clone()
	for i := range s.Pts {
		s.Pts[i] = s.Pts[i].add(dx, dy)
	}
	return s
}

// empty reports a shape too small to keep, as a click without a drag.
func (s capShape) empty() bool {
	if len(s.Pts) < 2 {
		return true
	}
	b := rectOf(s.Pts[0], s.Pts[0])
	for _, p := range s.Pts[1:] {
		b = b.union(rectOf(p, p))
	}
	return b.X1-b.X0 < 2 && b.Y1-b.Y0 < 2
}

// commentBox is where an arrow's comment shows: below and right of the
// tail, sized by measure (text width and height in pixels for the shape's
// font size).
func (s capShape) commentBox(measure func(text string, size float64) (w, h float64)) (frect, bool) {
	if s.Kind != toolArrow || s.Text == "" || len(s.Pts) < 2 {
		return frect{}, false
	}
	w, h := measure(s.Text, s.FontSize)
	pad := s.FontSize * 0.3
	t := s.Pts[1]
	return frect{t.X, t.Y, t.X + w + 2*pad, t.Y + h + 2*pad}, true
}

// bounds is the box a shape covers, without its comment.
func (s capShape) bounds() frect {
	if len(s.Pts) == 0 {
		return frect{}
	}
	b := rectOf(s.Pts[0], s.Pts[0])
	for _, p := range s.Pts[1:] {
		b = b.union(rectOf(p, p))
	}
	return b.inset(s.Width / 2)
}

// segDist is the distance from p to the segment a–b.
func segDist(p, a, b fpt) float64 {
	dx, dy := b.X-a.X, b.Y-a.Y
	l2 := dx*dx + dy*dy
	t := 0.0
	if l2 > 0 {
		t = ((p.X-a.X)*dx + (p.Y-a.Y)*dy) / l2
		t = math.Max(0, math.Min(1, t))
	}
	x, y := a.X+t*dx-p.X, a.Y+t*dy-p.Y
	return math.Sqrt(x*x + y*y)
}

// hit reports whether p is on s, within tol pixels of its strokes.
func (s capShape) hit(p fpt, tol float64, measure func(string, float64) (float64, float64)) bool {
	if len(s.Pts) == 0 {
		return false
	}
	near := tol + s.Width/2
	switch s.Kind {
	case toolMask:
		return rectOf(s.Pts[0], s.Pts[len(s.Pts)-1]).inset(tol).contains(p)
	case toolRect:
		r := rectOf(s.Pts[0], s.Pts[len(s.Pts)-1])
		c := []fpt{{r.X0, r.Y0}, {r.X1, r.Y0}, {r.X1, r.Y1}, {r.X0, r.Y1}}
		for i := range c {
			if segDist(p, c[i], c[(i+1)%4]) <= near {
				return true
			}
		}
		return false
	case toolArrow:
		if box, ok := s.commentBox(measure); ok && box.contains(p) {
			return true
		}
	}
	if len(s.Pts) == 1 {
		return segDist(p, s.Pts[0], s.Pts[0]) <= near
	}
	for i := 1; i < len(s.Pts); i++ {
		if segDist(p, s.Pts[i-1], s.Pts[i]) <= near {
			return true
		}
	}
	return false
}

// hitTest returns the topmost shape at p, or -1.
func hitTest(shapes []capShape, p fpt, tol float64, measure func(string, float64) (float64, float64)) int {
	for i := len(shapes) - 1; i >= 0; i-- {
		if shapes[i].hit(p, tol, measure) {
			return i
		}
	}
	return -1
}

// capDoc is the editor's shapes with their undo history: every change
// replaces the shapes with a new slice, and the old one goes on the undo
// stack.
type capDoc struct {
	shapes []capShape
	undos  [][]capShape
	redos  [][]capShape
}

func cloneShapes(s []capShape) []capShape {
	out := make([]capShape, len(s))
	for i := range s {
		out[i] = s[i].clone()
	}
	return out
}

// set makes shapes the document's content, as one undoable step.
func (d *capDoc) set(shapes []capShape) {
	d.undos = append(d.undos, d.shapes)
	d.redos = nil
	d.shapes = shapes
}

func (d *capDoc) add(s capShape) {
	d.set(append(cloneShapes(d.shapes), s.clone()))
}

// replace puts s in place of shape i.
func (d *capDoc) replace(i int, s capShape) {
	if i < 0 || i >= len(d.shapes) {
		return
	}
	next := cloneShapes(d.shapes)
	next[i] = s.clone()
	d.set(next)
}

func (d *capDoc) remove(i int) {
	if i < 0 || i >= len(d.shapes) {
		return
	}
	next := cloneShapes(d.shapes)
	d.set(append(next[:i], next[i+1:]...))
}

func (d *capDoc) canUndo() bool { return len(d.undos) > 0 }
func (d *capDoc) canRedo() bool { return len(d.redos) > 0 }

func (d *capDoc) undo() {
	if !d.canUndo() {
		return
	}
	d.redos = append(d.redos, d.shapes)
	d.shapes = d.undos[len(d.undos)-1]
	d.undos = d.undos[:len(d.undos)-1]
}

func (d *capDoc) redo() {
	if !d.canRedo() {
		return
	}
	d.undos = append(d.undos, d.shapes)
	d.shapes = d.redos[len(d.redos)-1]
	d.redos = d.redos[:len(d.redos)-1]
}
