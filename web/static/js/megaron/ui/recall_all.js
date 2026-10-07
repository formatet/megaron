import {esc} from './format.js';

export function recallAllControlsHTML(units, result = '', busy = false) {
  const disabled = busy || !units.some(u => u.status === 'marching');
  return '<button class="btn-small btn-primary" id="recall-all-send" onclick="unitRecallAll()"' + (disabled ? ' disabled' : '') + '>Recall all</button>'
    + '<div id="recall-all-result" role="status" aria-live="polite">' + result + '</div>';
}

// Only the client loops. recallOne is the existing single-unit action.
export async function recallAll(units, recallOne) {
  const results = [];
  for (const u of units.filter(u => u.status === 'marching')) {
    try {
      const result = await recallOne(u.id);
      results.push({name: u.display_name || u.name || u.id, ...result});
    } catch (error) {
      results.push({name: u.display_name || u.name || u.id, ok: false, error: error.message || 'Recall failed.'});
    }
  }
  return results;
}

export function recallAllSummary(count) {
  if (!count) return 'No units are being called home.';
  const small = ['Zero','One','Two','Three','Four','Five','Six','Seven','Eight','Nine','Ten','Eleven','Twelve','Thirteen','Fourteen','Fifteen','Sixteen','Seventeen','Eighteen','Nineteen'];
  return (small[count] || String(count)) + (count === 1 ? ' unit is' : ' units are') + ' being called home.';
}

export function recallAllResultHTML(results) {
  return '<p>' + recallAllSummary(results.filter(r => r.ok).length) + '</p>'
    + results.filter(r => !r.ok).map(r => '<p>' + esc(r.name) + ': ' + esc(r.error || 'Recall failed.') + '</p>').join('');
}
