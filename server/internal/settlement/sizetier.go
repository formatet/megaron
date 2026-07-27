package settlement

// SizeTier klassar en bosättnings befolkning i fyra led. Den finns för att
// kartan ska kunna rita en nygrundad koloni som ett par hus och en palatsstad
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
// Trösklarna är kalibrering, inte invariant — de bor här i koden och får
// justeras. Spannet de delar är 101 (grundningsgolvet) till 30 000 (den mjuka
// befolkningstaket i kharis/tick.go), och delningen är ungefär logaritmisk
// eftersom skillnaden mellan 200 och 2 000 invånare betyder mer för hur en
// plats SER UT än skillnaden mellan 20 000 och 30 000.
const (
	SizeTierHamlet = 0 // koloni / nygrundad metropolis — några hus
	SizeTierTown   = 1 // ordentlig by med gårdar
	SizeTierCity   = 2 // stad med megaron — palatskulten syns
	SizeTierPalace = 3 // anaktoron: palatskomplex och lägre stad
)

func SizeTier(population int) int {
	switch {
	case population >= 15000:
		return SizeTierPalace
	case population >= 5000:
		return SizeTierCity
	case population >= 1000:
		return SizeTierTown
	default:
		return SizeTierHamlet
	}
}
