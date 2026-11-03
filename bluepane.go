package main

import (
	"runtime"

	"github.com/egoist/mygo/ui"
)

// bpPalette holds the Bluepane design tokens the windows use, for one
// appearance. The names follow the tokens: face is --face, fieldEdge is
// --field-edge and so on.
type bpPalette struct {
	ink, muted, inverse, inkDisabled               ui.Color
	chrome, face, bar, field, fieldEdge, fieldOff  ui.Color
	cellEdge, groupEdge, infoEdge                  ui.Color
	sel, hl, focus                                 ui.Color
	btn, btnEdge, btnHover, btnPress, btnDefEdge   ui.Color
	alert, alertInk, mark, star, nav, desk, banner ui.Color
	bevelHi, bevelLo                               ui.Color
}

var bpLight = &bpPalette{
	ink: ui.Hex("#000000"), muted: ui.Hex("#404040"), inverse: ui.Hex("#ffffff"), inkDisabled: ui.Hex("#6d6d6d"),
	chrome: ui.Hex("#ffffff"), face: ui.Hex("#f0f0f0"), bar: ui.Hex("#c8c8c8"), field: ui.Hex("#ffffff"),
	fieldEdge: ui.Hex("#7a7a7a"), fieldOff: ui.Hex("#f0f0f0"),
	cellEdge: ui.Hex("#999999"), groupEdge: ui.Hex("#dcdcdc"), infoEdge: ui.Hex("#b4b4b4"),
	sel: ui.Hex("#3d3dd6"), hl: ui.Hex("#0077d4"), focus: ui.Hex("#000000"),
	btn: ui.Hex("#e1e1e1"), btnEdge: ui.Hex("#adadad"), btnHover: ui.Hex("#e5f1fb"), btnPress: ui.Hex("#cce4f7"), btnDefEdge: ui.Hex("#0077d4"),
	alert: ui.Hex("#ee2222"), alertInk: ui.Hex("#c41010"), mark: ui.Hex("#99ff99"), star: ui.Hex("#e6b800"),
	nav: ui.Hex("#7b7bff"), desk: ui.Hex("#000080"), banner: ui.Hex("#95b9f9"),
	bevelHi: ui.Hex("#ffffff"), bevelLo: ui.Hex("#808080"),
}

var bpDark = &bpPalette{
	ink: ui.Hex("#e8ecf7"), muted: ui.Hex("#a9b4cc"), inverse: ui.Hex("#ffffff"), inkDisabled: ui.Hex("#8a94ad"),
	chrome: ui.Hex("#1c2233"), face: ui.Hex("#262c3c"), bar: ui.Hex("#323a52"), field: ui.Hex("#0f1526"),
	fieldEdge: ui.Hex("#6b7591"), fieldOff: ui.Hex("#1c2233"),
	cellEdge: ui.Hex("#4a5577"), groupEdge: ui.Hex("#48526f"), infoEdge: ui.Hex("#4a5577"),
	sel: ui.Hex("#3d3dd6"), hl: ui.Hex("#2f6fd0"), focus: ui.Hex("#ffffff"),
	btn: ui.Hex("#323a52"), btnEdge: ui.Hex("#6b7591"), btnHover: ui.Hex("#2b3a5c"), btnPress: ui.Hex("#22314f"), btnDefEdge: ui.Hex("#4a8ae8"),
	alert: ui.Hex("#ff6b6b"), alertInk: ui.Hex("#ff6b6b"), mark: ui.Hex("#7fe07f"), star: ui.Hex("#ffd23f"),
	nav: ui.Hex("#3d3dd6"), desk: ui.Hex("#05082a"), banner: ui.Hex("#24376e"),
	bevelHi: ui.Hex("#48526f"), bevelLo: ui.Hex("#05070f"),
}

// bpOf returns the palette of the window's appearance.
func bpOf(c *ui.Context) *bpPalette {
	if c.Theme().Dark {
		return bpDark
	}
	return bpLight
}

// The font stacks of the tokens: --font-ui and --font-mono. Tahoma and
// Courier New ship with macOS and Windows; elsewhere the system's fonts
// stand in.
func bpUIFont() string {
	switch runtime.GOOS {
	case "windows":
		return "Segoe UI"
	case "darwin":
		return "Tahoma"
	}
	return ""
}

func bpMonoFont() string {
	switch runtime.GOOS {
	case "windows", "darwin":
		return "Courier New"
	}
	return "monospace"
}

// bpThemes caches the themes bpApply sets.
var bpThemes = map[bpThemeKey]*ui.Theme{}

type bpThemeKey struct {
	dark, dialog bool
	ground       ui.Color
}

