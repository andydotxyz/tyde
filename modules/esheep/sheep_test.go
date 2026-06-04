package esheep

import (
	"math/rand"
	"testing"
)

// floorWorld is a simple play area with just the screen floor.
func floorWorld(w, h float32) *world {
	return &world{
		width: w, height: h,
		ledges: []ledge{{y: h, x0: 0, x1: w, floor: true}},
	}
}

func TestLandingCatchesFallingFoot(t *testing.T) {
	w := floorWorld(800, 600)
	// Foot moving downward from 590 to 605 should catch the floor at 600.
	l := w.landing(400, 590, 605)
	if l == nil {
		t.Fatal("expected to land on the floor")
	}
	if !l.floor || l.y != 600 {
		t.Fatalf("unexpected ledge: %+v", l)
	}
}

func TestLandingPrefersTopmostLedge(t *testing.T) {
	w := floorWorld(800, 600)
	w.ledges = append(w.ledges, ledge{y: 300, x0: 100, x1: 500}) // a window top
	// A foot falling past y=300 within the window span lands on the window,
	// not the floor below it.
	l := w.landing(200, 250, 320)
	if l == nil || l.y != 300 {
		t.Fatalf("expected to land on window top at y=300, got %+v", l)
	}
	// Outside the window's horizontal span the window top is ignored: the foot
	// only catches once it actually reaches the floor at y=600.
	if l := w.landing(600, 250, 320); l != nil {
		t.Fatalf("expected to keep falling past the window, got %+v", l)
	}
	if l := w.landing(600, 590, 605); l == nil || !l.floor {
		t.Fatalf("expected to land on the floor, got %+v", l)
	}
}

func TestSupportLostWhenWindowMovesAway(t *testing.T) {
	w := floorWorld(800, 600)
	if s := w.supportAt(400, 600, snapTol); s == nil || !s.floor {
		t.Fatalf("expected floor support, got %+v", s)
	}
	// 50 units above any surface => unsupported.
	if s := w.supportAt(400, 550, snapTol); s != nil {
		t.Fatalf("expected no support in mid-air, got %+v", s)
	}
}

func TestFallingSheepLandsAndWalks(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	w := floorWorld(800, 600)
	s := &sheep{x: 400, y: 0, state: stateFalling}

	landed := false
	for i := 0; i < 600; i++ { // up to ~20s of simulation
		s.advance(frameDur, w, rng)
		if s.state == stateWalking || s.state == stateSitting || s.state == stateEating {
			landed = true
			break
		}
	}
	if !landed {
		t.Fatalf("sheep never landed; state=%d y=%.1f", s.state, s.y)
	}
	// Once landed it should be resting on the floor (feet at floor height).
	if got := s.feet(); got < 599 || got > 601 {
		t.Fatalf("feet not resting on floor: %.2f", got)
	}
	if s.facing == 0 {
		t.Fatal("expected a walking direction to be chosen on landing")
	}
}

func TestWalkingSheepStaysWithinScreen(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	w := floorWorld(400, 300)
	s := &sheep{x: 300, y: 300 - spriteSize, facing: 1, state: stateWalking}
	s.resetWalkTimer(rng)

	for i := 0; i < 2000; i++ {
		s.advance(frameDur, w, rng)
		if s.x < -0.01 || s.x+spriteSize > w.width+0.01 {
			t.Fatalf("sheep left the screen: x=%.2f", s.x)
		}
		// Force it to keep walking so we exercise the edge logic.
		if s.state != stateWalking {
			s.state = stateWalking
			s.timer = 100
		}
	}
}

func TestDoomedSheepSplatsAndRespawns(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	w := floorWorld(800, 600)
	s := &sheep{x: 400, y: -40, state: stateFalling, doomed: true}

	// Falls and splats rather than walking.
	for i := 0; i < 600 && s.state != stateSplat; i++ {
		s.advance(frameDur, w, rng)
	}
	if s.state != stateSplat {
		t.Fatalf("doomed sheep did not splat; state=%d", s.state)
	}
	// While dead it must never be walking, and eventually goes (hidden).
	for i := 0; i < 200 && s.state != stateGone; i++ {
		s.advance(frameDur, w, rng)
		if s.state == stateWalking {
			t.Fatal("dead sheep started walking before respawn")
		}
	}
	if s.state != stateGone {
		t.Fatalf("splatted sheep never vanished; state=%d", s.state)
	}
	if s.currentFrames(sheepPoses()) != nil {
		t.Fatal("a gone sheep should not be drawn")
	}
	// After the gone period it reincarnates as a fresh, non-doomed faller.
	for i := 0; i < 200 && s.state == stateGone; i++ {
		s.advance(frameDur, w, rng)
	}
	if s.state != stateFalling || s.doomed {
		t.Fatalf("expected a fresh fall after death, got state=%d doomed=%v", s.state, s.doomed)
	}
}

