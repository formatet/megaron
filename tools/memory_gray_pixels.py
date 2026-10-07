#!/usr/bin/env python3
"""Actual Chromium canvas: lightness preservation, containment and hard edge.
Usage: python3 tools/memory_gray_pixels.py OUT
No game state or image editing. Draws a functional test canvas, reads pixels.
"""
import json,sys
from pathlib import Path
from playwright.sync_api import sync_playwright
root=Path(__file__).resolve().parents[1];out=Path(sys.argv[1]);out.mkdir(parents=True,exist_ok=True)
source=(root/'web/static/js/megaron/render/memory.js').read_text().replace('export function','function')
with sync_playwright() as p:
    browser=p.chromium.launch();page=browser.new_page()
    result=page.evaluate('''() => {'''+source+'''
      const c=document.createElement('canvas');c.width=64;c.height=32;const ctx=c.getContext('2d');
      const colors=['#1A5276','#3E7C9E','#859248','#ABA692','#9EA361','#315B36'];
      for(let x=0;x<64;x++) {ctx.fillStyle=colors[Math.floor(x/7)%colors.length];ctx.fillRect(x,0,1,32);}
      const before=ctx.getImageData(0,0,64,32).data;
      ctx.translate(0.35,0.15);ctx.scale(0.75,0.75);
      const points=[[2,2],[50,2],[55,18],[45,35],[2,35]];
      const m=ctx.getTransform(),spans=pixelSpans(points.map(([x,y])=>[m.a*x+m.e,m.d*y+m.f]),64,32);
      const mask=new Set();for(const [x,y,w] of spans) for(let i=x;i<x+w;i++)mask.add(y*64+i);
      drawRemembered(ctx,[{tier:'remembered',terrain:'plains'}],()=>points);
      const after=ctx.getImageData(0,0,64,32).data;let gray=0,untouched=0,maxLightError=0;
      const values=new Set();
      for(let n=0;n<64*32;n++) {const i=n*4;
        if(mask.has(n)) {
          if(Math.max(...after.slice(i,i+3))-Math.min(...after.slice(i,i+3))>1)throw Error('partially colored memory pixel');
          maxLightError=Math.max(maxLightError,Math.abs(after[i]-(.3*before[i]+.59*before[i+1]+.11*before[i+2])));
          values.add(after[i]);gray++;
        } else {
          if(before.slice(i,i+4).some((v,k)=>v!==after[i+k]))throw Error('changed pixel outside remembered mask');untouched++;
        }
      }
      if(maxLightError>1)throw Error('lightness changed');if(values.size<6)throw Error('flattened gray');
      return {grayPixels:gray,untouchedPixels:untouched,maxLightError,grayLevels:[...values].sort((a,b)=>a-b),compositeRestored:ctx.globalCompositeOperation==='source-over'};
    }''')
    assert result['compositeRestored'];browser.close()
(out/'canvas-pixels.json').write_text(json.dumps(result,indent=2)+'\n');print(json.dumps(result))
