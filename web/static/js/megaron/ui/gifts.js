// Exact quantities come from the durable delivery outcome, never a prediction.
export function giftText(kind, body) {
  const route = `${body.sender_name || 'A Wanax'} → ${body.recipient_name || 'another Wanax'}`;
  if (kind === 'GiftDispatched') return `Gift sent: ${body.quantity} ${body.good_key} · ${route}. Nothing requested in return.`;
  let text = `${kind === 'GiftLost' ? 'Gift lost' : 'Gift delivered'}: ${route} · ${body.credited_quantity ?? 0} ${body.good_key} received; ${body.lost_quantity ?? 0} lost.`;
  if (body.returned_quantity > 0) text += ` ${body.returned_quantity} sent home aboard the damaged ship; the return voyage can still be intercepted.`;
  if (body.reason === 'foundered') text += ' The ship foundered in a storm.';
  else if (body.reason) text += ` ${body.reason.replaceAll('_', ' ')}.`;
  if (body.owner_changed) text += ` The city is now held by ${body.actual_recipient_name || 'another Wanax'}; intended for ${body.recipient_name || 'the former owner'}.`;
  return text;
}

// List outcomes preserve every per-good quantity, including fractional losses.
export function manifestOutcomeText(kind, body) {
  const safe = value => String(value).replaceAll('&','&amp;').replaceAll('<','&lt;').replaceAll('>','&gt;').replaceAll('\"','&quot;').replaceAll("'",'&#39;');
  const gift = kind.startsWith('Gift');
  const route = gift ? `${safe(body.sender_name || 'A Wanax')} → ${safe(body.recipient_name || 'another Wanax')} · ` : '';
  const sent = kind === 'GiftDispatchedV2';
  const label = sent ? 'Gift sent' : kind.includes('Lost') ? (gift ? 'Gift lost' : 'Transfer lost') : (gift ? 'Gift delivered' : 'Transfer delivered');
  const items = (body.items || []).map(item => sent ? `${item.quantity} ${safe(item.good_key)}`
    : `${item.credited_quantity ?? 0} ${safe(item.good_key)} received; ${item.lost_quantity ?? 0} lost`
      + (item.returned_quantity > 0 ? `; ${item.returned_quantity} sent home aboard the damaged ship` : '')).join(', ');
  let text = `${label}: ${route}${items}.`;
  if (sent) text += ' Nothing requested in return.';
  if (body.reason) text += ` ${safe(body.reason.replaceAll('_',' '))}.`;
  if ((body.items || []).some(item => item.returned_quantity > 0)) text += ' The return voyage can still be intercepted.';
  if (body.owner_changed) text += ` The city is now held by ${safe(body.actual_recipient_name || 'another Wanax')}; intended for ${safe(body.recipient_name || 'the former owner')}.`;
  return text;
}
