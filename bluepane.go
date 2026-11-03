package main

import (
	"runtime"
	"strings"

	"github.com/egoist/mygo/ui"
)

// bpPalette holds the Bluepane design tokens the windows use, for one
// appearance. The names follow the tokens: face is --face, fieldEdge is
// --field-edge and so on.
type bpPalette struct {
	ink, muted, inverse, inkDisabled                 ui.Color
	chrome, face, bar, field, fieldEdge, fieldOff    ui.Color
	cellEdge, groupEdge, infoEdge                    ui.Color
	sel, hl, focus                                   ui.Color
	btn, btnEdge, btnHover, btnPress, btnDefEdge     ui.Color
	alert, alertInk, mark, markInk, hit, star        ui.Color
	note, noteEdge, link, nav, desk, banner          ui.Color
	bevelHi, bevelLo                                 ui.Color
	helpGround, helpChapter, helpTitleInk            ui.Color
	helpHead, helpCell, helpEdge                     ui.Color
	helpNotice, helpNoticeEdge, helpHeadlineEdge     ui.Color
	helpFeature, helpFeatureInk                      ui.Color
	helpLicense, helpLicenseEdge, go_, goInk, goEdge ui.Color
}

var bpLight = &bpPalette{
	ink: ui.Hex("#000000"), muted: ui.Hex("#404040"), inverse: ui.Hex("#ffffff"), inkDisabled: ui.Hex("#6d6d6d"),
	chrome: ui.Hex("#ffffff"), face: ui.Hex("#f0f0f0"), bar: ui.Hex("#c8c8c8"), field: ui.Hex("#ffffff"),
	fieldEdge: ui.Hex("#7a7a7a"), fieldOff: ui.Hex("#f0f0f0"),
	cellEdge: ui.Hex("#999999"), groupEdge: ui.Hex("#dcdcdc"), infoEdge: ui.Hex("#b4b4b4"),
	sel: ui.Hex("#3d3dd6"), hl: ui.Hex("#0077d4"), focus: ui.Hex("#000000"),
	btn: ui.Hex("#e1e1e1"), btnEdge: ui.Hex("#adadad"), btnHover: ui.Hex("#e5f1fb"), btnPress: ui.Hex("#cce4f7"), btnDefEdge: ui.Hex("#0077d4"),
	alert: ui.Hex("#ee2222"), alertInk: ui.Hex("#c41010"), mark: ui.Hex("#99ff99"), markInk: ui.Hex("#000000"), hit: ui.Hex("#ff8080"), star: ui.Hex("#e6b800"),
	note: ui.Hex("#e8fbe8"), noteEdge: ui.Hex("#8fd88f"), link: ui.Hex("#0000ee"),
	nav: ui.Hex("#7b7bff"), desk: ui.Hex("#000080"), banner: ui.Hex("#95b9f9"),
	bevelHi: ui.Hex("#ffffff"), bevelLo: ui.Hex("#808080"),
	helpGround: ui.Hex("#eeffff"), helpChapter: ui.Hex("#0000cc"), helpTitleInk: ui.Hex("#ff5555"),
	helpHead: ui.Hex("#aaffaa"), helpCell: ui.Hex("#ddffdd"), helpEdge: ui.Hex("#aaaaaa"),
	helpNotice: ui.Hex("#bbffbb"), helpNoticeEdge: ui.Hex("#77ff77"), helpHeadlineEdge: ui.Hex("#88cc88"),
	helpFeature: ui.Hex("#ccffcc"), helpFeatureInk: ui.Hex("#333333"),
	helpLicense: ui.Hex("#dddddd"), helpLicenseEdge: ui.Hex("#777777"),
	go_: ui.Hex("#2f7a22"), goInk: ui.Hex("#ffffff"), goEdge: ui.Hex("#1f5416"),
}

