package ui

import "fyne.io/fyne/v2/canvas"

// overviewShaderGL and overviewShaderES drive the "reveal all" desktop overview.
// All virtual desktops are stitched into one tall texture ("strip"), desk0 at the
// top, each desktop occupying a screen-sized slice. The shader is a 2D camera over
// that strip that also arranges the slices into a grid: the "progress" uniform
// zooms from a single desktop filling the view (0) out to every desktop laid out in
// a cols-wide grid with a gap between cells (1).
//
// The camera works in "desktop units" where one unit is the height of a single
// desktop and the horizontal extent is the view aspect, so pixels stay square and
// each desktop keeps its aspect ratio. Cell (col,row) sits at
// (col*(aspect+gap), row*(1+gap)); the gaps read as the dark background. At
// progress 0 the view is the "focus" cell exactly; at progress 1 it frames the
// whole grid, centred, with a small margin. The fully-zoomed-out framing does not
// depend on focus, so the focus uniform can change between the zoom-out (entering,
// focused on the current desktop) and the zoom-in (leaving, focused on the chosen
// desktop) without any visible jump.
//
// The overviewMargin and overviewGap constants below MUST match the Go constants of
// the same value in overview.go so the interactive selection panels line up exactly
// with the rendered desktops.
//
// overviewShaderGL targets desktop OpenGL (core profile); overviewShaderES targets
// OpenGL ES / mobile / web. They are identical apart from the version preamble.
// The bounds layout (x1, y1, x2, y2) matches the cube transition shader.
var overviewShaderGL = []byte(`#version 110

uniform vec2 frame;
uniform vec4 bounds;      // x1, y1, x2, y2 in pixels (canvas-top origin)
uniform float progress;   // 0 = focus desktop fills the view, 1 = whole grid shown
uniform float count;      // number of virtual desktops
uniform float cols;       // grid columns
uniform float gap;        // gap between cells, in desktop-height units
uniform float focus;      // index of the desktop framed at progress 0
uniform sampler2D strip;  // all desktops stacked vertically, desk0 at the top

const float MARGIN = 1.08; // extra view around the grid at full zoom-out

void main() {
    float w = bounds[2] - bounds[0];
    float h = bounds[3] - bounds[1];
    float aspect = w / h;
    float canvasY = frame.y - gl_FragCoord.y;
    vec2 local = vec2(gl_FragCoord.x - bounds[0], canvasY - bounds[1]);
    vec2 c = (local / vec2(w, h)) * 2.0 - 1.0; // -1..1, y positive downward

    float p = smoothstep(0.0, 1.0, clamp(progress, 0.0, 1.0));

    float rows = ceil(count / cols);
    float pitchX = aspect + gap;
    float pitchY = 1.0 + gap;
    float totalW = cols * aspect + (cols - 1.0) * gap;
    float totalH = rows * 1.0 + (rows - 1.0) * gap;

    float fCol = mod(focus, cols);
    float fRow = floor(focus / cols);

    float zoomOverview = max(totalH * 0.5, totalW / (2.0 * aspect)) * MARGIN;
    float zoomH = mix(0.5, zoomOverview, p);
    float cx = mix(fCol * pitchX + aspect * 0.5, totalW * 0.5, p);
    float cy = mix(fRow * pitchY + 0.5, totalH * 0.5, p);

    float wx = cx + c.x * zoomH * aspect;
    float wy = cy + c.y * zoomH;

    float colIdx = floor(wx / pitchX);
    float rowIdx = floor(wy / pitchY);
    float localX = wx - colIdx * pitchX;
    float localY = wy - rowIdx * pitchY;
    float d = rowIdx * cols + colIdx;

    vec4 col = vec4(0.0, 0.0, 0.0, 1.0); // dark background, also fills the gaps
    if (colIdx >= 0.0 && colIdx < cols && rowIdx >= 0.0 && rowIdx < rows && d < count &&
        localX >= 0.0 && localX <= aspect && localY >= 0.0 && localY <= 1.0) {
        vec2 uv = vec2(localX / aspect, (d + localY) / count);
        col = vec4(texture2D(strip, uv).rgb, 1.0);
    }
    gl_FragColor = col;
}
`)

var overviewShaderES = []byte(`#version 100

#ifdef GL_ES
# ifdef GL_FRAGMENT_PRECISION_HIGH
precision highp float;
# else
precision mediump float;
#endif
#endif

uniform vec2 frame;
uniform vec4 bounds;
uniform float progress;
uniform float count;
uniform float cols;
uniform float gap;
uniform float focus;
uniform sampler2D strip;

const float MARGIN = 1.08;

void main() {
    float w = bounds[2] - bounds[0];
    float h = bounds[3] - bounds[1];
    float aspect = w / h;
    float canvasY = frame.y - gl_FragCoord.y;
    vec2 local = vec2(gl_FragCoord.x - bounds[0], canvasY - bounds[1]);
    vec2 c = (local / vec2(w, h)) * 2.0 - 1.0;

    float p = smoothstep(0.0, 1.0, clamp(progress, 0.0, 1.0));

    float rows = ceil(count / cols);
    float pitchX = aspect + gap;
    float pitchY = 1.0 + gap;
    float totalW = cols * aspect + (cols - 1.0) * gap;
    float totalH = rows * 1.0 + (rows - 1.0) * gap;

    float fCol = mod(focus, cols);
    float fRow = floor(focus / cols);

    float zoomOverview = max(totalH * 0.5, totalW / (2.0 * aspect)) * MARGIN;
    float zoomH = mix(0.5, zoomOverview, p);
    float cx = mix(fCol * pitchX + aspect * 0.5, totalW * 0.5, p);
    float cy = mix(fRow * pitchY + 0.5, totalH * 0.5, p);

    float wx = cx + c.x * zoomH * aspect;
    float wy = cy + c.y * zoomH;

    float colIdx = floor(wx / pitchX);
    float rowIdx = floor(wy / pitchY);
    float localX = wx - colIdx * pitchX;
    float localY = wy - rowIdx * pitchY;
    float d = rowIdx * cols + colIdx;

    vec4 col = vec4(0.0, 0.0, 0.0, 1.0);
    if (colIdx >= 0.0 && colIdx < cols && rowIdx >= 0.0 && rowIdx < rows && d < count &&
        localX >= 0.0 && localX <= aspect && localY >= 0.0 && localY <= 1.0) {
        vec2 uv = vec2(localX / aspect, (d + localY) / count);
        col = vec4(texture2D(strip, uv).rgb, 1.0);
    }
    gl_FragColor = col;
}
`)

// newOverviewShader builds the hidden full-window overview shader. It shares the
// same uniform conventions as the cube transition; each screen drives its own
// stitched desktop strip.
func newOverviewShader() *canvas.Shader {
	s := canvas.NewShader("tydeDeskOverview", overviewShaderGL, overviewShaderES)
	s.Uniforms = map[string]float32{"progress": 0, "count": 0, "cols": 1, "gap": 0, "focus": 0}
	s.Hide()
	return s
}
