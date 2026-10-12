// Shared realm and current-battle retreat choices. The server owns the
// loyalty-derived threshold; players choose a mode, never a percentage.
import { esc } from './format.js';

export const BY_LOYALTY_LABEL = 'Retreat when it goes badly';
export const RETREAT_PRESETS = [
  { value: 'loyalty', label: BY_LOYALTY_LABEL },
  { value: 'hold', label: 'Hold to the last man' },
];

export function retreatBody(value) {
  if (value === 'hold') return { hold_to_last_man: true };
  if (value === 'loyalty') return { by_loyalty: true };
  return null;
}

export function retreatDefaultValue(d) {
  if (!d) return null;
  // A stale pre-migration response must not invent a percentage option or
  // claim that a fixed threshold is the loyalty setting.
  if (d.retreat_at_loss != null) return null;
  return d.hold_to_last_man ? 'hold' : 'loyalty';
}

export function retreatDefaultLabel(d) {
  return RETREAT_PRESETS.find(p => p.value === retreatDefaultValue(d))?.label
    || 'Unknown retreat setting';
}

export function retreatDefaultSectionHTML(d, errText) {
  let html = '<div class="dsec"><div class="dsec-title">When to retreat</div>';
  const cur = retreatDefaultValue(d);
  if (!cur) {
    return html + '<p class="retreat-note">'
      + esc(errText || 'Could not load your retreat setting. Reload to try again.')
      + '</p></div>';
  }
  html += '<div class="retreat-row">'
    + '<select id="war-retreat-default" class="retreat-select">'
    + RETREAT_PRESETS.map(o => '<option value="' + o.value + '"' + (o.value === cur ? ' selected' : '') + '>' + o.label + '</option>').join('')
    + '</select>'
    + '<button class="retreat-btn" onclick="saveRetreatDefault()">Save</button>'
    + '</div>'
    + '<div id="war-retreat-default-res" class="retreat-res"></div></div>';
  return html;
}

// The prompt is not a third mode. An override is available only in battle,
// and does not pretend the API has supplied that unit's current choice.
export function unitRetreatControlHTML(u) {
  if (!u?.in_battle) return '';
  return '<select id="uretreat-' + esc(u.id) + '" class="retreat-select retreat-select-sm" title="This battle only">'
    + '<option value="" disabled selected>this battle: retreat…</option>'
    + RETREAT_PRESETS.map(p => '<option value="' + p.value + '">' + p.label + '</option>').join('')
    + '</select> '
    + '<button class="retreat-btn retreat-btn-sm" onclick="unitRetreatOrder(\'' + esc(u.id) + '\')">Set</button> ';
}
