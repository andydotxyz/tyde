package esheep

import (
	"image"
	"math/rand"

	"fyne.io/fyne/v2/canvas"
)

// Simulation constants, all in canvas units and seconds.
const (
	spriteSize = 48.0       // on-screen size of a sheep (source tiles are 40px)
	daisySize  = 48.0 * 0.7 // on-screen size of the daisy plant a sheep eats

	gravity     = 900.0 // downward acceleration
	maxFall     = 640.0 // terminal velocity
	walkSpeed   = 34.0  // horizontal walking speed
	jumpSpeedY  = 360.0 // initial upward speed of a hop
	jumpSpeedX  = 70.0  // horizontal drift while hopping
	snapTol     = 8.0   // how far a surface may move and still carry the sheep
	fallOffOdds = 0.1   // chance of walking off a window edge (normally the sheep just turns around)

	hopCooldown = 4.5  // minimum seconds between a sheep's hops
	deathOdds   = 0.15 // chance a long fall ends in a splat rather than a happy landing

	splatDur = 1.4 // seconds the charred remains linger
	goneDur  = 1.6 // seconds a dead sheep stays gone before raining down again

	walkFrameDur   = 0.13
	tumbleFrameDur = 0.07
	splatFrameDur  = 0.35

	biteDur         = 0.5 // seconds per chomp (one daisy eaten per chomp)
	daisyStateCount = 5   // daisy plant frames, full -> bare (must match daisyCells)
)

type sheepState int

const (
	stateFalling sheepState = iota
	stateWalking
	stateSitting
	stateEating
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
	daisyState int     // index into the daisy plant frames while eating (0 = full)
	requestHop bool    // herd sets this to make the sheep hop (e.g. over a neighbour)
	hopCD      float32 // seconds remaining before this sheep may hop again
	doomed     bool    // this fall will end in a splat

	img   *canvas.Image
	daisy *canvas.Image
}

func (s *sheep) centerX() float32 { return s.x + spriteSize/2 }
func (s *sheep) feet() float32    { return s.y + spriteSize }

// advance steps the sheep's logic forward by dt seconds within world w.
func (s *sheep) advance(dt float32, w *world, rng *rand.Rand) {
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
			s.timer = biteDur
			s.daisyState = 0
			s.frame, s.frameTime = 0, 0
			s.wantDaisy = true
		}
	case stateEating:
		s.timer -= dt
		if s.timer <= 0 {
			if s.daisyState >= daisyStateCount-1 {
				// The plant is bare - finished eating.
				s.wantDaisy = false
				s.beginWalking(rng)
				return
			}
			s.daisyState++ // chomp: one daisy gone
			s.timer = biteDur
		}
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

func (s *sheep) advanceWalk(dt float32, w *world, rng *rand.Rand) {
	sup := w.supportAt(s.centerX(), s.feet(), snapTol)
	if sup == nil {
		// The surface vanished (window moved/closed): start falling. A short
		// fall like this is never fatal.
		s.state = stateFalling
		s.vx, s.vy = 0, 0
		s.doomed = false
		s.frame, s.frameTime = 0, 0
		return
	}
	s.y = sup.y - spriteSize
	s.x += float32(s.facing) * walkSpeed * dt

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

	// Walk-cycle animation.
	s.frameTime += dt
	if s.frameTime >= walkFrameDur {
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
	case r < 0.34:
		// Sit down and eat a daisy.
		s.state = stateSitting
		s.timer = 0.5 + rng.Float32()*0.4
		s.frame, s.frameTime = 0, 0
		s.wantDaisy = false
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
	case r < 0.40 && s.hopCD <= 0:
		s.startHop(rng)
	case r < 0.54:
		s.facing = -s.facing
		s.resetWalkTimer(rng)
	default:
		s.resetWalkTimer(rng)
	}
}

func (s *sheep) startHop(rng *rand.Rand) {
	s.state = stateJumping
	s.vy = -jumpSpeedY
	s.vx = float32(s.facing) * jumpSpeedX
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
	s.frame, s.frameTime = 0, 0
	s.doomed = canDie && rng.Float32() < deathOdds
}

// beginWalking settles the sheep into a walk after landing.
func (s *sheep) beginWalking(rng *rand.Rand) {
	s.state = stateWalking
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
	case stateSplat:
		return p.splat
	case stateGone:
		return nil
	case stateSitting:
		// First graze frame (head up), facing the daisy.
		return pick(p.grazeLeft, p.grazeRight)[:1]
	case stateEating:
		// Each bite: the head dips and the mouth opens (medium) for the first
		// part of the chomp, then closes. Both frames share the same feet, so
		// nothing bobs - only the head and mouth move.
		// Each bite: stand briefly, then dip the head and open the mouth for the
		// rest of the chomp, so the mouth is open as the flower is taken (the
		// daisy is consumed when timer reaches 0).
		g := pick(p.grazeLeft, p.grazeRight)
		if s.timer < biteDur*0.65 {
			return g[1:] // mouth open (head down on the plant) - the bite
		}
		return g[:1] // mouth closed (standing) - brief, at the start of the bite
	default:
		return pick(p.walkLeft, p.walkRight)
	}
}
