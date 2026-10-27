package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestRealmCultCLIThreeTemplesAndCityPrayerRouting(t *testing.T) {
	temples := []cultTemple{}
	prayers := []cultPrayer{}
	for i, city := range []string{"Tylissos", "Amnissos", "Kommos"} {
		god := "asterion"
		if i == 0 {
			god = "sea_daimons_stillness"
		}
		temples = append(temples, cultTemple{ID: city, SettlementID: "s-" + city, ProvinceID: "p-" + city, CityName: city, GodID: god, Name: god, Level: 1, Staffed: 1, Capacity: 2, Offer: map[string]any{"name": city, "temple": god, "fed": true, "offering": map[string]any{"grain": .25}}})
		prayers = append(prayers, cultPrayer{SettlementID: "s-" + city, ProvinceID: "p-" + city, CityName: city, HasTemple: true, riteAvailablePrayer: riteAvailablePrayer{ID: god, God: god, Name: "Prayer", Affordable: true, CanPrepareOffering: true}})
	}
	round3Client(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/cult") {
			t.Errorf("unexpected API %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(cultView{Kharis: map[string]any{}, Temples: temples, Prayers: prayers, Unbuilt: []cultUnbuilt{{CityName: "Kommos", Name: "Potnia"}}})
	})
	out, err := captureStdout(t, func() error { return listRealmPrayers(newClient(cfg), "") })
	if err != nil {
		t.Fatal(err)
	}
	for _, city := range []string{"Tylissos", "Amnissos", "Kommos"} {
		if !strings.Contains(out, "--settlement s-"+city) {
			t.Fatalf("missing city-specific cast %s: %s", city, out)
		}
	}
	filtered, err := captureStdout(t, func() error { return listRealmPrayers(newClient(cfg), "s-Kommos") })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(filtered, "--settlement s-Kommos") || strings.Contains(filtered, "s-Amnissos") {
		t.Fatal(filtered)
	}
	status, err := captureStdout(t, func() error { printRealmTemples(newClient(cfg)); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(status, "workers 1/2") != 3 || !strings.Contains(status, "--province p-Kommos --god asterion") || strings.Index(status, "Not built") < strings.LastIndex(status, "workers 1/2") {
		t.Fatal(status)
	}
}
