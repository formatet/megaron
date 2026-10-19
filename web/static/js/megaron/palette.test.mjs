import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const css = readFileSync(new URL('../../megaron.css', import.meta.url), 'utf8');
const root = css.match(/:root\s*\{([^}]+)\}/)[1].replace(/\/\*[\s\S]*?\*\//g, '');
const colors = new Map([...root.matchAll(/(--[\w-]+)\s*:\s*(#[\da-f]{6})\s*;/gi)]
  .map(([, key, value]) => [key, value]));
function luminance(hex) {
  return [0.2126, 0.7152, 0.0722].reduce((sum, weight, i) => {
    const channel = parseInt(hex.slice(1 + i * 2, 3 + i * 2), 16) / 255;
    return sum + weight * (channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4);
  }, 0);
}
function contrast(a, b) {
  const [lo, hi] = [luminance(a), luminance(b)].sort((x, y) => x - y);
  return (hi + 0.05) / (lo + 0.05);
}
test('contrast calibration: black/white 21, identical colors 1', () => {
  assert.equal(contrast('#000000', '#ffffff'), 21);
  assert.equal(contrast('#F2EAD3', '#F2EAD3'), 1);
});
test('semantic text tokens exist and every *-text token meets 4.5:1 on papyrus', () => {
  for (const key of ['--safe-text', ...['trade', 'city', 'oikos', 'system'].map(k => `--accent-${k}-text`)]) {
    assert.ok(colors.has(key), `missing ${key}`);
  }
  const tokens = [...root.matchAll(/(--[\w-]+-text)\s*:\s*([^;]+);/g)];
  assert.ok(tokens.length >= 5);
  for (const [, key] of tokens) {
    assert.ok(colors.has(key), `${key}: expected explicit six-digit sRGB color`);
    const ratio = contrast(colors.get(key), colors.get('--bg-panel'));
    assert.ok(ratio >= 4.5, `${key}: ${ratio.toFixed(4)}:1 < 4.5:1`);
  }
});
