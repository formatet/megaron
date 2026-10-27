package main

import (
	"encoding/json"
	"fmt"
	"formatet/megaron/server/internal/unit"
	"github.com/spf13/cobra"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

func TestForcesRecruitLegacyAndCanonicalLabelsAccepted(t *testing.T) {
	for label, want := range map[string]string{"Elite Infantry": "elite_infantry", "elite_infantry": "elite_infantry", "Bronze Guard": "elite_infantry", "bronze_guard": "elite_infantry", "War Chariot": "war_chariot", "war_chariot": "war_chariot", "Chariot": "war_chariot"} {
		if got := unitAliases[strings.ToLower(strings.TrimSpace(label))]; got != want {
			t.Fatalf("%q→%q want%q", label, got, want)
		}
	}
	pending := []*cobra.Command{unitCmd(), recruitCmd(), armyAliasCmd(), forcesAliasCmd()}
	for len(pending) > 0 {
		cmd := pending[0]
		pending = pending[1:]
		pending = append(pending, cmd.Commands()...)
		if regexp.MustCompile(`(?i)\bunits\b`).MatchString(cmd.Short + cmd.Long + cmd.Example) {
			t.Fatalf("retired Units noun in %s help", cmd.Name())
		}
		for _, old := range unit.RetiredDisplayLabels {
			if strings.Contains(cmd.Short, old) || strings.Contains(cmd.Long, old) {
				t.Fatalf("retired %q in %s help", old, cmd.Name())
			}
		}
	}
}

func TestForcesRosterKeepsLegacyUnitsAlias(t *testing.T) {
	c := forcesAliasCmd()
	if c.Use != "forces" || len(c.Aliases) != 1 || c.Aliases[0] != "units" || c.RunE == nil {
		t.Fatalf("roster alias unavailable %+v", c)
	}
}

func TestForcesRosterDisplaysBoundRouteAndCurrentLeg(t *testing.T) {
	round3Client(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/units") {
			fmt.Fprint(w, `{"units":[{"id":"11111111-1111-4111-8111-111111111111","type":"merchantman","category":"naval","size":1,"status":"freighting","display_name":"White Dolphin","bound_route_id":"22222222-2222-4222-8222-222222222222","bound_route_name":"Home → Colony","freighting_note":"carrying goods to Faraway"}]}`)
		} else {
			fmt.Fprint(w, `[]`)
		}
	})
	out, err := captureStdout(t, forcesAliasCmd().Execute)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"on the Home → Colony route", "carrying goods to Faraway", "Land Forces\n  None.", "Ships"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %s", want, out)
		}
	}
}

func TestForcesRosterGroupsAndPeopleOnlyWithHost(t *testing.T) {
	for _, withHost := range []bool{false, true} {
		t.Run(fmt.Sprintf("host_%v", withHost), func(t *testing.T) {
			forces := []map[string]any{
				{"id": "11111111-1111-4111-8111-111111111111", "type": "merchantman", "category": "naval", "size": 1, "status": "freighting", "display_name": "Trading Dolphin"},
				{"id": "22222222-2222-4222-8222-222222222222", "type": "elite_infantry", "category": "land", "size": 100, "status": "garrison", "display_name": "First Bronze Guard"},
			}
			if withHost {
				forces = append(forces, map[string]any{"id": "33333333-3333-4333-8333-333333333333", "type": "nomadic_host", "category": "land", "size": 1, "status": "positioned", "display_name": "Wandering People"})
			}
			round3Client(t, func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/units") {
					_ = json.NewEncoder(w).Encode(map[string]any{"units": forces})
				} else {
					fmt.Fprint(w, `[]`)
				}
			})
			out, err := captureStdout(t, forcesAliasCmd().Execute)
			if err != nil {
				t.Fatal(err)
			}
			land, ships := strings.Index(out, "Land Forces"), strings.Index(out, "Ships")
			guard, ship := strings.Index(out, "First Bronze Guard"), strings.Index(out, "Trading Dolphin")
			if land < 0 || ships <= land || guard < land || guard >= ships || ship <= ships {
				t.Fatalf("wrong group/name ordering: %s", out)
			}
			people := strings.Index(out, "\nPeople\n")
			if withHost {
				if people <= ships || strings.Index(out, "Wandering People") <= people {
					t.Fatalf("Host missing People group: %s", out)
				}
			} else if people >= 0 {
				t.Fatalf("People present without Host: %s", out)
			}
			if !strings.Contains(out, "freighting") || !strings.Contains(out, "garrison") {
				t.Fatalf("statuses lost: %s", out)
			}
		})
	}
}
