import { esc } from './format.js';

// Knowledge belongs to the Cities API, also consumed by Diplomacy → Known.
export function destinationCities(rows) {
  return (Array.isArray(rows) ? rows : []).filter(c => c.knowledge === 'known'
    && c.settlement_id && c.name && Number.isInteger(c.q) && Number.isInteger(c.r))
    .sort((a,b) => Number(!!b.own)-Number(!!a.own) || a.name.localeCompare(b.name)
      || a.settlement_id.localeCompare(b.settlement_id));
}
export function destinationCityOptions(cities) {
  const group = (own, label) => {
    const entries=cities.filter(c => !!c.own === own);
    if (!entries.length) return '';
    return `<optgroup label="${label}">`+entries.map(c => `<option value="${esc(c.settlement_id)}">${esc(c.name)}${c.own ? '' : ' — '+esc(c.owner || 'Owner unknown')} (${c.q},${c.r})</option>`).join('')+'</optgroup>';
  };
  return '<option value="">Choose a city…</option>'+group(true,'Your cities')+group(false,'Known cities');
}
export function destinationFromCity(city) {
  return {q:city.q,r:city.r,terrain:city.terrain,isSea:false,known:true,
    name:city.name,isSettlement:true,allied:!!city.own,settlementID:city.settlement_id};
}
export function cityDestinationLabel(dest) {
  return dest?.isSettlement && dest.name ? ` to ${dest.name}` : '';
}