// bpApply gives the window the Bluepane theme on the given ground: chrome
// for the message windows, face for dialogs. Corners are square, depth
// comes from bevels, and selections are the token blue: --select in the
// message windows, the system's --hl in dialogs.
func bpApply(c *ui.Context, ground func(*bpPalette) ui.Color, dialog bool) *bpPalette {
	p := bpOf(c)
	g := ground(p)
	key := bpThemeKey{p == bpDark, dialog, g}
	t := bpThemes[key]
	if t == nil {
		if p == bpDark {
			t = ui.DarkTheme()
		} else {
			t = ui.LightTheme()
		}
		t.Background = g
		t.Surface = p.field
		t.SurfaceHover = p.btnHover
		t.SurfacePressed = p.btnPress
		t.Border = p.fieldEdge
		t.Text, t.TextMuted = p.ink, p.muted
		t.Accent, t.AccentHover, t.AccentPressed, t.AccentText = p.sel, p.hl, p.sel, p.inverse
		if dialog {
			t.Accent, t.AccentPressed = p.hl, p.hl
		}
		t.Danger = p.alertInk
		t.Focus = p.focus
		t.Radius = 0
		t.Spacing = 3
		t.FontSize = 13
		t.Font = bpUIFont()
		bpThemes[key] = t
	}
	c.SetTheme(t)
	return p
}

func groundChrome(p *bpPalette) ui.Color { return p.chrome }
func groundFace(p *bpPalette) ui.Color   { return p.face }

// bpBevelOut raises e as --bevel-out does; bpBevelIn sinks it.
func bpBevelOut(e ui.Element, p *bpPalette) ui.Element {
	return e.InsetShadow(1, 1, 0, 0, p.bevelHi).InsetShadow(-1, -1, 0, 0, p.bevelLo)
}

func bpBevelIn(e ui.Element, p *bpPalette) ui.Element {
	return e.InsetShadow(1, 1, 0, 0, p.bevelLo).InsetShadow(-1, -1, 0, 0, p.bevelHi)
}

// bpButton is the classic push button of the message windows (bp-btn): a
// grey face with a raised bevel that sinks while pressed.
func bpButton(c *ui.Context, label string) ui.Element {
	b := bpButtonBase(c, label)
	b.Children(func() { ui.Text(c, label).SingleLine() })
	return b
}

// bpBoldButton is bpButton with a bold label, the window's main action.
func bpBoldButton(c *ui.Context, label string) ui.Element {
	b := bpButtonBase(c, label)
	b.Children(func() { ui.Text(c, label).Bold().SingleLine() })
	return b
}

// bpIconButton is a bpButton showing an icon, named label.
func bpIconButton(c *ui.Context, icon *ui.SVG, label string) ui.Element {
	b := bpButtonBase(c, label).MinWidth(0).Padding(4, 6)
	b.Children(func() { ui.Icon(c, icon).Size(16, 16) })
	return b.Tooltip(label)
}

func bpButtonBase(c *ui.Context, label string) ui.Element {
	p := bpOf(c)
	b := ui.ButtonBase(c).Label(label).MinWidth(72).Padding(3, 12).Gap(4).
		Background(p.face).Border(1, p.fieldEdge).Radius(0).TextColor(p.ink)
	if b.Pressed() {
		bpBevelIn(b, p)
	} else {
		bpBevelOut(b, p)
	}
	return b
}

// bpDlgButton is the flat button of settings dialogs (bp-dlgbtn); the
// default one, which Enter chooses, has a blue edge.
func bpDlgButton(c *ui.Context, label string, isDefault bool) ui.Element {
	p := bpOf(c)
	b := ui.ButtonBase(c).Label(label).MinWidth(88).Height(28).Padding(0, 12).
		Background(p.btn).Border(1, p.btnEdge).Radius(0).TextColor(p.ink).FontSize(14)
	switch {
	case b.IsDisabled():
	case b.Pressed():
		b.Background(p.btnPress).Border(1, p.btnDefEdge)
	case b.Hovered() || isDefault:
		if b.Hovered() {
			b.Background(p.btnHover)
		}
		b.Border(1, p.btnDefEdge)
	}
	b.Children(func() { ui.Text(c, label).SingleLine() })
	return b
}

// bpMiniButton is the small dialog button of lists (bp-dlgbtn bp-mini).
func bpMiniButton(c *ui.Context, label string) ui.Element {
	return bpDlgButton(c, label, false).MinWidth(0).Height(22).Padding(0, 6).FontSize(12)
}

