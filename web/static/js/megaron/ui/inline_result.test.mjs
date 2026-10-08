import test from 'node:test';
import assert from 'node:assert/strict';
import { showInlineResult, confirmInline } from './inline_result.js';
class Element {
 constructor(tag='div') { this.tag=tag;this.children=[];this.dataset={};this.events={};this.attributes={}; }
 append(...items){this.children.push(...items);}
 appendChild(item){this.append(item);}
 replaceChildren(){this.children=[];}
 querySelector(){return this.children.find(x=>x.dataset.inlineResult);}
 setAttribute(key,value){this.attributes[key]=value;}
 addEventListener(key,fn){this.events[key]=fn;}
}
test('K: inline result is plain text, persistent, accessible and replaces the previous result',()=>{
 const prior=globalThis.document;globalThis.document={createElement:tag=>new Element(tag)};
 try {
  const host=new Element();const line=showInlineResult(host,'<img onerror=attack()>',true);
  assert.equal(line.textContent,'<img onerror=attack()>','server text stays literal');
  assert.equal(line.attributes['aria-live'],'polite');assert.match(line.className,/stat-warn/);
  assert.equal(showInlineResult(host,'Saved'),line,'reuse result');assert.equal(host.children.length,1);assert.equal(line.textContent,'Saved');assert.doesNotMatch(line.className,/stat-warn/);
 }finally{globalThis.document=prior;}
});
test('K: abandon opening and cancelling never dispatch, confirmation dispatches once',async()=>{
 const prior=globalThis.document;globalThis.document={createElement:tag=>new Element(tag)};
 try {
  const host=new Element();let calls=0;const send=async()=>{calls++;};
  confirmInline(host,'Abandon <city>?',send);assert.equal(calls,0,'opening must not dispatch');
  assert.equal(host.children[0].textContent,'Abandon <city>?');await host.children[2].events.click();assert.equal(calls,0,'cancel must not dispatch');assert.equal(host.children.length,0);
  confirmInline(host,'Abandon?',send);const yes=host.children[1];await yes.events.click();await yes.events.click();assert.equal(calls,1,'one confirmed dispatch');assert.equal(yes.disabled,true);
 }finally{globalThis.document=prior;}
});