var bpDark = &bpPalette{
	ink: ui.Hex("#e8ecf7"), muted: ui.Hex("#a9b4cc"), inverse: ui.Hex("#ffffff"), inkDisabled: ui.Hex("#8a94ad"),
	chrome: ui.Hex("#1c2233"), face: ui.Hex("#262c3c"), bar: ui.Hex("#323a52"), field: ui.Hex("#0f1526"),
	fieldEdge: ui.Hex("#6b7591"), fieldOff: ui.Hex("#1c2233"),
	cellEdge: ui.Hex("#4a5577"), groupEdge: ui.Hex("#48526f"), infoEdge: ui.Hex("#4a5577"),
	sel: ui.Hex("#3d3dd6"), hl: ui.Hex("#2f6fd0"), focus: ui.Hex("#ffffff"),
	btn: ui.Hex("#323a52"), btnEdge: ui.Hex("#6b7591"), btnHover: ui.Hex("#2b3a5c"), btnPress: ui.Hex("#22314f"), btnDefEdge: ui.Hex("#4a8ae8"),
	alert: ui.Hex("#ff6b6b"), alertInk: ui.Hex("#ff6b6b"), mark: ui.Hex("#7fe07f"), markInk: ui.Hex("#0c1222"), hit: ui.Hex("#ff8080"), star: ui.Hex("#ffd23f"),
	note: ui.Hex("#132a1a"), noteEdge: ui.Hex("#3f8f4a"), link: ui.Hex("#9fb4ff"),
	nav: ui.Hex("#3d3dd6"), desk: ui.Hex("#05082a"), banner: ui.Hex("#24376e"),
	bevelHi: ui.Hex("#48526f"), bevelLo: ui.Hex("#05070f"),
	helpGround: ui.Hex("#0c1a1f"), helpChapter: ui.Hex("#2a2ad0"), helpTitleInk: ui.Hex("#ff8a8a"),
	helpHead: ui.Hex("#1f5a2a"), helpCell: ui.Hex("#12301a"), helpEdge: ui.Hex("#5f6f80"),
	helpNotice: ui.Hex("#15401f"), helpNoticeEdge: ui.Hex("#3fae4f"), helpHeadlineEdge: ui.Hex("#4f8f4f"),
	helpFeature: ui.Hex("#12301a"), helpFeatureInk: ui.Hex("#d8e2d8"),
	helpLicense: ui.Hex("#2a2f3a"), helpLicenseEdge: ui.Hex("#8a8f99"),
	go_: ui.Hex("#2f7a22"), goInk: ui.Hex("#ffffff"), goEdge: ui.Hex("#5fb34f"),
}

// bpOf returns the palette of the window's appearance.
func bpOf(c *ui.Context) *bpPalette {
	if c.Theme().Dark {
		return bpDark
	}
	return bpLight
}

// The spacing scale of the tokens: --space-1 to --space-8.
const (
	sp1 = 2
	sp2 = 4
	sp3 = 8
	sp4 = 12
	sp5 = 16
	sp6 = 24
	sp8 = 32
)

// The font stacks of the tokens: --font-ui, --font-mono and --font-serif.
// Tahoma, Courier New and Times New Roman ship with macOS and Windows;
// elsewhere the system's fonts stand in.
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

func bpSerifFont() string {
	switch runtime.GOOS {
	case "windows", "darwin":
		return "Times New Roman"
	}
	return "serif"
}

// bpThemes caches the themes bpApply sets.
var bpThemes = map[bpThemeKey]*ui.Theme{}

type bpThemeKey struct {
	dark, dialog bool
	ground       ui.Color
}

// bpApply gives the window the Bluepane theme on the given ground: chrome
// for the message windows (win-body, 13px), face for dialogs (dlg-body,
// 14px). Corners are square, depth comes from bevels, and selections are
// the token blue: --select in the message windows, the system's --hl in
// dialogs.
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
		t.Accent, t.AccentHover, t.AccentPressed, t.AccentText = p.sel, p.sel, p.sel, p.inverse
		t.FontSize = 13
		if dialog {
			t.Accent, t.AccentHover, t.AccentPressed = p.hl, p.hl, p.hl
			t.FontSize = 14
		}
		t.Selection = p.hl
		t.Danger = p.alertInk
		t.Focus = p.focus
		t.Scrollbar = p.fieldEdge
		t.Radius = 0
		t.Spacing = 3
		t.Font = bpUIFont()
		bpThemes[key] = t
	}
	c.SetTheme(t)
	return p
}

func groundChrome(p *bpPalette) ui.Color { return p.chrome }
func groundFace(p *bpPalette) ui.Color   { return p.face }

