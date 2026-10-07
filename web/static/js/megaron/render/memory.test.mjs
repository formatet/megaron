import test from 'node:test';
import assert from 'node:assert/strict';
import { drawRemembered, pixelSpans } from './memory.js';

function context() {
  return {canvas: {width: 20, height: 20}, globalCompositeOperation: 'source-over', globalAlpha: 0.4, fillStyle: 'red', calls: [],
    getTransform: () => ({a: 1, b: 0, c: 0, d: 1, e: 0, f: 0}),
    save() { this.saved = [this.globalCompositeOperation, this.globalAlpha, this.fillStyle]; },
    restore() { [this.globalCompositeOperation, this.globalAlpha, this.fillStyle] = this.saved; },
    resetTransform() {}, beginPath() {}, rect(...args) { this.calls.push(args); },
    fill() { this.filled = [this.globalCompositeOperation, this.globalAlpha, this.fillStyle]; }};
}
test('only remembered hexes are desaturated; live, fog and absent tier stay untouched', () => {
  const ctx=context(), chosen=[];
  drawRemembered(ctx, [
    {q:1,tier:'remembered',terrain:'plains'}, {q:2,tier:'live',terrain:'plains'},
    {q:3,tier:'fog',terrain:'fog'}, {q:4,terrain:'plains'}, {q:5,tier:'remembered',terrain:'fog'},
  ], t => { chosen.push(t.q); return [[2,2],[6,2],[6,6],[2,6]]; });
  assert.deepEqual(chosen,[1], 'remembered only');
  assert.deepEqual(ctx.filled,['saturation',1,'#808080'],'full desaturation preserves destination lightness');
  assert.equal(ctx.globalCompositeOperation,'source-over','composite restored');
  assert.equal(ctx.globalAlpha,0.4); assert.equal(ctx.fillStyle,'red');
  assert.deepEqual(ctx.calls,[[2,2,4,1],[2,3,4,1],[2,4,4,1],[2,5,4,1]]);
});
test('frontier spans are integer device pixels and clipped to viewport at fractional zoom', () => {
  assert.deepEqual(pixelSpans([[-1.2,0.2],[4.8,0.2],[2.1,3.9]],4,3),[[0,0,4],[0,1,4],[1,2,2]]);
});
test('empty memory makes no canvas change; failures restore canvas state', () => {
  const ctx=context(); drawRemembered(ctx,[{tier:'live'}],()=>{throw Error('unexpected')});
  assert.equal(ctx.saved,undefined);
  assert.throws(()=>drawRemembered(ctx,[{tier:'remembered'}],()=>{throw Error('fixture')}),/fixture/);
  assert.equal(ctx.globalCompositeOperation,'source-over');
});
