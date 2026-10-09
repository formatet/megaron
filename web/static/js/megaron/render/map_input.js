// One gesture path for mouse, touch and pen. Coordinates remain in viewport
// pixels; the map owns hit-testing, camera clamps and all game affordances.
export function bindMapInput(canvas, { pan, zoom, tap, orders, hover, hideHover }, {
  schedule = setTimeout, cancel = clearTimeout,
} = {}) {
  const pointers = new Map();
  let timer = null, consumed = false, pair = null, lastPointerType = null;
  const stopTimer = () => { if (timer !== null) cancel(timer); timer = null; };
  const centre = () => {
    const [a, b] = [...pointers.values()];
    return { x: (a.x + b.x) / 2, y: (a.y + b.y) / 2,
      distance: Math.hypot(a.x - b.x, a.y - b.y) };
  };
  const moved = (p, e) => Math.hypot(e.clientX - p.startX, e.clientY - p.startY) >= 4;
  const capture = id => {
    // Synthetic browser probes and detached canvases have no active native
    // pointer to capture. Real pointerdown always does.
    if (canvas.setPointerCapture) {
      try { canvas.setPointerCapture(id); } catch (_) { /* no active pointer */ }
    }
  };
  const release = id => {
    if (canvas.hasPointerCapture?.(id)) canvas.releasePointerCapture(id);
  };
  canvas.addEventListener('pointerdown', e => {
    lastPointerType = e.pointerType;
    if (e.pointerType === 'mouse' && e.button !== 0) return;
    if (!pointers.size) consumed = false;
    pointers.set(e.pointerId, { x: e.clientX, y: e.clientY,
      startX: e.clientX, startY: e.clientY });
    if (e.isTrusted !== false) capture(e.pointerId);
    stopTimer();
    if (pointers.size > 1) { consumed = true; pair = centre(); }
    else if (e.pointerType !== 'mouse') {
      hideHover();
      timer = schedule(() => {
        timer = null;
        if (pointers.size !== 1 || consumed) return;
        consumed = true;
        orders(e.clientX, e.clientY);
      }, 500);
    }
  });
  canvas.addEventListener('pointermove', e => {
    lastPointerType = e.pointerType;
    const p = pointers.get(e.pointerId);
    if (p) {
      if (moved(p, e)) { consumed = true; stopTimer(); }
      const dx = e.clientX - p.x, dy = e.clientY - p.y;
      p.x = e.clientX; p.y = e.clientY;
      if (pointers.size > 1) {
        const next = centre();
        // Scale around the old midpoint, then carry it to the new midpoint.
        // This also permits two-finger panning; the map clamps once per step.
        if (pair.distance > 0 && next.distance > 0) {
          zoom(next.distance / pair.distance, pair.x, pair.y,
            next.x - pair.x, next.y - pair.y);
        }
        pair = next;
      } else {
        pan(dx, dy);
      }
    }
    if (e.pointerType === 'mouse') hover(e.clientX, e.clientY);
  });
  canvas.addEventListener('pointerup', e => {
    const p = pointers.get(e.pointerId);
    if (!p) return;
    stopTimer();
    const select = pointers.size === 1 && !consumed && !moved(p, e);
    pointers.delete(e.pointerId);
    pair = pointers.size > 1 ? centre() : null;
    release(e.pointerId);
    if (select) tap(e.clientX, e.clientY);
  });
  const cancelPointer = e => {
    if (!pointers.has(e.pointerId)) return;
    stopTimer(); consumed = true;
    pointers.delete(e.pointerId);
    pair = pointers.size > 1 ? centre() : null;
    release(e.pointerId);
    hideHover();
  };
  canvas.addEventListener('pointercancel', cancelPointer);
  canvas.addEventListener('lostpointercapture', cancelPointer);
  canvas.addEventListener('pointerleave', hideHover);
  canvas.addEventListener('contextmenu', e => {
    e.preventDefault();
    // Native touch contextmenu must not compete with our 500ms timer. Some
    // engines expose it as a MouseEvent, so active captured contacts also gate it.
    if (e.pointerType === 'touch' || e.pointerType === 'pen' ||
        (!e.pointerType && (lastPointerType === 'touch' || lastPointerType === 'pen')) || pointers.size) return;
    orders(e.clientX, e.clientY);
  });
  canvas.addEventListener('wheel', e => {
    e.preventDefault();
    zoom(e.deltaY < 0 ? 1.1 : 0.91, e.clientX, e.clientY, 0, 0);
  }, { passive: false });
  return () => {
    stopTimer(); consumed = true;
    const ids = [...pointers.keys()]; pointers.clear(); pair = null;
    ids.forEach(release); hideHover();
  };
}
