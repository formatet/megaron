// The server category owns grouping; state belongs to each force card.
export function forceSections(forces) {
 const people=forces.filter(u=>u.type==='nomadic_host');
 const land=forces.filter(u=>u.type!=='nomadic_host'&&u.category!=='naval');
 const ships=forces.filter(u=>u.category==='naval'&&u.type!=='nomadic_host');
 return [{name:'Land Forces',forces:land},{name:'Ships',forces:ships},...(people.length?[{name:'People',forces:people}]:[])];
}
