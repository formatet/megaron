// Player prose uses words; editable numeric inputs keep their numeric values.
const small = ['zero','one','two','three','four','five','six','seven','eight','nine','ten','eleven','twelve','thirteen','fourteen','fifteen','sixteen','seventeen','eighteen','nineteen'];
const tens = ['','','twenty','thirty','forty','fifty','sixty','seventy','eighty','ninety'];
export function numberWords(value) {
  const n = Number(value);
  if (!Number.isFinite(n)) return 'unknown';
  if (n < 0) return 'minus ' + numberWords(-n);
  if (!Number.isInteger(n)) {
    const text = String(Math.round(n * 100) / 100);
    if (!text.includes('.')) return numberWords(Number(text));
    const [whole, fraction] = text.split('.');
    return numberWords(Number(whole)) + ' point ' + [...fraction].map(d => small[Number(d)]).join(' ');
  }
  if (n < 20) return small[n];
  if (n < 100) return tens[Math.floor(n / 10)] + (n % 10 ? ' ' + small[n % 10] : '');
  for (const [scale, label] of [[1e12,'trillion'],[1e9,'billion'],[1e6,'million'],[1e3,'thousand'],[100,'hundred']]) {
    if (n >= scale) return numberWords(Math.floor(n / scale)) + ' ' + label + (n % scale ? ' ' + numberWords(n % scale) : '');
  }
}
