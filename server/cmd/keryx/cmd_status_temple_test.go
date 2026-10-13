package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func templeStatusFixture(t *testing.T) map[string]any {
	t.Helper()
	var sett map[string]any
	err := json.Unmarshal([]byte(`{"id":"local","temple_offers":[
 {"settlement_id":"local","name":"Phaistos","required_goods":{"grain":0.375},"alternatives":{"fish":0.75,"timber":0.125,"silver":0.5},"stock":{"grain":0.1,"fish":0.55,"timber":0,"silver":3},"offering":{"grain":0.1,"fish":0.55},"fed":true},
 {"settlement_id":"remote","name":"Knossos","required_goods":{"grain":0.375},"alternatives":{},"stock":{"grain":0,"fish":0,"timber":0,"silver":0},"offering":{},"fed":false},
 {"settlement_id":"remote-rich","offering":{"silver":12},"fed":true}
 ]}`), &sett)
	if err != nil {
		t.Fatal(err)
	}
	return sett
}

func TestTempleOfferingLineRendersServerReferenceAndMixedPlan(t *testing.T) {
	sett := templeStatusFixture(t)
	row := sett["temple_offers"].([]any)[0].(map[string]any)
	line := templeOfferingLine(row)
	for _, want := range []string{"Temple in Phaistos: reference 0.375 grain per day", "alternatives: 0.75 fish or 0.125 timber or 0.5 silver", "stock: 0.1 grain, 0.55 fish, 0 timber, 3 silver", "next gift: 0.1 grain + 0.55 fish", "✓"} {
		if !strings.Contains(line, want) {
			t.Errorf("line %q missing %q", line, want)
		}
	}
	if strings.Contains(line, "oil") || strings.Contains(line, "wine") {
		t.Fatal(line)
	}
}

func TestTempleOfferingLineUnfundedHasNoDebit(t *testing.T) {
	row := templeStatusFixture(t)["temple_offers"].([]any)[1].(map[string]any)
	line := templeOfferingLine(row)
	if !strings.Contains(line, "next gift: unfunded (nothing taken)  ✗") {
		t.Fatal(line)
	}
}

func TestLocalTempleOfferingCountsOnlyLocalPlan(t *testing.T) {
	sett := templeStatusFixture(t)
	got := localTempleOffering(sett)
	if len(got) != 2 || got["grain"] != .1 || got["fish"] != .55 {
		t.Fatalf("local offering %v, want only local mixed grain/fish gift", got)
	}
	// Reference quantities never create a debit for an unfunded local temple.
	sett["id"] = "remote"
	if got := localTempleOffering(sett); len(got) != 0 {
		t.Fatal(got)
	}
	delete(sett, "id")
	if got := localTempleOffering(sett); len(got) != 0 {
		t.Fatal(got)
	}
}
