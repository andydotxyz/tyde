package ui

// welcomeWaveBody is the fragment shader shared between the desktop and ES
// variants. It fills the welcome splash panel with the FyshOS brand water as a
// stack of undulating horizontal bands, each a flatter shade of blue - pale at
// the surface, through teal, to deep blue below - to match the layered waves in
// theme/assets/logo_fade.png rather than white crests on a flat background. Each
// band's wavy top edge gently scrolls, and the "reveal" uniform fades the bands
// in from the pale surface tone (in place, no slide) as the panel animates on;
// "time" keeps them undulating afterwards.
//
// It follows the uniform contract used by the built-in vector shaders and the
// flames/cube shaders in this project, so it is driven the same way: bounds
// gives the object's bounds (canvas-top origin), letting the shader confine
// itself to the panel like the built-in shapes do.
//
// The version header (and, for ES, the precision preamble) is the only thing
// that differs between targets, so it is prepended below rather than duplicated.
const welcomeWaveBody = `
uniform vec2 frame;        // size of the output frame, in pixels
uniform vec4 bounds;       // this object's bounds: x1 [0], y1 [1], x2 [2], y2 [3]
uniform float time;        // elapsed animation time, in seconds
uniform float reveal;      // 0..1 wash-in: bands fade in from the surface (no slide)
uniform float fade;        // >0: underlay mode - square edges, top fades to transparent

// bandColor maps a 0..1 depth (0 surface -> 1 floor) to the FyshOS water palette
// sampled from logo_fade.png: pale at the top, through sky and teal, to deep blue.
vec3 bandColor(float s) {
    vec3 pale = vec3(0.91, 0.95, 0.99);
    vec3 sky  = vec3(0.62, 0.84, 0.94);
    vec3 teal = vec3(0.36, 0.71, 0.85);
    vec3 blue = vec3(0.20, 0.53, 0.82);
    vec3 deep = vec3(0.11, 0.39, 0.74);
    if (s < 0.25) return mix(pale, sky,  s / 0.25);
    if (s < 0.50) return mix(sky,  teal, (s - 0.25) / 0.25);
    if (s < 0.75) return mix(teal, blue, (s - 0.50) / 0.25);
    return mix(blue, deep, (s - 0.75) / 0.25);
}

void main() {
    // Discard anything outside this object's bounds, like the built in shapes.
    if (gl_FragCoord.x < bounds[0] || gl_FragCoord.x > bounds[2] ||
        gl_FragCoord.y < frame.y - bounds[3] ||
        gl_FragCoord.y > frame.y - bounds[1]) {
        discard;
    }

    float w = bounds[2] - bounds[0];
    float h = bounds[3] - bounds[1];

    // Local coordinates within the rect: uv.x 0..1 left->right, uv.y 0..1
    // top->bottom (bounds is canvas-top origin, gl_FragCoord.y grows upward).
    float lx = gl_FragCoord.x - bounds[0];
    float ly = (frame.y - gl_FragCoord.y) - bounds[1];
    vec2 uv = vec2(lx / w, ly / h);

    // Rounded corners: fade to transparent outside a rounded-rectangle so the
    // panel reads as a soft card floating over the dimmed desktop rather than a
    // hard-edged box. edge feeds the final alpha for a 1px anti-aliased border.
    // In underlay mode (fade>0) the band meets the window edges square instead.
    float edge = 1.0;
    if (fade <= 0.5) {
        float radius = 22.0;
        vec2 corner = clamp(vec2(lx, ly), vec2(radius), vec2(w - radius, h - radius));
        float cornerDist = distance(vec2(lx, ly), corner);
        edge = 1.0 - smoothstep(radius - 1.0, radius + 1.0, cornerDist);
        if (edge <= 0.0) {
            discard;
        }
    }

    // Build the water by painting a stack of bands over the palest surface tone.
    // Each band fills from its wavy top edge downwards; lower (front) bands
    // overwrite the ones behind, so their undulating edges read as layered waves.
    vec3 col = bandColor(0.0);

    const int N = 7;
    for (int i = 0; i < N; i++) {
        float fi = float(i);
        float s = fi / float(N - 1);                 // 0..1 palette position
        float baseY = 0.08 + 0.84 * s;               // resting top edge of band
        float amp = 0.015 + 0.004 * fi;              // lower bands swell a little more
        float freq = 6.2831 * (0.6 + 0.16 * fi);     // long, gentle humps
        float speed = 0.15 + 0.08 * fi;
        float ph = fi * 1.7;
        // Two summed sines give an organic, non-repeating undulation.
        float yEdge = baseY
            + amp * sin(uv.x * freq + time * speed + ph)
            + amp * 0.4 * sin(uv.x * freq * 1.9 - time * speed * 0.7 + ph * 1.3);
        // Soft edge so the band silhouettes are anti-aliased rather than jagged.
        float cover = smoothstep(-0.005, 0.005, uv.y - yEdge);
        col = mix(col, bandColor(s), cover);
    }

    // No slide-up: the bands stay at their resting heights and instead fade in
    // from the pale surface tone as reveal grows, so the water develops in place.
    col = mix(bandColor(0.0), col, smoothstep(0.0, 1.0, reveal));

    // Underlay mode: dissolve the top of the band into the background so the sea
    // reads as deep water rising from the bottom edge and never competes with the
    // icons drawn over it. Fully opaque at the bottom, transparent by ~2/3 up.
    float vis = 1.0;
    if (fade > 0.5) {
        vis = smoothstep(0.0, 0.66, uv.y);
    }

    gl_FragColor = vec4(col, edge * vis);
}
`

// welcomeWaveGL is the desktop OpenGL (core profile) variant.
var welcomeWaveGL = []byte("#version 110\n" + welcomeWaveBody)

// welcomeWaveES is the OpenGL ES / mobile / web variant - same body, with the ES
// version header and the float precision preamble the built in shaders use.
var welcomeWaveES = []byte(`#version 100

#ifdef GL_ES
# ifdef GL_FRAGMENT_PRECISION_HIGH
precision highp float;
# else
precision mediump float;
#endif
precision mediump int;
#endif
` + welcomeWaveBody)
