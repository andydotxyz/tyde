package esheep

import (
	"math/rand"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
)

const (
	tickRate = 30                      // simulation frames per second
	frameDur = float32(1.0 / tickRate) // fixed simulation timestep

	herdMin = 6 // number of sheep to spawn
	herdMax = 9

	hopGapMin = 6.0  // a neighbour this close ahead...
	hopGapMax = 38.0 // ...up to this far triggers a hop over it

	redropMin = 7.0 // seconds between sending a sheep back up to rain down again
	redropMax = 16.0
)

// herd owns the simulation: the live sheep, the canvas container they draw
// into, and the ticker goroutine that drives them.
type herd struct {
	container *fyne.Container
	poses     *poses
	rng       *rand.Rand
	world     *world
	sheep     []*sheep

	redropTimer float32

	done    chan struct{}
	started bool
}

func newHerd() *herd {
	return &herd{
		container: container.NewWithoutLayout(),
		poses:     sheepPoses(),
		rng:       rand.New(rand.NewSource(time.Now().UnixNano())),
		done:      make(chan struct{}),
	}
}

// start populates the herd and launches the simulation loop. It is safe to call
// once; subsequent calls are ignored.
func (h *herd) start() {
	if h.started {
		return
	}
	h.started = true

	if len(h.poses.walkLeft) == 0 {
		return // sprite sheet failed to load; nothing to animate
	}

	h.world = buildWorld()
	n := herdMin + h.rng.Intn(herdMax-herdMin+1)
	for i := 0; i < n; i++ {
		h.sheep = append(h.sheep, h.spawn(i))
	}
	h.redropTimer = redropMin + h.rng.Float32()*(redropMax-redropMin)

	go h.loop()
}

// spawn creates a sheep above the screen so it rains down and lands. Sheep are
// staggered vertically so they do not all arrive at once.
func (h *herd) spawn(index int) *sheep {
	w := h.world
	x := h.rng.Float32() * (w.width - spriteSize)
	y := -spriteSize - float32(index)*spriteSize*1.8 - h.rng.Float32()*spriteSize
	return &sheep{x: x, y: y, state: stateFalling}
}

func (h *herd) stop() {
	select {
	case <-h.done:
		// already closed
	default:
		close(h.done)
	}
}

func (h *herd) loop() {
	t := time.NewTicker(time.Second / tickRate)
	defer t.Stop()
	for {
		select {
		case <-h.done:
			return
		case <-t.C:
			fyne.Do(h.step)
		}
	}
}

// step advances every sheep one timestep and redraws them.
func (h *herd) step() {
	h.world = buildWorld()
	h.maybeRedrop()
	h.detectHops()
	for _, s := range h.sheep {
		s.advance(frameDur, h.world, h.rng)
	}
	h.render()
}

// maybeRedrop occasionally sends a grounded sheep back above the screen so it
// rains down again. This keeps the "falling from above" charm going and lets
// sheep land on the borders of windows opened after startup.
func (h *herd) maybeRedrop() {
	h.redropTimer -= frameDur
	if h.redropTimer > 0 {
		return
	}
	h.redropTimer = redropMin + h.rng.Float32()*(redropMax-redropMin)

	// Pick a random sheep that is calmly on the ground (not already airborne,
	// and not mid-death).
	var grounded []*sheep
	for _, s := range h.sheep {
		switch s.state {
		case stateWalking, stateSitting, stateEating:
			grounded = append(grounded, s)
		}
	}
	if len(grounded) == 0 {
		return
	}
	// A re-drop may be a fatal plunge - that is the occasional splat.
	grounded[h.rng.Intn(len(grounded))].launchFromTop(h.world, h.rng, true)
}

// detectHops asks a walking sheep to jump when another sheep is just ahead of
// it, producing the classic "leap over each other" behaviour.
func (h *herd) detectHops() {
	for _, s := range h.sheep {
		if s.state != stateWalking || s.hopCD > 0 {
			continue
		}
		for _, o := range h.sheep {
			if o == s || o.state == stateGone || o.state == stateSplat {
				continue
			}
			if abs32(o.feet()-s.feet()) > spriteSize*0.5 {
				continue
			}
			ahead := (o.centerX() - s.centerX()) * float32(s.facing)
			if ahead >= hopGapMin && ahead <= hopGapMax {
				s.requestHop = true
				break
			}
		}
	}
}

// render reconciles each sheep's logical state with its canvas objects. Must be
// called on the main goroutine (it is, via fyne.Do in loop).
func (h *herd) render() {
	for _, s := range h.sheep {
		frames := s.currentFrames(h.poses)
		if len(frames) == 0 {
			// Dead and gone: hide the sprite until it respawns.
			if s.img != nil {
				s.img.Hide()
			}
			h.renderDaisy(s)
			continue
		}
		img := frames[s.frame%len(frames)]

		if s.img == nil {
			s.img = canvas.NewImageFromImage(img)
			s.img.ScaleMode = canvas.ImageScalePixels
			s.img.Resize(fyne.NewSize(spriteSize, spriteSize))
			h.container.Add(s.img)
		} else if s.img.Image != img {
			s.img.Image = img
			s.img.Refresh()
		}
		if !s.img.Visible() {
			s.img.Show()
		}
		s.img.Move(fyne.NewPos(s.x, s.y))

		h.renderDaisy(s)
	}
}

func (h *herd) renderDaisy(s *sheep) {
	frames := h.poses.daisies
	if s.wantDaisy && len(frames) > 0 {
		state := s.daisyState
		if state >= len(frames) {
			state = len(frames) - 1
		}
		img := frames[state]
		if s.daisy == nil {
			s.daisy = canvas.NewImageFromImage(img)
			s.daisy.ScaleMode = canvas.ImageScalePixels
			s.daisy.Resize(fyne.NewSize(daisySize, daisySize))
			h.container.Add(s.daisy)
		} else if s.daisy.Image != img {
			s.daisy.Image = img
			s.daisy.Refresh()
		}
		s.daisy.Move(fyne.NewPos(s.daisyX, s.daisyY))
		return
	}
	if s.daisy != nil {
		h.container.Remove(s.daisy)
		s.daisy = nil
	}
}
