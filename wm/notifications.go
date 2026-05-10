package wm

import (
	"fyne.io/fyne/v2"
)

var (
	server             *notifications
	lastNotificationID uint32
)

// Notification is a simple struct representing message that can be displayed in the notification area
type Notification struct {
	ID          uint32
	Title, Body string
	Icon        fyne.Resource
}

// NewNotification creates a new message that can be passed to SendNotification
func NewNotification(title, body, icon string) *Notification {
	lastNotificationID++

	var res fyne.Resource
	if icon != "" {
		r, err := fyne.LoadResourceFromPath(icon)
		if err != nil {
			fyne.LogError("Failed to read notification icon: "+icon, err)
		} else {
			res = r
		}
	}
	item := &Notification{ID: lastNotificationID, Title: title, Body: body, Icon: res}

	return item
}

// SetNotificationListener connects the user interface to display notifications.
// Other developers should not use this call.
func SetNotificationListener(listen func(*Notification)) {
	s := startNotifications()

	s.listener = listen
	server = s
}

// SendNotification posts a given notification into the user interface's notification area
func SendNotification(n *Notification) {
	if server == nil || server.listener == nil {
		fyne.LogError("No notifications listener attached", nil)
		return
	}

	server.listener(n)
}

type notifications struct {
	listener func(*Notification)
}

func (n *notifications) Notify(appName string, replacesID uint32, appIcon, summary, body string,
	actions []string, hints map[string]interface{}, timeout int32,
) (uint32, error) {
	item := NewNotification(summary, body, appIcon)

	SendNotification(item)
	return item.ID, nil
}

func (n *notifications) GetServerInformation() (string, string, string, string) {
	return "Tyde", "Fyne.io", "0", "0"
}

func (n *notifications) GetCapabilities() []string {
	return []string{"body", "icon-static", "persistence"}
}

func (n *notifications) register() {
	err := RegisterService(n, "/org/freedesktop/Notifications", "org.freedesktop.Notifications")
	if err != nil {
		fyne.LogError("Could not start DBus notifications server, using local only", err)
	}
}

func startNotifications() *notifications {
	n := &notifications{}
	go n.register()

	return n
}
