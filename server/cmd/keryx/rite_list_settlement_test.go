package main

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// GPT19: after login the default province was a colony without a temple, so
// `rite --list` said no prayers while a cast from the capital succeeded. The
// list must read the same city the cast uses.
func TestRiteListReadsTheCastsCity(t *testing.T) {
	round3Client(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/cult"):
			fmt.Fprint(w, `{"kharis":{},"temples":[],"unbuilt_temples":[],"prayers":[{"id":"minoan_potnia_vision_v2","name":"Potnia Vision","settlement_id":"capital","city_name":"Capital"}]}`)
		case strings.HasSuffix(r.URL.Path, "/provinces"):
			fmt.Fprint(w, `[{"id":"colony-prov","settlement_id":"colony","own":true,"is_capital":false},
			                {"id":"capital-prov","settlement_id":"capital","own":true,"is_capital":true}]`)
		case strings.HasSuffix(r.URL.Path, "/provinces/capital-prov"):
			fmt.Fprint(w, `{"settlement":{"kharis_mood":"Content","kharis":50,"available_prayers":[{"id":"minoan_potnia_vision_v2","name":"Potnia's Vision"}]}}`)
		default:
			fmt.Fprint(w, `{"settlement":{}}`)
		}
	})
	cfg.ProvinceID = "colony-prov"
	c := riteCmd()
	c.SetArgs([]string{"--list"})
	out, err := captureStdout(t, c.Execute)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "minoan_potnia_vision_v2") {
		t.Fatalf("rite --list read the login default, not the capital:\n%s", out)
	}
}
