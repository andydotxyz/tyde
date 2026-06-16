package flames

// flameBody is the fragment shader shared between the desktop and ES variants.
// It draws rising flames anchored to the bottom of the object and leaves
// everything above them transparent, so the shader can sit over other content.
// It is lifted from the proof of concept in fyne.io/fyne/v2/cmd/hello.
//
// The version header (and, for ES, the precision preamble) is the only thing
// that differs between targets, so it is prepended below rather than duplicated.
const flameBody = `
/* the standard uniform contract shared with the built in vector shaders */
uniform vec2 frame_size;   // size of the output frame, in pixels
uniform vec4 rect_coords;  // this object's bounds: x1 [0], x2 [1], y1 [2], y2 [3]
uniform float time;        // elapsed animation time, in seconds

// hash -> value noise -> fbm: cheap procedural turbulence so the flames need no
// texture. fbm sums a few octaves of value noise for a billowing look.
float hash(vec2 p) {
    p = fract(p * vec2(127.1, 311.7));
    p += dot(p, p + 34.5);
    return fract(p.x * p.y);
}

float noise(vec2 p) {
    vec2 i = floor(p);
    vec2 f = fract(p);
    f = f * f * (3.0 - 2.0 * f); // smoothstep weighting for the bilinear blend
    float a = hash(i);
    float b = hash(i + vec2(1.0, 0.0));
    float c = hash(i + vec2(0.0, 1.0));
    float d = hash(i + vec2(1.0, 1.0));
    return mix(mix(a, b, f.x), mix(c, d, f.x), f.y);
}

float fbm(vec2 p) {
    float v = 0.0;
    float amp = 0.5;
    for (int i = 0; i < 5; i++) {
        v += amp * noise(p);
        p *= 2.0;
        amp *= 0.5;
    }
    return v;
}

void main() {
    // Discard anything outside this object's bounds, like the built in shapes.
    if (gl_FragCoord.x < rect_coords[0] || gl_FragCoord.x > rect_coords[1] ||
        gl_FragCoord.y < frame_size.y - rect_coords[3] ||
        gl_FragCoord.y > frame_size.y - rect_coords[2]) {
        discard;
    }

    // Local coordinates within the rect: uv.x 0..1 left->right, uv.y 0..1
    // bottom->top (gl_FragCoord.y grows upward, the rect's bottom edge sits at
    // frame_size.y - rect_coords[3]).
    float w = rect_coords[1] - rect_coords[0];
    float h = rect_coords[3] - rect_coords[2];
    vec2 uv = vec2(gl_FragCoord.x - rect_coords[0],
                   gl_FragCoord.y - (frame_size.y - rect_coords[3])) / vec2(w, h);

    // Confine the flames to the bottom 10% of the object. fy is 0 at the very
    // bottom and 1 at the top of that band; above it the shader stays transparent.
    float region = 0.10;
    float fy = uv.y / region;
    if (fy >= 1.0) {
        discard;
    }

    // Sample turbulent noise that scrolls upward over time so the flames rise.
    // The vertical coordinate spans the thin band (fy), so the horizontal one is
    // divided by region to match its density - otherwise the tongues are stretched
    // sideways by 1/region. A second, faster layer is mixed in to add flicker.
    float aspect = w / h;
    vec2 q = vec2(uv.x * aspect * 3.0 / region, fy * 3.0 - time * 2.5);
    float n = fbm(q);
    n = mix(n, fbm(q * 1.8 + vec2(0.0, -time * 1.5)), 0.5);

    // Flames are hottest at the base and taper towards the top of the band.
    float falloff = 1.0 - fy;
    // Subtracting a threshold before scaling lights fewer pixels, so the tongues
    // are sparser and more distinct rather than a solid sheet of fire.
    float tongues = (n - 0.30) * 2.4 * falloff;
    // A bright, near-continuous source concentrated at the very bottom gives the
    // flames a hot glowing base, like real fire feeding off a surface.
    float source = falloff * falloff * falloff;
    float flame = clamp(tongues + source * 0.8, 0.0, 1.0);

    // Map intensity through a black -> red -> orange -> yellow -> white ramp.
    vec3 col = mix(vec3(0.0), vec3(0.9, 0.1, 0.0), smoothstep(0.0, 0.30, flame));
    col = mix(col, vec3(1.0, 0.55, 0.0), smoothstep(0.30, 0.55, flame));
    col = mix(col, vec3(1.0, 0.90, 0.4), smoothstep(0.55, 0.80, flame));
    col = mix(col, vec3(1.0, 1.0, 0.9), smoothstep(0.80, 1.00, flame));

    // Alpha tracks the intensity, so the area above the flames stays transparent.
    float alpha = smoothstep(0.02, 0.20, flame);
    gl_FragColor = vec4(col, alpha);
}
`

// flameShader is the desktop OpenGL (core profile) variant.
var flameShader = []byte("#version 110\n" + flameBody)

// flameShaderES is the OpenGL ES / mobile / web variant - same body, but with
// the ES version header and the float precision preamble the built in shaders use.
var flameShaderES = []byte(`#version 100

#ifdef GL_ES
# ifdef GL_FRAGMENT_PRECISION_HIGH
precision highp float;
# else
precision mediump float;
#endif
precision mediump int;
#endif
` + flameBody)
