import { fmtNum, fmtDays } from './fmt_num.js';

// One question, one answer: "is this city's food going up or down, and how long
// does it last?" The server's city forecast (pd.forecast.food, the same owner as
// the FoodRunning notice) is the net after everything that eats: citizens, army,
// temples, and the herd's overflow. Every screen reads this and nothing else.
// keryx says the same words (foodAnswerLine in food_format.go).
const LONGEST_DAY_COUNT = 1000;

// {net, days} from the forecast; days is null when the food is not running down.
export function foodAnswer(pd) {
  const f = pd?.forecast?.food;
  const net = Number(f?.net);
  if (f == null || f.net == null || !Number.isFinite(net)) return null;
  const days = f.days == null ? null : Number(f.days);
  return { net, days: Number.isFinite(days) ? days : null };
}

// "+0.4 per game day", or "about even" when it rounds to nothing.
export function foodNetText(net) {
  const shown = Math.round(Math.abs(net) * 10) / 10;
  if (shown === 0) return 'about even';
  return `${net < 0 ? '\u2212' : '+'}${fmtNum(shown)} per game day`;
}

// Plus how long it lasts when it is running down:
// "\u22120.2 per game day · lasts 8 game days".
export function foodAnswerValue(a) {
  if (!a) return '';
  const head = foodNetText(a.net);
  if (!(a.net < 0) || a.days == null || head === 'about even') return head;
  return `${head} · lasts ${a.days > LONGEST_DAY_COUNT ? 'over ' + fmtDays(LONGEST_DAY_COUNT) : fmtDays(a.days)}`;
}

export const foodAnswerText = a => (a ? 'Food ' + foodAnswerValue(a) : '');
