// Note that you need to have github.com/knightpp/dbus-codegen-go installed
//go:generate dbus-codegen-go -prefix org.freedesktop -package screensaver -output generated/screensaver.go dbus/ScreenSaver.xml

//go:build linux || openbsd || freebsd || netbsd || darwin

package ui

import (
	"math/rand"

	"fyne.io/fyne/v2"
	screensaver "fyshos.com/fynedesk/internal/ui/generated"
	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/godbus/dbus/v5/prop"
)

func watchScreensaver() {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		fyne.LogError("failed to connect to DBus to watch for screensaver inhibits", err)
		return
	}

	name := "org.freedesktop.ScreenSaver"
	r, err := conn.RequestName(name, dbus.NameFlagDoNotQueue)
	if err != nil || r != dbus.RequestNameReplyPrimaryOwner {
		fyne.LogError("could not watch DBus screensaver, another is registered", err)
		return
	}

	s := &screenSaverWatcher{}
	path := "/org/freedesktop/ScreenSaver"
	err = conn.ExportAll(s, dbus.ObjectPath(path), "org.freedesktop.ScreenSaver")
	if err != nil {
		fyne.LogError("failed to export inhibits", err)
		return
	}

	node := introspect.Node{
		Name: path,
		Interfaces: []introspect.Interface{
			introspect.IntrospectData,
			prop.IntrospectData,
			screensaver.IntrospectDataScreenSaver,
		},
	}
	err = conn.Export(introspect.NewIntrospectable(&node), dbus.ObjectPath(path),
		"org.freedesktop.DBus.Introspectable")
	if err != nil {
		fyne.LogError("could not export our node data", err)
	}
}

type screenSaverWatcher struct {
}

func (s *screenSaverWatcher) Inhibit(_ dbus.Sender, who, why string) (uint, *dbus.Error) {
	id := rand.Uint32()
	inhibitCount++

	// TODO also check these are still alive every so often
	return uint(id), nil
}

func (s *screenSaverWatcher) UnInhibit(_ dbus.Sender, cookie uint32) *dbus.Error {
	// TODO compare to the cookies logged
	inhibitCount--
	return nil
}
