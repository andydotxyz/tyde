package wm

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image/color"
	"log"
	"os"
	"os/exec"
	"os/user"
	"sync"

	"fyshos.com/tyde"
	wmTheme "fyshos.com/tyde/theme"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/godbus/dbus/v5"
)

type subj struct {
	Kind    string
	Details map[string]dbus.Variant
}

type auth struct {
	dialogs map[string]func() // cookie -> dismiss the modal and end the session

	rememberPass string
	encoder      *base64.Encoding
}

func (a *auth) register() {
	conn2, err := dbus.SystemBus()
	if err != nil {
		fyne.LogError("Could not connect to DBus for authentication events", err)
		return
	}

	err = conn2.ExportAll(a, "/AuthenticationAgent", "org.freedesktop.PolicyKit1.AuthenticationAgent")
	if err != nil {
		fyne.LogError("Could not start auth agent server", err)
	}

	obj := conn2.Object("org.freedesktop.PolicyKit1", "/org/freedesktop/PolicyKit1/Authority")
	call := obj.Call("org.freedesktop.PolicyKit1.Authority.RegisterAuthenticationAgent", 0,

		&subj{"unix-session", map[string]dbus.Variant{
			"session-id": dbus.MakeVariant("c1"),
		}}, "en_US",
		"/AuthenticationAgent")
	if call.Err != nil {
		fyne.LogError("Failed to register auth agent", call.Err)
	}
}

type ident struct {
	ID      string
	Details map[string]dbus.Variant
}

func (a *auth) BeginAuthentication(actionID, message, iconName string, details map[string]string, cookie string, ids []ident, sender dbus.Sender) (err *dbus.Error) {
	username, err2 := a.resolveUser(ids)
	if err2 != nil {
		fyne.LogError("Failed to look up user", err)
	}
	if a.rememberPass != "" {
		err2 = a.reply(username, cookie, a.decode(a.rememberPass))
		if err2 == nil {
			return nil
		}

		// fall through to asking again
		a.rememberPass = ""
	}

	wg := &sync.WaitGroup{}
	wg.Add(1)
	pass := widget.NewPasswordEntry()
	remember := widget.NewCheck("", func(bool) {})
	remember.Checked = true
	f := widget.NewForm(
		widget.NewFormItem("Ident", widget.NewLabel(username)),
		widget.NewFormItem("Password", pass),
		widget.NewFormItem("Remember", remember),
	)
	// dismiss tears down the modal and ends the auth session. It is guarded so the
	// buttons and a CancelAuthentication call cannot double-close.
	var closeModal func()
	var once sync.Once
	dismiss := func() {
		once.Do(func() {
			if closeModal != nil {
				closeModal()
			}
			delete(a.dialogs, cookie)
			wg.Done()
		})
	}
	a.dialogs[cookie] = dismiss

	var auth *widget.Button
	auth = widget.NewButton("Authorize", func() {
		auth.Disable()
		err3 := a.reply(username, cookie, pass.Text)

		if err3 != nil {
			log.Println("Auth err", err3)
		} else {
			if remember.Checked {
				a.rememberPass = a.encode(pass.Text)
			}
			dismiss()
		}
		auth.Enable()
	})
	auth.Importance = widget.HighImportance
	cancel := widget.NewButton("Cancel", func() {
		dismiss()
	})
	pass.OnSubmitted = func(string) {
		auth.OnTapped()
	}

	header := widget.NewRichTextFromMarkdown(fmt.Sprintf("### Authorise\n\n_%s_", message))
	header.Truncation = fyne.TextTruncateEllipsis
	header.Refresh()
	bottomPad := canvas.NewRectangle(color.Transparent)
	bottomPad.SetMinSize(fyne.NewSquareSize(10))
	content := container.NewBorder(
		header,
		container.NewVBox(
			container.NewHBox(layout.NewSpacer(),
				container.NewGridWithColumns(2, cancel, auth),
				layout.NewSpacer()), bottomPad,
		),
		nil, nil, f,
	)

	r, g, b, _ := theme.Color(theme.ColorNameOverlayBackground).RGBA()
	bgCol := &color.NRGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 230}

	bg := canvas.NewRectangle(bgCol)
	icon := canvas.NewImageFromResource(wmTheme.LockIcon)
	iconBox := container.NewWithoutLayout(icon)
	icon.Resize(fyne.NewSize(92, 92))
	icon.Move(fyne.NewPos(300-92-theme.Padding(), theme.Padding()))
	dialog := container.NewStack(
		iconBox, bg,
		container.NewPadded(content),
	)

	// Show as a modal: the desktop centres the dialog over a blurred backdrop that
	// does not dismiss on tap or mouse-out, so it stays until a button calls dismiss.
	closeModal = tyde.Instance().ShowModal(dialog, fyne.NewSize(340, 250))
	fyne.Do(func() {
		tyde.Instance().Root().Canvas().Focus(pass)
	})

	wg.Wait()
	return nil
}

func (a *auth) resolveUser(ids []ident) (string, error) {
	uid := ""
	_, err := fmt.Sscanf(ids[0].Details["uid"].String(), "@u %s", &uid)
	if err != nil {
		currentUser, err2 := user.Current()
		if err2 != nil {
			return "", err2
		}

		return currentUser.Username, nil
	}

	usr, err2 := user.LookupId(uid)
	if err2 != nil {
		return "", err2
	}
	return usr.Username, nil
}

func (a *auth) reply(username string, cookie string, pass string) error {
	cmd := exec.Command("/usr/lib/polkit-1/polkit-agent-helper-1", username)

	buffer := bytes.Buffer{}
	buffer.Write([]byte(cookie + "\n"))
	buffer.Write([]byte(pass + "\n"))
	cmd.Stdin = &buffer

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (a *auth) CancelAuthentication(cookie string, sender dbus.Sender) (err *dbus.Error) {
	if dismiss, ok := a.dialogs[cookie]; ok {
		dismiss() // tears down the modal and ends the session
	}
	return nil
}

func (a *auth) decode(in string) string {
	out, err := a.encoder.DecodeString(in)
	if err != nil {
		fyne.LogError("Codinging remembered password err", err)
		return ""
	}
	return string(out)
}

func (a *auth) encode(in string) string {
	return a.encoder.EncodeToString([]byte(in))
}

// StartAuthAgent asks our policy kit agent to start listening for auth requests.
func StartAuthAgent() {
	a := &auth{dialogs: make(map[string]func()), encoder: base64.StdEncoding}
	go a.register()
}
