// Pure text helpers for the Demolish button in the city drawer.

// "Farm @ (1,0)" for a hex building, "Market" for a city one.
export function demolishLabel(name, hexQ, hexR) {
  return hexQ != null && hexR != null ? `${name} @ (${hexQ},${hexR})` : name;
}

export function demolishPrompt(label, kharis) {
  return `Demolish ${label}? Nothing is refunded and the people working in it go idle. This cannot be undone.${kharis ? ` ${demolitionKharisText(kharis)}` : ""}`;
}

// citizens_lifted comes straight from the server's reply.
export function demolishResultLine(label, lifted, kharis) {
  const n = Number(lifted) || 0;
  const who = n === 0 ? 'No one was working there.' : n === 1 ? '1 citizen is idle now.' : `${n} citizens are idle now.`;
  return `${label} demolished — the place is free to build on. ${who}${kharis ? ` ${demolitionKharisText(kharis)}` : ""}`;
}

// All values come from the server quote or committed result; no local penalty rule.
export function demolitionKharisText(k) {
  return `The god ${k.god_id} is angered: kharis −${k.kharis_loss} (${k.kharis_before} → ${k.kharis_after}; floor 1). Realm ceiling ${k.cap_before} → ${k.cap_after}. A lower ceiling stops growth; it does not cut existing kharis. Above the ceiling, ordinary daily upkeep continues and temple gains do not offset it.`;
}
