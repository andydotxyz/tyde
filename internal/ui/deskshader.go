package ui

// cubeShaderGL and cubeShaderES are the desktop transition shaders, lifted from
// the proof of concept in fyne.io/fyne/v2/cmd/hello. They render a virtual
// desktop switch as a 3D cube roll: the "progress" uniform drives the turn from
// 0 (showing desk0 filling the view) to 1 (showing desk1). In between it zooms
// out to reveal a cube, rolls it 90 degrees about the X axis so the next desktop
// comes into view, then zooms back in. The faces are sized to the view's aspect
// so each desktop fills exactly at the endpoints.
//
// cubeShaderGL targets desktop OpenGL (core profile); cubeShaderES targets
// OpenGL ES / mobile / web. They are identical apart from the version preamble.
var cubeShaderGL = []byte(`#version 110

uniform vec2 frame_size;
uniform vec4 rect_coords; // x1, x2, y1, y2 in pixels (canvas-top origin)
uniform float progress;
uniform sampler2D desk0;
uniform sampler2D desk1;

const float PI = 3.14159265;
const float FOCAL = 2.0;

mat3 gRot;  // cube rotation for this frame
vec3 gBox;  // cube half-extents (sized to the view aspect)

mat3 rotX(float a) {
    float c = cos(a); float s = sin(a);
    return mat3(1.0, 0.0, 0.0, 0.0, c, -s, 0.0, s, c);
}

float sdBox(vec3 p, vec3 b) {
    vec3 d = abs(p) - b;
    return length(max(d, 0.0)) + min(max(d.x, max(d.y, d.z)), 0.0);
}

float scene(vec3 p) {
    return sdBox(gRot * p, gBox);
}

vec3 normal(vec3 p) {
    vec2 e = vec2(0.001, 0.0);
    return normalize(vec3(
        scene(p + e.xyy) - scene(p - e.xyy),
        scene(p + e.yxy) - scene(p - e.yxy),
        scene(p + e.yyx) - scene(p - e.yyx)));
}

// faceColor maps a desktop onto the cube: desk0 on the -Z face (shown at rest)
// and desk1 on the -Y face (rolled into view during the transition). Other faces
// are the dark cube body. t is flipped because image row 0 is the top.
vec3 faceColor(vec3 lp) {
    vec3 na = abs(lp) / gBox;
    if (na.z >= na.x && na.z >= na.y && lp.z < 0.0) {
        vec2 uv = vec2(lp.x / gBox.x, lp.y / gBox.y) * 0.5 + 0.5;
        return texture2D(desk0, vec2(uv.x, 1.0 - uv.y)).rgb;
    }
    if (na.y >= na.x && na.y >= na.z && lp.y < 0.0) {
        vec2 uv = vec2(lp.x / gBox.x, -lp.z / gBox.z) * 0.5 + 0.5;
        return texture2D(desk1, vec2(uv.x, 1.0 - uv.y)).rgb;
    }
    return vec3(0.08);
}

void main() {
    float w = rect_coords[1] - rect_coords[0];
    float h = rect_coords[3] - rect_coords[2];
    float aspect = w / h;
    float canvasY = frame_size.y - gl_FragCoord.y;
    vec2 local = vec2(gl_FragCoord.x - rect_coords[0], canvasY - rect_coords[2]);
    vec2 uv = (local / vec2(w, h)) * 2.0 - 1.0;
    uv.y = -uv.y;
    uv.x *= aspect;

    float p = smoothstep(0.0, 1.0, clamp(progress, 0.0, 1.0)); // ease the turn

    // Faces sized to the view aspect; the Y/Z cross section is square so the roll
    // about X is symmetric and both desktops fill the view at the endpoints.
    gBox = vec3(aspect, 1.0, 1.0);
    gRot = rotX(p * (PI * 0.5));

    float dMin = 1.0 + FOCAL;        // front face exactly fills the view
    float dMax = dMin + 1.4;         // pulled back to reveal the cube
    float dist = mix(dMin, dMax, sin(p * PI));

    vec3 ro = vec3(0.0, 0.0, -dist);
    vec3 rd = normalize(vec3(uv, FOCAL));

    float t = 0.0;
    float hit = 0.0;
    for (int i = 0; i < 96; i++) {
        vec3 pos = ro + rd * t;
        float d = scene(pos);
        if (d < 0.001) { hit = 1.0; break; }
        t += d;
        if (t > 24.0) break;
    }

    vec4 col = vec4(0.0);
    if (hit > 0.5) {
        vec3 pos = ro + rd * t;
        vec3 nrm = normal(pos);
        vec3 viewDir = normalize(ro - pos);
        float face = max(dot(nrm, viewDir), 0.0);
        vec3 c = faceColor(gRot * pos);
        col = vec4(c * (0.55 + 0.45 * face), 1.0);
    }
    gl_FragColor = col;
}
`)

