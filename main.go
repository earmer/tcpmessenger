package main

import (
	"log"
	"os"
	"os/user"
	"time"

	"my-app/internal/core"

	"github.com/egoist/mygo"
)

// appName is the name the app shows in its windows, menus and notifications.
const appName = "TCP Messenger"

func main() {
	// One TCP Messenger per user: it owns the port. A second start brings
	// the running one's send window forward.
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	a := newTCPApp()
	mygo.App.SetName(appName)
	mygo.App.OnSecondInstance(func([]string, string) { a.showMain() })
	mygo.App.OnActivate(func(hasVisibleWindows bool) {
		if !hasVisibleWindows {
			a.showMain()
		}
	})
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { a.quitting = true })
	// Quitting announces the exit to the LAN.
	mygo.App.OnQuit(func() {
		if err := a.net.Stop(); err != nil {
			log.Println("stop:", err)
		}
	})
	mygo.App.WhenReady(func() {
		a.desk = &nativeDesk{a: a}
		nativeWindows = func() bool { return true }
		openSendWindow(a, nil, "")
		setupTray(a)
		startAutoAbsence(a)
		if err := startMessenger(a); err != nil {
			log.Println("start:", err)
			go mygo.Dialog.Error(appName+" could not start", err.Error())
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// newTCPApp opens the backends (backends.go) and loads the settings.
func newTCPApp() *tcpApp {
	net, st := openBackends()
	a := newAppWith(net, st)
	return a
}

// newAppWith makes the shared state over the given backends, with a desk
// that opens no windows until main replaces it: tests keep it.
func newAppWith(net core.Messenger, st core.Store) *tcpApp {
	a := &tcpApp{
		net:      net,
		store:    st,
		pending:  map[string]*pendingSend{},
		failures: map[string][]deliveryFailure{},
	}
	a.desk = &headlessDesk{}
	s, err := st.LoadSettings()
	if err != nil {
		log.Println("settings:", err)
		s = core.DefaultSettings()
	}
	if s.Port == 0 {
		s.Port = core.DefaultPort
	}
	if s.QuoteMark == "" {
		s.QuoteMark = "> "
	}
	if s.Nickname == "" {
		if u, err := user.Current(); err == nil {
			s.Nickname = u.Username
		}
	}
	a.settings = s
	return a
}

// startMessenger starts the network layer with events that hop to the
// main thread before they touch the app's state.
func startMessenger(a *tcpApp) error {
	err := a.net.Start(a.settings, core.Events{
		PeersChanged: func(users []core.User) {
			a.desk.post(func() { a.peers = a.checkKeys(users) })
		},
		Received: func(m core.Message) {
			a.desk.post(func() { a.handleReceived(m) })
		},
		Opened: func(from core.User, id string) {
			a.desk.post(func() { a.handleOpened(from, id) })
		},
		Delivered: func(to core.User, id string, err error) {
			a.desk.post(func() { a.handleDelivered(to, id, err) })
		},
	})
	if err != nil {
		return err
	}
	a.peers = a.checkKeys(a.net.Peers())
	return nil
}

// showMain brings the main send window forward, opening it again if it
// was closed.
func (a *tcpApp) showMain() {
	if a.mainSend == nil || a.mainSend.closed {
		a.mainSend = nil
		openSendWindow(a, nil, "")
		return
	}
	if w := a.mainSend.win; w != nil {
		w.Show()
		w.Restore()
		w.Focus()
	}
}

// invalidateAll redraws every window of the send, receive and main kinds,
// after state they show changed.
func (a *tcpApp) invalidateAll() {
	for _, sw := range a.sendWins {
		if sw.win != nil {
			sw.win.Invalidate()
		}
	}
	for _, rw := range a.recvWins {
		if rw.win != nil {
			rw.win.Invalidate()
		}
	}
}

// deskAPI is what the windows need from the desktop. nativeDesk is the
// real one; headlessDesk runs everything in place and opens nothing, for
// tests.
type deskAPI interface {
	// post runs fn on the main thread, then redraws the windows.
	post(fn func())
	// background runs fn off the main thread.
	background(fn func())
	// newWindow opens a window, or returns nil without a desktop.
	newWindow(opts mygo.WindowOptions) *mygo.Window
	// openFiles asks for files to open; done gets them on the main
	// thread, and is not called when canceled.
	openFiles(parent *mygo.Window, title string, done func(paths []string))
	// saveFile asks where to save; done gets the path on the main thread,
	// and is not called when canceled.
	saveFile(parent *mygo.Window, defaultPath string, done func(path string))
	// notify shows a system notification; onClick runs on the main thread.
	notify(title, body string, onClick func())
	beep()
	// native reports whether menus, the tray and dialogs exist.
	native() bool
}

type nativeDesk struct{ a *tcpApp }

func (d *nativeDesk) post(fn func()) {
	run := func() {
		fn()
		d.a.invalidateAll()
		syncMenus(d.a)
	}
	if w := d.a.main; w != nil && !w.IsDestroyed() {
		w.Update(run)
		return
	}
	mygo.RunOnMain(run)
}

func (d *nativeDesk) background(fn func()) { go fn() }

func (d *nativeDesk) newWindow(opts mygo.WindowOptions) *mygo.Window {
	return mygo.NewWindow(opts)
}

func (d *nativeDesk) openFiles(parent *mygo.Window, title string, done func([]string)) {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: parent, Title: title, Multiple: true})
		if err != nil {
			log.Println("open dialog:", err)
		}
		if len(paths) > 0 {
			d.post(func() { done(paths) })
		}
	}()
}

