package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/spf13/cobra"
)

// sightingUnit mirrors the server's GET /worlds/{id}/foreign-units row (see
// server/api/handlers/foreign_units.go), plus the distance/bearing keryx
// derives locally — same pattern as mapCmd's `cand` type.
type sightingUnit struct {
	Owner    string `json:"owner"`
	Type     string `json:"type"`
	Category string `json:"category"`
	Size     int    `json:"size"`
	Status   string `json:"status"`
	Stance   string `json:"stance,omitempty"`
	Q        int    `json:"q"`
	R        int    `json:"r"`

	// Cargo mirrors foreignUnit.Cargo (server/api/handlers/foreign_units.go) —
	// the embarked land cohort a foreign ship carries, a market signal ("tenn
	// rör sig från Knossos"). Nil unless the unit has cargo aboard.
	Cargo *sightingCargo `json:"cargo,omitempty"`

	TargetQ   *int       `json:"target_q,omitempty"`
	TargetR   *int       `json:"target_r,omitempty"`
	ArrivesAt *time.Time `json:"arrives_at,omitempty"`

	Distance int    `json:"distance"`
	Bearing  string `json:"bearing"`
}

// sightingCargo mirrors foreignCargo (server/api/handlers/foreign_units.go).
type sightingCargo struct {
	Type string `json:"type"`
	Size int    `json:"size"`
}

// sightingGarrison mirrors the fields of a /provinces marker relevant to a
// foreign city's garrison, plus derived distance/bearing.
type sightingGarrison struct {
	Name      string `json:"name"`
	Owner     string `json:"owner"`
	Q         int    `json:"q"`
	R         int    `json:"r"`
	ArmyTotal int    `json:"army_total"`

	Distance int    `json:"distance"`
	Bearing  string `json:"bearing"`
}

func sightingsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sightings",
		Short: "List every foreign unit and garrisoned city currently in your live vision",
		Long: `List every foreign (non-owned) unit and garrisoned city your eyes currently
reach — GET /foreign-units for units (temenos_synlighet.md tier 1: full type,
size and stance, never a blur), GET /provinces for garrisons (own=false,
army_total>0). Remembered (tier 2) tiles never appear here — memory carries no
activity, only live sight does.

Distance and bearing are measured from your NEAREST city or unit, nearest
sighting first — the question a sighting raises is "how close is this to
something of mine", not "how far is it from my palace".`,
		Args: noPositionalArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)

			// Every own position on the map: cities and units alike. Measuring from
			// a single seat (capital, or the host during founder phase) reads as a
			// lie the moment a scout is out — the acceptance run 2026-08-03 reported
			// "13 hexar NE" for units a spearman was standing 2 hexes from, because
			// the host had stayed home. `keryx map` still measures from the seat:
			// there the question genuinely is "what is near my city".
			own := ownPositions(c)
			if len(own) == 0 {
				// No unit and no city on the map — fall back to the seat so the
				// command still answers instead of erroring out.
				oq, or, err := ownCoordinates(c)
				if err != nil {
					return err
				}
				own = [][2]int{{oq, or}}
			}
			nearest := func(q, r int) (int, string) {
				bd, bq, br := -1, own[0][0], own[0][1]
				for _, p := range own {
					if d := hexDist(p[0], p[1], q, r); bd < 0 || d < bd {
						bd, bq, br = d, p[0], p[1]
					}
				}
				return bd, compassDirection(bq, br, q, r)
			}

			unitsData, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/foreign-units", cfg.WorldID))
			if err != nil {
				return err
			}
			var rawUnits []sightingUnit
			if err := json.Unmarshal(unitsData, &rawUnits); err != nil {
				return err
			}
			for i := range rawUnits {
				rawUnits[i].Distance, rawUnits[i].Bearing = nearest(rawUnits[i].Q, rawUnits[i].R)
			}
			sort.Slice(rawUnits, func(i, j int) bool { return rawUnits[i].Distance < rawUnits[j].Distance })

			provData, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/provinces", cfg.WorldID))
			if err != nil {
				return err
			}
			var markers []struct {
				Name      string `json:"name"`
				Owner     string `json:"owner"`
				Q         int    `json:"q"`
				R         int    `json:"r"`
				Own       bool   `json:"own"`
				ArmyTotal int    `json:"army_total"`
			}
			if err := json.Unmarshal(provData, &markers); err != nil {
				return err
			}
			var garrisons []sightingGarrison
			for _, m := range markers {
				if m.Own || m.ArmyTotal <= 0 {
					continue
				}
				gd, gb := nearest(m.Q, m.R)
				garrisons = append(garrisons, sightingGarrison{
					Name: m.Name, Owner: m.Owner, Q: m.Q, R: m.R, ArmyTotal: m.ArmyTotal,
					Distance: gd, Bearing: gb,
				})
			}
			sort.Slice(garrisons, func(i, j int) bool { return garrisons[i].Distance < garrisons[j].Distance })

			if jsonMode {
				printJSON(map[string]any{"units": rawUnits, "garrisons": garrisons})
				return nil
			}

			if len(rawUnits) == 0 && len(garrisons) == 0 {
				fmt.Println("Inga främmande enheter i sikte.")
				return nil
			}

			fmt.Println("Sightings — vad dina ögon ser just nu")
			if len(rawUnits) > 0 {
				fmt.Println("\n  ENHETER")
				for _, u := range rawUnits {
					owner := u.Owner
					if owner == "" {
						owner = "—"
					}
					var detail string
					if u.Status == "marching" && u.TargetQ != nil && u.TargetR != nil {
						eta := "okänt"
						if u.ArrivesAt != nil {
							eta = countdown(*u.ArrivesAt)
						}
						detail = fmt.Sprintf("marching → (%d,%d), landar om %s", *u.TargetQ, *u.TargetR, eta)
					} else {
						stance := u.Stance
						if stance == "" {
							stance = "-"
						}
						detail = fmt.Sprintf("%-8s (%d,%d)", stance, u.Q, u.R)
					}
					fmt.Printf("  %d hexar %-2s   %-13s %-13s ×%-5d %s\n",
						u.Distance, u.Bearing, owner, u.Type, u.Size, detail)
					if u.Cargo != nil {
						fmt.Printf("                                                   bär: %s ×%d\n",
							u.Cargo.Type, u.Cargo.Size)
					}
				}
			}
			if len(garrisons) > 0 {
				fmt.Println("\n  GARNISONER")
				for _, g := range garrisons {
					owner := g.Owner
					if owner == "" {
						owner = "—"
					}
					fmt.Printf("  %d hexar %-2s   %s (%s)   %d man\n", g.Distance, g.Bearing, g.Name, owner, g.ArmyTotal)
				}
			}
			fmt.Print("\nMarschera dit: keryx unit march --unit <id> --q <q> --r <r>   (dina enheter: keryx unit list)\n")
			return nil
		},
	}
}

