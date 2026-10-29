import test from 'node:test';
import assert from 'node:assert/strict';
import { notifText, notifDomain, godsAngeredText } from './format.js';

test('GodsAngered says the gods heard and what the realm lost', () => {
  const body = { words: 2, where: 'letter', kharis_lost: 10, kharis_before: 50, kharis_after: 40 };
  assert.equal(notifText('GodsAngered', body), 'The gods heard what was written in your name. Your realm loses 10 kharis.');
  assert.equal(notifDomain('GodsAngered'), 'kult');
});

test('GodsAngered at the floor says nothing more was lost', () => {
  assert.match(godsAngeredText({ kharis_lost: 0 }), /already at its floor/);
});