// bpRadioTheme is the dialog theme with the radio dots in ink, as
// bp-check radios set accent-color to --ink.
func bpRadioTheme(c *ui.Context) *ui.Theme {
	p := bpOf(c)
	t := *c.Theme()
	t.Accent, t.AccentHover, t.AccentPressed, t.AccentText = p.ink, p.ink, p.ink, p.field
	return &t
}

// bpBevelOut raises e as --bevel-out does; bpBevelIn sinks it.
func bpBevelOut(e ui.Element, p *bpPalette) ui.Element {
	return e.InsetShadow(1, 1, 0, 0, p.bevelHi).InsetShadow(-1, -1, 0, 0, p.bevelLo)
}

func bpBevelIn(e ui.Element, p *bpPalette) ui.Element {
	return e.InsetShadow(1, 1, 0, 0, p.bevelLo).InsetShadow(-1, -1, 0, 0, p.bevelHi)
}

// ---------- Window rows: menubar, toolbar, status ----------

// bpMenu is a menu of the window's menu bar: its title carries the access
// key in parentheses, "File(F)", and Alt with the key opens it.
type bpMenu struct {
	Title string
	Build func(m *ui.Menu)
}

// bpMenubar is the text menu bar under the title bar (bp-menubar): chrome,
// win-title 12px, the titles 16px apart; hovered titles underline.
func bpMenubar(c *ui.Context, menus ...bpMenu) {
	p := bpOf(c)
	ui.Row(c).Gap(sp5).Padding(sp1, sp3).Background(p.chrome).Children(func() {
		for i, m := range menus {
			b := ui.ButtonBase(c.Key(i)).Label(strings.SplitN(m.Title, "(", 2)[0]).FontSize(12).TextColor(p.ink)
			b.Menu(m.Build)
			if k := bpAccessKey(m.Title); k != 0 && c.Shortcut(ui.Alt, k) {
				b.Focus()
			}
			b.Children(func() {
				t := ui.Text(c, m.Title).SingleLine()
				if b.Hovered() || b.Pressed() {
					t.Underline()
				}
			})
		}
	})
}

// bpAccessKey is the key in the parentheses of "File(F)".
func bpAccessKey(title string) ui.Key {
	i := strings.LastIndexByte(title, '(')
	if i < 0 || i+2 >= len(title) {
		return 0
	}
	ch := title[i+1]
	if ch >= 'a' && ch <= 'z' {
		ch -= 'a' - 'A'
	}
	if ch < 'A' || ch > 'Z' {
		return 0
	}
	return ui.KeyA + ui.Key(ch-'A')
}

// bpToolbar is the grey strip of controls under the menu bar (bp-toolbar):
// face, with cell-edge hairlines above and below.
func bpToolbar(c *ui.Context, fn func()) ui.Element {
	p := bpOf(c)
	return ui.Row(c).Gap(sp2).Padding(sp2, sp3).AlignItems(ui.Center).Background(p.face).
		BorderWidth(1, 0, 1, 0).BorderColor(p.cellEdge).Children(fn)
}

// bpSep is the hairline between groups of toolbar controls (bp-sep).
func bpSep(c *ui.Context) ui.Element {
	return ui.Box(c).Width(1).AlignSelf(ui.Stretch).Margin(0, sp2).Background(bpOf(c).cellEdge)
}

// bpLabel is a bold 12px label (bp-label, win-label): counters in
// toolbars, field names over controls.
func bpLabel(c *ui.Context, s string) ui.Element {
	return ui.Text(c, s).FontSize(12).Bold().SingleLine().TextColor(bpOf(c).ink)
}

// bpStatusBar is the strip of monospaced counters along a window's bottom
// (bp-status), set to its end, 16px apart.
func bpStatusBar(c *ui.Context, fn func()) ui.Element {
	p := bpOf(c)
	return ui.Row(c).Gap(sp5).Padding(sp1, sp3).AlignItems(ui.Center).Justify(ui.End).
		Background(p.face).BorderWidth(1, 0, 0, 0).BorderColor(p.cellEdge).
		Font(bpMonoFont()).FontSize(13).TextColor(p.muted).Children(fn)
}

// ---------- Classic buttons (message windows) ----------

// bpButton is the classic push button of the message windows (bp-btn): a
// face with a raised bevel that sinks while pressed.
func bpButton(c *ui.Context, label string) ui.Element {
	b := bpButtonBase(c, label)
	b.Children(func() { ui.Text(c, label).SingleLine() })
	return b
}