// bpField sinks an input or a list into the window, as --bevel-in edges
// a field.
func bpField(c *ui.Context, e ui.Element) ui.Element {
	p := bpOf(c)
	return bpBevelIn(e.Background(p.field).Border(1, p.fieldEdge).Radius(0), p)
}

// bpGroup is a group box (bp-group): a faint frame whose legend sits in
// its top edge, as an HTML fieldset draws it.
func bpGroup(c *ui.Context, legend string, fn func()) ui.Element {
	p := bpOf(c)
	ground := c.Theme().Background
	return ui.Column(c).Margin(8, 0, 0, 0).Padding(14, 12, 12, 12).Gap(6).
		Border(1, p.groupEdge).Radius(0).Children(func() {
		ui.Text(c, legend).FontSize(14).SingleLine().
			Absolute().Top(-9).Left(8).Padding(0, 3).Background(ground)
		fn()
	})
}

// bpInfo is the explanation that leads a pane (bp-info): a face box with
// a shaded top and left edge.
func bpInfo(c *ui.Context, text string) ui.Element {
	p := bpOf(c)
	return ui.Text(c, text).Wrap().Padding(2, 4).Background(p.face).
		BorderWidth(1, 0, 0, 1).BorderColor(p.infoEdge)
}

// bpStatusBar is the strip of monospaced counters along a window's
// bottom (bp-status).
func bpStatusBar(c *ui.Context, fn func()) ui.Element {
	p := bpOf(c)
	return ui.Row(c).Gap(16).Padding(2, 8).AlignItems(ui.Center).
		Background(p.face).BorderWidth(1, 0, 0, 0).BorderColor(p.cellEdge).
		Font(bpMonoFont()).FontSize(13).TextColor(p.muted).Children(fn)
}

// Icons drawn after the mock's inline SVG glyphs.
var (
	iconRefresh = ui.MustParseSVG([]byte(`<svg width="16" height="16" viewBox="0 0 16 16"><path d="M13 8a5 5 0 1 1-1.6-3.7" fill="none" stroke="currentColor" stroke-width="2"/><path d="M13.5 1.5v3.8H9.7" fill="none" stroke="currentColor" stroke-width="2"/></svg>`))
	iconMenu    = ui.MustParseSVG([]byte(`<svg width="10" height="6" viewBox="0 0 10 6"><path d="M0 0h10L5 6z" fill="currentColor"/></svg>`))
	iconCapture = ui.MustParseSVG([]byte(`<svg width="16" height="14" viewBox="0 0 16 14"><rect x="1" y="3" width="14" height="10" fill="none" stroke="currentColor" stroke-width="1.4"/><rect x="5" y="1" width="6" height="2" fill="currentColor"/><circle cx="8" cy="8" r="2.6" fill="none" stroke="currentColor" stroke-width="1.4"/></svg>`))
	iconAttach  = ui.MustParseSVG([]byte(`<svg width="16" height="16" viewBox="0 0 16 16"><path d="M11 4.5 5.8 9.7a1.4 1.4 0 0 0 2 2l5.4-5.4a2.8 2.8 0 0 0-4-4L3.8 7.7a4.2 4.2 0 0 0 6 6L14 9.5" fill="none" stroke="currentColor" stroke-width="1.4"/></svg>`))
	iconLog     = ui.MustParseSVG([]byte(`<svg width="12" height="14" viewBox="0 0 12 14"><rect x="1" y="1" width="10" height="12" fill="none" stroke="currentColor" stroke-width="1.3"/><path d="M3 5h6M3 8h6M3 11h4" stroke="currentColor" stroke-width="1.2"/></svg>`))
)

// bpToggle is a bevelled button that stays pressed in while *on.
func bpToggle(c *ui.Context, on *bool, label string) ui.Element {
	p := bpOf(c)
	b := ui.ButtonBase(c).Label(label).Padding(2, 8).Gap(4).
		Background(p.face).Border(1, p.fieldEdge).Radius(0).TextColor(p.ink)
	b.Role(ui.RoleToggleButton).Checked(*on)
	if b.Clicked() {
		*on = !*on
		b.SetChanged()
	}
	if *on || b.Pressed() {
		bpBevelIn(b.Background(p.btnPress), p)
	} else {
		bpBevelOut(b, p)
	}
	b.Children(func() { ui.Text(c, label).SingleLine() })
	return b
}

// bpChip is a bevelled bold name tag (bp-chip), as the log's heads show.
func bpChip(c *ui.Context, text string) ui.Element {
	p := bpOf(c)
	return bpBevelOut(ui.Text(c, text).Bold().FontSize(12).SingleLine().Padding(2, 8).
		Background(p.face).Border(1, p.fieldEdge), p)
}
