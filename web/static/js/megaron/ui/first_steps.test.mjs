import test from 'node:test';
import assert from 'node:assert/strict';
import { cardModel, renderCard } from './first_steps.js';

const body = (over = {}) => ({
  hidden: false, done_count: 1, total: 4, next: 'work', closing_text: 'That is the beginning.',
  steps: [
    { key: 'found', done: true, text: 'Find us a home.' },
    { key: 'work', done: false, text: 'There are idle hands in Petras.' },
    { key: 'build', done: false, text: 'Order a building.' },
    { key: 'explore', done: false, text: 'Send people out.' },
  ], ...over,
});

test('shows exactly the next step as one line with progress', () => {
  assert.deepEqual(cardModel(body()), { text: 'There are idle hands in Petras.', progress: '1/4', closing: false });
});

test('hidden by the player: nothing shows', () => {
  assert.equal(cardModel(body({ hidden: true })), null);
});

test('closing line shows once, then never again', () => {
  const done = body({ next: '', done_count: 4 });
  assert.deepEqual(cardModel(done), { text: 'That is the beginning.', progress: '4/4', closing: true });
  assert.equal(cardModel(done, { closedBefore: true }), null);
});

test('the wait line gets a wall-clock time, or "soon" without an anchor', () => {
  const wait = body({ next: 'build', done_count: 4, steps: [{ key: 'build', done: true, ready_tick: 1010, text: 'The Granary is ready {when}. Come back then.' }] });
  assert.equal(cardModel(wait, { when: t => `today ${t}` }).text, 'The Granary is ready today 1010. Come back then.');
  assert.equal(cardModel(wait, { when: () => 'time unavailable' }).text, 'The Granary is ready soon. Come back then.');
});

test('renderCard writes text and progress, hides on null', () => {
  const el = { hidden: true, classList: { toggle() {} }, parts: {}, querySelector(s) { return this.parts[s] ??= { textContent: '' }; } };
  renderCard(el, { text: 'Hello', progress: '2/4', closing: false });
  assert.equal(el.hidden, false);
  assert.equal(el.parts['.lawagetas-text'].textContent, 'Hello');
  assert.equal(el.parts['.lawagetas-progress'].textContent, '2/4');
  renderCard(el, null);
  assert.equal(el.hidden, true);
});
