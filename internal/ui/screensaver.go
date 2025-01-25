package ui

import (
	"os/exec"
	"time"

	"github.com/FyshOS/saver"

	"fyne.io/fyne/v2"
)

func (l *desktop) startXscreensaver() {
	_, err := exec.LookPath("xscreensaver")
	if err != nil {
		fyne.LogError("xscreensaver command not found", err)
		return
	}
	err = exec.Command("xscreensaver", "-no-splash").Start()
	if err != nil {
		fyne.LogError("Failed to lock screen", err)
	}
}

func (l *desktop) TriggerScreensaver() {
	s := saver.NewScreenSaver(nil)
	s.ClockFormat = l.settings.ClockFormatting()
	if l.settings.ScreenSaverClock() {
		s.Label = "(clock)"
	} else {
		s.Label = l.settings.ScreenSaverLabel()
	}
	s.Lock = true

	go l.wm.ShowScreensaver(s)
}

var lastActivity time.Time

func (l *desktop) DelayScreensaver() {
	lastActivity = time.Now()
}

func (l *desktop) watchScreenActivity() {
	idle := false
	to := time.NewTicker(5 * time.Second)

	for range to.C {
		if lastActivity.Add(time.Minute * 5).Before(time.Now()) {

			if !idle {
				idle = true

				l.TriggerScreensaver()
			}
		} else {
			idle = false
		}
	}
}
