package main

import (
	"strings"
	"testing"
)

// Tests för P7 (megaron_plan_utforskaren.md): `unit list` ska läsa en sentrad
// landenhet utanför sin egen stad som vad den ÄR — en framskjuten post, med
// avstånd hemifrån — utan att ändra någon mekanik. Mekaniken finns redan
// (province.LoadLiveEyes, combat/unit_intercept_scan.go); detta låser bara
// LÄSBARHETEN: att raden namnger posten NÄR den ska, och INTE när enheten
// bara står i garnison.

func strPtr(s string) *string { return &s }
func intPtr(i int) *int       { return &i }

// TestIsForwardPost_SentryOutsideCity fångar den positiva vägen: en
// landenhet som står (status=positioned) med stance=sentry ÄR en post.
func TestIsForwardPost_SentryOutsideCity(t *testing.T) {
	u := unitRow{Category: "land", Status: "positioned", Stance: strPtr("sentry")}
	if !isForwardPost(u) {
		t.Error("en sentrad landenhet i status positioned ska räknas som framskjuten post")
	}
}

// TestIsForwardPost_Garrisoned är mutationstestets kärna: en garnisonerad
// enhet — även om den råkar bära stance sentry (satt innan den marscherade
// hem) — får ALDRIG namnges som post. Tar man bort statusvillkoret faller
// detta test.
func TestIsForwardPost_Garrisoned(t *testing.T) {
	u := unitRow{Category: "land", Status: "garrison", Stance: strPtr("sentry")}
	if isForwardPost(u) {
		t.Error("en garnisonerad enhet ska INTE namnges som framskjuten post, oavsett stance")
	}
}

// TestIsForwardPost_PositionedButNotSentry: fortify/storm är också
// "positioned" men är inte en post — sentry är den specifika hållningen.
func TestIsForwardPost_PositionedButNotSentry(t *testing.T) {
	for _, stance := range []string{"fortify", "storm"} {
		u := unitRow{Category: "land", Status: "positioned", Stance: strPtr(stance)}
		if isForwardPost(u) {
			t.Errorf("stance %q positioned ska inte räknas som post — bara sentry", stance)
		}
	}
}

// TestIsForwardPost_NavalExcluded: `unit post` är en LAND-order (unit patrol
// är sjöns motsvarighet och har sin egen semantik) — en naval enhet ska
// aldrig läsas som en "post" här även om den råkar dela statusfälten.
func TestIsForwardPost_NavalExcluded(t *testing.T) {
	u := unitRow{Category: "naval", Status: "positioned", Stance: strPtr("sentry")}
	if isForwardPost(u) {
		t.Error("en naval enhet ska inte namnges som (land-)framskjuten post")
	}
}

// TestLocationStr_ForwardPost_NamesItAndShowsDistance verifierar att raden
// säger "forward post", avståndet hemifrån, och kostnaden (dubbla ransoner)
// bredvid varandra — kontrakt C: spelaren som inte känner priset kan inte
// välja.
func TestLocationStr_ForwardPost_NamesItAndShowsDistance(t *testing.T) {
	homeID := "11111111-1111-1111-1111-111111111111"
	u := unitRow{
		Category:           "land",
		Status:             "positioned",
		Stance:             strPtr("sentry"),
		Q:                  intPtr(10),
		R:                  intPtr(0),
		OriginSettlementID: strPtr(homeID),
	}
	homes := map[string]settlementPos{
		homeID: {Q: 0, R: 0, Name: "Knossos"},
	}
	got := locationStr(nil, u, homes)
	if !strings.Contains(got, "forward post") {
		t.Errorf("raden namnger inte posten: %q", got)
	}
	if !strings.Contains(got, "10 hexes from Knossos") {
		t.Errorf("raden visar inte rätt avstånd/hemstad: %q", got)
	}
	if !strings.Contains(got, "double rations") {
		t.Errorf("raden nämner inte den dubbla fältransonen (kontrakt C): %q", got)
	}
}

// TestLocationStr_ForwardPost_DistanceMatchesHexgrid mäter avståndet mot ett
// annat par koordinater, så testet inte bara råkar stämma på (10,0)-fallet.
func TestLocationStr_ForwardPost_DistanceMatchesHexgrid(t *testing.T) {
	homeID := "22222222-2222-2222-2222-222222222222"
	u := unitRow{
		Category:           "land",
		Status:             "positioned",
		Stance:             strPtr("sentry"),
		Q:                  intPtr(5),
		R:                  intPtr(-3),
		OriginSettlementID: strPtr(homeID),
	}
	homes := map[string]settlementPos{
		homeID: {Q: 1, R: 1, Name: "Pylos"},
	}
	got := locationStr(nil, u, homes)
	// hexgrid.Distance((5,-3),(1,1)): dq=4, dr=-4, ds=0 -> max(4,4,0)=4
	if !strings.Contains(got, "4 hexes from Pylos") {
		t.Errorf("fel avstånd beräknat: %q (väntade 4 hexes from Pylos)", got)
	}
}

// TestLocationStr_Garrisoned_NeverNamedAsPost är den negativa spegeln till
// ForwardPost-testet ovan: en garnisonerad enhet ska aldrig få "forward post"
// i sin platsrad.
func TestLocationStr_Garrisoned_NeverNamedAsPost(t *testing.T) {
	sid := "33333333-3333-3333-3333-333333333333"
	u := unitRow{
		Category:     "land",
		Status:       "garrison",
		Stance:       strPtr("sentry"),
		SettlementID: strPtr(sid),
	}
	got := locationStr(nil, u, nil)
	if strings.Contains(got, "forward post") {
		t.Errorf("en garnisonerad enhet fick posttext: %q", got)
	}
}

// TestLocationStr_ForwardPost_UnknownHome: saknas hemstadens position (t.ex.
// äldre enhet utan origin_settlement_id, eller /provinces-anropet
// misslyckades) ska raden ändå namnge posten och priset — bara utan
// avståndet, aldrig krascha eller gissa en siffra.
func TestLocationStr_ForwardPost_UnknownHome(t *testing.T) {
	u := unitRow{
		Category: "land",
		Status:   "positioned",
		Stance:   strPtr("sentry"),
		Q:        intPtr(8),
		R:        intPtr(-2),
	}
	got := locationStr(nil, u, nil)
	if !strings.Contains(got, "forward post") {
		t.Errorf("raden namnger inte posten utan känd hemstad: %q", got)
	}
	if strings.Contains(got, "hexes from") {
		t.Errorf("raden ska inte gissa ett avstånd utan känd hemstad: %q", got)
	}
	if !strings.Contains(got, "double rations") {
		t.Errorf("kostnaden ska visas även utan känd hemstad: %q", got)
	}
}
