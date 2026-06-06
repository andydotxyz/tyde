package esheep

import (
	"testing"

	"fyne.io/fyne/v2/test"
)

// TestHerdRenderSmoke drives the full step+image-update path (without the ticker
// fyne.Do, which a unit test cannot run) to ensure image reconciliation -
// creating sheep, swapping animation frames and spawning daisies - never panics
// and yields drawable accessories.
func TestHerdRenderSmoke(t *testing.T) {
	test.NewApp()

	h := newHerd()
	if len(h.poses.walkLeft) == 0 {
		t.Skip("sprite sheet unavailable")
	}
	h.world = buildWorld()
	for i := 0; i < 4; i++ {
		h.sheep = append(h.sheep, h.spawn(i))
	}

	for i := 0; i < 600; i++ { // ~20s of simulation
		h.world = buildWorld()
		h.detectHops()
		for _, s := range h.sheep {
			s.advance(frameDur, h.world, h.rng)
		}
		h.updateImages()
	}

	if len(h.WindowAccessories()) == 0 {
		t.Fatal("expected sheep to be reported as window accessories")
	}
}

// TestSpritesSliced verifies the embedded sheet decodes and every pose slice is
// populated with correctly-sized frames.
func TestSpritesSliced(t *testing.T) {
	p := sheepPoses()
	if len(p.walkLeft) == 0 {
		t.Fatal("sprite sheet failed to load")
	}
	if len(p.walkLeft) != len(p.walkRight) {
		t.Fatalf("walk direction frame counts differ: %d vs %d", len(p.walkLeft), len(p.walkRight))
	}
	for _, f := range p.walkLeft {
		b := f.Bounds()
		if b.Dx() != tileSize || b.Dy() != tileSize {
			t.Fatalf("unexpected frame size %dx%d", b.Dx(), b.Dy())
		}
	}
	if len(p.daisies) == 0 || len(p.tumble) == 0 || len(p.grazeLeft) == 0 || len(p.splat) == 0 {
		t.Fatal("a required pose is missing")
	}
	if len(p.grazeLeft) != len(p.grazeRight) {
		t.Fatal("mirrored graze frame counts differ")
	}
	if len(p.daisies) != daisyStateCount {
		t.Fatalf("daisy frame count %d does not match daisyStateCount %d", len(p.daisies), daisyStateCount)
	}
}
