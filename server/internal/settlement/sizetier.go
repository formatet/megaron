package settlement

// SizeTier klassar en bosättnings befolkning i två led. Den finns för att
// kartan ska kunna rita en nygrundad koloni som ett par hus och en riktig stad
// som en stadsmassa — kartmarkören bar tidigare ingen storlekssignal alls, så
// en stad på 101 invånare och Knossos på 30 000 ritades som samma lilla ruta.
//
// Varför ett LED och inte befolkningen? Två skäl:
//
//  1. FOW. En stads fysiska omfång ÄR synligt utifrån — man ser murar och tak
//     på håll. Dess exakta invånarantal är underrättelse och ska förbli det.
//     Ett led svarar på precis den fråga renderaren ställer, och inte mer.
//  2. En kanonisk definition. Härleddes ledet i klienten skulle serverns
//     sanning och kartans bild kunna glida isär (megaron_terrangrendering
//     princip 9: grafiken får förstärka Temenos sanning, aldrig hitta på).
//
// **Två led, inte fyra** (Timothy 2026-07-27). Leden var ursprungligen fyra —
// hamlet, town, city, anaktoron — men de två största siluetterna gick inte att
// få att läsa som städer: de blev modeller stående på en bricka, utan liv och
// med muren knappt synlig. Hellre två led som båda fungerar än fyra där hälften
// ljuger om vad en stad är. Priset är att megaron inte längre reser sig ur
// massan på kartan; palatskulten bärs tills vidare av stadsvyn.
//
// Tröskeln är kalibrering, inte invariant — den bor här i koden och får
// justeras. Spannet den delar är 101 (grundningsgolvet) till 30 000 (det mjuka
// befolkningstaket i kharis/tick.go).
const (
	SizeTierHamlet = 0 // koloni / nygrundad metropolis — några hus på en gård
	SizeTierTown   = 1 // stad: myllret innanför ringmuren
)

// SizeTierThreshold är befolkningen där en bosättning slutar vara en koloni och
// börjar ritas som en stad (Timothy 2026-07-27).
const SizeTierThreshold = 800

func SizeTier(population int) int {
	if population >= SizeTierThreshold {
		return SizeTierTown
	}
	return SizeTierHamlet
}
