//go:build linux || openbsd || freebsd || netbsd
// +build linux openbsd freebsd netbsd

package wm

import (
	"log"
	"os/exec"
	"time"

	"fyshos.com/fynedesk"
	"github.com/BurntSushi/xgb/screensaver"
	"github.com/BurntSushi/xgb/xproto"
	"github.com/FyshOS/saver"
)

func (x *x11WM) initScreensaver() {
	err := screensaver.Init(x.x.Conn())
	if err != nil {
		log.Println("Failed to init screensaver extension")
		return
	}

	//screensaver.SelectInput(conn.Conn(), xproto.Drawable(conn.Screen().Root),
	//	screensaver.EventNotifyMask)
	go x.watchScreensaver()
}

func (x *x11WM) watchScreensaver() {
	to := time.NewTicker(5 * time.Second)

	for range to.C {
		info, err := screensaver.QueryInfo(x.x.Conn(), xproto.Drawable(x.x.Screen().Root)).Reply()
		if err != nil {
			log.Println("ERR", err)
			continue
		}

		if info.MsSinceUserInput <= 5500 {
			fynedesk.Instance().DelayScreenSaver()
		}
	}
}

var screenSaverActive bool

func (x *x11WM) ShowScreensaver(s *saver.ScreenSaver) {
	if fynedesk.Instance().Settings().ScreenSaverType() == "XScreensaver" {
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

	s.ShowWindow()
}
