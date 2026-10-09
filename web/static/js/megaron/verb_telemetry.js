// Successful API mutations, not clicks or eventual worker outcomes.
// Policy: megaron_plan_umami.md. Only explicit routes and finite public enums;
// no URL, request/response object, names, IDs, quantities or text leave here.
import { track } from './telemetry.js';

export const VERB_EVENTS = [
  {"verb": "build", "method": "POST", "route": "/api/v1/worlds/:world/provinces/:province/build", "event": "build_started"},
  {"verb": "cancel-build", "method": "DELETE", "route": "/api/v1/worlds/:world/provinces/:province/build-queue/:queue", "event": "build_cancelled"},
  {"verb": "recruit", "method": "POST", "route": "/api/v1/worlds/:world/provinces/:province/recruit", "event": "recruit_started"},
  {"verb": "place", "method": "POST", "route": "/api/v1/worlds/:world/provinces/:province/placements", "event": "worker_placed"},
  {"verb": "staff", "method": "POST", "route": "/api/v1/worlds/:world/provinces/:province/placements", "event": "workplace_staffed"},
  {"verb": "unplace", "method": "DELETE", "route": "/api/v1/worlds/:world/provinces/:province/placements/:ordinal", "event": "placement_removed"},
  {"verb": "labor", "method": "PUT", "route": "/api/v1/worlds/:world/provinces/:province/labor", "event": "cult_allocated"},
  {"verb": "slaughter", "method": "POST", "route": "/api/v1/worlds/:world/provinces/:province/slaughter-livestock", "event": "livestock_slaughtered"},
  {"verb": "disband", "method": "POST", "route": "/api/v1/worlds/:world/provinces/:province/disband", "event": "army_disbanded"},
  {"verb": "transfer", "method": "POST", "route": "/api/v1/worlds/:world/provinces/:province/trade", "event": "transfer_sent"},
  {"verb": "gift/tribute", "method": "POST", "route": "/api/v1/worlds/:world/provinces/:province/trade", "event": "gift_sent"},
  {"verb": "abandon", "method": "POST", "route": "/api/v1/worlds/:world/settlements/:settlement/abandon", "event": "settlement_abandoned"},
  {"verb": "rite", "method": "POST", "route": "/api/v1/worlds/:world/settlements/:settlement/rite", "event": "rite_performed"},
  {"verb": "gift", "method": "POST", "route": "/api/v1/worlds/:world/settlements/:settlement/gift", "event": "loyalty_gift_sent"},
  {"verb": "occupation-order", "method": "POST", "route": "/api/v1/worlds/:world/settlements/:settlement/occupation-order", "event": "occupation_order_sent"},
  {"verb": "march", "method": "POST", "route": "/api/v1/worlds/:world/units/:unit/march", "event": "march_sent"},
  {"verb": "recall", "method": "POST", "route": "/api/v1/worlds/:world/units/:unit/recall", "event": "recall_sent"},
  {"verb": "redirect", "method": "POST", "route": "/api/v1/worlds/:world/units/:unit/recall", "event": "redirect_sent"},
  {"verb": "stance", "method": "POST", "route": "/api/v1/worlds/:world/units/:unit/stance", "event": "stance_sent"},
  {"verb": "retreat-order", "method": "POST", "route": "/api/v1/worlds/:world/units/:unit/standing-orders", "event": "retreat_order_sent"},
  {"verb": "retreat-default", "method": "PUT", "route": "/api/v1/worlds/:world/retreat-default", "event": "retreat_default_changed"},
  {"verb": "reinforce", "method": "POST", "route": "/api/v1/worlds/:world/units/:unit/reinforce", "event": "reinforce_sent"},
  {"verb": "repair", "method": "POST", "route": "/api/v1/worlds/:world/units/:unit/repair", "event": "repair_started"},
  {"verb": "load", "method": "POST", "route": "/api/v1/worlds/:world/units/:unit/load", "event": "troops_loaded"},
  {"verb": "unload", "method": "POST", "route": "/api/v1/worlds/:world/units/:unit/unload", "event": "troops_unloaded"},
  {"verb": "join", "method": "POST", "route": "/api/v1/worlds/:world/join", "event": "world_joined"},
  {"verb": "founding settle", "method": "POST", "route": "/api/v1/worlds/:world/founding/settle", "event": "settle"},
  {"verb": "message", "method": "POST", "route": "/api/v1/worlds/:world/settlements/:settlement/messengers", "event": "messenger_sent"},
  {"verb": "trade-offer", "method": "POST", "route": "/api/v1/worlds/:world/settlements/:settlement/messengers", "event": "trade_offer"},
  {"verb": "message", "method": "POST", "route": "/api/v1/worlds/:world/founding/messengers", "event": "messenger_sent"},
  {"verb": "trade-offer", "method": "POST", "route": "/api/v1/worlds/:world/founding/messengers", "event": "trade_offer"},
  {"verb": "reply", "method": "POST", "route": "/api/v1/worlds/:world/messengers/:messenger/reply", "event": "reply_sent"},
  {"verb": "trade-accept", "method": "POST", "route": "/api/v1/worlds/:world/messengers/:messenger/trade-accept", "event": "trade_accepted"},
  {"verb": "trade-decline", "method": "POST", "route": "/api/v1/worlds/:world/messengers/:messenger/trade-decline", "event": "trade_declined"},
  {"verb": "trade-cancel", "method": "POST", "route": "/api/v1/worlds/:world/messengers/:messenger/trade-cancel", "event": "trade_cancelled"},
  {"verb": "arrange passage", "method": "POST", "route": "/api/v1/worlds/:world/messengers/:messenger/passage", "event": "passage_arranged"},
  {"verb": "fetch by ship", "method": "POST", "route": "/api/v1/worlds/:world/units/:unit/pickup", "event": "pickup_sent"},
  {"verb": "call back", "method": "POST", "route": "/api/v1/worlds/:world/messengers/:messenger/call-back", "event": "runner_called_back"},
  {"verb": "standing order", "method": "POST", "route": "/api/v1/worlds/:world/standing-orders", "event": "standing_order_created"},
  {"verb": "standing order pause", "method": "POST", "route": "/api/v1/worlds/:world/standing-orders/:order/pause", "event": "standing_order_paused"},
  {"verb": "standing order resume", "method": "POST", "route": "/api/v1/worlds/:world/standing-orders/:order/resume", "event": "standing_order_resumed"},
  {"verb": "standing order delete", "method": "DELETE", "route": "/api/v1/worlds/:world/standing-orders/:order", "event": "standing_order_deleted"},
  {"verb": "report", "method": "POST", "route": "/api/v1/worlds/:world/reports", "event": "report_sent"},
  {"verb": "mark notifications read (all)", "method": "POST", "route": "/api/v1/worlds/:world/notifications/read-all", "event": "notifications_read"},
  {"verb": "mark notification read (one)", "method": "POST", "route": "/api/v1/worlds/:world/notifications/:notification/read", "event": "notification_read"},
  {"verb": "delete notifications", "method": "DELETE", "route": "/api/v1/worlds/:world/notifications", "event": "notifications_deleted"},
  {"verb": "password", "method": "POST", "route": "/api/v1/auth/password", "event": "password_changed"},
  {"verb": "agora password", "method": "POST", "route": "/api/v1/agora/password", "event": "agora_password_changed"},
  {"verb": "notification-preferences", "method": "PUT", "route": "/api/v1/notification-preferences/:kind", "event": "notification_preferences_changed"},
  {"verb": "notification-preferences", "method": "DELETE", "route": "/api/v1/notification-preferences/:kind", "event": "notification_preferences_changed"},
];