// bpIconButton is a square bp-btn bp-icon showing an icon, named label.
func bpIconButton(c *ui.Context, icon *ui.SVG, label string) ui.Element {
	b := bpButtonBase(c, label).MinWidth(0).Padding(sp2)
	b.Children(func() { ui.Icon(c, icon).Size(16, 16) })
	return b.Tooltip(label)
}

func bpButtonBase(c *ui.Context, label string) ui.Element {
	p := bpOf(c)
	b := ui.ButtonBase(c).Label(label).MinWidth(72).Padding(sp2, sp4).Gap(sp2).
		Justify(ui.Center).AlignItems(ui.Center).
		Background(p.face).Border(1, p.fieldEdge).Radius(0).TextColor(p.ink).FontSize(13)
	switch {
	case b.IsDisabled():
		bpBevelOut(b.TextColor(p.muted), p)
	case b.Pressed():
		bpBevelIn(b, p)
	default:
		bpBevelOut(b, p)
	}
	return b
}

// bpToggle is a bp-btn that stays pressed in while *on
// (aria-pressed="true").
func bpToggle(c *ui.Context, on *bool, label string) ui.Element {
	return bpToggleWith(c, on, label, func() { ui.Text(c, label).SingleLine() })
}

func bpToggleWith(c *ui.Context, on *bool, label string, content func()) ui.Element {
	p := bpOf(c)
	b := ui.ButtonBase(c).Label(label).MinWidth(0).Padding(sp2, sp3).Gap(sp2).AlignItems(ui.Center).
		Background(p.face).Border(1, p.fieldEdge).Radius(0).TextColor(p.ink).FontSize(13)
	b.Role(ui.RoleToggleButton).Checked(*on)
	if b.Clicked() {
		*on = !*on
		b.SetChanged()
	}
	if *on || b.Pressed() {
		bpBevelIn(b, p)
	} else {
		bpBevelOut(b, p)
	}
	b.Children(content)
	return b
}

// bpGo is the one decisive action of a window (bp-go): the green button,
// the only one with rounded corners and a soft raise, which moves 1px
// while pressed.
func bpGo(c *ui.Context, label string) ui.Element {
	p := bpOf(c)
	b := ui.ButtonBase(c).Label(label).MinWidth(72).Padding(sp2, sp4).Gap(sp2).
		Justify(ui.Center).AlignItems(ui.Center).
		Background(p.go_).Border(1, p.goEdge).Radius(5).TextColor(p.goInk).FontSize(13).FontWeight(600)
	switch {
	case b.IsDisabled():
	case b.Pressed():
		b.Translate(1, 1)
	default:
		a := float32(0.35)
		if p == bpDark {
			a = 0.7
		}
		b.Shadow(1, 1, 2, 0, ui.RGBA(0, 0, 0, a))
	}
	b.Children(func() { ui.Text(c, label).SingleLine() })
	return b
}

// ---------- Dialog buttons ----------

// bpDlgButton is the flat button of settings dialogs (bp-dlgbtn); the
// default one, which Enter chooses, has a blue edge.
func bpDlgButton(c *ui.Context, label string, isDefault bool) ui.Element {
	b := bpDlgButtonBase(c, label, isDefault)
	b.Children(func() { ui.Text(c, label).SingleLine() })
	return b
}

func bpDlgButtonBase(c *ui.Context, label string, isDefault bool) ui.Element {
	p := bpOf(c)
	b := ui.ButtonBase(c).Label(label).MinWidth(88).Height(28).Padding(0, sp4).
		Justify(ui.Center).AlignItems(ui.Center).
		Background(p.btn).Border(1, p.btnEdge).Radius(0).TextColor(p.ink).FontSize(14)
	switch {
	case b.IsDisabled():
		b.TextColor(p.inkDisabled).Border(1, p.groupEdge)
	case b.Pressed():
		b.Background(p.btnPress).Border(1, p.btnDefEdge)
	case b.Hovered():
		b.Background(p.btnHover).Border(1, p.btnDefEdge)
	case isDefault:
		b.Border(1, p.btnDefEdge)
	}
	return b
}

