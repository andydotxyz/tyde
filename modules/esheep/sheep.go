package esheep

import (
	"image"
	"math/rand"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"fyshos.com/tyde"
)

// Simulation constants, all in canvas units and seconds.
const (
	spriteSize = 48.0       // on-screen size of a sheep (source tiles are 40px)
	daisySize  = 48.0 * 0.7 // on-screen size of the daisy plant a sheep eats

	gravity     = 900.0 // downward acceleration
	maxFall     = 640.0 // terminal velocity
	walkSpeed   = 28.0  // horizontal walking speed
	runSpeed    = 82.0  // horizontal speed during an occasional sprint
	jumpSpeedY  = 360.0 // initial upward speed of a hop
	jumpSpeedX  = 70.0  // horizontal drift while hopping
	snapTol     = 8.0   // how far a surface may move and still carry the sheep
	fallOffOdds = 0.1   // chance of walking off a window edge (normally the sheep just turns around)

	hopCooldown = 4.5  // minimum seconds between a sheep's hops
	deathOdds   = 0.15 // chance a long fall ends in a splat rather than a happy landing

	splatDur = 1.4 // seconds the charred remains linger
	goneDur  = 1.6 // seconds a dead sheep stays gone before raining down again

	walkFrameDur   = 0.13
	runFrameDur    = 0.08 // legs cycle faster while sprinting
	tumbleFrameDur = 0.07
	splatFrameDur  = 0.35

	runDurMin = 1.5 // shortest sprint, seconds
	runDurMax = 3.5 // longest sprint, seconds

	eatFrameDur     = 0.2 // seconds per frame of the eat cycle
	daisyStateCount = 5   // daisy plant frames, full -> bare (must match daisyCells)

	// Sleeping: the sheep nods off, naps a while, then wakes the same way in
	// reverse. The frame indices below address poses.sleepLeft/Right, which is
	// laid out [nod, deeper-nod, asleep, deeper-nod, nod] (see sprites.go).
	sleepFrameDur      = 0.45 // seconds per nod-off / wake frame
	sleepHoldMin       = 5.0  // shortest nap, seconds
	sleepHoldMax       = 10.0 // longest nap, seconds
	sleepAsleepFrame   = 2    // index of the deep-sleep (held) frame
	sleepWakeLastFrame = 4    // last wake frame; past this the sheep is back up
)

type sheepState int

const (
	stateFalling sheepState = iota
	stateWalking
	stateSitting
	stateEating
	stateSleeping // nodding off, napping, then waking
	stateJumping
	stateSplat // landed fatally: showing charred remains
	stateGone  // dead and hidden, waiting to respawn
)

// sheep is one desktop pet: its physics state, behaviour state and the canvas
// image used to draw it. The advance method is pure (no canvas access) so the
// behaviour can be unit-tested; render applies the result to the canvas.
type sheep struct {
	x, y   float32
	vx, vy float32
	facing int // -1 left, +1 right

	state     sheepState
	frame     int
	frameTime float32
	timer     float32 // seconds until the next behaviour decision / state change

	wantDaisy  bool
	daisyX     float32
	daisyY     float32
	daisyState int         // index into the daisy plant frames while eating (0 = full)
	requestHop bool        // herd sets this to make the sheep hop (e.g. over a neighbour)
	hopCD      float32     // seconds remaining before this sheep may hop again
	running    bool        // sprinting: moves faster with the run gait
	runTimer   float32     // seconds left in the current sprint
	doomed     bool        // this fall will end in a splat
	win        tyde.Window // window the sheep currently stands on (nil = floor/airborne => drawn on top)
	// Where win was at the last tick.
	lastWinPos fyne.Position

	img   *canvas.Image
	daisy *canvas.Image
}

func (s *sheep) centerX() float32 { return s.x + spriteSize/2 }
func (s *sheep) feet() float32    { return s.y + spriteSize }

// setSupport records the window the sheep is standing on (nil for the floor or
// mid-air), along with where that window is right now - see rideWindow.
func (s *sheep) setSupport(win tyde.Window) {
	s.win = win
	if win == nil {
		s.lastWinPos = fyne.Position{}
		return
	}
	s.lastWinPos = win.Position()
}