var cubeShaderES = []byte(`#version 100

#ifdef GL_ES
# ifdef GL_FRAGMENT_PRECISION_HIGH
precision highp float;
# else
precision mediump float;
#endif
#endif

uniform vec2 frame_size;
uniform vec4 rect_coords;
uniform float progress;
uniform sampler2D desk0;
uniform sampler2D desk1;

const float PI = 3.14159265;
const float FOCAL = 2.0;

mat3 gRot;
vec3 gBox;

mat3 rotX(float a) {
    float c = cos(a); float s = sin(a);
    return mat3(1.0, 0.0, 0.0, 0.0, c, -s, 0.0, s, c);
}

float sdBox(vec3 p, vec3 b) {
    vec3 d = abs(p) - b;
    return length(max(d, 0.0)) + min(max(d.x, max(d.y, d.z)), 0.0);
}

float scene(vec3 p) {
    return sdBox(gRot * p, gBox);
}

vec3 normal(vec3 p) {
    vec2 e = vec2(0.001, 0.0);
    return normalize(vec3(
        scene(p + e.xyy) - scene(p - e.xyy),
        scene(p + e.yxy) - scene(p - e.yxy),
        scene(p + e.yyx) - scene(p - e.yyx)));
}

vec3 faceColor(vec3 lp) {
    vec3 na = abs(lp) / gBox;
    if (na.z >= na.x && na.z >= na.y && lp.z < 0.0) {
        vec2 uv = vec2(lp.x / gBox.x, lp.y / gBox.y) * 0.5 + 0.5;
        return texture2D(desk0, vec2(uv.x, 1.0 - uv.y)).rgb;
    }
    if (na.y >= na.x && na.y >= na.z && lp.y < 0.0) {
        vec2 uv = vec2(lp.x / gBox.x, -lp.z / gBox.z) * 0.5 + 0.5;
        return texture2D(desk1, vec2(uv.x, 1.0 - uv.y)).rgb;
    }
    return vec3(0.08);
}

void main() {
    float w = rect_coords[1] - rect_coords[0];
    float h = rect_coords[3] - rect_coords[2];
    float aspect = w / h;
    float canvasY = frame_size.y - gl_FragCoord.y;
    vec2 local = vec2(gl_FragCoord.x - rect_coords[0], canvasY - rect_coords[2]);
    vec2 uv = (local / vec2(w, h)) * 2.0 - 1.0;
    uv.y = -uv.y;
    uv.x *= aspect;

    float p = smoothstep(0.0, 1.0, clamp(progress, 0.0, 1.0));

    gBox = vec3(aspect, 1.0, 1.0);
    gRot = rotX(p * (PI * 0.5));

    float dMin = 1.0 + FOCAL;
    float dMax = dMin + 1.4;
    float dist = mix(dMin, dMax, sin(p * PI));

    vec3 ro = vec3(0.0, 0.0, -dist);
    vec3 rd = normalize(vec3(uv, FOCAL));

    float t = 0.0;
    float hit = 0.0;
    for (int i = 0; i < 96; i++) {
        vec3 pos = ro + rd * t;
        float d = scene(pos);
        if (d < 0.001) { hit = 1.0; break; }
        t += d;
        if (t > 24.0) break;
    }

    vec4 col = vec4(0.0);
    if (hit > 0.5) {
        vec3 pos = ro + rd * t;
        vec3 nrm = normal(pos);
        vec3 viewDir = normalize(ro - pos);
        float face = max(dot(nrm, viewDir), 0.0);
        vec3 c = faceColor(gRot * pos);
        col = vec4(c * (0.55 + 0.45 * face), 1.0);
    }
    gl_FragColor = col;
}
`)
