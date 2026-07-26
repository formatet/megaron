package unit

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Namnstandarden (megaron_aktorer_plan.md §7, avgjord 2026-07-26).
//
// **Servern formaterar, aldrig klienterna.** Grammatiken — ordningstal,
// genitiv, vilket led som faller bort när en uppgift saknas — får inte
// dupliceras i webben, keryx och iOS-klienten. Var och en skulle glida isär,
// och det var precis den dubbleringen `merchantman → Emporos` illustrerade i
// sju filer innan Fas 0.
//
//	LAND:    [Ordinal] [Unit Type] of [Support Settlement]
//	HOST:    Nomadic Host of [Wanax]
//	RUNNER:  [Wanax]'s Runner from [Origin] to [Destination]
//	CARAVAN: [Wanax]'s [Good] Caravan from [Origin] to [Destination]
//	SHIP:    [Ship Name], [Ship Type] — supported by [Support Settlement]
//
// Ärotitlar (post-MVP) hakas på landförband som tillhör ett oikos. De byggs
// inte här förrän kingdoms återkommer.

// Ordinal ger det engelska ordningstalet: 1st, 2nd, 3rd, 4th …
//
// 11, 12 och 13 är undantagen som varje naiv implementation missar — de slutar
// på 1/2/3 men heter 11th, 12th, 13th. 111 och 112 likaså.
func Ordinal(n int) string {
	suffix := "th"
	if n%100 < 11 || n%100 > 13 {
		switch n % 10 {
		case 1:
			suffix = "st"
		case 2:
			suffix = "nd"
		case 3:
			suffix = "rd"
		}
	}
	return fmt.Sprintf("%d%s", n, suffix)
}

// Name samlar de delar ett aktörsnamn kan byggas av. Klienten får både det
// färdiga namnet och delarna, så en drawer kan visa dem var för sig utan att
// bygga om grammatiken.
type Name struct {
	Ordinal     int    `json:"ordinal,omitempty"`
	Type        string `json:"type"`       // units.type, intern nyckel
	TypeLabel   string `json:"type_label"` // "Spearmen", "Emporos"
	SupportTown string `json:"support_settlement_name,omitempty"`
	ShipName    string `json:"ship_name,omitempty"`
	Wanax       string `json:"wanax,omitempty"`
	Origin      string `json:"origin,omitempty"`
	Destination string `json:"destination,omitempty"`
	Good        string `json:"good,omitempty"`
	Honorific   string `json:"honorific,omitempty"` // post-MVP, oikos-gatad
	DisplayName string `json:"display_name"`
}

// LandUnitName: "2nd Spearmen of Knossos".
//
// Varje led kan saknas och faller då bort utan att lämna skräp: en enhet utan
// försörjande stad (en warband efter en kollaps) heter bara "2nd Spearmen",
// och en utan ordinal bara "Spearmen of Knossos".
func LandUnitName(unitType string, ordinal int, supportTown string) Name {
	n := Name{Ordinal: ordinal, Type: unitType, TypeLabel: DisplayName(unitType), SupportTown: supportTown}
	parts := make([]string, 0, 4)
	if ordinal > 0 {
		parts = append(parts, Ordinal(ordinal))
	}
	parts = append(parts, n.TypeLabel)
	if supportTown != "" {
		parts = append(parts, "of "+supportTown)
	}
	n.DisplayName = strings.Join(parts, " ")
	return n
}

// ShipDisplayName: "White Dolphin, Galley — supported by Kydonia".
//
// Ett skepp utan namn är inte ett fel: bygget kan ha lagts utan namnförslag.
// Då faller namnledet bort och typen bär namnet, precis som för landförband.
func ShipDisplayName(unitType, shipName, supportTown string) Name {
	n := Name{Type: unitType, TypeLabel: DisplayName(unitType), ShipName: shipName, SupportTown: supportTown}
	s := n.TypeLabel
	if shipName != "" {
		s = shipName + ", " + n.TypeLabel
	}
	if supportTown != "" {
		s += " — supported by " + supportTown
	}
	n.DisplayName = s
	return n
}

// HostName: "Nomadic Host of Ariadne". Wanaxens grundande folk i rörelse, inte
// en förbandskategori från en stad — därför ägaren och inte en stad.
func HostName(wanax string) Name {
	n := Name{Type: string(TypeNomadicHost), TypeLabel: DisplayName(string(TypeNomadicHost)), Wanax: wanax}
	n.DisplayName = n.TypeLabel
	if wanax != "" {
		n.DisplayName += " of " + wanax
	}
	return n
}

// RunnerName: "Ariadne's Runner from Knossos to Minos".
//
// Runnerns namn är en PÅGÅENDE RESA, inte ett personnamn: hon identifieras
// genom vem som sänt henne och vilken kontakt hon etablerar.
func RunnerName(wanax, origin, destination string) Name {
	n := Name{Type: "runner", TypeLabel: "Runner", Wanax: wanax, Origin: origin, Destination: destination}
	n.DisplayName = journey(wanax, "Runner", origin, destination)
	return n
}

// CaravanName: "Ariadne's Silver Caravan from Minos to Knossos".
//
// Varan står i namnet därför att varan är den militärt och ekonomiskt relevanta
// informationen — karavaner kan interceptas, Runners är fredade.
func CaravanName(wanax, good, origin, destination string) Name {
	n := Name{Type: "caravan", TypeLabel: "Caravan", Wanax: wanax, Good: good,
		Origin: origin, Destination: destination}
	label := "Caravan"
	if good != "" {
		label = strings.ToUpper(good[:1]) + good[1:] + " Caravan"
	}
	n.DisplayName = journey(wanax, label, origin, destination)
	return n
}

func journey(wanax, label, origin, destination string) string {
	s := label
	if wanax != "" {
		s = possessive(wanax) + " " + label
	}
	if origin != "" {
		s += " from " + origin
	}
	if destination != "" {
		s += " to " + destination
	}
	return s
}

// possessive: "Ariadne" → "Ariadne's", men "Sarpedones" → "Sarpedones'".
// Grekiska namn slutar ofta på s, och "Sarpedones's" läser fel.
func possessive(name string) string {
	if strings.HasSuffix(name, "s") || strings.HasSuffix(name, "S") {
		return name + "'"
	}
	return name + "'s"
}

// AllocateOrdinal delar ut nästa regementsnummer för (stad, enhetstyp).
//
// **Numret återanvänds ALDRIG** (Timothy 2026-07-26). Räknaren är monoton och
// bor i unit_ordinals; den frågar aldrig units efter MAX(ordinal), vilket hade
// återanvänt numret så fort ett förband upplöstes. Upplöses 2nd Spearmen of
// Knossos blir nästa rekryt 4th — "historiska regementen får isf vara
// historiska".
//
// Atomiciteten sitter i SATSEN, inte i en transaktion: ON CONFLICT DO UPDATE
// låser raden, så två samtidiga rekryteringar serialiseras och kan inte få
// samma nummer. Därför duger både en pool och en transaktion som `q`.
type ordinalQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func AllocateOrdinal(ctx context.Context, q ordinalQuerier, settlementID uuid.UUID, unitType string) (int, error) {
	var n int
	err := q.QueryRow(ctx,
		`INSERT INTO unit_ordinals (settlement_id, unit_type, next_ordinal)
		 VALUES ($1, $2, 2)
		 ON CONFLICT (settlement_id, unit_type)
		 DO UPDATE SET next_ordinal = unit_ordinals.next_ordinal + 1
		 RETURNING next_ordinal - 1`,
		settlementID, unitType,
	).Scan(&n)
	return n, err
}