// rideWindow carries the sheep along with the window it is standing on, so that
// dragging a window takes its passengers with it instead of sliding out from
// under them.
func (s *sheep) rideWindow() {
	if s.win == nil {
		return
	}
	pos := s.win.Position()
	if pos == s.lastWinPos {
		return
	}

	s.x += pos.X - s.lastWinPos.X
	s.y += pos.Y - s.lastWinPos.Y
	s.daisyX += pos.X - s.lastWinPos.X
	s.daisyY += pos.Y - s.lastWinPos.Y
	s.lastWinPos = pos
}

// drawPos converts a world position into the coordinates its accessory is drawn
// in: relative to the window the sheep stands on, or the screen when it has none.
// It offsets by the position rideWindow last saw, so a window moved mid-frame
// doesn't shunt the sheep off it until the physics catch up on the next tick.
func (s *sheep) drawPos(x, y float32) fyne.Position {
	if s.win == nil {
		return fyne.NewPos(x, y)
	}
	return fyne.NewPos(x-s.lastWinPos.X, y-s.lastWinPos.Y)
}

// advance steps the sheep's logic forward by dt seconds within world w.
func (s *sheep) advance(dt float32, w *world, rng *rand.Rand) {
	s.rideWindow()
	if s.hopCD > 0 {
		s.hopCD -= dt
	}
	if s.requestHop && s.state == stateWalking && s.hopCD <= 0 {
		s.startHop(rng)
	}
	s.requestHop = false

	switch s.state {
	case stateFalling, stateJumping:
		s.advanceAir(dt, w, rng)
	case stateWalking:
		s.advanceWalk(dt, w, rng)
	case stateSitting:
		s.frameTime += dt
		s.timer -= dt
		if s.timer <= 0 {
			// Start grazing: a fresh, full daisy plant.
			s.state = stateEating
			s.daisyState = 0
			s.frame, s.frameTime = 0, 0
			s.wantDaisy = true
		}
	case stateEating:
		s.advanceEat(dt, rng)
	case stateSleeping:
		s.advanceSleep(dt, rng)
	case stateSplat:
		s.timer -= dt
		s.frameTime += dt
		if s.frameTime >= splatFrameDur {
			s.frameTime = 0
			s.frame++
		}
		if s.timer <= 0 {
			s.state = stateGone
			s.timer = goneDur
		}
	case stateGone:
		s.timer -= dt
		if s.timer <= 0 {
			s.launchFromTop(w, rng, false) // reincarnate; never doomed twice in a row
		}
	}
}

func (s *sheep) advanceAir(dt float32, w *world, rng *rand.Rand) {
	s.vy += gravity * dt
	if s.vy > maxFall {
		s.vy = maxFall
	}
	prevFeet := s.feet()
	s.x += s.vx * dt
	s.y += s.vy * dt

	// Stay within the horizontal play area, bouncing back off the edges.
	if s.x < 0 {
		s.x = 0
		s.vx = abs32(s.vx)
	} else if s.x+spriteSize > w.width {
		s.x = w.width - spriteSize
		s.vx = -abs32(s.vx)
	}

	s.frameTime += dt
	if s.frameTime >= tumbleFrameDur {
		s.frameTime = 0
		s.frame++
	}

	if s.vy > 0 {
		if l := w.landing(s.centerX(), prevFeet, s.feet()); l != nil {
			s.y = l.y - spriteSize
			s.vx, s.vy = 0, 0
			s.setSupport(l.win) // draw at this surface's z-level from now on
			if s.doomed {
				s.splat()
			} else {
				s.beginWalking(rng)
			}
		}
	}
}

// splat kills the sheep on a fatal landing: it shows charred remains, then
// vanishes (see stateGone) before raining down again later.
func (s *sheep) splat() {
	s.state = stateSplat
	s.timer = splatDur
	s.frame, s.frameTime = 0, 0
	s.wantDaisy = false
	if s.facing == 0 {
		s.facing = -1
	}
}

// startSleep settles the sheep down for a nap: it nods off, holds the asleep
// frame for a few seconds, then wakes (see advanceSleep). It keeps its window so
// it naps at that surface's z-level.
func (s *sheep) startSleep(rng *rand.Rand) {
	s.state = stateSleeping
	s.frame, s.frameTime = 0, 0
	s.wantDaisy = false
	if s.facing == 0 {
		s.facing = -1
	}
}

