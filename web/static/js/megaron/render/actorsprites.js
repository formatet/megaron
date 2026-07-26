// ── Kartaktörernas siluetter (Fas 2, megaron_aktorer_plan.md) ────────────
//
// Nio aktörer delade tidigare FYRA sprites, och enhetstypen renderades aldrig:
// enda förgreningen var `u.category === 'naval'`. En spjutlinje, en stridsvagn
// och en karavan såg identiskt likadana ut. Här får varje aktör sin egen form.
//
// **Enheten är det fasta måttet all terränggrafik underordnar sig** — inte
// tvärtom (megaron_terrangrendering princip 5). Hexen är 44×38 logiska px och
// enheten ska vara 14–16 px, ca 40 % av hexhöjden. Den gamla gångaren var 10 px
// och därmed en platshållare som varje terränggodkännande hittills mätts mot.
//
// Samma pixelregler som träden lyder under: fillRect på heltal, aldrig arc()
// (den kantutjämnas, och canvasen är skalad zoom × SCALE, så mjuka kurvor
// resamplas till gröt på varje zoomnivå), ¾-elevation med ljus uppifrån
// vänster, 1px charcoal-kontur, ingen gradient, inga rundade hörn.
//
// A = accentfärgen. Den sätts vid ritning ur `intent` och är den ENDA pixel som
// byter färg — formen är permanent identitet, färgen är betraktarens relation
// (Timothy 2026-07-26). Fas 6 byter intent mot relation här; formen rörs inte då.
//
// Iterera med `node tools/actorsheet.mjs` — den ritar hela tabellen förstorad
// mot de tre terrängerna, utan browser, deploy eller server.

export const ACTOR_PALETTE = {
  K: '#1F1A14',  // charcoal-kontur
  G: '#3B3325',  // djup skugga, ben, hjuleker
  W: '#6B5334',  // trä — skaft, vagn, skrov
  w: '#8A6C44',  // trä, solbelyst sida
  B: '#7A5A28',  // brons, skuggad sida
  M: '#A87C38',  // brons, kropp
  L: '#D2A24E',  // brons, solbelyst
  H: '#A87548',  // hud. MÖRKARE än gamla #F4D0A0, som hade ΔL ≈ 0 mot
                 // kalksten #E0D4B8 och alltså försvann på bergshexar.
  T: '#CFC0A0',  // tunika/segel, ljus
  D: '#9A8B6C',  // tunika/segel, skuggad
  S: '#2A2419',  // markskugga
  R: '#8C2F22',  // oxblod — plym, elittecken
  N: '#6E7A46',  // last, packning, tältduk
  n: '#8B9459',  // last, solbelyst
};

// **Regeln som avgör allt vid den här storleken: EN kontur runt hela massan,
// aldrig mellan gestalterna.** Första utkastet outlinade varje man för sig och
// blev en streckkod — tre soldater lästes som ett staket. Inuti massan skiljs
// gestalterna åt med TON (L ljus vänster → M kropp → B skuggad höger), aldrig
// med charcoal. En formation är ETT objekt vid 14 px, inte tre.

// Spearmen — mikroformation, inte hjälte. Tre hjälmkullar på en gemensam
// kroppsmassa, spjut vinklade uppåt-höger som bryter horisontalen och ger
// riktning, sköldarna som ett accentband över bålen. Vertikal och kompakt.
const SPEARMAN = [
  '...........W.',
  '.......W..W..',
  '......W..W...',
  '....W.W.W....',
  '..RWRW.RW....',
  '.KLMLMLMK....',
  '.KHDHDHDK....',
  'KTAATAATAATK.',
  'KTAATAATAATK.',
  'KDTTDTTDTTDK.',
  '.KGDGDGDGK...',
  '.KG.G.G.GK...',
  '..K.K.K.K....',
  'SSSSSSSSSSS..',
];

// Elite Infantry — bredare, tyngre, MÖRKARE kroppsmassa. Bronsburen tyngd, inte
// "röda Spearmen": hög plym, helbronsad överkropp, sköldar som täcker hela bålen
// och en pansarkjol under. INGA spjut — det är siluettskillnaden mot Spearmen.
const ELITE = [
  '..R..R..R....',
  '..R..R..R....',
  '.KLMLMLMK....',
  '.KMBMBMBK....',
  '.KHDHDHDK....',
  'KLAALAALAALK.',
  'KMAAMAAMAAMK.',
  'KMAAMAAMAAMK.',
  'KBAABAABAABK.',
  'KBBBBBBBBBBK.',
  '.KGGGGGGGGK..',
  '.KG.GG.GG.GK.',
  '..K.KK.KK.K..',
  'SSSSSSSSSSSS.',
];

