import { fetchAuth, getAgoraAccount, requestAgoraPassword } from '../api.js';
import { formatApiError, esc } from './format.js';

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

let accountGeneration = 0;

function clearChatPassword() {
  const secret = document.getElementById('acc-chat-password');
  if (secret) { secret.textContent = ''; secret.hidden = true; }
}

export function closeAccountWindow() {
  accountGeneration++;
  clearChatPassword();
  const el = document.getElementById('account-window-overlay');
  if (el) el.classList.remove('open');
}

export function toggleAccountWindow() {
  const overlay = document.getElementById('account-window-overlay');
  if (!overlay) return;
  if (overlay.classList.contains('open')) { closeAccountWindow(); return; }
  openAccountWindow();
}

export function openAccountWindow() {
  const overlay = document.getElementById('account-window-overlay');
  if (!overlay) return;
  const generation = ++accountGeneration;
  clearChatPassword();
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
    <section id="acc-chat" class="account-chat" aria-labelledby="acc-chat-title" hidden></section>
    <p class="account-delete-note">To delete your account, write to the admin in the community chat.</p>
    <button class="dw-goto-btn" id="acc-signout-btn">Sign out</button>
  `;
  overlay.classList.add('open');

  document.getElementById('acc-pw-form').addEventListener('submit', e => {
    e.preventDefault();
    submitPasswordChange();
  });
  document.getElementById('acc-signout-btn').addEventListener('click', signOut);
  loadChatAccount(generation);
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
  closeAccountWindow();
  try {
    localStorage.removeItem('poleia_token');
    localStorage.removeItem('poleia_refresh');
  } catch (e) { /* private-browsing / storage disabled — /logout still clears the cookie */ }
  window.location.href = '/logout';
}

function accountIsCurrent(generation) {
  return generation === accountGeneration && document.getElementById('account-window-overlay')?.classList.contains('open');
}

async function loadChatAccount(generation) {
  const section = document.getElementById('acc-chat');
  try {
    const response = await getAgoraAccount();
    if (!accountIsCurrent(generation)) return;
    if (!response.ok) throw new Error('unavailable');
    const account = await response.json();
    if (!accountIsCurrent(generation) || !account.enabled) return;
    section.hidden = false;
    const title = '<h3 id="acc-chat-title">Community chat</h3>';
    if (account.state !== 'ready') {
      section.innerHTML = title + '<p>' + (account.state === 'provisioning'
        ? 'Your chat account is being created. Check again later.'
        : 'Your chat account is created after you own your first city. If you already do, account creation is pending; check again later.') + '</p>';
      return;
    }
    section.innerHTML = title + `
      <p>Outside the game. Your Wanax name identifies you in chat.</p>
      <dl><dt>Chat ID</dt><dd>${esc(account.user_id)}</dd><dt>Homeserver</dt><dd>${esc(account.homeserver)}</dd></dl>
      <p><a href="${esc(account.homeserver)}" target="_blank" rel="noopener noreferrer">Open community chat</a></p>
      <p>Each request sets a new chat password and replaces the previous one. Save it before closing this window.</p>
      <button type="button" class="btn-primary" id="acc-chat-get">Get chat password</button>
      <div id="acc-chat-status" role="status"></div>
      <pre id="acc-chat-password" class="account-chat-secret" hidden></pre>`;
    document.getElementById('acc-chat-get').addEventListener('click', () => getChatPassword(generation));
  } catch (_) {
    if (!accountIsCurrent(generation)) return;
    section.hidden = false;
    section.innerHTML = '<h3 id="acc-chat-title">Community chat</h3><p>Could not check your chat account. Close this window and try again later.</p>';
  }
}

async function getChatPassword(generation) {
  clearChatPassword();
  const button = document.getElementById('acc-chat-get');
  const status = document.getElementById('acc-chat-status');
  button.disabled = true;
  status.textContent = 'Requesting a new chat password…';
  try {
    const response = await requestAgoraPassword();
    if (!accountIsCurrent(generation)) return;
    if (!response.ok) {
      status.textContent = response.status === 409 ? 'Your chat account is not ready. Close this window and check again later.' : 'Could not get a chat password. Try again later.';
      return;
    }
    const data = await response.json();
    if (!accountIsCurrent(generation)) { data.password = ''; return; }
    const secret = document.getElementById('acc-chat-password');
    if (typeof data.password !== 'string' || !data.password) { status.textContent = 'No chat password was returned. Try again later.'; return; }
    secret.textContent = data.password;
    secret.hidden = false;
    data.password = '';
    status.textContent = 'New chat password shown once. The previous password no longer works.';
  } catch (_) {
    if (accountIsCurrent(generation)) status.textContent = 'Could not get a chat password. Try again later.';
  } finally {
    if (accountIsCurrent(generation)) button.disabled = false;
  }
}