// bpMiniButton is the small dialog button of lists (bp-dlgbtn bp-mini).
func bpMiniButton(c *ui.Context, label string) ui.Element {
	return bpDlgButton(c, label, false).MinWidth(0).Height(22).Padding(0, sp2).FontSize(12)
}

// bpSqButton is the square "?" of a pane's explanation (bp-dlgbtn bp-sq).
func bpSqButton(c *ui.Context, text, label string) ui.Element {
	b := bpDlgButtonBase(c, label, false).MinWidth(0).Width(28).Padding(0)
	b.Children(func() { ui.Text(c, text) })
	return b
}

// ---------- Fields and lists ----------

// bpField sinks an input or a list into a message window, as --bevel-in
// edges a field.
func bpField(c *ui.Context, e ui.Element) ui.Element {
	p := bpOf(c)
	if e.IsDisabled() {
		return e.Background(p.fieldOff).Border(1, p.fieldEdge).Radius(0).TextColor(p.inkDisabled)
	}
	return bpBevelIn(e.Background(p.field).Border(1, p.fieldEdge).Radius(0), p)
}

// bpDlgField is a field inside a dialog: flat, 14px, field-off when
// disabled or read-only.
func bpDlgField(c *ui.Context, e ui.Element) ui.Element {
	p := bpOf(c)
	e.Border(1, p.fieldEdge).Radius(0).FontSize(14)
	if e.IsDisabled() {
		return e.Background(p.fieldOff).TextColor(p.inkDisabled)
	}
	return e.Background(p.field)
}

// bpReadonly shows a fact the user may select and copy, as a read-only
// field: field-off, flat.
func bpReadonly(c *ui.Context, value string, width float32) ui.Element {
	p := bpOf(c)
	e := ui.Text(c, value).SingleLine().Selectable().Padding(sp1+1, sp2).
		Background(p.fieldOff).Border(1, p.fieldEdge).FontSize(14)
	if width > 0 {
		e.Width(width)
	}
	return e
}

// bpCol is a column of a bpList.
type bpCol struct {
	Title string
	Width float32 // 0 shares the room the others leave
	Align ui.Align
}

// bpList is the list view of the message windows (bp-list): a sunken well
// whose header row sits on face with cell-edge hairlines, rows of 12px
// text, the chosen ones in --select.
func bpList(c *ui.Context, s *ui.ListState, cols []bpCol, n int, cell func(row, col int)) ui.Element {
	p := bpOf(c)
	well := bpField(c, ui.Column(c)).FontSize(12)
	var list ui.Element
	well.Children(func() {
		head := ui.Row(c).Background(p.face).Clip().AlignItems(ui.Stretch).
			BorderWidth(0, 0, 1, 0).BorderColor(p.cellEdge)
		head.Children(func() {
			for j, col := range cols {
				h := ui.Row(c.Key(j)).Padding(sp1, sp3).BorderWidth(0, 1, 0, 0).BorderColor(p.cellEdge).Justify(col.Align)
				bpColWidth(h, col)
				h.Children(func() { ui.Text(c, col.Title).SingleLine() })
			}
		})
		list = ui.List(c, s, n, func(i int) {
			ui.Row(c).FillWidth().Children(func() {
				for j, col := range cols {
					d := ui.Row(c.Key(j)).Padding(sp1, sp3).AlignItems(ui.Center).Justify(col.Align)
					bpColWidth(d, col)
					d.Children(func() { cell(i, j) })
				}
			})
		}).Grow(1)
		head.ScrollWith(list)
	})
	return well
}

func bpColWidth(e ui.Element, col bpCol) {
	if col.Width > 0 {
		e.Width(col.Width).Shrink(0)
	} else {
		e.Grow(1).MinWidth(80)
	}
}

// bpCatList is a dialog's white list of choices (bp-catlist, the open
// list of a combo): 14px rows, the chosen one in the system --hl.
func bpCatList(c *ui.Context, s *ui.ListState, n int, row func(i int) string) ui.Element {
	p := bpOf(c)
	return ui.List(c, s, n, func(i int) {
		ui.Text(c, row(i)).SingleLine().Padding(0, sp1).FontSize(14)
	}).Padding(sp1, 0).Background(p.field).Border(1, p.fieldEdge).Radius(0)
}

// ---------- Dialog panes ----------

