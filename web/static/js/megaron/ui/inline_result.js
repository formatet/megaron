// Results stay beside the action. Server text is always plain text.
export function showInlineResult(host, text, failed = false) {
  const parent = typeof host === 'string' ? document.getElementById(host) : host;
  if (!parent) return;
  let result = parent.querySelector('[data-inline-result]');
  if (!result) {
    result = document.createElement('div');
    result.dataset.inlineResult = '1';
    result.setAttribute('role', 'status');
    result.setAttribute('aria-live', 'polite');
    parent.appendChild(result);
  }
  result.className = failed ? 'action-result stat-warn' : 'action-result';
  result.textContent = text;
  return result;
}

// Irreversible actions need a deliberate second click. Merely opening this row or
// cancelling it cannot send the order; disable confirmation before awaiting.
export function confirmInline(host, text, send, { confirmLabel = 'Abandon', cancelLabel = 'Keep settlement' } = {}) {
  if (!host) return;
  host.replaceChildren();
  const prompt = document.createElement('p');
  prompt.textContent = text;
  const yes = document.createElement('button');
  yes.className = 'btn-small btn-danger';
  yes.textContent = confirmLabel;
  const no = document.createElement('button');
  no.className = 'btn-small';
  no.textContent = cancelLabel;
  let sent = false;
  yes.addEventListener('click', async () => {
    if (sent) return;
    sent = true;
    yes.disabled = no.disabled = true;
    await send();
  });
  no.addEventListener('click', () => host.replaceChildren());
  host.append(prompt, yes, no);
}