// War Chariot — LÅG, BRED, horisontell. Får aldrig likna en karavan: hästen
// sträcker sig framåt, draglinan är rak, hjulet är stort och ekat, korgen låg.
const CHARIOT = [
  '................',
  '.............R..',
  '............KMK.',
  '...........KKHKK',
  '.KwwK.......KTK.',
  'KwAAwK.....KTATK',
  'KwAAwKWWWWWWTATK',
  'KWWWWK......KTK.',
  'KWWWWK.KHHHK....',
  'KKGKKKKHHHHHK...',
  '.GKKG.KHHHHHHHK.',
  'GKKKKGKHK...KHK.',
  'GKKKKG.KG....KG.',
  '.GKKG..KG....KG.',
  'SSSSSSSSSSSSSSS.',
];

// Galley — lång, LÄTT roddlinje, smalt skrov. Den kanoniska standardgaläten:
// snabb och obeväpnad, årorna är dess signatur och seglet är litet.
// Ett bronsålderssegel är FYRKANTIGT och hänger under en rå. Första utkastets
// triangelsegel gjorde varje skepp till en pil eller ett tält — det är en senare
// marin konvention och läser dessutom som en symbol, inte som ett fartyg.
const GALLEY = [
  '......K.......',
  '...KKKKKKK....',
  '...KTTATTK....',
  '...KTTATTK....',
  '...KDDADDK....',
  '......K.......',
  '......K.......',
  '.KwwwwwwwwwK..',
  'KwWWWWWWWWWwK.',
  '.KWWWWWWWWWK..',
  '..GKGKGKGKG...',
  '..............',
  '..SSSSSSSSS...',
];

// War Galley — tätare, TYNGRE, stridsklar skrovprofil: högre skrov, sköldrad
// längs relingen, ramm i fören. Läses som ett vapen, inte som ett fordon.
const WARGALLEY = [
  '......K.......',
  '.KKKKKKKKKKK..',
  '.KTTTATTTTTK..',
  '.KTTTATTTTTK..',
  '.KTTTATTTTTK..',
  '.KDDDADDDDDK..',
  '......K.......',
  'KMLMLMLMLMLMK.',   // sköldrad längs relingen
  'KwwwwwwwwwwwwK',
  'KWWWWWWWWWWWWW',   // ramm som sticker ut till höger
  'KWWWWWWWWWWWWK',
  'KKWWWWWWWWWWK.',
  'GKGKGKGKGKGK..',   // dubbel årbank
  'KGKGKGKGKGK...',
  '.SSSSSSSSSSS..',
];

// Emporos — bredare, FYLLIGARE skrov, lastad segelprofil. Funktion och last
// först: däckslasten syns över relingen, seglet är brett och buktande, inga åror.
// Mätningen fällde första utkastet: Emporos och War Galley låg 9,7 % isär i
// diskriminerbarhetsriggen — ögat sa samma sak, "två bruna båtar". Skillnaden
// måste sitta i SKROVET, inte i småpixlar: handelsskrovet är djupt och trågigt
// med upphöjd akter, krigsskrovet långt och lågt med ramm. Seglet är brett och
// lågt (lastseglet), inte högt.
const EMPOROS = [
  'K.....K.......',
  'K.KKKKKKKKKK..',
  'KKKTTTATTTTK..',
  'KKKTTTATTTTK..',
  'KKKDDDADDDDK..',
  'K.....K.......',
  'K..KNnKKNnK...',
  'KKwwwwwwwwwwK.',
  'KWWWWWWWWWWWWK',
  'KWWWWWWWWWWWWK',
  '.KWWWWWWWWWWK.',
  '..KWWWWWWWWK..',
  '...KKKKKKKK...',
  '..SSSSSSSSSS..',
];

// Nomadic Host — oregelbunden grupp: människor, oxvagn, tältlast, en unge.
// Stor nog att läsas som en BEFOLKNING MED FRAMTID, inte en spjutlinje och inte
// en krigsstandard. Ingen siluett upprepas — oregelbundenheten är hela poängen.
// Oregelbundenheten är hela poängen: olika höjd, olika hållning, en unge i
// bakre ledet. Ett andra led UNDER vagnen lästes som vagnens ben — folket måste
// stå bredvid den, inte bakom den.
const HOST = [
  '......KNNNNK....',
  '.....KNnnnnNK...',
  '..LM.KNnnnnNK...',
  '..HD.KNNNNNNK.LM',
  '.KTATKwWWWWwK.HD',
  '.KTATKWWWWWWKTAT',
  'LM.DTKWWWWWWKTAT',
  'HD.G.GKG..GK..DT',
  'TAT.G.G...G..G.G',
  'TAT.LM....G..G.G',
  '.DT.HD..........',
  '.G.GTAT.........',
  '.G.G.DT.........',
  '.....G.G........',
  'SSSSSSSSSSSSSSSS',
];

