import { BASE } from './config.js';
import { noteServerDate } from './clock.js';

// fetchAuth — the one place a Bearer token is attached to an API request.
// Call sites pass a path relative to the API root (e.g. `/api/v1/worlds/...`);
// BASE is prepended here so a future non-same-origin client only needs to
// change config.js, not every call site. Doubling as the one place every
// response passes through, it also anchors the server clock (clock.js) off
// the response's Date header.
export async function fetchAuth(url, opts = {}) {
  const token = localStorage.getItem('poleia_token');
  const headers = Object.assign(token ? {'Authorization': 'Bearer ' + token} : {}, opts.headers || {});
  const res = await fetch(BASE + url, Object.assign({}, opts, {headers}));
  noteServerDate(res.headers.get('Date'));
  return res;
}
