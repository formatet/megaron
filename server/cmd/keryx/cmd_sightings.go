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

	TargetQ   *int       `json:"target_q,omitempty"`
	TargetR   *int       `json:"target_r,omitempty"`
	ArrivesAt *time.Time `json:"arrives_at,omitempty"`

	Distance int    `json:"distance"`
	Bearing  string `json:"bearing"`
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

Sorted by hex distance from your capital, nearest first.`,
		Args: noPositionalArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)

			// Own coordinates, to sort by distance and derive a compass bearing —
			// same resolution order as `keryx map` (capital province, or the
			// wandering host during founder phase).
			oq, or, err := ownCoordinates(c)
			if err != nil {
				return err
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
				rawUnits[i].Distance = hexDist(oq, or, rawUnits[i].Q, rawUnits[i].R)
				rawUnits[i].Bearing = compassDirection(oq, or, rawUnits[i].Q, rawUnits[i].R)
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
				garrisons = append(garrisons, sightingGarrison{
					Name: m.Name, Owner: m.Owner, Q: m.Q, R: m.R, ArmyTotal: m.ArmyTotal,
					Distance: hexDist(oq, or, m.Q, m.R),
					Bearing:  compassDirection(oq, or, m.Q, m.R),
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

// ownCoordinates resolves the caller's current map position — their capital
// province, or (during founder phase, before any settlement exists) the
// wandering host's position. Same resolution order `keryx map` uses, kept
// local to this file rather than factored out, since it is the only other
// command that needs "where am I" outside a specific --province flag.
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
