// Redirect from the War force card: the destination comes from a tap on the map (ui/hexpick.js) or from typed Q/R.
import { esc } from './format.js';

export const REDIRECT_NEEDS_BOTH = 'Type both Q and R.';

// The request body of POST .../units/{id}/recall, or the reason it must not be sent. Same body as before.
export function redirectBody(qText, rText) {
  const q = parseInt(String(qText ?? '').trim(), 10), r = parseInt(String(rText ?? '').trim(), 10);
  if (!Number.isFinite(q) || !Number.isFinite(r)) return { error: REDIRECT_NEEDS_BOTH };
  return { body: { target_q: q, target_r: r } };
}

// The line that replaces the old instruction once a hex has been picked.
export function redirectPickedHTML(unitID, q, r) {
  const id = esc(unitID);
  return '<div class="redirect-picked">Redirect to (' + Number(q) + ',' + Number(r) + ') — a messenger carries the order. '
    + '<button class="redirect-send" onclick="unitRedirect(\'' + id + '\')">Send order →</button> · '
    + '<a href="#" onclick="unitRedirectToggle(\'' + id + '\');return false">Change</a></div>';
}