// advanceSleep steps the sleep animation: nod off, nap for sleepHoldMin..Max
// seconds on the asleep frame, then play the nod frames in reverse and walk off.
func (s *sheep) advanceSleep(dt float32, rng *rand.Rand) {
	s.frameTime += dt
	switch {
	case s.frame < sleepAsleepFrame:
		// Eyes drooping shut.
		if s.frameTime >= sleepFrameDur {
			s.frameTime = 0
			s.frame++
			if s.frame == sleepAsleepFrame {
				s.timer = sleepHoldMin + rng.Float32()*(sleepHoldMax-sleepHoldMin)
			}
		}
	case s.frame == sleepAsleepFrame:
		// Napping: hold the asleep frame until the timer runs out.
		s.timer -= dt
		if s.timer <= 0 {
			s.frame++ // start waking
			s.frameTime = 0
		}
	default:
		// Waking up, replaying the nod frames in reverse, then back on its feet.
		if s.frameTime >= sleepFrameDur {
			s.frameTime = 0
			s.frame++
			if s.frame > sleepWakeLastFrame {
				s.beginWalking(rng)
			}
		}
	}
}

// advanceEat steps the four-frame eat cycle: reach for the plant (frames 0,1),
// then drop the head to chew (frames 2,3). One flower comes off as the head goes
// down for each chomp; the sheep walks off once the plant is bare.
func (s *sheep) advanceEat(dt float32, rng *rand.Rand) {
	s.frameTime += dt
	if s.frameTime < eatFrameDur {
		return
	}
	s.frameTime = 0
	s.frame++
	switch {
	case s.frame == 2:
		// The bite: one daisy gone as the head goes down to chew.
		s.daisyState++
	case s.frame >= len(eatCells):
		// Chomp finished.
		s.frame = 0
		if s.daisyState >= daisyStateCount-1 {
			// The plant is bare - finished eating.
			s.wantDaisy = false
			s.beginWalking(rng)
		}
	}
}

func (s *sheep) advanceWalk(dt float32, w *world, rng *rand.Rand) {
	sup := w.supportAt(s.centerX(), s.feet(), snapTol)
	if sup == nil {
		// The surface vanished (window moved/closed): start falling. A short
		// fall like this is never fatal. Keep the window so the sheep
		// stays at that z-level as it falls (and is occluded by higher windows)
		// until it lands somewhere new.
		s.state = stateFalling
		s.vx, s.vy = 0, 0
		s.doomed = false
		s.frame, s.frameTime = 0, 0
		return
	}
	if s.running {
		s.runTimer -= dt
		if s.runTimer <= 0 {
			s.running = false
		}
	}

	s.setSupport(sup.win)
	s.y = sup.y - spriteSize
	speed := float32(walkSpeed)
	if s.running {
		speed = runSpeed
	}
	s.x += float32(s.facing) * speed * dt

	c := s.centerX()
	if sup.floor {
		if s.x < 0 {
			s.x = 0
			s.facing = 1
		} else if s.x+spriteSize > w.width {
			s.x = w.width - spriteSize
			s.facing = -1
		}
	} else if (s.facing > 0 && c > sup.x1) || (s.facing < 0 && c < sup.x0) {
		// Reached the end of a window border: usually turn back, occasionally
		// step off.
		if rng.Float32() < fallOffOdds {
			// Step off the edge: keep the window so the sheep stays at
			// that z-level (occluded by higher windows) until it lands.
			s.state = stateFalling
			s.vx = float32(s.facing) * 12
			s.vy = 0
			s.doomed = false
			s.frame, s.frameTime = 0, 0
			return
		}
		// Turn around, pulling the centre back onto the ledge so the sheep keeps
		// its footing next frame instead of stepping into thin air.
		if c > sup.x1 {
			s.x = sup.x1 - spriteSize/2
		} else {
			s.x = sup.x0 - spriteSize/2
		}
		s.facing = -s.facing
	}

	// Walk- (or run-) cycle animation.
	fd := float32(walkFrameDur)
	if s.running {
		fd = runFrameDur
	}
	s.frameTime += dt
	if s.frameTime >= fd {
		s.frameTime = 0
		s.frame++
	}

	// Behaviour decisions.
	s.timer -= dt
	if s.timer <= 0 {
		s.chooseBehaviour(rng)
	}
}

