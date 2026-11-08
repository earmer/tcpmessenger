package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"runtime"
	"strings"

	"github.com/egoist/mygo"
)

// setupTray puts TCP Messenger in the menu bar or the notification area,
// and gives the app its menu bar.
func setupTray(a *tcpApp) {
	tray, err := mygo.NewTray(mygo.TrayOptions{
		ID:             "tcpmessenger",
		Icon:           trayIcon(),
		IconIsTemplate: true,
		ToolTip:        appName,
	})
	if err != nil {
		log.Println("tray:", err)
	} else {
		a.tray = tray
	}
	a.menuSig = ""
	syncMenus(a)
}

// setAbsence turns absence off (i < 0) or on with mode i.
func (a *tcpApp) setAbsence(i int) {
	if i < 0 {
		a.settings.Absent = false
	} else {
		a.settings.Absent, a.settings.AbsenceIndex = true, i
	}
	if err := a.saveSettings(); err != nil {
		log.Println("settings:", err)
	}
	syncMenus(a)
}

// showAbout shows what TCP Messenger is.
func showAbout() {
	if runtime.GOOS == "darwin" {
		mygo.App.ShowAboutPanel(mygo.AboutPanelOptions{
			ApplicationName: appName,
			Credits:         "A LAN messenger after IP Messenger.",
		})
		return
	}
	go mygo.Dialog.Message(mygo.MessageOptions{
		Type:    mygo.MessageInfo,
		Title:   "About " + appName,
		Message: appName + " " + appVersion(),
		Detail:  "A LAN messenger after IP Messenger.",
		Buttons: []string{"OK"},
	})
}

// menuSignature is what the menus show that changes.
func menuSignature(a *tcpApp) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%v|%d|%d", a.settings.Absent, a.settings.AbsenceIndex, a.unreadCount())
	for _, m := range a.settings.AbsenceModes {
		b.WriteString("|" + m.Title)
	}
	return b.String()
}

// syncMenus rebuilds the tray menu and the app menu when what they show
// changed: the absence modes, the absence, the unread count. It is cheap to
// call every frame.
func syncMenus(a *tcpApp) {
	if a.desk == nil || !a.desk.native() {
		return
	}
	sig := menuSignature(a)
	if sig == a.menuSig {
		return
	}
	a.menuSig = sig
	unread := a.unreadCount()
	if a.tray != nil {
		a.tray.SetMenu(mygo.NewMenu(trayItems(a, unread)))
		tip := appName
		title := ""
		if unread > 0 {
			tip = fmt.Sprintf("%s - %d unread", appName, unread)
			title = fmt.Sprint(unread)
		}
		if a.settings.Absent {
			tip += " (absent)"
		}
		a.tray.SetToolTip(tip)
		if runtime.GOOS == "darwin" {
			a.tray.SetTitle(title)
		}
	}
	mygo.App.SetBadgeCount(unread)
	mygo.App.SetMenu(mygo.NewMenu(appMenuItems(a, unread)))
}

func click(fn func()) func(*mygo.MenuItem, *mygo.Window) {
	return func(*mygo.MenuItem, *mygo.Window) { fn() }
}

// absenceItems are the Absence Mode submenu's items.
func absenceItems(a *tcpApp) []*mygo.MenuItem {
	items := []*mygo.MenuItem{{
		Label: "Not absent", Type: mygo.MenuItemRadio, Checked: !a.settings.Absent,
		Click: click(func() { a.setAbsence(-1) }),
	}}
	for i, m := range a.settings.AbsenceModes {
		items = append(items, &mygo.MenuItem{
			Label: m.Title, Type: mygo.MenuItemRadio,
			Checked: a.settings.Absent && a.settings.AbsenceIndex == i,
			Click:   click(func() { a.setAbsence(i) }),
		})
	}
	return append(items, mygo.Separator(), &mygo.MenuItem{Label: "Absence Settings…", Click: click(func() { openAbsence(a) })})
}

func trayItems(a *tcpApp, unread int) []*mygo.MenuItem {
	return []*mygo.MenuItem{
		{Label: "Send Message", Click: click(a.showMain)},
		{Label: fmt.Sprintf("Open Received Messages (%d)", len(a.queued)), Hidden: len(a.queued) == 0, Click: click(a.openQueued)},
		mygo.Separator(),
		{Label: "Absence Mode", Submenu: absenceItems(a)},
		{Label: "Open LogViewer", Click: click(func() { openLogViewer(a) })},
		{Label: "Settings…", Click: click(func() { openSettings(a) })},
		mygo.Separator(),
		{Label: "About " + appName, Click: click(showAbout)},
		{Label: "Quit", Click: click(mygo.App.Quit)},
	}
}

func appMenuItems(a *tcpApp, unread int) []*mygo.MenuItem {
	return []*mygo.MenuItem{
		{Role: mygo.RoleAppMenu},
		{Label: "Messenger", Submenu: []*mygo.MenuItem{
			{Label: "Send Message", Accelerator: "CmdOrCtrl+N", Click: click(a.showMain)},
			{Label: "Open Received Messages", Disabled: len(a.queued) == 0, Click: click(a.openQueued)},
			{Label: "Refresh Members", Accelerator: "F5", Click: click(a.net.Refresh)},
			mygo.Separator(),
			{Label: "Absence Mode", Submenu: absenceItems(a)},
			{Label: "Sort/Display…", Click: click(func() { openSortDisplay(a) })},
			{Label: "Log Viewer", Accelerator: "CmdOrCtrl+L", Click: click(func() { openLogViewer(a) })},
			{Label: "Settings…", Accelerator: "CmdOrCtrl+,", Click: click(func() { openSettings(a) })},
			mygo.Separator(),
			{Label: "About " + appName, Click: click(showAbout)},
			{Role: mygo.RoleClose},
			{Label: "Quit " + appName, Accelerator: "CmdOrCtrl+Q", Click: click(mygo.App.Quit)},
		}},
		{Role: mygo.RoleEditMenu},
		{Role: mygo.RoleWindowMenu},
	}
}

// trayIcon draws the tray's icon: an envelope, black on transparent, 32×32
// pixels, which macOS tints for the menu bar.
func trayIcon() []byte {
	const n = 32
	img := image.NewNRGBA(image.Rect(0, 0, n, n))
	ink := color.NRGBA{0, 0, 0, 255}
	set := func(x, y int) {
		if x >= 0 && y >= 0 && x < n && y < n {
			img.SetNRGBA(x, y, ink)
		}
	}
	x0, y0, x1, y1 := 3, 7, 28, 24
	for t := 0; t < 2; t++ {
		for x := x0; x <= x1; x++ {
			set(x, y0+t)
			set(x, y1-t)
		}
		for y := y0; y <= y1; y++ {
			set(x0+t, y)
			set(x1-t, y)
		}
	}
	// The flap: two lines from the top corners to the middle.
	mid := (x0 + x1) / 2
	for x := x0; x <= x1; x++ {
		d := x - x0
		if x > mid {
			d = x1 - x
		}
		y := y0 + d*9/(mid-x0)
		set(x, y)
		set(x, y+1)
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}
