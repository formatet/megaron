// Main-thread queue for one worker. Only one chunk is in flight, so camera
// changes replace queued work immediately instead of sitting behind a stale view.
export function createChunkWorker(makeWorker, onChunk, onFailure) {
  let worker;
  try { worker = makeWorker(); } catch { return null; }
  let ready = false, failed = false, generation = 0, active = null, queue = [];
  const dispose = ch => { ch?.img?.close?.(); ch?.mem?.close?.(); };
  const pump = () => {
    if (!ready || failed || active || !queue.length) return;
    active = { ...queue.shift(), generation };
    try { worker.postMessage({ type: 'bake', ...active }); } catch { fail(); }
  };
  const fail = () => {
    if (failed) return;
    failed = true; clearTimeout(startup); worker.terminate();
    active = null; queue = []; onFailure();
  };
  const startup = setTimeout(fail, 5000);
  worker.onerror = fail;
  worker.onmessageerror = fail;
  worker.onmessage = ({ data }) => {
    if (failed) { dispose(data.chunk); return; }
    if (data.type === 'ready') { ready = true; clearTimeout(startup); pump(); return; }
    if (data.type === 'failed') { fail(); return; }
    if (data.type !== 'chunk') return;
    const job = active;
    active = null;
    // A refresh/world change must never publish an old scene into the new cache.
    if (job && data.generation === generation && data.generation === job.generation && data.key === job.key) {
      onChunk(job, data.chunk, data.bakeMs);
    } else dispose(data.chunk);
    pump();
  };
  return {
    get pending() { return failed ? 0 : queue.length + (active ? 1 : 0); },
    setScene(scene) {
      generation++; queue = [];
      try { worker.postMessage({ type: 'scene', scene }); } catch { fail(); }
    },
    request(jobs) {
      queue = jobs.filter(j => !(active?.generation === generation && j.key === active.key && j.sig === active.sig));
      pump();
    },
  };
}