// Karavan — LAST och RIKTNING först: två packade åsnor, amforor, säckar. Ägandet
// är sekundärt. Ska kunna se EXPONERAD ut — en ensam drivare bredvid mycket
// last, ingen eskort, inget vapen någonstans i siluetten.
const CARAVAN = [
  '................',
  '...KNNK...KNNK..',
  '..KNnnNK.KNnnNK.',
  '..KNnnNK.KNnnNK.',
  '.LKNNNNK.KNNNNK.',
  '.HKwWWwK.KwWWwK.',
  'KTAKHHHHHKHHHHHK',
  'KTAKHHHHHKHHHHHK',
  '.DK.HK.HK.HK.HK.',
  '.G..G..G..G..G..',
  '.G..G..G..G..G..',
  '................',
  '................',
  'SSSSSSSSSSSSSSSS',
];

// Runner — liten, snabb, ENSAM löpare med förseglad budväska. MINIMAL
// identitetsmarkör. Aldrig attackaffordans, aldrig hotring: Runnern är fredad,
// och att rita henne som ett mål vore en lögn om reglerna.
// Framåtlutad hållning och utsträckta ben = fart utan animation.
const RUNNER = [
  '.........',
  '.........',
  '...KHK...',
  '..KKHK...',
  '...KTK...',
  '..KTATK..',
  '.KTAATKA.',
  'KKTATKKK.',
  '..KTKK...',
  '.KGKGK...',
  'KG...GK..',
  'G.....GK.',
  'K......K.',
  '.........',
  '.SSSSSS..',
];

/** Sprite → horisontella löpor av samma färg: en fillRect per löpa i stället
 *  för en per pixel. Samma teknik som träden i render/map.js. */
export function spriteRuns(rows) {
  const runs = [];
  const w = Math.max(...rows.map(r => r.length));
  rows.forEach((row, y) => {
    let x = 0;
    while (x < row.length) {
      const ch = row[x];
      let n = 1;
      while (x + n < row.length && row[x + n] === ch) n++;
      if (ch !== '.') runs.push({ ch, x, y, n });
      x += n;
    }
  });
  return { runs, w, h: rows.length };
}

export const ACTOR_SPRITES = {
  spearman:       spriteRuns(SPEARMAN),
  elite_infantry: spriteRuns(ELITE),
  war_chariot:    spriteRuns(CHARIOT),
  galley:         spriteRuns(GALLEY),
  war_galley:     spriteRuns(WARGALLEY),
  merchantman:    spriteRuns(EMPOROS),
  nomadic_host:   spriteRuns(HOST),
  caravan:        spriteRuns(CARAVAN),
  runner:         spriteRuns(RUNNER),
};

// Accentfärgen — de enda pixlar i en aktör som byter färg. Neutral när ingen
// avsikt är känd (en stillastående enhet skickar ''), vilket är exakt den
// tystnad Fas 5 ska ersätta med en riktig ägarsignal.
export const INTENT_ACCENT = {
  attack:    '#922B21',
  reinforce: '#1A5276',
  support:   '#145A32',
  scout:     '#7D6608',
  explore:   '#0E7490',
};
export const NEUTRAL_ACCENT = '#6A6252';

/** Aktörens ursprung är dess FOT, som trädens: aktören står på sin position och
 *  en lägre aktör överlappar en högre korrekt när flera ritas i y-ordning. */
export function drawActor(ctx, kind, x, y, intent, walkPhase, accentOverride) {
  const sprite = ACTOR_SPRITES[kind];
  if (!sprite) return false;
  // Gången är ETT pixelsteg upp, inte en andra ritad pose: en extra pose
  // fördubblar underhållet för nio aktörer och syns knappt vid 15 px.
  const bob = walkPhase < 2 ? 0 : -1;
  const ox = Math.round(x) - (sprite.w >> 1);
  const oy = Math.round(y) - sprite.h + 2;   // fötterna strax under hexcentrum
  const accent = accentOverride || INTENT_ACCENT[intent] || NEUTRAL_ACCENT;
  for (const r of sprite.runs) {
    // Markskuggan ligger kvar på marken när kroppen studsar — annars hoppar
    // hela figuren och tappar sin förankring.
    const lift = r.ch === 'S' ? 0 : bob;
    ctx.fillStyle = r.ch === 'A' ? accent : ACTOR_PALETTE[r.ch];
    ctx.fillRect(ox + r.x, oy + r.y + lift, r.n, 1);
  }
  return true;
}