const routes = VERB_EVENTS.map(entry => ({ ...entry,
  pattern: new RegExp('^' + entry.route.replace(/:[a-z]+/g, '[^/?]+') + '$'),
}));
const BUILDINGS = ["farm", "barracks", "mine", "lumbermill", "stonequarry", "market", "wall", "harbour", "shipyard", "foundry", "stable", "temple", "olive_press", "winery"];
const UNITS = ["spearman", "elite_infantry", "war_chariot", "galley", "war_galley", "merchantman", "nomadic_host"];
const RITES = ["akhaier_oracle_deposits", "akhaier_harvest_blessing", "akhaier_battle_frenzy", "khemetiu_oracle_deposits", "khemetiu_harvest_blessing", "khemetiu_battle_frenzy", "knaani_oracle_deposits", "knaani_harvest_blessing", "knaani_battle_frenzy", "thrakes_oracle_deposits", "thrakes_harvest_blessing", "thrakes_battle_frenzy", "minoan_oracle_deposits", "minoan_harvest_blessing", "minoan_battle_frenzy", "hatti_oracle_deposits", "hatti_harvest_blessing", "hatti_battle_frenzy"];
const GOODS = ['grain', 'fish', 'livestock', 'horses', 'timber', 'stone', 'copper', 'tin', 'bronze', 'silver', 'oil', 'wine', 'pottery', 'purple'];
const INTENTS = ['march', 'colonize', 'explore', 'landing', 'fortify', 'sentry', 'raid', 'support', 'besiege'];
const REASON_CODES = ['insufficient_goods'];

