// Server base URL. Empty string = same origin, which is what the browser
// client uses today (map.html and the API are served by the same Temenos
// process). A future standalone client (e.g. a packaged binary) would set
// this to the server's absolute origin instead. api.js prepends this to
// every fetchAuth() call; ws.js builds the equivalent for the WebSocket URL.
export const BASE = '';

// spegel av server/internal/province/hex.go LiveRadius — used by render/map.js's
// FOV-förhandsband (hover on a march affordance). Keep numerically identical to
// the server; do not derive/tune these from anything but hex.go.
export const LIVE_RADIUS_SEA = 4;
export const LIVE_RADIUS_BASE = { settlement: 3, ship: 1, land: 2 };
export const LIVE_RADIUS_MOUNTAIN_BONUS = 2;

// ── LOD registers (megaron_lokal_varld.md §Zoom som spelinstrument) ────────
// The camera has two discrete rendering registers on one continuous zoom.
// LOCAL_ZOOM is the boundary: at/above it the "lokal trakt" register is active
// and local-only signals appear (rural projections, catchment tint) — the
// answer to "why does THIS place work?". Below it the "strategisk" register
// keeps the overview and tones those down. This is the register boundary; the
// other per-signal thresholds below are their own calibration (formerly
// scattered as magic numbers through render/map.js's render loop — moved here
// 2026-07-25, values unchanged, see fix/zoom-registers).
export const LOCAL_ZOOM = 0.55;

// Below LOCAL_ZOOM (still "strategisk"), these two signals fade in earlier —
// they read at a distance and don't depend on local-trakt detail.
export const GARRISON_DOT_ZOOM = 0.45;   // own-city garrison dot
export const ACTIVITY_BADGE_ZOOM = 0.4;  // own-city activity overlay badge

// Roads between owned/allied provinces and resource-deposit icons: both
// appear a notch before the local-trakt boundary.
export const ROAD_DEPOSIT_ZOOM = 0.5;

// ── Zoom bounds ─────────────────────────────────────────────────────────
// Ceiling: unchanged since the camera was first built — not part of the
// "zooms out by accident" complaint (Timothy 2026-07-25).
export const ZOOM_MAX = 5;
// Floor: raised from 0.2 (Timothy 2026-07-25 — "jag råkar zooma ut hela
// tiden vilket förstör spelet"). At 0.2 the world (default 56×40 hexes,
// server/cmd/create-world MAP_WIDTH/MAP_HEIGHT) shrinks to an island inside
// a much larger empty canvas on any normal viewport — effective scale
// zoom*SCALE means the full map only needs zoom≈0.25-0.35 to fill a
// 1024×768-1920×1080 window. 0.3 keeps the whole map roughly filling the
// screen at max zoom-out (the "strategisk" register's actual job — see
// whole risk/dependency picture) instead of shrinking past it into dead
// space. Still a continuous camera, not a third mode — see
// megaron_lokal_varld.md.
export const ZOOM_MIN = 0.3;

// ── Keyboard pan speed (WASD / arrows, render/map.js) ──────────────────────
// Screen-space, not world-space: camera.x/y are screen-pixel translate
// offsets (render/map.js's render() does ctx.translate(camera.x, camera.y)
// then ctx.scale(zoom*SCALE, …)), and this rate is deliberately constant in
// that same screen-pixel space at every zoom. A world-space-constant rate
// would have to cross the ZOOM_MIN..ZOOM_MAX range above (a ~16x span)
// unchanged — translated to screen pixels via zoom*SCALE that crawls when
// zoomed out to the strategic overview and rockets past hexes when zoomed
// in on one settlement, unusable at either end. Screen-space instead matches
// exactly what mouse-drag panning already feels like (map.js's mousemove
// handler moves camera.x/y 1:1 with the pointer, no zoom factor): holding a
// direction crosses the visible viewport in the same real time regardless of
// zoom, the same way a drag of the same screen distance always would.
export const PAN_SPEED_PX_PER_SEC = 700;
