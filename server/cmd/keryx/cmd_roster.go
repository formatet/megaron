package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

// rosterSettlement is one settlement's row in GET
// .../settlements/placement-roster (api/handlers/settlement.go PlacementRoster).
type rosterSettlement struct {
	ID          string         `json:"id"`
	ProvinceID  string         `json:"province_id"`
	Name        string         `json:"name"`
	IsCapital   bool           `json:"is_capital"`
	TotalGubbar int            `json:"total_gubbar"`
	Placed      int            `json:"placed"`
	Idle        int            `json:"idle"`
	Assignments []rosterAssign `json:"assignments"`
}

type rosterAssign struct {
	GoodKey      string `json:"good_key"`
	TargetKind   string `json:"target_kind"`
	HexOrdinal   *int   `json:"hex_ordinal,omitempty"`
	HexQ         *int   `json:"hex_q,omitempty"`
	HexR         *int   `json:"hex_r,omitempty"`
	BuildingType string `json:"building_type,omitempty"`
	Count        int    `json:"count"`
}

// rosterCmd handles `keryx roster` — a realm-wide roll of every placed gubbe,
// grouped by settlement, with the idle pool per settlement front and centre.
// Answers player report 19ed51f1 ("se en lista över våra 'gubbar' och var de
// är och arbetar utan att behöva gå in på särskilda hexar … för att minska
// bemanningen och frigöra gubbar"): `keryx city` shows ONE settlement; this
// shows all of them at once, so a Wanax can find idle citizens to place, or
// spot which city to thin out, without opening each one. cult is not here —
// it's temple devotion (`keryx allocate --cult`), not a placed gubbe.
func rosterCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "roster",
		Short: "List every placed gubbe across all your settlements, with idle pools",
		Long: `A realm-wide view of where your citizens work. Each settlement shows how many
of its gubbar are placed vs idle, then every hex/building assignment with a
count. Use ` + "`keryx city <name>`" + ` to drill into one settlement and ` +
			"`keryx place`/`keryx staff`" + ` to move gubbar. (Temple devotion is set with ` +
			"`keryx allocate --cult`" + `, not shown here — it isn't a placed gubbe.)`,
		Args: noPositionalArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			data, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/settlements/placement-roster", cfg.WorldID))
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var roster []rosterSettlement
			if err := json.Unmarshal(data, &roster); err != nil {
				return fmt.Errorf("decode placement-roster: %w", err)
			}
			if len(roster) == 0 {
				fmt.Println("No settlements yet — found your capital first (see `keryx founding`).")
				return nil
			}

			// Realm summary line, so the single most-asked question ("how many
			// idle citizens do I have to place?") is answered before any detail.
			var total, placed, idle int
			for _, s := range roster {
				total += s.TotalGubbar
				placed += s.Placed
				idle += s.Idle
			}
			plural := "settlements"
			if len(roster) == 1 {
				plural = "settlement"
			}
			fmt.Printf("Gubbar across %d %s — %d/%d placed, %d idle\n", len(roster), plural, placed, total, idle)

			for _, s := range roster {
				role := ""
				if s.IsCapital {
					role = " (capital)"
				}
				idleNote := ""
				if s.Idle > 0 {
					idleNote = fmt.Sprintf(", %d idle", s.Idle)
				}
				fmt.Printf("\n%s%s  —  %d/%d placed%s\n", s.Name, role, s.Placed, s.TotalGubbar, idleNote)
				if len(s.Assignments) == 0 {
					fmt.Println("  (no gubbar placed — every citizen is idle)")
					continue
				}
				for _, a := range s.Assignments {
					fmt.Printf("  %2d× %-11s %s\n", a.Count, a.GoodKey, rosterLocation(a))
				}
			}
			return nil
		},
	}
}

// rosterLocation renders where an assignment sits: "hex #5" for a catchment
// hex (matching the ordinal `keryx place` takes), "hex (q,r)" if the ordinal
// couldn't be resolved, or the building type for a workplace.
func rosterLocation(a rosterAssign) string {
	switch a.TargetKind {
	case "hex":
		if a.HexOrdinal != nil {
			return fmt.Sprintf("hex #%d", *a.HexOrdinal)
		}
		if a.HexQ != nil && a.HexR != nil {
			return fmt.Sprintf("hex (%d,%d)", *a.HexQ, *a.HexR)
		}
		return "hex"
	case "building":
		return a.BuildingType
	default:
		return a.TargetKind
	}
}
