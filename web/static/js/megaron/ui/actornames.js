// ── Aktörsnamn — ENDA stället som mappar intern nyckel → spelarvänt namn ──
//
// Interna nycklar (`merchantman`, `war_galley`, …) får leva vidare i DB och API,
// men de får aldrig läcka till spelaren. Före den här modulen låg samma mappning
// dubblerad i sju filer, vilket gjorde varje namnbyte till en sjufilsjakt —
// `merchantman → Emporos` är det stående exemplet.
//
// Servern har samma tabell i `internal/unit/model.go` (`unit.DisplayName`).
// Ändras den ena ska den andra följa med.

// Kanoniska nycklar = units.type i DB.
const UNIT_LABELS = {
  spearman:       'Spearmen',
  elite_infantry: 'Elite Infantry',
  war_chariot:    'War Chariot',
  galley:         'Galley',
  war_galley:     'War Galley',
  merchantman:    'Emporos',
  nomadic_host:   'Nomadic Host',
};

// Alias → kanonisk nyckel. Två källor: provins-API:ts ArmyComposition-fält är
// CamelCase (Go-structfältnamn), och `ship`/`trireme` är legacy-nycklar som
// servern fortfarande accepterar som indata (`unit.Canonical`).
const ALIASES = {
  Spearman: 'spearman', EliteInfantry: 'elite_infantry', WarChariot: 'war_chariot',
  Ship: 'galley', WarGalley: 'war_galley', Merchantman: 'merchantman',
  NomadicHost: 'nomadic_host',
  ship: 'galley', trireme: 'galley', chariot: 'war_chariot',
};

// Systemaktörer som inte bor i units-tabellen men som spelaren ser på kartan.
// Budbäraren heter **Runner** (Timothy 2026-07-26) — inte messenger, inte
// hemerodromos, inte keryx (Keryx är CLI-verktyget och skulle krocka).
const ACTOR_LABELS = {
  runner:  'Runner',
  caravan: 'Caravan',
};

/** Kanonisk units.type för en nyckel som kan vara CamelCase eller legacy. */
export function canonicalUnitType(type) {
  return ALIASES[type] || type;
}

/** Spelarvänt namn för en enhetstyp. Okänd nyckel faller tillbaka på sig själv. */
export function unitTypeLabel(type) {
  const key = canonicalUnitType(type);
  return UNIT_LABELS[key] || type;
}

/** Spelarvänt namn för en systemaktör (Runner, karavan). */
export function actorLabel(actor) {
  return ACTOR_LABELS[actor] || actor;
}
