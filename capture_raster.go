package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"math"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// The rasterizer burns the annotation shapes into the captured image, in
// plain Go: thick antialiased segments, rectangles, masks, arrow heads and
// comment boxes in Go Regular (golang.org/x/image, already a dependency).

// covMask collects the coverage of one shape before it is blended, so
// that the overlapping segments of a polyline do not darken their joints.
type covMask struct {
	r image.Rectangle
	a []float32
}

func newCovMask(r image.Rectangle) *covMask {
	return &covMask{r: r, a: make([]float32, r.Dx()*r.Dy())}
}

func (m *covMask) set(x, y int, c float32) {
	if !(image.Point{x, y}).In(m.r) {
		return
	}
	i := (y-m.r.Min.Y)*m.r.Dx() + (x - m.r.Min.X)
	if c > m.a[i] {
		m.a[i] = c
	}
}

// blendInto paints col over dst where the mask covers.
func (m *covMask) blendInto(dst *image.RGBA, col color.RGBA) {
	for y := m.r.Min.Y; y < m.r.Max.Y; y++ {
		for x := m.r.Min.X; x < m.r.Max.X; x++ {
			c := m.a[(y-m.r.Min.Y)*m.r.Dx()+(x-m.r.Min.X)]
			if c > 0 {
				blendPixel(dst, x, y, col, c)
			}
		}
	}
}

func blendPixel(dst *image.RGBA, x, y int, col color.RGBA, cov float32) {
	if !(image.Point{x, y}).In(dst.Rect) {
		return
	}
	a := cov * float32(col.A) / 255
	i := dst.PixOffset(x, y)
	p := dst.Pix[i : i+4 : i+4]
	mix := func(d, s uint8) uint8 { return uint8(float32(d)*(1-a) + float32(s)*a + 0.5) }
	p[0] = mix(p[0], col.R)
	p[1] = mix(p[1], col.G)
	p[2] = mix(p[2], col.B)
	p[3] = mix(p[3], 255)
}

// pixelBox is the pixels r touches, clipped to bounds.
func pixelBox(r frect, bounds image.Rectangle) image.Rectangle {
	return image.Rect(int(math.Floor(r.X0)), int(math.Floor(r.Y0)), int(math.Ceil(r.X1))+1, int(math.Ceil(r.Y1))+1).Intersect(bounds)
}

// strokeSegment covers the segment a–b, w pixels thick, with round ends.
func strokeSegment(m *covMask, a, b fpt, w float64) {
	box := pixelBox(rectOf(a, b).inset(w/2+1), m.r)
	for y := box.Min.Y; y < box.Max.Y; y++ {
		for x := box.Min.X; x < box.Max.X; x++ {
			d := segDist(fpt{float64(x) + 0.5, float64(y) + 0.5}, a, b)
			if c := w/2 + 0.5 - d; c > 0 {
				m.set(x, y, float32(math.Min(1, c)))
			}
		}
	}
}

// fillTriangle covers the triangle, sampled 4×4 a pixel.
func fillTriangle(m *covMask, p0, p1, p2 fpt) {
	box := pixelBox(rectOf(p0, p1).union(rectOf(p2, p2)), m.r)
	edge := func(a, b, p fpt) float64 { return (b.X-a.X)*(p.Y-a.Y) - (b.Y-a.Y)*(p.X-a.X) }
	inside := func(p fpt) bool {
		e0, e1, e2 := edge(p0, p1, p), edge(p1, p2, p), edge(p2, p0, p)
		return (e0 >= 0 && e1 >= 0 && e2 >= 0) || (e0 <= 0 && e1 <= 0 && e2 <= 0)
	}
	for y := box.Min.Y; y < box.Max.Y; y++ {
		for x := box.Min.X; x < box.Max.X; x++ {
			n := 0
			for sy := 0; sy < 4; sy++ {
				for sx := 0; sx < 4; sx++ {
					if inside(fpt{float64(x) + (float64(sx)+0.5)/4, float64(y) + (float64(sy)+0.5)/4}) {
						n++
					}
				}
			}
			if n > 0 {
				m.set(x, y, float32(n)/16)
			}
		}
	}
}

// arrowGeometry returns the arrow head's triangle (its tip at head) and
// where the shaft meets it, for an arrow w pixels thick.
func arrowGeometry(head, tail fpt, w float64) (tip, left, right, neck fpt) {
	dx, dy := head.X-tail.X, head.Y-tail.Y
	l := math.Hypot(dx, dy)
	if l == 0 {
		return head, head, head, head
	}
	ux, uy := dx/l, dy/l
	size := math.Min(math.Max(12, w*5), l)
	half := size * 0.45
	base := fpt{head.X - ux*size, head.Y - uy*size}
	left = fpt{base.X - uy*half, base.Y + ux*half}
	right = fpt{base.X + uy*half, base.Y - ux*half}
	neck = fpt{head.X - ux*size*0.7, head.Y - uy*size*0.7}
	return head, left, right, neck
}

var (
	capFontOnce sync.Once
	capFont     *opentype.Font
	capFaces    = map[int]font.Face{}
	capFacesMu  sync.Mutex
)