// ownPositions returns every hex the player occupies — units on the map plus
// own city markers. Units that are forming/garrisoned carry no q/r of their own
// and are represented by their settlement's marker, so they are simply skipped.
func ownPositions(c *Client) [][2]int {
	var out [][2]int
	if data, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/units", cfg.WorldID)); err == nil {
		var d struct {
			Units []struct {
				Q *int `json:"q"`
				R *int `json:"r"`
			} `json:"units"`
		}
		if json.Unmarshal(data, &d) == nil {
			for _, u := range d.Units {
				if u.Q != nil && u.R != nil {
					out = append(out, [2]int{*u.Q, *u.R})
				}
			}
		}
	}
	if data, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/provinces", cfg.WorldID)); err == nil {
		var ms []struct {
			Q   int  `json:"q"`
			R   int  `json:"r"`
			Own bool `json:"own"`
		}
		if json.Unmarshal(data, &ms) == nil {
			for _, m := range ms {
				if m.Own {
					out = append(out, [2]int{m.Q, m.R})
				}
			}
		}
	}
	return out
}

// ownCoordinates resolves the caller's SEAT — their capital province, or
// (during founder phase, before any settlement exists) the wandering host's
// position. Same resolution order `keryx map` uses. Only the fallback for a
// player with nothing on the map; sightings measure from ownPositions.
func ownCoordinates(c *Client) (int, int, error) {
	prov := cfg.ProvinceID
	if prov == "" {
		fp, err := fetchFoundingStatus(c)
		if err != nil {
			return 0, 0, err
		}
		if !fp.Active || fp.Q == nil || fp.R == nil {
			return 0, 0, fmt.Errorf("no province in config and no active founder phase — rejoin the world or set province_id")
		}
		return *fp.Q, *fp.R, nil
	}
	statusData, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/provinces/%s", cfg.WorldID, prov))
	if err != nil {
		return 0, 0, err
	}
	var status struct {
		MapTile struct{ Q, R int } `json:"map_tile"`
	}
	if err := json.Unmarshal(statusData, &status); err != nil {
		return 0, 0, err
	}
	return status.MapTile.Q, status.MapTile.R, nil
}
