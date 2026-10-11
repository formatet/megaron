import { createMarkRenderer } from './markraster.js';

const scene = { WORLD_ID: null, tileData: [], provinceData: [], camera: { zoom: 1 } };
const ground = createMarkRenderer(scene, () => new OffscreenCanvas(1, 1));
self.onmessage = ({ data }) => {
  try {
    if (data.type === 'scene') { Object.assign(scene, data.scene); return; }
    if (data.type !== 'bake') return;
    const start = performance.now();
    const chunk = ground.bakeChunk(data.ix, data.iy);
    chunk.img = chunk.img.transferToImageBitmap();
    if (chunk.mem) chunk.mem = chunk.mem.transferToImageBitmap();
    const transfer = [chunk.img, chunk.code.buffer, chunk.surf.buffer];
    if (chunk.mem) transfer.push(chunk.mem);
    self.postMessage({ type: 'chunk', generation: data.generation, key: data.key,
      chunk, bakeMs: performance.now() - start }, transfer);
  } catch { self.postMessage({ type: 'failed' }); }
};
// Probe inside the worker: OffscreenCanvas existing on Window is insufficient
// on engines that cannot use its 2D context or bitmap transfer in a worker.
try {
  const probe = new OffscreenCanvas(1, 1);
  if (!probe.getContext('2d')) throw new Error('No worker 2D canvas');
  probe.transferToImageBitmap().close();
  self.postMessage({ type: 'ready' });
} catch { self.postMessage({ type: 'failed' }); }
