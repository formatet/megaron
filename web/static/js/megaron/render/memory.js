// Full desaturation after terrain/buildings and before live actors/UI. Raster
// spans use device-pixel centres: a frontier pixel belongs wholly to one side,
// even at fractional camera zoom/pan. Polygon fills would antialias the edge,
// leaving partially coloured memory and a soft live/memory boundary.
export function pixelSpans(points, width, height) {
  const spans = [];
  const top = Math.max(0, Math.ceil(Math.min(...points.map(p => p[1])) - 0.5));
  const bottom = Math.min(height, Math.ceil(Math.max(...points.map(p => p[1])) - 0.5));
  for (let y = top; y < bottom; y++) {
    const intersections = [];
    for (let i = 0; i < points.length; i++) {
      const [ax, ay] = points[i], [bx, by] = points[(i + 1) % points.length];
      if ((ay <= y + 0.5 && by > y + 0.5) || (by <= y + 0.5 && ay > y + 0.5)) {
        intersections.push(ax + (y + 0.5 - ay) * (bx - ax) / (by - ay));
      }
    }
    intersections.sort((a, b) => a - b);
    for (let i = 0; i + 1 < intersections.length; i += 2) {
      const left = Math.max(0, Math.ceil(intersections[i] - 0.5));
      const right = Math.min(width, Math.ceil(intersections[i + 1] - 0.5));
      if (right > left) spans.push([left, y, right - left]);
    }
  }
  return spans;
}

export function drawRemembered(ctx, tiles, polygon) {
  const remembered = tiles.filter(t => t.tier === 'remembered' && t.terrain !== 'fog');
  if (!remembered.length) return;
  const m = ctx.getTransform();
  ctx.save();
  try {
    ctx.resetTransform();
    ctx.globalAlpha = 1;
    ctx.globalCompositeOperation = 'saturation';
    ctx.fillStyle = '#808080'; // zero saturation; source lightness is irrelevant
    ctx.beginPath();
    for (const tile of remembered) {
      const points = polygon(tile).map(([x, y]) => [m.a*x + m.c*y + m.e, m.b*x + m.d*y + m.f]);
      for (const [x, y, width] of pixelSpans(points, ctx.canvas.width, ctx.canvas.height)) {
        ctx.rect(x, y, width, 1);
      }
    }
    // One fill avoids double compositing along shared edges. Saturation keeps
    // the destination's luminosity and detail, rather than painting flat grey.
    ctx.fill();
  } finally {
    ctx.restore();
  }
}
