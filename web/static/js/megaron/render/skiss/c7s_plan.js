// C7 + statuslagret.
import { draw } from './c7_plan.js';
const STATUS = { Nygrundad: 'svält', 'Växande': 'bygge', Kuststad: 'brand', Palatsstad: ['belägrad', 'offer'] };
export const render = (ctx, scene) => draw(ctx, scene, STATUS[scene.title.split(' ')[0]] || null);
