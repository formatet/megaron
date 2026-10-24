import test from 'node:test';
import assert from 'node:assert/strict';
import {fmtNum} from './fmt_num.js';
test('player quantities are plain digits with at most one decimal',()=>{
  for(const [n,expected] of [[0,'0'],[14,'14'],[123,'123'],[2026,'2,026'],[4.2,'4.2'],[5.64,'5.6'],[-0.04,'0'],[-3,'-3'],[Infinity,'unknown'],[NaN,'unknown'],[1.999,'2']]) assert.equal(fmtNum(n),expected);
});
import {fmtDays} from './fmt_num.js';
test('game days agree in number',()=>{assert.equal(fmtDays(1),'1 game day');assert.equal(fmtDays(5),'5 game days');assert.equal(fmtDays(0),'0 game days');});
test('duration formatter handles displayed singular and qualifiers', () => {
  assert.equal(fmtDays(1.04), '1 game day');
  assert.equal(fmtDays(1000), '1,000 game days');
  assert.equal(fmtDays(1, 'unchallenged'), '1 unchallenged game day');
  assert.equal(fmtDays(3, 'unchallenged'), '3 unchallenged game days');
});
import {fmtDuration} from './fmt_num.js';
test('game duration uses actual world cadence, with no alpha-speed fallback',()=>{
 assert.equal(fmtDuration(12,1800),'12 game days · about 6 hours');
 assert.equal(fmtDuration(12,3600),'12 game days · about 12 hours');
 assert.equal(fmtDuration(12,6),'12 game days · about 1.2 minutes');
 assert.equal(fmtDuration(1,3600),'1 game day · about 1 hour');
 assert.equal(fmtDuration(12,null),'12 game days');
 assert.equal(fmtDuration(12,Infinity),'12 game days');
});
