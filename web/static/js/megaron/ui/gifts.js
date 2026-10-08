// Exact quantities come from the durable delivery outcome, never a prediction.
export function giftText(kind, body) {
  const route = `${body.origin_name || 'Unknown city'} → ${body.destination_name || 'Unknown city'}`;
  if (kind === 'GiftDispatched') return `Gift sent: ${body.quantity} ${body.good_key} · ${route}. Nothing requested in return.`;
  let text = `${kind === 'GiftLost' ? 'Gift lost' : 'Gift delivered'}: ${route} · ${body.credited_quantity ?? 0} ${body.good_key} received; ${body.lost_quantity ?? 0} lost.`;
  if (body.reason) text += ` ${body.reason.replaceAll('_', ' ')}.`;
  if (body.owner_changed) text += ` The city is now held by ${body.actual_recipient_name || 'another Wanax'}; intended for ${body.recipient_name || 'the former owner'}.`;
  return text;
}
