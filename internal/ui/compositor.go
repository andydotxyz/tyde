package ui

import (
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"fyshos.com/fynedesk"
)

// WindowImage holds the Fyne image and pixel-space geometry for a single
// composited window. The platform compositor populates these fields.
type WindowImage struct {
	ID   uint32
	Img  *canvas.Image
	X, Y int16
	W, H uint16
}

// CompositorWidget is a Fyne widget that displays composited window images.
// It is platform-agnostic; the platform-specific compositor (e.g. X11) is
// responsible for capturing window content and calling the methods below.
type CompositorWidget struct {
	widget.BaseWidget

	mu     sync.RWMutex
	images []*WindowImage // Fyne draw order: first = bottom, last = top
}

// NewCompositorWidget creates a new compositor widget.
func NewCompositorWidget() *CompositorWidget {
	w := &CompositorWidget{}
	w.ExtendBaseWidget(w)
	return w
}

func (cw *CompositorWidget) CreateRenderer() fyne.WidgetRenderer {
	cont := container.NewWithoutLayout()
	return &compositorRenderer{
		widget:  cw,
		cont:    cont,
		objects: []fyne.CanvasObject{cont},
	}
}

// EnsureWindow ensures a window image entry exists for the given ID. Returns it.
func (cw *CompositorWidget) EnsureWindow(id uint32) *WindowImage {
	cw.mu.Lock()
	defer cw.mu.Unlock()

	for _, wi := range cw.images {
		if wi.ID == id {
			return wi
		}
	}

	img := canvas.NewImageFromImage(nil)
	img.ScaleMode = canvas.ImageScaleFastest
	img.FillMode = canvas.ImageFillStretch

	wi := &WindowImage{ID: id, Img: img}
	cw.images = append(cw.images, wi) // append = on top
	return wi
}

// RemoveWindow removes a window image entry.
func (cw *CompositorWidget) RemoveWindow(id uint32) {
	cw.mu.Lock()
	defer cw.mu.Unlock()

	for i, wi := range cw.images {
		if wi.ID == id {
			cw.images = append(cw.images[:i], cw.images[i+1:]...)
			return
		}
	}
}

// GetWindow returns the window image for a given ID.
func (cw *CompositorWidget) GetWindow(id uint32) *WindowImage {
	cw.mu.RLock()
	defer cw.mu.RUnlock()

	for _, wi := range cw.images {
		if wi.ID == id {
			return wi
		}
	}
	return nil
}

// Reorder reorders images to match the given ID list (top-first order).
// Only windows present in this widget are kept.
func (cw *CompositorWidget) Reorder(topFirst []uint32) {
	cw.mu.Lock()
	defer cw.mu.Unlock()

	byID := make(map[uint32]*WindowImage, len(cw.images))
	for _, wi := range cw.images {
		byID[wi.ID] = wi
	}

	// Rebuild in Fyne draw order (bottom first = reverse of top-first)
	newImages := make([]*WindowImage, 0, len(cw.images))
	for i := len(topFirst) - 1; i >= 0; i-- {
		if wi, ok := byID[topFirst[i]]; ok {
			newImages = append(newImages, wi)
		}
	}
	cw.images = newImages
}

type compositorRenderer struct {
	widget  *CompositorWidget
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

	scale := screenScale()

	objs := make([]fyne.CanvasObject, len(r.widget.images))
	for i, wi := range r.widget.images {
		wi.Img.Move(fyne.NewPos(float32(wi.X)/scale, float32(wi.Y)/scale))
		wi.Img.Resize(fyne.NewSize(float32(wi.W)/scale, float32(wi.H)/scale))
		objs[i] = wi.Img
	}

	// Update the stable container's objects in place — never replace the container itself.
	r.cont.Objects = objs
}

func screenScale() float32 {
	inst := fynedesk.Instance()
	if inst == nil {
		return 1
	}
	return inst.Screens().Primary().CanvasScale()
}
