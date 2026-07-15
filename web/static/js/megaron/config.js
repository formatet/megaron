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
