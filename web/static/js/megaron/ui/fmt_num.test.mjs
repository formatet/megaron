import test from 'node:test';
import assert from 'node:assert/strict';
import {fmtNum} from './fmt_num.js';
test('player quantities are plain digits with at most one decimal',()=>{
  for(const [n,expected] of [[0,'0'],[14,'14'],[123,'123'],[2026,'2,026'],[4.2,'4.2'],[5.64,'5.6'],[-0.04,'0'],[-3,'-3'],[Infinity,'unknown'],[NaN,'unknown'],[1.999,'2']]) assert.equal(fmtNum(n),expected);
});
import {fmtDays} from './fmt_num.js';
test('days agree in number',()=>{assert.equal(fmtDays(1),'1 game day');assert.equal(fmtDays(5),'5 game days');assert.equal(fmtDays(0),'0 game days');});
