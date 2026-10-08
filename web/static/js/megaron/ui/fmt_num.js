// Player prose shows plain digits: "5.6", "1,000". Spelled-out numbers read
// slower and hide the size of a quantity (Timothy 2026-10-08). At most one
// decimal; a non-number is "unknown", never "NaN".
export function fmtNum(value) {
  const n = Number(value);
  if (!Number.isFinite(n)) return 'unknown';
  const rounded = Math.round(n * 10) / 10;
  return (Object.is(rounded, -0) ? 0 : rounded).toLocaleString('en-US', { maximumFractionDigits: 1 });
}

// "1 game day" / "5 game days".
export function fmtDays(value) {
  return fmtNum(value) + (Number(value) === 1 ? ' game day' : ' game days');
}
