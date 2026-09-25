// Stance orders, as the War drawer and the march menu word them — pure, so
// node can test them without a DOM (war.js touches `document` at import).
//
// A marching unit takes a stance too (megaron_styrande_beslut §11, Timothy
// 2026-09-25): the order rides a Runner that must catch up with it. The
// server's 202 says how: catch_up "on_the_march" (aimed at the hex where the
// Runner overtakes the unit — redirect's intercept) or "at_destination" (no
// Runner can overtake it, so it follows the unit to where it stops).

// canTakeStance mirrors the server's gate: land units only, standing
// (garrison/positioned) or on the march.
export function canTakeStance(u) {
  if (!u || u.category === 'naval') return false;
  return u.status === 'garrison' || u.status === 'positioned' || u.status === 'marching';
}

// stanceSentLine words a stance order's 202 receipt. `when` is the already
// formatted courier arrival (fmtArrival(d.courier_arrives_at)).
export function stanceSentLine(d, when) {
  const s = (d && d.stance) || 'stance';
  const at = d && d.intercept_q != null ? ' at (' + d.intercept_q + ',' + d.intercept_r + ')' : '';
  if (d && d.catch_up === 'on_the_march') {
    return '🏃 ' + s + ': the unit is marching, so the Runner must catch up — reaches it' + at + ' ~' + when
      + '. The stance bites where the unit stops.';
  }
  if (d && d.catch_up === 'at_destination') {
    return '🏃 ' + s + ': no Runner can overtake the marching unit — it follows it to its destination' + at
      + ', reaching it ~' + when + '. The stance applies where it stopped.';
  }
  return '🏃 Runner carries the stance order — reaches the unit ~' + when + ' and applies on delivery.';
}
