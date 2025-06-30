package composit

import (
	"os/exec"

	"fyne.io/fyne/v2"
	"fyshos.com/fynedesk"
)

var compizMeta = fynedesk.ModuleMetadata{
	Name:        "Compositor",
	NewInstance: newCompiz,
}

type comp struct {
}

func (c *comp) Destroy() {
	c.disable()
}

func (c *comp) Metadata() fynedesk.ModuleMetadata {
	return compizMeta
}

func (c *comp) disable() {
	_ = exec.Command("killall", "compton").Start()
	_ = exec.Command("killall", "picom").Start()
}

func (c *comp) enable() {
	params := []string{"-c", "-r", "20", "-f", "-i", "1.0", "--vsync"}

	path, err := exec.LookPath("compton")
	if err != nil {
		path, err = exec.LookPath("picom")
		if err != nil {
			fyne.LogError("Compositor requires compton or picom binary present", err)
			return
		}
		params = append(params, "--backend", "glx")
	} else {
		params = append(params, "drm", "-C")
	}

	_ = exec.Command(path, params...).Start()
}

// newCompiz creates a new module that will manage composition of the windows.
func newCompiz() fynedesk.Module {
	c := &comp{}
	c.enable()
	return c
}
