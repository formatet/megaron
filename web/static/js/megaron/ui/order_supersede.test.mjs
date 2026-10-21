import test from 'node:test';
import assert from 'node:assert/strict';
import { supersededOrderNote } from './format.js';

test('dispatch receipt explains that old unboarded order travels home physically',()=>{
 assert.equal(supersededOrderNote({}), '');
 assert.match(supersededOrderNote({superseded_messenger_ids:['old']}),/order withdrawn; its Runner is walking home/);
 assert.match(supersededOrderNote({superseded_messenger_ids:['a','b']}),/2 earlier unboarded orders withdrawn/);
});
