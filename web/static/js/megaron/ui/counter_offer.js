import {serverNow} from '../clock.js';
// A counter-offer is a new offer in the recipient's role, never an acceptance.
export function counterOfferDefaults(offer) {
 if(!offer||!['buy','sell'].includes(offer.kind||'buy'))throw Error('No trade offer to counter.');
 const buy=(offer.kind||'buy')==='buy';
 const good=buy?offer.want_good:offer.offer_good,qty=Number(buy?offer.want_qty:offer.offer_qty),silver=Number(buy?offer.offer_silver:offer.want_silver);
 if(!good||!Number.isFinite(qty)||qty<=0||!Number.isFinite(silver)||silver<=0)throw Error('Offer terms unavailable.');
 return {...(buy?{kind:'sell',offer_good:good,offer_qty:qty,want_silver:silver}:{kind:'buy',want_good:good,want_qty:qty,offer_silver:silver}),...(['land','sea'].includes(offer.by)?{by:offer.by}:{})};
}
export function canCounterOffer(letter,now=serverNow()) {
 return letter?.direction==='incoming'&&!!letter.trade_offer&&Array.isArray(letter.actions)&&letter.actions.includes('trade_counter')&&(!letter.expires_at||Date.parse(letter.expires_at)>now);
}
export function counterScopeMatches(scope,world,player,person,id) {
 return !!scope&&scope.world===world&&scope.player===player&&scope.person===person&&scope.id===id;
}
