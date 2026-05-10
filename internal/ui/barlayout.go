package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	wmtheme "fyshos.com/tyde/theme"
)

// Declare conformity with Layout interface
var _ fyne.Layout = (*barLayout)(nil)

const separatorWidth = 2

// barLayout returns a layout used for a linear groups of icons
type barLayout struct {
	bar *bar
}

// Layout is called to pack all icons into a specified size.
func (bl *barLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	bg := objects[0]
	objects = objects[1:]
	x := theme.Padding()
	bl.layoutNarrowBar(objects)

	for _, child := range objects {
		height := child.Size().Height

		child.Move(fyne.NewPos(theme.Padding(), x))
		x += height + theme.Padding()
	}
	bg.Move(fyne.NewPos(0, 0))
	bg.Resize(fyne.NewSize(wmtheme.NarrowBarWidth, size.Height))
}

// MinSize finds the smallest size that satisfies all the child objects.
// For a barLayout this is the width of the widest item and the height is
// the sum of of all children combined with padding between each.
func (bl *barLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	barWidth := bl.calculateBarWidth(objects)

	return fyne.NewSize(wmtheme.NarrowBarWidth, barWidth)
}

func (bl *barLayout) calculateBarWidth(objects []fyne.CanvasObject) float32 {
	iconCount := float32(len(objects))
	if !bl.bar.disableTaskbar {
		iconCount--
		return (iconCount * (bl.bar.iconSize + theme.Padding())) + separatorWidth
	}

	return iconCount * (bl.bar.iconSize + theme.Padding())
}

func (bl *barLayout) layoutFullBar(size fyne.Size, icons []fyne.CanvasObject) (x float32) {
	offset := float32(0.0)
	barWidth := bl.calculateBarWidth(icons)
	barLeft := (size.Width - barWidth) / 2
	iconLeft := barLeft

	for _, child := range icons {
		if _, ok := child.(*canvas.Rectangle); ok {
			child.Resize(fyne.NewSize(separatorWidth, bl.bar.iconSize))
		} else {
			child.Resize(fyne.NewSize(bl.bar.iconSize, bl.bar.iconSize))
		}

		if _, ok := child.(*canvas.Rectangle); ok {
			iconLeft += separatorWidth
		} else {
			iconLeft += bl.bar.iconSize
		}
		iconLeft += theme.Padding()
	}

	return barLeft - offset
}

func (bl *barLayout) layoutNarrowBar(icons []fyne.CanvasObject) {
	iconSize := wmtheme.NarrowBarWidth - theme.Padding()*2
	iconLeft := theme.Padding()

	for _, child := range icons {
		if _, ok := child.(*canvas.Rectangle); ok {
			child.Resize(fyne.NewSize(iconSize, separatorWidth))
		} else {
			child.Resize(fyne.NewSize(iconSize, iconSize))
		}

		if _, ok := child.(*canvas.Rectangle); ok {
			iconLeft += separatorWidth
		} else {
			iconLeft += iconSize
		}
		iconLeft += theme.Padding()
	}
}

// newBarLayout returns a horizontal icon bar
func newBarLayout(bar *bar) *barLayout {
	return &barLayout{bar}
}
