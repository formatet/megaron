// Pure text helpers for the Demolish button in the city drawer.

// "Farm @ (1,0)" for a hex building, "Market" for a city one.
export function demolishLabel(name, hexQ, hexR) {
  return hexQ != null && hexR != null ? `${name} @ (${hexQ},${hexR})` : name;
}

export function demolishPrompt(label) {
  return `Demolish ${label}? Nothing is refunded and the people working in it go idle. This cannot be undone.`;
}

// citizens_lifted comes straight from the server's reply.
export function demolishResultLine(label, lifted) {
  const n = Number(lifted) || 0;
  const who = n === 0 ? 'No one was working there.' : n === 1 ? '1 citizen is idle now.' : `${n} citizens are idle now.`;
  return `${label} demolished — the place is free to build on. ${who}`;
}