// capFace is Go Regular at size pixels, or nil if it fails to load.
func capFace(size float64) font.Face {
	capFontOnce.Do(func() { capFont, _ = opentype.Parse(goregular.TTF) })
	if capFont == nil {
		return nil
	}
	px := int(math.Round(size))
	if px < 6 {
		px = 6
	}
	capFacesMu.Lock()
	defer capFacesMu.Unlock()
	if f, ok := capFaces[px]; ok {
		return f
	}
	f, err := opentype.NewFace(capFont, &opentype.FaceOptions{Size: float64(px), DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return nil
	}
	capFaces[px] = f
	return f
}

// capMeasure measures a comment in pixels: the widest line, and the
// lines' height.
func capMeasure(text string, size float64) (w, h float64) {
	f := capFace(size)
	lines := strings.Split(text, "\n")
	if f == nil {
		return float64(len(text)) * size * 0.6, float64(len(lines)) * size * 1.2
	}
	for _, l := range lines {
		w = math.Max(w, float64(font.MeasureString(f, l))/64)
	}
	m := f.Metrics()
	return math.Ceil(w), float64(len(lines)) * float64(m.Height) / 64
}

// drawComment draws an arrow's comment: a white box framed in its colour
// with the text in it.
func drawComment(dst *image.RGBA, s capShape) {
	box, ok := s.commentBox(capMeasure)
	if !ok {
		return
	}
	r := pixelBox(frect{box.X0, box.Y0, box.X1 - 1, box.Y1 - 1}, dst.Rect)
	draw.Draw(dst, r, image.NewUniform(color.White), image.Point{}, draw.Src)
	edge := math.Max(1, s.Width/2)
	m := newCovMask(r)
	c := []fpt{{box.X0, box.Y0}, {box.X1, box.Y0}, {box.X1, box.Y1}, {box.X0, box.Y1}}
	for i := range c {
		strokeSegment(m, c[i], c[(i+1)%4], edge)
	}
	m.blendInto(dst, s.Color)
	f := capFace(s.FontSize)
	if f == nil {
		return
	}
	pad := s.FontSize * 0.3
	met := f.Metrics()
	d := font.Drawer{Dst: dst, Src: image.NewUniform(s.Color), Face: f}
	for i, l := range strings.Split(s.Text, "\n") {
		y := box.Y0 + pad + float64(met.Ascent)/64 + float64(i)*float64(met.Height)/64
		d.Dot = fixed.Point26_6{X: fixed.Int26_6((box.X0 + pad) * 64), Y: fixed.Int26_6(y * 64)}
		d.DrawString(l)
	}
}

// drawShape burns s into dst.
func drawShape(dst *image.RGBA, s capShape) {
	if len(s.Pts) == 0 {
		return
	}
	box := pixelBox(s.bounds().inset(s.Width+2), dst.Rect)
	m := newCovMask(box)
	first, last := s.Pts[0], s.Pts[len(s.Pts)-1]
	switch s.Kind {
	case toolMask:
		r := rectOf(first, last)
		for y := box.Min.Y; y < box.Max.Y; y++ {
			for x := box.Min.X; x < box.Max.X; x++ {
				if r.contains(fpt{float64(x) + 0.5, float64(y) + 0.5}) {
					m.set(x, y, 1)
				}
			}
		}
	case toolRect:
		r := rectOf(first, last)
		c := []fpt{{r.X0, r.Y0}, {r.X1, r.Y0}, {r.X1, r.Y1}, {r.X0, r.Y1}}
		for i := range c {
			strokeSegment(m, c[i], c[(i+1)%4], s.Width)
		}
	case toolArrow:
		tip, l, r, neck := arrowGeometry(first, last, s.Width)
		strokeSegment(m, last, neck, s.Width)
		fillTriangle(m, tip, l, r)
	default: // Pen, Line
		if len(s.Pts) == 1 {
			strokeSegment(m, first, first, s.Width)
		}
		for i := 1; i < len(s.Pts); i++ {
			strokeSegment(m, s.Pts[i-1], s.Pts[i], s.Width)
		}
	}
	m.blendInto(dst, s.Color)
	if s.Kind == toolArrow {
		drawComment(dst, s)
	}
}

// renderCapture returns base with the shapes burnt in.
func renderCapture(base image.Image, shapes []capShape) *image.RGBA {
	b := base.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Rect, base, b.Min, draw.Src)
	for _, s := range shapes {
		drawShape(out, s)
	}
	return out
}

// encodeCapture encodes img as "png" or "jpg".
func encodeCapture(img image.Image, format string) ([]byte, error) {
	var buf bytes.Buffer
	switch normFormat(format) {
	case "jpg":
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
			return nil, err
		}
	case "png":
		if err := png.Encode(&buf, img); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unknown image format %q", format)
	}
	return buf.Bytes(), nil
}

// normFormat maps "PNG", "jpeg" and the like to "png" or "jpg".
func normFormat(f string) string {
	switch strings.ToLower(strings.TrimPrefix(f, ".")) {
	case "jpg", "jpeg":
		return "jpg"
	case "png", "":
		return "png"
	}
	return strings.ToLower(f)
}
