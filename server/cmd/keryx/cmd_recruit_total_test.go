package main

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestRecruitListShowsTotalDraftAndSpecificShortfalls(t *testing.T) {
	round3Client(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/units":
			fmt.Fprint(w, `[{"type":"galley","costs":{"timber":30},"batch_men":20,"pop_cost":10,"duration_game_days":3,"requires_shipyard":true}]`)
		case "/api/v1/worlds/world/provinces/default":
			fmt.Fprint(w, `{"settlement":{"can_recruit":[{"unit":"galley","can_recruit":false,"reasons":["needs 30 timber (or cedar) (have 10)","needs a shipyard"]}]}}`)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	})
	c := newClient(cfg)
	out, err := captureStdout(t, func() error { return printRecruitCatalogue(c, "world", "default") })
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"30 timber (or cedar)", "20 crew", "needs 30 timber (or cedar) (have 10)", "needs a shipyard"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q: %s", want, out)
		}
	}
	if strings.Contains(out, "Pop") {
		t.Fatalf("misleading population gate: %s", out)
	}
}

func TestRecruitReceiptNamesActualCedarPayment(t *testing.T) {
	if got := recruitmentPaidGoods(map[string]float64{"cedar": 20, "timber": 10}); got != "10 timber + 20 cedar" {
		t.Fatal(got)
	}
}
