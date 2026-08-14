package esheep

import (
	"math/rand"
	"testing"

	wmtest "fyshos.com/tyde/test"
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
		t.Fatalf("expected no win in mid-air, got %+v", s)
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

func TestEatingUsesEatCycleAndRemovesOneFlowerPerChomp(t *testing.T) {
	p := sheepPoses()
	// The eat cycle uses the dedicated eat frames, mirrored by facing.
	left := &sheep{state: stateEating, facing: -1}
	right := &sheep{state: stateEating, facing: 1}
	if got := left.currentFrames(p); len(got) != len(p.eatLeft) || &got[0] != &p.eatLeft[0] {
		t.Fatal("left-facing eater should use the eatLeft cycle")
	}
	if got := right.currentFrames(p); &got[0] != &p.eatRight[0] {
		t.Fatal("right-facing eater should use the eatRight cycle")
	}

	// One chomp removes exactly one flower, and it comes off as the head drops to
	// chew (frame 2), not during the reach (frames 0,1).
	rng := rand.New(rand.NewSource(1))
	s := &sheep{state: stateEating, facing: -1, wantDaisy: true}
	before := s.daisyState
	for s.daisyState == before {
		s.advance(frameDur, w0, rng)
		if s.daisyState == before && s.frame >= 2 {
			t.Fatal("flower removed too early: head not yet down to chew")
		}
	}
	if s.daisyState != before+1 {
		t.Fatalf("a chomp should remove exactly one flower: %d -> %d", before, s.daisyState)
	}
	if s.frame != 2 {
		t.Fatalf("flower should come off as the head drops (frame 2), was frame %d", s.frame)
	}
}

// w0 is a throwaway floor world for eat-cycle tests (the sheep does not move).
var w0 = floorWorld(800, 600)

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

func TestRunningSheepOutpacesWalkAndUsesRunGait(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	w := floorWorld(4000, 600)
	walk := &sheep{x: 100, y: 600 - spriteSize, facing: 1, state: stateWalking, timer: 1e6}
	run := &sheep{x: 100, y: 600 - spriteSize, facing: 1, state: stateWalking, timer: 1e6, running: true, runTimer: 1e6}

	for i := 0; i < tickRate; i++ { // ~1s
		walk.advance(frameDur, w, rng)
		run.advance(frameDur, w, rng)
	}
	if run.x <= walk.x {
		t.Fatalf("running sheep should outpace a walking one: walk=%.1f run=%.1f", walk.x, run.x)
	}

	p := sheepPoses()
	if got := run.currentFrames(p); &got[0] != &p.runRight[0] {
		t.Fatal("running sheep should use the run gait frames")
	}
	if got := walk.currentFrames(p); &got[0] != &p.walkRight[0] {
		t.Fatal("walking sheep should use the walk gait frames")
	}

	// A sprint is finite: once its timer runs out the sheep drops back to a walk.
	run.running, run.runTimer = true, 0.1
	for i := 0; i < tickRate && run.running; i++ {
		run.advance(frameDur, w, rng)
	}
	if run.running {
		t.Fatal("sprint should end after its timer expires")
	}
}

func TestSleepingSheepNapsThenWakes(t *testing.T) {
	rng := rand.New(rand.NewSource(13))
	w := floorWorld(800, 600)
	s := &sheep{x: 400, y: 600 - spriteSize, facing: -1, state: stateSleeping}

	// It nods off and reaches (and holds on) the deep-sleep frame.
	for i := 0; i < 30 && s.frame != sleepAsleepFrame; i++ {
		s.advance(frameDur, w, rng)
	}
	if s.state != stateSleeping || s.frame != sleepAsleepFrame {
		t.Fatalf("sheep never reached the deep-sleep frame; state=%d frame=%d", s.state, s.frame)
	}
	// The asleep frame is the head-resting pose, and it does not drift.
	if got := s.currentFrames(sheepPoses()); &got[s.frame] != &sheepPoses().sleepLeft[sleepAsleepFrame] {
		t.Fatal("deep sleep should show the asleep frame")
	}

	// The nap holds for at least sleepHoldMin seconds before the sheep stirs.
	napStart := s.x
	naps := 0
	for s.frame == sleepAsleepFrame && s.state == stateSleeping {
		s.advance(frameDur, w, rng)
		naps++
		if naps > int((sleepHoldMax+5)*tickRate) {
			t.Fatal("sheep never woke from its nap")
		}
	}
	if napped := float32(naps) * frameDur; napped < sleepHoldMin-frameDur {
		t.Fatalf("nap too short: %.2fs (want >= %.1fs)", napped, sleepHoldMin)
	}
	if s.x != napStart {
		t.Fatalf("a sleeping sheep should not move: %.1f -> %.1f", napStart, s.x)
	}

	// It finishes waking and walks off.
	for i := 0; i < 30 && s.state == stateSleeping; i++ {
		s.advance(frameDur, w, rng)
	}
	if s.state != stateWalking {
		t.Fatalf("sheep did not wake into a walk; state=%d", s.state)
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

// TestSupportZLevelFollowsSurface verifies the sheep keeps a reference to the
// window it is standing on while walking and hopping (so the compositor draws it
// at that window's z-level and higher windows occlude it), and that only a fresh
// fall from the sky detaches that reference (drawn on top).
func TestSupportZLevelFollowsSurface(t *testing.T) {
	rng := rand.New(rand.NewSource(9))
	win := wmtest.NewWindow("w")
	w := floorWorld(800, 600)
	w.ledges = append(w.ledges, ledge{y: 300, x0: 200, x1: 400, win: win})

	s := &sheep{x: 300, y: 300 - spriteSize, facing: 1, state: stateWalking}
	s.timer = 1e6 // suppress behaviour changes so we isolate win tracking
	s.advance(frameDur, w, rng)
	if s.win != win {
		t.Fatalf("a walking sheep should anchor to its window, got %v", s.win)
	}

	// Hopping happens on the surface, so it stays at the window's z-level.
	s.startHop(rng)
	if s.win != win {
		t.Fatalf("a hopping sheep should stay anchored to its window, got %v", s.win)
	}

	// A fresh fall from above is not yet on any window => drawn on top.
	s.launchFromTop(w, rng, false)
	if s.win != nil {
		t.Fatalf("a sky drop should draw on top (nil support), got %v", s.win)
	}
}

// TestSheepRidesItsWindow verifies that a sheep standing on a window travels
// with it when it is dragged (rather than the surface sliding out from under it)
// and that it is drawn at a position relative to that window, as the compositor
// expects of a window accessory.
func TestSheepRidesItsWindow(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	win := wmtest.NewWindow("w")
	win.SetGeometry(200, 300, 200, 150)

	w := floorWorld(800, 600)
	w.ledges = append(w.ledges, ledge{y: 300, x0: 200, x1: 400, win: win})

	s := &sheep{x: 300, y: 300 - spriteSize, facing: 1, state: stateWalking}
	s.timer = 1e6 // suppress behaviour changes so we isolate the carrying
	s.advance(frameDur, w, rng)
	if s.win != win {
		t.Fatalf("expected the sheep to stand on the window, got %v", s.win)
	}
	drawn := s.drawPos(s.x, s.y)
	if drawn.X != s.x-200 || drawn.Y != s.y-300 {
		t.Fatalf("expected a position relative to the window, got %v for %v,%v", drawn, s.x, s.y)
	}

	// Drag the window 60 right and 20 down; its ledge moves with it.
	win.SetGeometry(260, 320, 200, 150)
	w = floorWorld(800, 600)
	w.ledges = append(w.ledges, ledge{y: 320, x0: 260, x1: 460, win: win})

	before := s.x
	s.advance(frameDur, w, rng)
	if s.state != stateWalking {
		t.Fatalf("the sheep fell off a window it was carried by, state %v", s.state)
	}
	if moved := s.x - before; moved < 60 {
		t.Fatalf("expected the sheep to be carried 60 to the right, it moved %v", moved)
	}
	if drawn := s.drawPos(s.x, s.y); drawn.X < 0 || drawn.X > 200 {
		t.Fatalf("expected the sheep to stay on its window, drawn at %v", drawn)
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
