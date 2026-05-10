//go:build linux || openbsd || freebsd || netbsd
// +build linux openbsd freebsd netbsd

package x11 // import "fyshos.com/tyde/internal/x11"

import (
	"github.com/BurntSushi/xgb"
	"github.com/BurntSushi/xgb/xproto"
	"github.com/BurntSushi/xgbutil"

	"fyshos.com/tyde"
)

// XWM describes the additional elements that an X11 window manager exposes
type XWM interface {
	tyde.WindowManager

	X() *xgbutil.XUtil
	Conn() *xgb.Conn

	RootID() xproto.Window
	RootIDForScreen(screenName string) xproto.Window
}
