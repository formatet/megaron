// Server-owned physical projection, shared by Host marker, camera and panels.
// A newer founding read can beat a unit poll; never prefer a stale departure q/r.
export function physicalHostState(phase, units=[]) {
 if (!phase?.active) return null;
 const host=units.find(u=>u.id===phase.host_unit_id && u.type==='nomadic_host');
 const states=[host?.host_state,phase.host_state].filter(s=>s && Number.isInteger(s.q) && Number.isInteger(s.r));
 return states.sort((a,b)=>(b.at_milli??-1)-(a.at_milli??-1))[0] || null;
}
export function hostStatusText(state) {
 const text={marching:'Marching',waiting:'Waiting',can_found_here:'Can found here'}[state?.status] || '';
 return text && state.reason ? text+' — '+state.reason : text;
}