// bpGroup is a group box (bp-group): a faint frame whose legend sits in
// its top edge, as an HTML fieldset draws it.
func bpGroup(c *ui.Context, legend string, fn func()) ui.Element {
	p := bpOf(c)
	ground := c.Theme().Background
	return ui.Column(c).Margin(9, 0, 0, 0).Padding(sp3+6, sp4, sp4, sp4).Gap(sp4).
		Border(1, p.groupEdge).Radius(0).Children(func() {
		ui.Text(c, legend).FontSize(14).SingleLine().TextColor(p.ink).
			Absolute().Top(-10).Left(sp3).Padding(0, sp1).Background(ground)
		ui.Form(c, fn).Gap(sp4)
	})
}

// bpInfo is the explanation that leads a pane (bp-info): a face box with
// an info-edge top and left, which reads as cut into the group.
func bpInfo(c *ui.Context, text string) ui.Element {
	p := bpOf(c)
	return ui.Text(c, text).Wrap().Grow(1).Padding(sp1, sp2).Background(p.face).FontSize(14).
		TextColor(p.ink).BorderWidth(1, 0, 0, 1).BorderColor(p.infoEdge)
}

// bpInfoBox is a bp-info framed on every side, for a longer explanation
// further down a pane.
func bpInfoBox(c *ui.Context, text string) ui.Element {
	return bpInfo(c, text).BorderWidth(1, 1, 1, 1)
}

// bpLead is the row every pane starts with: its explanation and a square
// "?" that opens the help at topic.
func bpLead(c *ui.Context, text, topic string) {
	ui.Row(c).Gap(sp3).AlignItems(ui.Start).Children(func() {
		bpInfo(c, text)
		if bpSqButton(c, "?", "Help on this page").Height(28).Tooltip("Help on this page").Clicked() {
			openHelp(topic)
		}
	})
}

// bpRow is a dialog row (bp-row): a label and controls, 8px apart;
// labels end without colons. In a group the labels line up, and a click
// on one focuses its control.
func bpRow(c *ui.Context, label string, fn func()) ui.Element {
	return ui.Field(c, label, func() {
		ui.Row(c).Gap(sp3).AlignItems(ui.Center).Children(fn)
	})
}

// bpIndent indents options that depend on the one above (bp-indent).
func bpIndent(c *ui.Context, fn func()) ui.Element {
	return ui.Column(c).PaddingStart(sp8).Gap(sp4).Children(fn)
}

// bpRadios builds radio buttons with dots in ink, as dialogs draw them.
func bpRadios(c *ui.Context, fn func()) ui.Element {
	return ui.Themed(c, bpRadioTheme(c)).Background(c.Theme().Background).Gap(sp4).Children(fn)
}

// ---------- Log vocabulary ----------

// bpChip is a bevelled bold name tag (bp-chip), as the log's heads show.
func bpChip(c *ui.Context, text string) ui.Element {
	p := bpOf(c)
	return bpBevelOut(ui.Text(c, text).Bold().FontSize(12).SingleLine().TextColor(p.ink).Padding(sp1, sp3).
		Background(p.face).Border(1, p.fieldEdge).Radius(0), p)
}

// bpNote is the pale green reference note (bp-note), the one box with
// rounded corners besides the go button.
func bpNote(c *ui.Context, text string) ui.Element {
	p := bpOf(c)
	return ui.Text(c, text).Padding(sp2, sp4).Background(p.note).Border(1, p.noteEdge).Radius(3).
		TextColor(p.ink).AlignSelf(ui.Start)
}

// bpHighlight returns the spans of text with each match of query, ignoring
// case, on --hit (bp-hit), and the whole text on --mark when marked.
func bpHighlight(c *ui.Context, text, query string, marked bool) []ui.Span {
	p := bpOf(c)
	base := ui.Span{}
	if marked {
		base = ui.Span{Background: p.mark, Color: p.markInk}
	}
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		base.Text = text
		return []ui.Span{base}
	}
	var out []ui.Span
	lower := strings.ToLower(text)
	for {
		i := strings.Index(lower, q)
		if i < 0 || len(lower) != len(text) {
			break
		}
		if i > 0 {
			s := base
			s.Text = text[:i]
			out = append(out, s)
		}
		out = append(out, ui.Span{Text: text[i : i+len(q)], Background: p.hit, Color: p.markInk})
		text, lower = text[i+len(q):], lower[i+len(q):]
	}
	if text != "" {
		s := base
		s.Text = text
		out = append(out, s)
	}
	return out
}

