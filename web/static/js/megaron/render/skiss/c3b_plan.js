// C3b: som C3a men snett uppifrån — södra fasader och murens yttersida syns.
import { draw } from './c2_plan.js';
export const render = (ctx, scene) => draw(ctx, scene, { ink: true, elev: true });
