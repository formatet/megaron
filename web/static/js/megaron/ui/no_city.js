// What an empty city / economy / recruit window says before the first city exists.
// The way to a city is the Host, not a hex: one sentence, three windows.
const WAY = 'Click your Host on the map, then press Found the metropolis here.';

export function noCityText(what, founderPhase) {
  if (!founderPhase) return what;
  return `${what} ${WAY}`;
}
