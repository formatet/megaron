import { fetchAuth } from '../api.js';
import { formatApiError } from './format.js';

const pending = new Set();
const receipts = new Map();
const key = (world, city, type = '') => `${world}/${city}/${type}`;
export const recruitmentPending = (world, city, type) => pending.has(key(world, city, type));
export const recruitmentReceipt = (world, city) => receipts.get(key(world, city)) || '';
export function rememberRecruitmentReceipt(world, city, text) { receipts.set(key(world, city), text); }

// Keep the lock through the accepted refresh: a second click must not become a
// second cohort merely because the POST finished before the drawer refreshed.
export async function submitRecruitment({ world, city, type, name, status, accepted }) {
  const id = key(world, city, type);
  if (pending.has(id)) return false;
  pending.add(id);
  status('Sending order…');
  let received = false;
  try {
    const body = { unit_type: type };
    if (name) body.name = name;
    const response = await fetchAuth(`/api/v1/worlds/${world}/provinces/${city}/recruit`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body),
    });
    const data = await response.json().catch(() => ({}));
    if (!response.ok) { status(formatApiError(data, 'Recruitment failed')); return false; }
    received = true;
    await accepted(data);
    return true;
  } catch (error) {
    status(received ? 'Order received. Refresh Army to see the updated units.' : 'Could not confirm the order. Check Army before trying again.');
    return received;
  } finally { pending.delete(id); }
}
