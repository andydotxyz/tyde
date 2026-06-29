//go:build linux || openbsd || freebsd || netbsd
// +build linux openbsd freebsd netbsd

package wm

import (
	"log"
	"os/exec"
	"time"

	"fyshos.com/tyde"
	"github.com/BurntSushi/xgb/screensaver"
	"github.com/BurntSushi/xgb/xproto"
	"github.com/FyshOS/saver"

	"fyne.io/fyne/v2"
)

func (x *x11WM) initScreensaver() {
	err := screensaver.Init(x.x.Conn())
	if err != nil {
		log.Println("Failed to init screensaver extension")
		return
	}

	// screensaver.SelectInput(conn.Conn(), xproto.Drawable(conn.Screen().Root),
	//	screensaver.EventNotifyMask)
	go x.watchScreensaver()
}

func (x *x11WM) watchScreensaver() {
	// A ticker drops missed ticks during computer sleeps. We still measure the
	// wall-clock gap between ticks to notice a sleep happened and lock at once.
	t := time.NewTicker(time.Second)
	defer t.Stop()
	previous := time.Now()

	for range t.C {
		info, err := screensaver.QueryInfo(x.x.Conn(), xproto.Drawable(x.x.Screen().Root)).Reply()
		if err != nil {
			fyne.LogError("Failed to query screensaver info", err)
			continue
		}

		now := time.Now()
		slept := now.Sub(previous).Seconds() > 2 // skipped a tick
		previous = now
		if slept {
			tyde.Instance().TriggerScreenSaver(false) // no delay on lock prompt after sleep
		} else if info.MsSinceUserInput <= 1500 {
			tyde.Instance().DelayScreenSaver()
		}
	}
}

var screenSaverActive bool

func (x *x11WM) ShowScreensaver(s *saver.ScreenSaver) {
	if tyde.Instance().Settings().ScreenSaverType() == "XScreensaver" {
		task := "-activate"
		if s.Lock {
			task = "-lock"
		}
		cmd := exec.Command("xscreensaver-command", task)
		cmd.Start()
		return
	}

	if screenSaverActive {
		return
	}

	screenSaverActive = true
	s.OnUnlocked = func() {
		screenSaverActive = false
	}

	if path, err := exec.LookPath("fyshsaver"); err == nil {
		var params []string
		if s.Lock {
			params = append(params, "-lock")
			if !s.LockImmediately {
				params = append(params, "-lock-delay")
			}
		}
		if s.Label != "" {
			params = append(params, "-label", s.Label)
		}

		go func() {
			time.Sleep(time.Millisecond * 100)

			err = exec.Command(path, params...).Run()
			if err != nil {
				fyne.LogError("Failed to activate fyne screensaver", err)
			}
			s.OnUnlocked()
		}()
		return
	}

	go func() {
		time.Sleep(time.Millisecond * 100)
		fyne.Do(s.ShowWindows)
	}()
}
