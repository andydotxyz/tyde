package composit

import (
	"sync"

	"github.com/BurntSushi/xgb/xproto"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type windowImage struct {
	win  xproto.Window
	img  *canvas.Image
	x, y int16
	w, h uint16
}

type compositorWidget struct {
	widget.BaseWidget

	mu     sync.RWMutex
	images []*windowImage // Fyne draw order: first = bottom, last = top
}

func newCompositorWidget() *compositorWidget {
	w := &compositorWidget{}
	w.ExtendBaseWidget(w)
	return w
}

func (cw *compositorWidget) CreateRenderer() fyne.WidgetRenderer {
	cont := container.NewWithoutLayout()
	return &compositorRenderer{
		widget:  cw,
		cont:    cont,
		objects: []fyne.CanvasObject{cont},
	}
}

// ensureWindow ensures a window image entry exists. Returns it.
// Safe to call from any goroutine (mutex-protected).
func (cw *compositorWidget) ensureWindow(win xproto.Window) *windowImage {
	cw.mu.Lock()
	defer cw.mu.Unlock()

	i := indexFunc(cw.images, func(wi *windowImage) bool { return wi.win == win })
	if i != -1 {
		return cw.images[i]
	}

	img := canvas.NewImageFromImage(nil)
	img.ScaleMode = canvas.ImageScaleFastest
	img.FillMode = canvas.ImageFillStretch

	wi := &windowImage{win: win, img: img}
	cw.images = append(cw.images, wi) // append = on top
	return wi
}

// removeWindow removes a window image entry.
// Safe to call from any goroutine (mutex-protected).
func (cw *compositorWidget) removeWindow(win xproto.Window) {
	cw.mu.Lock()
	defer cw.mu.Unlock()

	i := indexFunc(cw.images, func(wi *windowImage) bool {
		return wi.win == win
	})
	if i == -1 {
		return
	}
	cw.images = delete(cw.images, i, i+1)
}

// getWindow returns the window image for a given X window.
func (cw *compositorWidget) getWindow(win xproto.Window) *windowImage {
	cw.mu.RLock()
	defer cw.mu.RUnlock()

	i := indexFunc(cw.images, func(wi *windowImage) bool {
		return wi.win == win
	})
	if i == -1 {
		return nil
	}
	return cw.images[i]
}

// reorder reorders images to match the given window list (top-first order,
// same as the clients list). Only windows present in this widget are kept.
func (cw *compositorWidget) reorder(topFirst []xproto.Window) {
	cw.mu.Lock()
	defer cw.mu.Unlock()

	byWin := make(map[xproto.Window]*windowImage, len(cw.images))
	for _, wi := range cw.images {
		byWin[wi.win] = wi
	}

	// Rebuild in Fyne draw order (bottom first = reverse of top-first)
	newImages := make([]*windowImage, 0, len(cw.images))
	for i := len(topFirst) - 1; i >= 0; i-- {
		if wi, ok := byWin[topFirst[i]]; ok {
			newImages = append(newImages, wi)
		}
	}
	cw.images = newImages
}

type compositorRenderer struct {
	widget  *compositorWidget
	cont    *fyne.Container
	objects []fyne.CanvasObject // stable: always [cont]
}

func (r *compositorRenderer) Destroy() {}

func (r *compositorRenderer) Layout(size fyne.Size) {
	r.cont.Resize(size)
}

func (r *compositorRenderer) MinSize() fyne.Size {
	return fyne.NewSize(0, 0)
}

func (r *compositorRenderer) Objects() []fyne.CanvasObject {
	return r.objects
}

func (r *compositorRenderer) Refresh() {
	r.widget.mu.RLock()
	defer r.widget.mu.RUnlock()

	scale := getScreenScale()

	objs := make([]fyne.CanvasObject, len(r.widget.images))
	for i, wi := range r.widget.images {
		wi.img.Move(fyne.NewPos(float32(wi.x)/scale, float32(wi.y)/scale))
		wi.img.Resize(fyne.NewSize(float32(wi.w)/scale, float32(wi.h)/scale))
		objs[i] = wi.img
	}

	// Update the stable container's objects in place — never replace the container itself.
	r.cont.Objects = objs
}

func getScreenScale() float32 {
	inst := screenScaleFunc
	if inst != nil {
		return inst()
	}
	return 1
}

// screenScaleFunc is set by the compositor to provide screen scale without import cycles.
var screenScaleFunc func() float32
