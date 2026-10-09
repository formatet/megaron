// C6 + statuslagret: samma grundstad, status ritad ovanpå (statusLayer).
import { draw } from './c6_plan.js';
const STATUS = { Nygrundad: 'svält', 'Växande': 'bygge', Kuststad: 'brand', Palatsstad: 'belägrad' };
export const render = (ctx, scene) => draw(ctx, scene, STATUS[scene.title.split(' ')[0]] || null);
