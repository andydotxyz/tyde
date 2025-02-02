package embed

import (
	_ "embed"
	"image/color"

	tictactoeExample "github.com/fyne-io/examples/tictactoe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
)

//go:embed "icons/tictactoe.svg"
var ticTacToeIcon []byte

type ticTacToe struct {
	app
}

func newTicTacToe(multi *container.MultipleWindows) *ticTacToe {
	t := &ticTacToe{}
	t.m = multi
	t.name = "Tic-Tac-Toe"
	t.categories = []string{"games"}
	t.icon = fyne.NewStaticResource("tictactoe.svg", ticTacToeIcon)
	t.makeContent = t.makeUI
	return t
}

func (t *ticTacToe) makeUI() fyne.CanvasObject {
	dummy := test.NewWindow(canvas.NewRectangle(color.Transparent))
	return tictactoeExample.Show(dummy)
}