function requestBody(opts) {
  try { return typeof opts.body === 'string' ? JSON.parse(opts.body) || {} : {}; }
  catch (_) { return {}; }
}
function responseBody(res) {
  try { return res.clone().json().then(body => body || {}).catch(() => ({})); }
  catch (_) { return Promise.resolve({}); }
}
function redirected(body) { return body.target_q != null && body.target_r != null; }

export function verbForRequest(url, opts = {}) {
  const method = (opts.method || 'GET').toUpperCase();
  if (!['POST', 'PUT', 'PATCH', 'DELETE'].includes(method)) return null;
  const path = url.split('?')[0];
  const body = requestBody(opts);
  return routes.find(entry => entry.method === method && entry.pattern.test(path)
    && (entry.verb !== 'staff' || body.target_kind === 'building')
    && (entry.verb !== 'place' || body.target_kind !== 'building')
    && (entry.verb !== 'redirect' || redirected(body))
    && (entry.verb !== 'recall' || !redirected(body))
    && (entry.verb !== 'trade-offer' || !!body.trade_offer)
    && (entry.verb !== 'message' || !body.trade_offer)
    && entry.verb !== 'gift/tribute') || null;
}

function propsFor(entry, opts) {
  const body = requestBody(opts), props = {};
  const include = (key, value, allowed) => { if (allowed.includes(value)) props[key] = value; };
  if (entry.verb === 'build') include('building', body.building_type, BUILDINGS);
  if (entry.verb === 'recruit') include('unit', body.unit_type, UNITS);
  if (entry.verb === 'rite') include('rite', body.prayer, RITES);
  if (entry.verb === 'march') include('intent', body.intent || body.stance || 'march', INTENTS);
  if (entry.verb === 'stance') include('stance', body.stance, INTENTS);
  if (entry.verb === 'trade-offer') include('kind', body.trade_offer?.kind, ['buy', 'sell']);
  if (['place', 'staff', 'transfer'].includes(entry.verb)) include('good', body.good_key, GOODS);
  if (entry.verb === 'notification-preferences') props.action = opts.method.toUpperCase() === 'PUT' ? 'mute' : 'unmute';
  return Object.keys(props).length ? props : undefined;
}

// No response-body read delays the UI or consumes the caller's response. A
// blocked tracker, malformed JSON or an unsupported clone never changes play.
export function trackVerbResponse(url, opts, res) {
  try {
    const entry = verbForRequest(url, opts);
    if (!entry) return;
    if (res.ok) {
      const props = propsFor(entry, opts);
      if (entry.verb === 'transfer') {
        return responseBody(res).then(body => {
          track(body.kind === 'gift' ? 'gift_sent' : entry.event, props);
        }).catch(() => {});
      }
      track(entry.event, props);
    } else if (res.status >= 400 && res.status < 500) {
      const emit = body => {
        const props = { verb: entry.verb };
        const code = body.error_code || body.error;
        if (REASON_CODES.includes(code)) props.reason_code = code;
        track('verb_refused', props);
      };
      return responseBody(res).then(emit).catch(() => {});
    }
  } catch (_) { /* telemetry never changes API behavior */ }
}
