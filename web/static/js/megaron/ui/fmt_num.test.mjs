import test from 'node:test';
import assert from 'node:assert/strict';
import {fmtNum} from './fmt_num.js';
test('player quantities are plain digits with at most one decimal',()=>{
  for(const [n,expected] of [[0,'0'],[14,'14'],[123,'123'],[2026,'2,026'],[4.2,'4.2'],[5.64,'5.6'],[-0.04,'0'],[-3,'-3'],[Infinity,'unknown'],[NaN,'unknown'],[1.999,'2']]) assert.equal(fmtNum(n),expected);
});
import {fmtDays} from './fmt_num.js';
test('days agree in number',()=>{assert.equal(fmtDays(1),'1 day');assert.equal(fmtDays(5),'5 days');assert.equal(fmtDays(0),'0 days');});
import {fmtDay} from './fmt_num.js';
test('day formatter handles displayed singular, qualifiers and absolute dates', () => {
  assert.equal(fmtDays(1.04), '1 day');
  assert.equal(fmtDays(1000), '1,000 days');
  assert.equal(fmtDays(1, 'unchallenged'), '1 unchallenged day');
  assert.equal(fmtDays(3, 'unchallenged'), '3 unchallenged days');
  assert.equal(fmtDay(3017), 'day 3,017');
});
