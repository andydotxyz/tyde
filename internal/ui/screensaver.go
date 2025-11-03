package ui

import (
	"os/exec"
	"time"

	"fyne.io/fyne/v2"
	"github.com/FyshOS/saver"
)

var (
	inhibitCount = 0
	lastActivity = time.Now()
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

func (l *desktop) TriggerScreenSaver(delay bool) {
	s := saver.NewScreenSaver(nil)
	s.LockImmediately = !delay
	s.ClockFormat = l.settings.ClockFormatting()
	if l.settings.ScreenSaverClock() {
		s.Label = "(clock)"
	} else {
		s.Label = l.settings.ScreenSaverLabel()
	}
	s.Lock = true

	l.wm.ShowScreensaver(s)
}

func (l *desktop) DelayScreenSaver() {
	lastActivity = time.Now()
}

func (l *desktop) watchScreenActivity() {
	watchScreensaver()
	idle := false
	to := time.NewTicker(5 * time.Second)

	for range to.C {
		if inhibitCount == 0 && lastActivity.Add(time.Minute*5).Before(time.Now()) {

			if !idle {
				idle = true

				fyne.Do(func() {
					l.TriggerScreenSaver(true)
				})
			}
		} else {
			idle = false
		}
	}
}
