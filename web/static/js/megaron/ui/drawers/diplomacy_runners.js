// Diplomacy → Runners: one read (GET /worlds/{id}/runners), grouped by the
// server, drawn here. Grouping, wording of the explanation and which buttons
// exist are the server's; this file only lays them out. Passage and call-back
// go through diplomacy.js's own dipArrangePassage / dipCallBack — one flow.
import { State } from '../../state.js';
import { fetchAuth } from '../../api.js';
import { esc } from '../format.js';
import { arrivalHTML, fmtClock } from '../time.js';

const possessive = name => name.endsWith('s') ? name + '’' : name + '’s';
const wanaxPoss = name => 'Wanax ' + possessive(name);

function since(iso) {
  const ms = Date.parse(iso);
  return Number.isFinite(ms) ? 'since ' + esc(fmtClock(ms)) : '';
}
function at(iso) { return iso ? arrivalHTML(iso, undefined, 'any moment') : ''; }
const join = bits => bits.filter(Boolean).join(' · ');

const OWN_GROUPS = [
  ['waiting', 'Waiting for a ship'],
  ['on_the_way', 'On the way'],
  ['aboard', 'Aboard a ship'],
  ['homebound', 'On the way home'],
  ['no_word', 'No word'],
];
const FOREIGN_GROUPS = [
  ['in_your_city', 'Others’ runners in your cities'],
  ['aboard_your_ship', 'Others’ runners aboard your ships'],
];
const KIND_WORD = { letter: 'Letter', offer: 'Trade offer', order: 'Order' };

function ownTitle(m) {
  if (m.group === 'no_word') return 'A runner of yours';
  if (m.order_summary) return esc(m.order_summary);
  if (m.group === 'homebound') return 'Home to ' + esc(m.to_city || 'your city');
  if (!m.to_city && !m.to_wanax) return 'A runner of yours';
  return 'To ' + (m.to_wanax ? esc('Wanax ' + m.to_wanax) + ' (' + esc(m.to_city || '') + ')' : esc(m.to_city));
}

function ownMeta(m) {
  const kind = KIND_WORD[m.kind];
  switch (m.group) {
    case 'waiting':
      return join([kind, m.port_name ? 'in ' + esc(m.port_name) : '', m.waiting_since ? since(m.waiting_since) : '']);
    case 'aboard':
      return join([kind, 'aboard ' + esc(m.ship_wanax ? wanaxPoss(m.ship_wanax) + ' ' : '') + esc(m.ship_name || 'a ship'),
        m.boarded_at ? since(m.boarded_at) : '', m.lands_at ? 'landfall ' + at(m.lands_at) : '']);
    case 'homebound':
      return join([kind, 'home ' + at(m.arrives_at)]);
    case 'no_word':
      return 'No word has come from it.';
    default:
      return join([kind, 'arrives ' + at(m.arrives_at)]);
  }
}

function ownActions(m) {
  if (m.group !== 'waiting') return '';
  let html = '';
  const ships = Array.isArray(m.eligible_ships) ? m.eligible_ships : [];
  if (m.can_arrange_passage && ships.length > 0) {
    const selId = 'dip-runner-ship-' + m.id;
    const opts = ships.map(s => '<option value="' + esc(s.id) + '">' + esc(s.name) + ' (' + esc(s.settlement_name) + ')</option>').join('');
    html += '<div class="runner-actions" id="dip-runner-block-' + esc(m.id) + '">'
      + '<select class="runner-select" id="' + selId + '">' + opts + '</select>'
      + '<button class="btn-small btn-primary" onclick="dipArrangePassage(\'' + esc(m.id) + '\',\'' + selId + '\',this,\'dip-runner-block-' + esc(m.id) + '\')">Arrange passage</button>'
      + '</div>';
  }
  if (m.can_call_back) {
    html += '<div class="runner-actions"><button class="btn-small btn-danger" onclick="dipCallBack(\'' + esc(m.id) + '\',this)">Call back</button></div>';
  }
  return html;
}

function ownRow(m) {
  return '<div class="runner-row">'
    + '<div class="runner-title">' + ownTitle(m) + '</div>'
    + '<div class="runner-meta">' + ownMeta(m) + '</div>'
    + (m.explanation ? '<div class="runner-explain">' + esc(m.explanation) + '</div>' : '')
    + ownActions(m) + '</div>';
}

function foreignRow(m) {
  const who = esc(wanaxPoss(m.sender_wanax)) + ' runner';
  if (m.group === 'aboard_your_ship') {
    return '<div class="runner-row"><div class="runner-title">' + who + ' aboard ' + esc(m.ship_name || 'your ship') + '</div>'
      + '<div class="runner-meta">' + join([m.heading_to ? 'bound for ' + esc(m.heading_to) : '', m.lands_at ? 'landfall ' + at(m.lands_at) : '']) + '</div></div>';
  }
  const waiting = m.waiting_for === 'a_ship' ? 'waiting for a ship' : 'waiting for your answer';
  return '<div class="runner-row"><div class="runner-title">' + who + ' in ' + esc(m.where || 'your city') + '</div>'
    + '<div class="runner-meta">' + join([waiting, m.since ? since(m.since) : '']) + '</div></div>';
}

// runnersTabHTML renders the whole tab from the server's answer.
export function runnersTabHTML(data) {
  const own = Array.isArray(data?.own) ? data.own : [];
  const foreign = Array.isArray(data?.foreign) ? data.foreign : [];
  if (!own.length && !foreign.length) {
    return '<p class="empty-state">No runners on the road, and none of anyone else’s that you know of.</p>';
  }
  const section = (rows, groups, render) => groups.map(([key, title]) => {
    const items = rows.filter(m => m.group === key);
    return items.length ? '<div class="dsec"><div class="dsec-title">' + title + '</div>' + items.map(render).join('') + '</div>' : '';
  }).join('');
  return section(own, OWN_GROUPS, ownRow) + section(foreign, FOREIGN_GROUPS, foreignRow);
}

export async function loadDipRunners() {
  const el = document.getElementById('dtab-runners');
  if (!el) return;
  el.innerHTML = '<div class="loading">Loading…</div>';
  try {
    const res = await fetchAuth('/api/v1/worlds/' + State.WORLD_ID + '/runners');
    if (!res.ok) throw new Error('runners unavailable');
    el.innerHTML = runnersTabHTML(await res.json());
  } catch (_) {
    el.innerHTML = '<p class="empty-state">Could not load your runners. Open Runners again to retry.</p>';
  }
}
