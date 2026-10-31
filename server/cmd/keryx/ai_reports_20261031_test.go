package main

import (
	"strings"
	"testing"
)

// Goods on one hex are always listed alphabetically, whatever order the
// server sends them in (Timothy 2026-10-31: oil was top on one hex, middle on
// another).
func TestCityCmd_GoodsAlphabeticalPerHex(t *testing.T) {
	placementOptions := `{
		"hexes": [
			{"hex_q":0,"hex_r":0,"hex_ordinal":1,"terrain":"plains","goods":[
				{"good_key":"olive_oil","rate_per_tick":2.0,"cap":3,"placed":0,"marginal_yield":2.0},
				{"good_key":"barley","rate_per_tick":2.0,"cap":3,"placed":0,"marginal_yield":2.0},
				{"good_key":"grain","rate_per_tick":2.0,"cap":3,"placed":0,"marginal_yield":2.0}
			]}
		],
		"buildings": [],
		"total_gubbar": 0,
		"pool_size": 0
	}`
	out := runCityCmd(t, placementOptions, `{"settlement":{}}`)
	b, g, o := strings.Index(out, "barley"), strings.Index(out, "grain"), strings.Index(out, "olive_oil")
	if b < 0 || g < 0 || o < 0 || !(b < g && g < o) {
		t.Fatalf("goods not alphabetical:\n%s", out)
	}
}
