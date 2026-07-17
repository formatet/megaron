import { BASE } from './config.js';
import { noteServerDate } from './clock.js';
import { track } from './telemetry.js';

// Strip UUIDs from a path so retry/fail events aggregate by shape
// (`/api/v1/worlds/:id/units`) instead of exploding per world/unit.
const UUID_RE = /[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/gi;
const telemetryPath = url => url.split('?')[0].replace(UUID_RE, ':id');

// fetchAuth — the one place a Bearer token is attached to an API request.
// Call sites pass a path relative to the API root (e.g. `/api/v1/worlds/...`);
// BASE is prepended here so a future non-same-origin client only needs to
// change config.js, not every call site. Doubling as the one place every
// response passes through, it also anchors the server clock (clock.js) off
// the response's Date header.
const sleep = ms => new Promise(r => setTimeout(r, ms));

// Discreet topbar indicator (map.html #net-status, styled in megaron.css — no
// colour lives here). Shown while a GET is retrying, cleared on a reachable reply.
function setNetStatus(active) {
  const el = document.getElementById('net-status');
  if (el) el.classList.toggle('visible', active);
}

export async function fetchAuth(url, opts = {}) {
  const token = localStorage.getItem('poleia_token');
  const headers = Object.assign(token ? {'Authorization': 'Bearer ' + token} : {}, opts.headers || {});
  const finalOpts = Object.assign({}, opts, {headers});
  // Retry only idempotent GETs — never a mutation — on a network error or 5xx.
  // Two retries with 1 s / 3 s backoff smooth over the pool-starvation blips and
  // reconnect windows; a 4xx or success returns immediately.
  const method = (opts.method || 'GET').toUpperCase();
  const backoffs = method === 'GET' ? [1000, 3000] : [];
  for (let attempt = 0; ; attempt++) {
    try {
      const res = await fetch(BASE + url, finalOpts);
      noteServerDate(res.headers.get('Date'));
      if (res.status >= 500 && attempt < backoffs.length) {
        setNetStatus(true);
        track('fetch_retry', { path: telemetryPath(url), status: res.status });
        await sleep(backoffs[attempt]);
        continue;
      }
      if (res.status >= 500) track('fetch_fail', { path: telemetryPath(url), status: res.status });
      if (res.status < 500) setNetStatus(false); // reachable → clear the pill
      return res;
    } catch (e) {
      if (attempt < backoffs.length) {
        setNetStatus(true);
        track('fetch_retry', { path: telemetryPath(url), status: 0 });
        await sleep(backoffs[attempt]);
        continue;
      }
      track('fetch_fail', { path: telemetryPath(url), status: 0 });
      throw e; // retries exhausted (or a mutation) — propagate as before
    }
  }
}
