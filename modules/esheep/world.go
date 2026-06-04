package esheep

import (
	"fyshos.com/tyde"
)

// ledge is a horizontal surface a sheep can stand on: either the top border of
// a window or the screen floor. All coordinates are in primary-screen canvas
// units (the same space as Window.Position()/Size()).
type ledge struct {
	y      float32 // height of the walkable surface
	x0, x1 float32 // horizontal extent [x0, x1]
	floor  bool    // true for the screen floor (turn at screen edges, never fall off)
}

// world is the per-frame snapshot of walkable surfaces and the play area.
type world struct {
	width, height float32
	ledges        []ledge // floor is always present; window tops follow
}

// buildWorld snapshots the current screen size and the top borders of every
// visible window on the active desktop. It is cheap enough to call each frame.
func buildWorld() *world {
	inst := tyde.Instance()
	w := &world{width: 1920, height: 1080}

	if inst != nil {
		if s := inst.Screens().Primary(); s != nil {
			scale := s.CanvasScale()
			if scale <= 0 {
				scale = 1
			}
			w.width = float32(s.Width) / scale
			w.height = float32(s.Height) / scale
		}
	}

	// The floor is always available so a falling sheep can never be lost.
	w.ledges = append(w.ledges, ledge{y: w.height, x0: 0, x1: w.width, floor: true})

	if inst == nil || inst.WindowManager() == nil {
		return w
	}

	desk := inst.Desktop()
	for _, win := range inst.WindowManager().Windows() {
		if win == nil || win.Iconic() || win.Fullscreened() {
			continue
		}
		if !win.Pinned() && win.Desktop() != desk {
			continue
		}
		pos := win.Position()
		sz := win.Size()
		if sz.Width <= 0 || sz.Height <= 0 {
			continue
		}
		// Keep ledges that are at least partly on screen and not above the top.
		top := pos.Y
		if top < 0 || top >= w.height {
			continue
		}
		w.ledges = append(w.ledges, ledge{y: top, x0: pos.X, x1: pos.X + sz.Width})
	}
	return w
}

// landing returns the topmost ledge crossed by a downward-moving foot between
// prevFeet and feet at the given centre x, or nil if none was crossed.
func (w *world) landing(centerX, prevFeet, feet float32) *ledge {
	var best *ledge
	for i := range w.ledges {
		l := &w.ledges[i]
		if centerX < l.x0 || centerX > l.x1 {
			continue
		}
		if l.y < prevFeet-0.001 || l.y > feet {
			continue // not crossed this frame
		}
		if best == nil || l.y < best.y {
			best = l
		}
	}
	return best
}

// supportAt returns the ledge a standing sheep rests on (centre within span and
// surface within tol of the feet), preferring the closest surface. Returns nil
// when the sheep has nothing beneath it (e.g. its window moved or closed).
func (w *world) supportAt(centerX, feet, tol float32) *ledge {
	var best *ledge
	for i := range w.ledges {
		l := &w.ledges[i]
		if centerX < l.x0 || centerX > l.x1 {
			continue
		}
		if abs32(l.y-feet) > tol {
			continue
		}
		if best == nil || abs32(l.y-feet) < abs32(best.y-feet) {
			best = l
		}
	}
	return best
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
