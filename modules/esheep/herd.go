package esheep

import (
	"math/rand"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"fyshos.com/tyde"
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

// herd owns the simulation: the live sheep, the canvas images they animate and
// the ticker goroutine that drives them. The images are handed to the
// compositor as WindowAccessory items (see WindowAccessories) so they stack at
// the z-level of the window each sheep stands on.
type herd struct {
	poses *poses
	rng   *rand.Rand
	world *world
	sheep []*sheep

	redropTimer float32

	stepPending atomic.Bool // a step is queued on the render thread but not yet run

	done    chan struct{}
	started bool
}

func newHerd() *herd {
	return &herd{
		poses: sheepPoses(),
		rng:   rand.New(rand.NewSource(time.Now().UnixNano())),
		done:  make(chan struct{}),
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
			// Don't accumulate draws whilst we are asleep.
			if !h.stepPending.CompareAndSwap(false, true) {
				continue
			}
			fyne.Do(func() {
				h.stepPending.Store(false)
				h.step()
			})
		}
	}
}

// step advances every sheep one timestep, updates their images and asks the
// compositor to re-stack them at their windows' z-levels.
func (h *herd) step() {
	h.world = buildWorld()
	h.maybeRedrop()
	h.detectHops()
	for _, s := range h.sheep {
		s.advance(frameDur, h.world, h.rng)
	}
	h.updateImages()
	if inst := tyde.Instance(); inst != nil {
		inst.RefreshWindowAccessories()
	}
}

// WindowAccessories returns the live sheep (and any daisy plants) as decorations
// anchored to the window each one stands on. Dead/gone sheep are omitted so the
// compositor drops them.
func (h *herd) WindowAccessories() []tyde.WindowAccessory {
	var acc []tyde.WindowAccessory
	for _, s := range h.sheep {
		if s.state == stateGone || s.img == nil {
			continue
		}
		acc = append(acc, tyde.WindowAccessory{Object: s.img, Window: s.win})
		if s.wantDaisy && s.daisy != nil {
			acc = append(acc, tyde.WindowAccessory{Object: s.daisy, Window: s.win})
		}
	}
	return acc
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
// called on the main goroutine (it is, via fyne.Do in loop). The compositor owns
// parenting of the images; here we only create/position/animate them. Positions
// are relative to the window a sheep stands on. Membership (which images are drawn,
// and at what z-level) is reported by WindowAccessories.
func (h *herd) updateImages() {
	for _, s := range h.sheep {
		frames := s.currentFrames(h.poses)
		if len(frames) == 0 {
			continue // dead and gone: omitted from WindowAccessories
		}
		img := frames[s.frame%len(frames)]

		if s.img == nil {
			s.img = canvas.NewImageFromImage(img)
			s.img.ScaleMode = canvas.ImageScalePixels
			s.img.Resize(fyne.NewSize(spriteSize, spriteSize))
		} else if s.img.Image != img {
			s.img.Image = img
			s.img.Refresh()
		}
		s.img.Move(s.drawPos(s.x, s.y))

		h.updateDaisy(s)
	}
}

func (h *herd) updateDaisy(s *sheep) {
	if !s.wantDaisy || len(h.poses.daisies) == 0 {
		return
	}
	state := s.daisyState
	if state >= len(h.poses.daisies) {
		state = len(h.poses.daisies) - 1
	}
	img := h.poses.daisies[state]
	if s.daisy == nil {
		s.daisy = canvas.NewImageFromImage(img)
		s.daisy.ScaleMode = canvas.ImageScalePixels
		s.daisy.Resize(fyne.NewSize(daisySize, daisySize))
	} else if s.daisy.Image != img {
		s.daisy.Image = img
		s.daisy.Refresh()
	}
	s.daisy.Move(s.drawPos(s.daisyX, s.daisyY))
}
