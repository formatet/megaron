// C5: C4:s vinkel, men kvarteret är hus — B:s och C1:s kontur, skugga och rundhet —
// och C1:s skepp, bryggor, träd, folk och marktextur.
import { draw } from './c2_plan.js';
export const render = (ctx, scene) => draw(ctx, scene, { ink: true, side: true, houses: true, rich: true });