func TestEatingUsesSideOnGrazeFrames(t *testing.T) {
	p := sheepPoses()
	// Open phase is the latter part of each bite (timer low) => the mouth-open
	// (second) graze frame, so the mouth is open as the flower is taken.
	left := &sheep{state: stateEating, facing: -1, timer: 0}
	right := &sheep{state: stateEating, facing: 1, timer: 0}
	if got := left.currentFrames(p); len(got) == 0 || &got[0] != &p.grazeLeft[1] {
		t.Fatal("left-facing eater (open) should use grazeLeft mouth-open frame")
	}
	if got := right.currentFrames(p); len(got) == 0 || &got[0] != &p.grazeRight[1] {
		t.Fatal("right-facing eater (open) should use grazeRight mouth-open frame")
	}
	// Start of the bite (timer high) => the standing, mouth-closed (first) frame.
	leftClosed := &sheep{state: stateEating, facing: -1, timer: biteDur}
	if got := leftClosed.currentFrames(p); len(got) == 0 || &got[0] != &p.grazeLeft[0] {
		t.Fatal("left-facing eater (closed) should use the standing graze frame")
	}
}

func TestEatingDepletesDaisyThenLeaves(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	w := floorWorld(800, 600)
	s := &sheep{x: 400, y: 600 - spriteSize, facing: -1, state: stateSitting, timer: 0.01}

	maxState := 0
	sawDaisy := false
	for i := 0; i < 2000 && s.state != stateWalking; i++ {
		s.advance(frameDur, w, rng)
		if s.state == stateEating {
			sawDaisy = s.wantDaisy
			if s.daisyState > maxState {
				maxState = s.daisyState
			}
		}
	}
	if !sawDaisy {
		t.Fatal("sheep never showed a daisy while eating")
	}
	if maxState != daisyStateCount-1 {
		t.Fatalf("daisy was not fully eaten: reached state %d, want %d", maxState, daisyStateCount-1)
	}
	if s.state != stateWalking {
		t.Fatalf("sheep did not resume walking after eating; state=%d", s.state)
	}
	if s.wantDaisy {
		t.Fatal("daisy should be gone once eating finished")
	}
}

func TestSheepTurnsAndKeepsFootingOnWindow(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	w := floorWorld(800, 600)
	w.ledges = append(w.ledges, ledge{y: 300, x0: 200, x1: 300}) // a window border

	s := &sheep{x: 230, y: 300 - spriteSize, facing: 1, state: stateWalking}
	s.timer = 1e6 // suppress sit/jump behaviour so we isolate edge handling

	turns := 0
	prev := s.facing
	for i := 0; i < 4000; i++ {
		s.advance(frameDur, w, rng)
		switch s.state {
		case stateWalking:
			// The core invariant: a walking sheep is always standing on something.
			if w.supportAt(s.centerX(), s.feet(), snapTol) == nil {
				t.Fatalf("walking sheep lost its footing (stepped off while turning) at x=%.1f", s.x)
			}
			if s.facing != prev {
				turns++
				prev = s.facing
			}
		default:
			// The occasional deliberate step-off is fine; put it back to keep testing.
			s.x, s.y = 250-spriteSize/2, 300-spriteSize
			s.vx, s.vy = 0, 0
			s.state, s.facing, s.timer = stateWalking, 1, 1e6
			prev = s.facing
		}
	}
	if turns < 5 {
		t.Fatalf("expected the sheep to turn at the window edges repeatedly, got %d", turns)
	}
}

func TestWindowEdgeTurnsOrFalls(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	w := floorWorld(800, 600)
	w.ledges = append(w.ledges, ledge{y: 300, x0: 200, x1: 400})
	// Walk right near the right edge of the window ledge.
	s := &sheep{x: 380, y: 300 - spriteSize, facing: 1, state: stateWalking}
	s.timer = 100 // avoid behaviour changes interfering

	turnedOrFell := false
	for i := 0; i < 200; i++ {
		prevFacing := s.facing
		s.advance(frameDur, w, rng)
		if s.state == stateFalling || s.facing != prevFacing {
			turnedOrFell = true
			break
		}
	}
	if !turnedOrFell {
		t.Fatal("sheep neither turned nor fell at the window edge")
	}
}