// bpStar is the favourite star: --star fill when on, an ink-muted stroke
// always.
func bpStar(c *ui.Context, on bool) ui.Element {
	if on {
		return ui.Icon(c, iconStarOn).Size(14, 14)
	}
	return ui.Icon(c, iconStarOff).Size(14, 14)
}

// Icons, drawn as the small inline SVG glyphs the system asks for.
var (
	iconRefresh = ui.MustParseSVG([]byte(`<svg width="16" height="16" viewBox="0 0 16 16"><path d="M13 8a5 5 0 1 1-1.6-3.7" fill="none" stroke="currentColor" stroke-width="1.6"/><path d="M13.5 1.5v3.8H9.7" fill="none" stroke="currentColor" stroke-width="1.6"/></svg>`))
	iconCapture = ui.MustParseSVG([]byte(`<svg width="16" height="16" viewBox="0 0 16 16"><path d="M1 5V1h4M11 1h4v4M15 11v4h-4M5 15H1v-4" fill="none" stroke="currentColor" stroke-width="1.4"/><rect x="5" y="5" width="6" height="6" fill="currentColor"/></svg>`))
	iconAttach  = ui.MustParseSVG([]byte(`<svg width="16" height="16" viewBox="0 0 16 16"><path d="M11 4.5 5.8 9.7a1.4 1.4 0 0 0 2 2l5.4-5.4a2.8 2.8 0 0 0-4-4L3.8 7.7a4.2 4.2 0 0 0 6 6L14 9.5" fill="none" stroke="currentColor" stroke-width="1.4"/></svg>`))
	iconLog     = ui.MustParseSVG([]byte(`<svg width="16" height="16" viewBox="0 0 16 16"><rect x="2.5" y="1.5" width="11" height="13" fill="none" stroke="currentColor" stroke-width="1.3"/><path d="M5 5h6M5 8h6M5 11h4" stroke="currentColor" stroke-width="1.2"/></svg>`))
	iconReply   = ui.MustParseSVG([]byte(`<svg width="16" height="16" viewBox="0 0 16 16"><path d="M6 3 1.5 7.5 6 12" fill="none" stroke="currentColor" stroke-width="1.6"/><path d="M2 7.5h7a5 5 0 0 1 5 5V14" fill="none" stroke="currentColor" stroke-width="1.6"/></svg>`))
	iconStarOn  = ui.MustParseSVG([]byte(`<svg width="14" height="14" viewBox="0 0 14 14"><path d="M7 1l1.8 3.9 4.2.4-3.2 2.8 1 4.1L7 10.1 3.2 12.2l1-4.1L1 5.3l4.2-.4z" fill="#e6b800" stroke="#404040" stroke-width="1"/></svg>`))
	iconStarOff = ui.MustParseSVG([]byte(`<svg width="14" height="14" viewBox="0 0 14 14"><path d="M7 1l1.8 3.9 4.2.4-3.2 2.8 1 4.1L7 10.1 3.2 12.2l1-4.1L1 5.3l4.2-.4z" fill="none" stroke="currentColor" stroke-width="1"/></svg>`))
	iconMore    = ui.MustParseSVG([]byte(`<svg width="14" height="14" viewBox="0 0 14 14"><circle cx="2.5" cy="7" r="1.2" fill="currentColor"/><circle cx="7" cy="7" r="1.2" fill="currentColor"/><circle cx="11.5" cy="7" r="1.2" fill="currentColor"/></svg>`))
	// The included tick: a go disc with a go-ink stroke.
	iconTick = ui.MustParseSVG([]byte(`<svg width="14" height="14" viewBox="0 0 14 14"><circle cx="7" cy="7" r="7" fill="#2f7a22"/><path d="M3.8 7.2 6 9.4l4.2-4.6" fill="none" stroke="#ffffff" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>`))
	// The excluded mark: an alert-ink ring with a slash.
	iconCross = ui.MustParseSVG([]byte(`<svg width="14" height="14" viewBox="0 0 14 14"><circle cx="7" cy="7" r="5.9" fill="none" stroke="#c41010" stroke-width="2"/><path d="M3 11 11 3" stroke="#c41010" stroke-width="2"/></svg>`))
)