func (d *nativeDesk) saveFile(parent *mygo.Window, defaultPath string, done func(string)) {
	go func() {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: parent, DefaultPath: defaultPath, CreateDirectories: true})
		if err != nil {
			log.Println("save dialog:", err)
		}
		if path != "" {
			d.post(func() { done(path) })
		}
	}()
}

func (d *nativeDesk) notify(title, body string, onClick func()) {
	if !mygo.NotificationsSupported() {
		return
	}
	n := mygo.NewNotification(mygo.NotificationOptions{Title: title, Body: body})
	if onClick != nil {
		n.OnClick(func() {
			d.post(onClick)
			n.Close()
		})
	}
	go func() {
		if err := n.Show(); err != nil {
			log.Println("notification:", err)
		}
	}()
}

func (d *nativeDesk) beep()        { mygo.Shell.Beep() }
func (d *nativeDesk) native() bool { return true }

// headlessDesk is the desk of tests: everything runs in place, no window
// opens, and the dialogs and notifications are recorded.
type headlessDesk struct {
	notes     []string // "title: body" of each notification
	beeps     int
	openPaths []string // what the next open dialog answers
	savePath  string   // what the next save dialog answers
	asked     []string // default paths of save dialogs
}

func (d *headlessDesk) post(fn func())                            { fn() }
func (d *headlessDesk) background(fn func())                      { fn() }
func (d *headlessDesk) newWindow(mygo.WindowOptions) *mygo.Window { return nil }
func (d *headlessDesk) openFiles(_ *mygo.Window, _ string, done func([]string)) {
	if len(d.openPaths) > 0 {
		done(d.openPaths)
	}
}
func (d *headlessDesk) saveFile(_ *mygo.Window, def string, done func(string)) {
	d.asked = append(d.asked, def)
	if d.savePath != "" {
		done(d.savePath)
	}
}
func (d *headlessDesk) notify(title, body string, _ func()) {
	d.notes = append(d.notes, title+": "+body)
}
func (d *headlessDesk) beep()        { d.beeps++ }
func (d *headlessDesk) native() bool { return false }

// longTime formats a message's time as the receive window shows it.
func longTime(t time.Time) string { return t.Format("2006/01/02 (Mon) 15:04:05") }

// fakeBackends are the demo backends of TCPMSG_FAKE=1.
func fakeBackends() (core.Messenger, core.Store) {
	now := time.Now()
	return core.NewFakeMessenger(
		core.User{ID: "kaga@H24", Name: "Kaga", Group: "Tech", Host: "H24", Logon: "kaga", Addr: "192.168.1.24:2425", Encrypt: true, LastSeen: now},
		core.User{ID: "motoyama@H23", Name: "Motoyama", Group: "Tech", Host: "H23", Logon: "motoyama", Addr: "192.168.1.23:2425", Encrypt: true, LastSeen: now},
		core.User{ID: "oda@H22", Name: "Oda", Group: "Tech", Host: "H22", Logon: "oda", Addr: "192.168.1.22:2425", Absent: true, AbsenceTitle: "Meeting", LastSeen: now},
		core.User{ID: "tanabe@S07", Name: "Tanabe", Group: "Sales", Host: "S07", Logon: "tanabe", Addr: "192.168.2.7:2425", Encrypt: true, LastSeen: now},
		core.User{ID: "hinomizu@Q01", Name: "Hinomizu", Group: "Strategy", Host: "Q01", Logon: "hinomizu", Addr: "192.168.3.1:2425", LastSeen: now},
	), core.NewMemStore()
}

func fakeMode() bool { return os.Getenv("TCPMSG_FAKE") == "1" }
