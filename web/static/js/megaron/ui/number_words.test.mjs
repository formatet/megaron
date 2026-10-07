import test from 'node:test';
import assert from 'node:assert/strict';
import {numberWords} from './number_words.js';
test('player quantities keep zero, decimal/sign and large counts readable',()=>{
  for(const [n,expected] of [[0,'zero'],[14,'fourteen'],[30,'thirty'],[123,'one hundred twenty three'],[2026,'two thousand twenty six'],[4.2,'four point two'],[-0.1,'minus zero point one'],[Infinity,'unknown'],[1.999,'two']]) assert.equal(numberWords(n),expected);
});
