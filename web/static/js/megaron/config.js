// Server base URL. Empty string = same origin, which is what the browser
// client uses today (map.html and the API are served by the same Temenos
// process). A future standalone client (e.g. a packaged binary) would set
// this to the server's absolute origin instead. api.js prepends this to
// every fetchAuth() call; ws.js builds the equivalent for the WebSocket URL.
export const BASE = '';
