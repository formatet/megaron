import { fetchAuth } from '../../api.js';
import { formatApiError } from '../format.js';

// Only this caller's server document is loaded. A case switch invalidates every
// pending read/save; no late response can replace another case's local text.
export async function mountCasePrivate(root, world, caseID, alive, externalDraft = null) {
  root.innerHTML = '<h3 class="dip-archive-heading">Private draft and note</h3><p class="dip-queue-note">Only you can read this. Save before leaving this case.</p>'
    + (externalDraft ? '' : '<label>Draft<textarea data-private-draft maxlength="1000" disabled></textarea></label>')
    + '<label>Private note<textarea data-private-note maxlength="10000" disabled></textarea></label><div class="dip-queue-actions"><button class="btn-small" data-private-save disabled>Save draft and note</button><button class="btn-small" data-private-reload>Load saved copy</button></div><p role="status">Loading saved text…</p>';
  const draft = externalDraft || root.querySelector('[data-private-draft]');
  const note = root.querySelector('[data-private-note]');
  const saveButton = root.querySelector('[data-private-save]');
  const reloadButton = root.querySelector('[data-private-reload]');
  const status = root.querySelector('[role="status"]');
  draft.disabled = true;
  let version = null, loading = false, saving = false, edits = 0;
  const path = `/api/v1/worlds/${world}/correspondence/cases/${encodeURIComponent(caseID)}/private`;
  for (const input of [draft, note]) input.addEventListener('input', () => { edits++; status.textContent = 'Unsaved changes.'; });
  async function load() {
    if (loading || saving || !alive()) return;
    loading = true; draft.disabled = note.disabled = saveButton.disabled = reloadButton.disabled = true;
    try {
      const response = await fetchAuth(path, { sensitive: true });
      const data = await response.json();
      if (!alive()) return;
      if (!response.ok) throw new Error(formatApiError(data, 'Could not load saved text.'));
      if (!Number.isSafeInteger(data.version) || typeof data.draft !== 'string' || typeof data.note !== 'string') throw new Error('Invalid saved-text response.');
      version = data.version; draft.value = data.draft; note.value = data.note;
      status.textContent = 'Saved text loaded. Only you can read this.';
    } catch (error) { if (alive()) status.textContent = error.message + ' Your text has not been saved.'; }
    finally { loading = false; if (alive()) { draft.disabled = note.disabled = saveButton.disabled = version === null; reloadButton.disabled = false; } }
  }
  async function save(clearSent = null) {
    if (version === null || saving || loading || !alive()) return false;
    const submittedDraft = draft.value, submittedNote = note.value, revision = edits;
    if (clearSent !== null && submittedDraft.trim() !== clearSent) return false;
    saving = true; saveButton.disabled = reloadButton.disabled = true;
    try {
      const response = await fetchAuth(path, {method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({draft:clearSent === null ? submittedDraft : '',note:submittedNote,expected_version:version}),sensitive:true});
      const data = await response.json().catch(() => ({}));
      if (!alive()) return false;
      if (response.status === 409) { status.textContent = 'Another tab or device saved newer text. Your text remains here. Copy it before loading the saved copy.'; return false; }
      if (!response.ok) throw new Error(formatApiError(data, 'Could not save your text.'));
      if (!Number.isSafeInteger(data.version)) throw new Error('Invalid saved-text response.');
      version = data.version;
      if (clearSent !== null && draft.value === submittedDraft) draft.value = '';
      status.textContent = edits === revision ? 'Saved. Only you can read this.' : 'Earlier text saved; unsaved changes remain here.';
      return true;
    } catch (error) { if (alive()) status.textContent = error.message + ' Your text remains here unsaved.'; return false; }
    finally { saving = false; if (alive()) saveButton.disabled = reloadButton.disabled = false; }
  }
  saveButton.addEventListener('click', () => save());
  reloadButton.addEventListener('click', () => load());
  await load();
  return { afterDispatch: text => save(text) };
}
