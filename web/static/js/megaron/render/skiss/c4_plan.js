// C4: A:s vinkel med C1:s konturer och C2:s kvarter — staden från sidan, himmel bakom.
import { draw } from './c2_plan.js';
export const render = (ctx, scene) => draw(ctx, scene, { ink: true, side: true });
