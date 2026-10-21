package main

import (
	"strings"
	"testing"
)

const runnersFixture = `{"own":[
 {"id":"w1","group":"waiting","kind":"letter","to_city":"Ugarit","to_wanax":"Kassandra","port_name":"Byblos","waiting_since":"2026-10-21T18:34:00Z",
  "explanation":"Your runner is waiting in Byblos: no ship has sailed.","can_call_back":true,"can_arrange_passage":true,
  "eligible_ships":[{"id":"s1","name":"Dawn Gull","settlement_name":"Byblos"}]},
 {"id":"r1","group":"on_the_way","kind":"order","order_summary":"Third Spears: Set stance: hold","arrives_at":"2026-10-21T20:00:00Z"},
 {"id":"a1","group":"aboard","kind":"letter","to_city":"Ugarit","to_wanax":"Kassandra","ship_name":"Sea Swallow","ship_wanax":"Kassandra","boarded_at":"2026-10-21T10:00:00Z","lands_at":"2026-10-22T01:30:00Z","arrives_at":"2026-10-22T03:00:00Z"},
 {"id":"h1","group":"homebound","kind":"letter","to_city":"Byblos","arrives_at":"2026-10-21T22:30:00Z"}],
 "foreign":[
 {"group":"in_your_city","sender_wanax":"Kassandra","where":"Byblos","waiting_for":"a_ship"},
 {"group":"aboard_your_ship","sender_wanax":"Dunios","ship_name":"Sea Swallow","heading_to":"Ugarit","lands_at":"2026-10-22T01:30:00Z"}]}`

func TestRenderRunners(t *testing.T) {
	out, err := renderRunners([]byte(runnersFixture))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"WAITING FOR A SHIP", "Byblos", "no ship has sailed", "keryx call-back --id w1", "keryx passage --id w1 --ship s1", "Dawn Gull",
		"ON THE WAY", "Third Spears: Set stance: hold",
		"ABOARD", "Wanax Kassandra's Sea Swallow",
		"HOMEBOUND",
		"IN YOUR CITIES", "Wanax Kassandra's runner in Byblos",
		"ABOARD YOUR SHIPS", "Wanax Dunios' runner aboard Sea Swallow",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "keryx call-back --id a1") || strings.Contains(out, "--id h1") {
		t.Errorf("an action was offered for a runner the server did not allow it for:\n%s", out)
	}
}

func TestRenderRunnersEmpty(t *testing.T) {
	out, err := renderRunners([]byte(`{"own":[],"foreign":[]}`))
	if err != nil || !strings.Contains(out, "No runners") {
		t.Errorf("empty: %q %v", out, err)
	}
}
