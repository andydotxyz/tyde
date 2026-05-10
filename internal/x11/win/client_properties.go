//go:build linux || openbsd || freebsd || netbsd
// +build linux openbsd freebsd netbsd

package win

import (
	"fyne.io/fyne/v2"

	"fyshos.com/tyde"
	"fyshos.com/tyde/internal/x11"
)

type clientProperties struct {
	c         *client
	decorated bool
	iconCache fyne.Resource
}

func (c *client) Properties() tyde.WindowProperties {
	if c.props == nil {
		c.props = &clientProperties{c: c}
		c.props.refreshCache()
	}

	return c.props
}

func (c *clientProperties) Class() []string {
	return windowClass(c.c.wm.X(), c.c.win)
}

func (c *clientProperties) Command() string {
	return windowCommand(c.c.wm.X(), c.c.win)
}

func (c *clientProperties) Decorated() bool {
	return c.decorated
}

func (c *clientProperties) lookupDecorated() bool {
	return !windowBorderless(c.c.wm.X(), c.c.win)
}

func (c *clientProperties) Icon() fyne.Resource {
	if c.iconCache != nil {
		return c.iconCache
	}

	settings := tyde.Instance().Settings()
	iconSize := int(settings.LauncherIconSize() * settings.LauncherZoomScale())
	xIcon := windowIcon(c.c.wm.X(), c.c.win, iconSize, iconSize)
	if xIcon == nil {
		return nil
	}

	c.iconCache = fyne.NewStaticResource(c.Title(), xIcon.Bytes())
	return c.iconCache
}

func (c *clientProperties) IconName() string {
	return windowIconName(c.c.wm.X(), c.c.win)
}

func (c *clientProperties) SkipTaskbar() bool {
	extendedHints := x11.WindowExtendedHintsGet(c.c.wm.X(), c.c.win)
	if extendedHints == nil {
		return false
	}
	for _, hint := range extendedHints {
		if hint == "_NET_WM_STATE_SKIP_TASKBAR" {
			return true
		}
	}
	return false
}

func (c *clientProperties) Title() string {
	return x11.WindowName(c.c.wm.X(), c.c.win)
}

func (c *clientProperties) refreshCache() {
	c.iconCache = nil
	c.decorated = c.lookupDecorated()
}
