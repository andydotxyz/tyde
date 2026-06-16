package christmas

// lightsBody is the fragment shader shared between the desktop and ES variants.
// It draws a string of multi-coloured fairy christmas around the edge of the
// object, sitting within 10% of each edge: a green wire links the bulbs, each
// bulb has a green base, a largely transparent white glass surround and a
// coloured glow inside that twinkles on and off independently. Everything away
// from the string stays transparent so it can be laid over other content. It is
// lifted from the proof of concept in fyne.io/fyne/v2/cmd/hello.
//
// The version header (and, for ES, the precision preamble) is the only thing
// that differs between targets, so it is prepended below rather than duplicated.
const lightsBody = `
/* the standard uniform contract shared with the built in vector shaders */
uniform vec2 frame_size;   // size of the output frame, in pixels
uniform vec4 rect_coords;  // this object's bounds: x1 [0], x2 [1], y1 [2], y2 [3]
uniform float time;        // elapsed animation time, in seconds

const int LIGHTS = 40; // bulbs spaced evenly around the string

// lightColour cycles through a small festive palette by bulb index. Integer mod
// is written out by hand because '%' is not available in these GLSL versions.
vec3 lightColour(int i) {
    int m = i - (i / 5) * 5;
    if (m == 0) return vec3(1.0, 0.15, 0.15); // red
    if (m == 1) return vec3(0.20, 0.45, 1.0); // blue
    if (m == 2) return vec3(0.20, 1.0, 0.35); // green
    if (m == 3) return vec3(1.0, 0.80, 0.15); // amber
    return vec3(0.85, 0.25, 1.0);             // magenta
}

// hash11 gives a stable pseudo-random value in [0,1) per bulb, used to give each
// one its own twinkle phase and rate so the string flickers irregularly.
float hash11(float n) {
    return fract(sin(n * 127.1) * 43758.5453);
}

// perimeterPoint walks distance d (in pixels) clockwise around the rectangle
// (x0,y0)-(x1,y1) and returns the point reached, so bulbs can be placed evenly
// by arc length: bottom edge, then right, then top, then left.
vec2 perimeterPoint(float d, float x0, float y0, float x1, float y1) {
    float pw = x1 - x0;
    float ph = y1 - y0;
    if (d < pw) return vec2(x0 + d, y0);
    d -= pw;
    if (d < ph) return vec2(x1, y0 + d);
    d -= ph;
    if (d < pw) return vec2(x1 - d, y1);
    d -= pw;
    return vec2(x0, y1 - d);
}

// sdSegment is the distance from p to the line segment a-b.
float sdSegment(vec2 p, vec2 a, vec2 b) {
    vec2 pa = p - a;
    vec2 ba = b - a;
    float t = clamp(dot(pa, ba) / dot(ba, ba), 0.0, 1.0);
    return length(pa - ba * t);
}

// bezier evaluates the quadratic curve from a to b with control point c, used to
// give each span of wire its drooping bow.
vec2 bezier(vec2 a, vec2 c, vec2 b, float t) {
    float mt = 1.0 - t;
    return mt * mt * a + 2.0 * mt * t * c + t * t * b;
}

// over composites straight-alpha colour a in front of b (the Porter-Duff "over").
vec4 over(vec4 a, vec4 b) {
    float outA = a.a + b.a * (1.0 - a.a);
    vec3 rgb = (a.rgb * a.a + b.rgb * b.a * (1.0 - a.a)) / max(outA, 0.0001);
    return vec4(rgb, outA);
}

// hash21 gives a stable pseudo-random value in [0,1) per 2D cell, used to seed
// each snowflake's presence, position and size.
float hash21(vec2 p) {
    p = fract(p * vec2(127.1, 311.7));
    p += dot(p, p + 34.5);
    return fract(p.x * p.y);
}

// snowLayer returns the coverage of one sheet of falling snow at the object uv
// (0..1, y growing downward). The plane is diced into a grid of 'density' cells;
// some cells hold a round flake that falls over time and sways gently sideways.
// 'seed' offsets the layer so stacked sheets at different speeds don't align,
// giving the fall a sense of depth.
float snowLayer(vec2 uv, float density, float speed, float seed, float t) {
    uv.y -= t * speed;                      // drift downward over time
    uv.x += sin(uv.y * 3.0 + seed) * 0.04;  // gentle sideways sway as it falls
    vec2 g = uv * density;
    vec2 id = floor(g);
    vec2 f = fract(g) - 0.5;
    float h = hash21(id + seed);
    if (h < 0.78) return 0.0;                // only a few cells carry a flake
    vec2 jitter = vec2(hash21(id + seed + 1.7), hash21(id + seed + 4.3)) - 0.5;
    float d = length(f - jitter * 0.7);
    float r = 0.02 + 0.04 * fract(h * 9.0);  // varied flake sizes
    return (1.0 - smoothstep(r, r + 0.04, d)) * (0.5 + 0.5 * h);
}

void main() {
    // Discard anything outside this object's bounds, like the built in shapes.
    if (gl_FragCoord.x < rect_coords[0] || gl_FragCoord.x > rect_coords[1] ||
        gl_FragCoord.y < frame_size.y - rect_coords[3] ||
        gl_FragCoord.y > frame_size.y - rect_coords[2]) {
        discard;
    }

    // Work in local pixel coordinates: p.x 0..w left->right, p.y 0..h bottom->top.
    float w = rect_coords[1] - rect_coords[0];
    float h = rect_coords[3] - rect_coords[2];
    vec2 p = vec2(gl_FragCoord.x - rect_coords[0],
                  gl_FragCoord.y - (frame_size.y - rect_coords[3]));

    // The wire runs around a rectangle inset slightly from each edge, so the
    // string sits just in from the bounds; the bulbs then hang inward off it.
    // The inset is a touch larger top/bottom than left/right. The path length
    // lets us space the bulbs evenly in pixels.
    float x0 = 0.005 * w, y0 = 0.008 * h;
    float x1 = 0.995 * w, y1 = 0.992 * h;
    float perimeter = 2.0 * ((x1 - x0) + (y1 - y0));
    vec2 pathCentre = vec2(x0 + x1, y0 + y1) * 0.5;

    // Feature sizes scale with the smaller dimension so the string stays
    // proportionate whatever the object's shape.
    float minDim = min(w, h);
    float wireHalf = 0.00125 * minDim; // half thickness of the connecting wire
    float baseR    = 0.0064 * minDim;  // green cap where a bulb meets the wire
    float glassR   = 0.0128 * minDim;  // half width of the glass bead
    float elong    = 1.4;              // bulbs are taller than wide, like real ones
    float offset   = 0.0176 * minDim;  // how far a bulb hangs inward off the wire
    float sagFrac  = 0.25;            // how far each span dips between bulbs
    float aa       = 1.5;             // edge softening, in pixels
    float spacing  = perimeter / float(LIGHTS);

    // The green wire is drawn first, behind every bulb. Rather than a taut
    // rectangle outline, each span between neighbouring bulbs is a quadratic bow
    // sagging towards the centre, so the string looks draped, not stretched.
    vec4 result = vec4(0.0);
    for (int i = 0; i < LIGHTS; i++) {
        float ad = (float(i) + 0.5) * spacing;
        vec2 a = perimeterPoint(ad, x0, y0, x1, y1);
        vec2 b = perimeterPoint(mod(ad + spacing, perimeter), x0, y0, x1, y1);
        vec2 mid = (a + b) * 0.5;
        vec2 ctrl = mid + normalize(pathCentre - mid) * length(b - a) * sagFrac;

        // Distance to the bow, approximated by sampling it into short segments.
        float wd = minDim * 4.0;
        vec2 prev = a;
        for (int k = 1; k <= 6; k++) {
            vec2 cur = bezier(a, ctrl, b, float(k) / 6.0);
            wd = min(wd, sdSegment(p, prev, cur));
            prev = cur;
        }
        float wcov = 1.0 - smoothstep(wireHalf, wireHalf + aa, wd);
        result = over(vec4(vec3(0.05, 0.22, 0.07), wcov), result);
    }

    for (int i = 0; i < LIGHTS; i++) {
        vec3 lc = lightColour(i);

        // Centre each bulb in its slot around the loop. It hangs inward off the
        // wire, so build a local frame: u along the bulb, v across it.
        float d = (float(i) + 0.5) / float(LIGHTS) * perimeter;
        vec2 base = perimeterPoint(d, x0, y0, x1, y1);
        vec2 u = normalize(pathCentre - base);
        vec2 v = vec2(-u.y, u.x);
        vec2 bulb = base + u * offset;

        vec2 local = p - bulb;
        float along = dot(local, u);
        float across = dot(local, v);
        float er = length(vec2(across, along / elong)); // elliptical bulb body
        float rd = length(local);                       // round dist for the halo

        // Each bulb twinkles on its own slow phase and rate.
        float rate = 0.5 + 1.0 * hash11(float(i) + 1.0);
        float phase = hash11(float(i) + 17.0) * 6.2831;
        float on = smoothstep(0.2, 0.8, 0.5 + 0.5 * sin(time * rate + phase));

        // Physical parts, composited back to front: green cap on the wire, then
        // a translucent coloured glass bead that stays visible even when unlit.
        float capCov = 1.0 - smoothstep(baseR, baseR + aa, length(p - base));
        result = over(vec4(vec3(0.10, 0.45, 0.13), capCov), result);

        float beadCov = 1.0 - smoothstep(glassR, glassR + aa, er);
        result = over(vec4(mix(vec3(1.0), lc, 0.6), beadCov * 0.18), result);

        // When lit: a soft coloured halo spilling past the glass, then a bright
        // body whose centre runs white-hot like a glowing filament. Composited
        // (not added) so brightness stays linear with how "on" the bulb is.
        float halo = 1.0 - smoothstep(0.0, glassR * 2.5, rd);
        result = over(vec4(lc, halo * on * 0.45), result);

        float fill = 1.0 - smoothstep(glassR * 0.85, glassR, er);
        float hot  = 1.0 - smoothstep(0.0, glassR * 0.5, er);
        result = over(vec4(mix(lc, vec3(1.0), hot * 0.7), fill * on), result);

        // A small off-centre glint so the glass reads as glossy.
        vec2 glint = bulb - u * glassR * 0.25 + v * glassR * 0.30;
        float spec = (1.0 - smoothstep(0.0, glassR * 0.22, length(p - glint))) * 0.4;
        result = over(vec4(vec3(1.0), spec), result);
    }

    // Snow drifting down over the whole area, in front of the christmas. Three
    // sheets at increasing density and speed give a sense of depth, the nearer
    // (faster) flakes larger and brighter. x is aspect-corrected so flakes stay
    // round whatever the object's shape.
    vec2 suv = vec2(p.x / w, (h - p.y) / h);
    suv.x *= w / h;
    float snow = 0.0;
    snow += snowLayer(suv,  8.0, 0.02,  0.0, time) * 0.7;
    snow += snowLayer(suv, 14.0, 0.035, 11.0, time) * 0.85;
    snow += snowLayer(suv, 22.0, 0.05, 27.0, time);
    result = over(vec4(vec3(1.0), clamp(snow, 0.0, 1.0) * 0.9), result);

    gl_FragColor = vec4(clamp(result.rgb, 0.0, 1.0), clamp(result.a, 0.0, 1.0));
}
`

// lightsShader is the desktop OpenGL (core profile) variant.
var lightsShader = []byte("#version 110\n" + lightsBody)

// lightsShaderES is the OpenGL ES / mobile / web variant - same body, but with
// the ES version header and the precision preamble the built in shaders use.
var lightsShaderES = []byte(`#version 100

#ifdef GL_ES
# ifdef GL_FRAGMENT_PRECISION_HIGH
precision highp float;
# else
precision mediump float;
#endif
precision mediump int;
#endif
` + lightsBody)
