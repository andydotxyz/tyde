//go:build linux || openbsd || freebsd || netbsd
// +build linux openbsd freebsd netbsd

package wm

import (
	"github.com/BurntSushi/xgb/xproto"
	"github.com/BurntSushi/xgbutil/ewmh"

	"fyne.io/fyne/v2"

	"fyshos.com/tyde"
	"fyshos.com/tyde/internal/x11"
)

type stack struct {
	clients, deleted []tyde.Window
	mappingOrder     []tyde.Window

	listeners []tyde.StackListener
}

func (s *stack) AddWindow(win tyde.Window) {
	if win == nil {
		return
	}
	s.addToStack(win)

	for _, l := range s.listeners {
		l.WindowAdded(win)
	}
}

func (s *stack) RaiseToTop(win tyde.Window) {
	if win.Iconic() {
		return
	}
	if len(s.clients) > 1 {
		win.RaiseAbove(s.TopWindow())
	}

	if s.indexForWin(win) == -1 {
		return
	}
	s.removeFromStack(win)
	s.addToStack(win)
	// "undelete"
	for i, w := range s.deleted {
		if w == win {
			s.deleted = append(s.deleted[:i], s.deleted[i+1:]...)
			break
		}
	}

	wm := tyde.Instance().WindowManager().(*x11WM)
	windowClientListStackingUpdate(wm)

	for _, l := range s.listeners {
		l.WindowOrderChanged()
	}
}

func (s *stack) RemoveWindow(win tyde.Window) {
	s.removeFromStack(win)

	if s.TopWindow() != nil {
		s.TopWindow().Focus()
	} else {
		// focus root
		if wm := tyde.Instance().WindowManager().(*x11WM); wm.X() != nil {
			err := ewmh.ActiveWindowReq(wm.X(), wm.RootID())
			if err != nil {
				fyne.LogError("There was an error trying to remove the window ", err)
			}
		}
	}

	for _, l := range s.listeners {
		l.WindowRemoved(win)
	}
}

func (s *stack) TopWindow() tyde.Window {
	if len(s.clients) == 0 {
		return nil
	}
	return s.clients[len(s.clients)-1]
}

func (s *stack) Windows() []tyde.Window {
	var ret []tyde.Window
	for i := len(s.clients) - 1; i >= 0; i-- {
		ret = append(ret, s.clients[i])
	}
	return ret
}

func (s *stack) addToStack(win tyde.Window) {
	s.clients = append(s.clients, win)
	s.mappingOrder = append(s.mappingOrder, win.(x11.XWin))
}

func (s *stack) clientForWin(id xproto.Window) x11.XWin {
	for _, w := range s.clients {
		if w.(x11.XWin).FrameID() == id || w.(x11.XWin).ChildID() == id {
			return w.(x11.XWin)
		}
	}

	return nil
}

func (s *stack) getWindowsFromClients(clients []tyde.Window) []xproto.Window {
	var wins []xproto.Window
	for _, cli := range clients {
		wins = append(wins, cli.(x11.XWin).ChildID())
	}
	return wins
}

func (s *stack) indexForWin(win tyde.Window) int {
	pos := -1
	for i, w := range s.clients {
		if w == win {
			pos = i
		}
	}
	return pos
}

func (s *stack) publishWindowChange(win tyde.Window) {
	for _, l := range s.listeners {
		l.WindowStateChanged(win)
	}
}

func (s *stack) removeFromStack(win tyde.Window) {
	pos := s.indexForWin(win)

	if pos == -1 {
		return
	}
	c := s.clients[pos]
	s.clients = append(s.clients[:pos], s.clients[pos+1:]...)
	s.deleted = append(s.deleted, c)

	pos = -1
	for i, w := range s.mappingOrder {
		if w == win {
			pos = i
		}
	}
	if pos == -1 {
		return
	}
	s.mappingOrder = append(s.mappingOrder[:pos], s.mappingOrder[pos+1:]...)
}
