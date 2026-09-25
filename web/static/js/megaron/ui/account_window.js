import { fetchAuth } from '../api.js';
import { formatApiError } from './format.js';

// ── Account window — every keryx verb lives in the web too (Timothy
// 2026-09-25, CLAUDE.md "A verb lives on FOUR surfaces"). `keryx password`
// had no web door; this is it. Opened from the one identity anchor already in
// the topbar (#gt-wanax) rather than a new settings icon — there is no
// settings/account area today, and the wanax name is the natural "this is
// about ME" door. Same overlay/panel chrome as the dispatch window
// (ui/dispatch_window.js) — one look for "a small window pops up over the
// map", not a second visual language.
//
// POST /api/v1/auth/password (server/api/handlers/auth.go) revokes every
// REFRESH token on success but leaves the caller's own access token alone —
// it's a stateless JWT, valid until accessTTL (24h) regardless of a password
// change (see the handler's doc comment). So this tab is never silently
// logged out by a successful change: fetchAuth's existing Bearer token in
// localStorage keeps working exactly as before. The only consequence is on
// OTHER sessions/devices, which lose the ability to silently renew and will
// have to sign in again next time their access token expires — the same
// tradeoff keryx's own `password` command already prints, echoed here.

export function closeAccountWindow() {
  const el = document.getElementById('account-window-overlay');
  if (el) el.classList.remove('open');
}

export function toggleAccountWindow() {
  const overlay = document.getElementById('account-window-overlay');
  if (!overlay) return;
  if (overlay.classList.contains('open')) { closeAccountWindow(); return; }
  openAccountWindow(overlay);
}

function openAccountWindow(overlay) {
  const body = document.getElementById('account-window-body');
  if (!body) return;

  body.innerHTML = `
    <form id="acc-pw-form">
      <div class="field">
        <label>Current password</label>
        <input type="password" id="acc-pw-old" required autocomplete="current-password">
      </div>
      <div class="field">
        <label>New password</label>
        <input type="password" id="acc-pw-new" required autocomplete="new-password">
      </div>
      <div class="field">
        <label>Repeat new password</label>
        <input type="password" id="acc-pw-new2" required autocomplete="new-password">
      </div>
      <div id="acc-pw-error" class="form-error"></div>
      <div id="acc-pw-ok" class="dw-grain" style="display:none">
        Password changed. This session stays signed in — other devices will need
        to sign in again next time they need to renew.
      </div>
      <button type="submit" class="btn-primary">Change password</button>
    </form>
    <button class="dw-goto-btn" id="acc-signout-btn">Sign out</button>
  `;
  overlay.classList.add('open');

  document.getElementById('acc-pw-form').addEventListener('submit', e => {
    e.preventDefault();
    submitPasswordChange();
  });
  document.getElementById('acc-signout-btn').addEventListener('click', signOut);
}

async function submitPasswordChange() {
  const errorEl = document.getElementById('acc-pw-error');
  const okEl = document.getElementById('acc-pw-ok');
  errorEl.textContent = '';
  okEl.style.display = 'none';

  const oldPw = document.getElementById('acc-pw-old').value;
  const newPw = document.getElementById('acc-pw-new').value;
  const newPw2 = document.getElementById('acc-pw-new2').value;
  if (newPw !== newPw2) {
    errorEl.textContent = 'The two new passwords do not match.';
    return;
  }

  let res;
  try {
    res = await fetchAuth('/api/v1/auth/password', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ old_password: oldPw, new_password: newPw }),
    });
  } catch (e) {
    errorEl.textContent = 'Network error — the password was not changed.';
    return;
  }
  if (!res.ok) {
    let data = null;
    try { data = await res.json(); } catch (e) { /* no JSON body */ }
    errorEl.textContent = formatApiError(data, 'Could not change password.');
    return;
  }
  document.getElementById('acc-pw-form').reset();
  okEl.style.display = '';
}

function signOut() {
  try {
    localStorage.removeItem('poleia_token');
    localStorage.removeItem('poleia_refresh');
  } catch (e) { /* private-browsing / storage disabled — /logout still clears the cookie */ }
  window.location.href = '/logout';
}
