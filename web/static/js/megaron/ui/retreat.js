// Retreat orders, as the War drawer shows them — pure, so node can test the
// wording without a DOM (war.js itself touches `document` at import).
//
// The server's retreat_at_loss is the fraction of starting strength LEFT when
// a side breaks (combat.sideRouts: current/start <= threshold). So "retreat at
// 25% losses" is 0.75, not 0.25. The per-unit control used to send the losses
// figure itself, which inverted 25% and 75%; both controls now share this list.
import { esc } from './format.js';

export const RETREAT_PRESETS = [
  { value: '0.75', label: 'retreat at 25% losses' },
  { value: '0.5',  label: 'retreat at 50% losses' },
  { value: '0.25', label: 'retreat at 75% losses' },
  { value: 'hold', label: 'hold to the last man' },
];

export const BY_LOYALTY_LABEL = "by the troops' loyalty (default)";

// retreatBody turns a select value into the API body. '' → null (nothing chosen).
export function retreatBody(value) {
  if (value === 'hold') return { hold_to_last_man: true };
  if (value === 'loyalty') return { by_loyalty: true };
  const f = parseFloat(value);
  return Number.isFinite(f) ? { retreat_at_loss: f } : null;
}

// retreatDefaultValue maps a GET …/retreat-default body onto a select value.
export function retreatDefaultValue(d) {
  if (!d || d.by_loyalty) return 'loyalty';
  if (d.hold_to_last_man) return 'hold';
  return String(d.retreat_at_loss);
}

// retreatDefaultLabel is the plain-words current setting. A threshold that is
// not one of the presets (set from keryx) still reads correctly.
export function retreatDefaultLabel(d) {
  const v = retreatDefaultValue(d);
  if (v === 'loyalty') return BY_LOYALTY_LABEL;
  const preset = RETREAT_PRESETS.find(p => p.value === v);
  if (preset) return preset.label;
  return 'retreat at ' + Math.round((1 - d.retreat_at_loss) * 100) + '% losses';
}

// retreatDefaultSectionHTML renders War → Army's realm-wide setting. d is the
// GET body, or null when it could not be read — then the section says so
// instead of pretending the setting is the default.
export function retreatDefaultSectionHTML(d, errText) {
  let html = '<div class="dsec"><div class="dsec-title">When to retreat</div>';
  if (!d) {
    html += '<p class="retreat-note">' + esc(errText || 'Could not load your retreat setting.') + '</p></div>';
    return html;
  }
  const cur = retreatDefaultValue(d);
  const opts = [{ value: 'loyalty', label: BY_LOYALTY_LABEL }, ...RETREAT_PRESETS];
  // A custom threshold set from keryx gets its own option so the select shows the truth.
  if (!opts.some(o => o.value === cur)) opts.push({ value: cur, label: retreatDefaultLabel(d) });
  html += '<div class="retreat-row">'
    + '<select id="war-retreat-default" class="retreat-select">'
    + opts.map(o => '<option value="' + esc(o.value) + '"' + (o.value === cur ? ' selected' : '') + '>' + esc(o.label) + '</option>').join('')
    + '</select>'
    + '<button class="retreat-btn" onclick="saveRetreatDefault()">Save</button>'
    + '</div>'
    + '<p class="retreat-note">Your whole realm: every unit carries this into a battle it enters. '
    + 'A change applies to battles entered from now on — a battle under way keeps what it had; '
    + 'change a fighting unit from its card below.</p>'
    + '<div id="war-retreat-default-res" class="retreat-res"></div>'
    + '</div>';
  return html;
}

// unitRetreatControlHTML is the per-unit override, shown only while the unit
// is fighting (u.in_battle) — the only time the server can take it.
export function unitRetreatControlHTML(u) {
  if (!u || !u.in_battle) return '';
  return '<select id="uretreat-' + esc(u.id) + '" class="retreat-select retreat-select-sm" title="Overrides your realm-wide retreat setting for this battle only">'
    + '<option value="">this battle: retreat…</option>'
    + RETREAT_PRESETS.map(p => '<option value="' + p.value + '">' + p.label + '</option>').join('')
    + '</select> '
    + '<button class="retreat-btn retreat-btn-sm" onclick="unitRetreatOrder(\'' + esc(u.id) + '\')">Set</button> ';
}