// chooseBehaviour is called periodically while walking to add some variety.
// Hopping is deliberately rare (and rate-limited via hopCD) so the herd does
// not constantly bounce around.
func (s *sheep) chooseBehaviour(rng *rand.Rand) {
	switch r := rng.Float32(); {
	case r < 0.08:
		// Settle down for a nap.
		s.startSleep(rng)
	case r < 0.42:
		// Settle in front of a fresh daisy plant, then eat it (see advanceEat).
		// The plant appears now, during the brief pause, so it is already there
		// before the sheep first opens its mouth to bite.
		s.state = stateSitting
		s.timer = 0.5 + rng.Float32()*0.4
		s.frame, s.frameTime = 0, 0
		s.wantDaisy = true
		s.daisyState = 0
		// Place the daisy plant just in front of the sheep, tucked in slightly so
		// the snout reaches it without the plant covering the body. The plant is
		// drawn at daisySize and rests on the ground - see renderDaisy.
		const daisyInset = spriteSize * 0.12
		if s.facing < 0 {
			s.daisyX = s.x - daisySize + daisyInset
		} else {
			s.daisyX = s.x + spriteSize - daisyInset
		}
		s.daisyY = s.feet() - daisySize
	case r < 0.48 && s.hopCD <= 0:
		s.startHop(rng)
	case r < 0.62:
		s.facing = -s.facing
		s.resetWalkTimer(rng)
	case r < 0.74:
		// Break into a sprint for a little while.
		s.running = true
		s.runTimer = runDurMin + rng.Float32()*(runDurMax-runDurMin)
		s.resetWalkTimer(rng)
	default:
		s.resetWalkTimer(rng)
	}
}

func (s *sheep) startHop(rng *rand.Rand) {
	s.state = stateJumping
	s.vy = -jumpSpeedY
	s.vx = float32(s.facing) * jumpSpeedX
	// Keep the current window: a hop happens on (or just above) the
	// surface the sheep is standing on, so it stays at that window's z-level and
	// is still occluded by windows above it - unlike a fall from the sky.
	s.frame, s.frameTime = 0, 0
	s.hopCD = hopCooldown + rng.Float32()*2
}

// launchFromTop sends the sheep above the screen to rain down again. When
// canDie is true the fall has a chance of ending in a splat.
func (s *sheep) launchFromTop(w *world, rng *rand.Rand, canDie bool) {
	s.wantDaisy = false
	s.x = rng.Float32() * (w.width - spriteSize)
	s.y = -spriteSize - rng.Float32()*spriteSize*2
	s.vx, s.vy = 0, 0
	s.facing = 0
	s.state = stateFalling
	s.setSupport(nil) // airborne => drawn on top
	s.frame, s.frameTime = 0, 0
	s.doomed = canDie && rng.Float32() < deathOdds
}

// beginWalking settles the sheep into a walk after landing.
func (s *sheep) beginWalking(rng *rand.Rand) {
	s.state = stateWalking
	s.running, s.runTimer = false, 0
	if s.facing == 0 {
		s.facing = 1
		if rng.Float32() < 0.5 {
			s.facing = -1
		}
	}
	s.frame, s.frameTime = 0, 0
	s.resetWalkTimer(rng)
}

func (s *sheep) resetWalkTimer(rng *rand.Rand) {
	s.timer = 2.5 + rng.Float32()*3.5
}

// currentFrames returns the animation frames for the sheep's current state.
// A nil result means the sheep should not be drawn (it is dead and gone).
func (s *sheep) currentFrames(p *poses) []image.Image {
	left := s.facing < 0
	pick := func(l, r []image.Image) []image.Image {
		if left {
			return l
		}
		return r
	}
	switch s.state {
	case stateFalling, stateJumping:
		return p.tumble
	case stateSleeping:
		return pick(p.sleepLeft, p.sleepRight)
	case stateSplat:
		return p.splat
	case stateGone:
		return nil
	case stateSitting:
		// Standing over the daisy with mouth closed, before leaning in to bite.
		return pick(p.walkLeft, p.walkRight)[1:]
	case stateEating:
		// The full eat cycle (reach, bite, chew, chew); s.frame indexes it.
		return pick(p.eatLeft, p.eatRight)
	default:
		if s.running {
			return pick(p.runLeft, p.runRight)
		}
		return pick(p.walkLeft, p.walkRight)
	}
}
