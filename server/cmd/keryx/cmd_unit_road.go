package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// ---- unit build-road / roads -------------------------------------------------
//
// Roads (megaron_plan_vagar.md V2): one edge between two neighbouring hexes,
// built by an army from stone its nearest city pays when the order is given.

// unitBuildRoadCmd orders one road edge. The Runner carries the order to the
// army; the stone travels as a caravan to the edge's first hex ("from").
func unitBuildRoadCmd() *cobra.Command {
	var unitID, from, to string
	var toQ, toR, fromQ, fromR int

	cmd := &cobra.Command{
		Use:   "build-road",
		Short: "Order an army to build one road edge between two neighbouring hexes",
		Long: `Order an army to build ONE road edge: --from is the hex it builds from, --to the
neighbouring hex the road reaches. The stone (8 per plains edge, more in rougher
ground) leaves your nearest city NOW and travels as a caravan to the --from hex;
the order travels by Runner to the army. Building begins when BOTH are at --from
and takes about a game day per hundred men on plains. Roads are free for everyone to
use. A road never crosses water or mountains and ends at a river ford. If the
caravan is intercepted the road is not finished and the stone is lost; see
'keryx roads' for roads still waiting. A new order to the army cancels its
unfinished road. --from defaults to the army's field position.`,
		Example: `  keryx unit build-road --unit <id> --to 6,0
  keryx unit build-road --unit <id> --from 5,0 --to 6,0`,
		Args: rejectPositionalArgs("unit"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			tq, tr, given, err := resolveRoadHex(cmd, "to", to, toQ, toR)
			if err != nil {
				return err
			}
			if !given {
				return fmt.Errorf("--to q,r (or --to-q/--to-r) is required: the neighbouring hex the road reaches")
			}
			c := newClient(cfg)
			resolvedID, rerr := resolveUnitID(c, cfg.WorldID, unitID)
			if rerr != nil {
				return rerr
			}
			unitID = resolvedID
			fq, fr, fromGiven, err := resolveRoadHex(cmd, "from", from, fromQ, fromR)
			if err != nil {
				return err
			}
			if !fromGiven {
				u, uerr := fetchUnitRow(c, cfg.WorldID, unitID)
				if uerr != nil {
					return uerr
				}
				if u.Q == nil || u.R == nil || u.Status != "positioned" {
					return fmt.Errorf("--from q,r is required: this army is not standing in the field, so it has no position of its own to start the road from")
				}
				fq, fr = *u.Q, *u.R
			}
			data, err := c.post(fmt.Sprintf("/api/v1/worlds/%s/units/%s/build-road", cfg.WorldID, unitID),
				map[string]any{"from_q": fq, "from_r": fr, "to_q": tq, "to_r": tr})
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var resp struct {
				Stone            float64 `json:"stone"`
				SiteID           string  `json:"site_id"`
				CourierArrivesAt string  `json:"courier_arrives_at"`
			}
			_ = json.Unmarshal(data, &resp)
			fmt.Printf("Road ordered: army %s builds (%d,%d) → (%d,%d); %g stone is on its way from your city", short8(unitID), fq, fr, tq, tr, resp.Stone)
			if t, perr := time.Parse(time.RFC3339, resp.CourierArrivesAt); perr == nil {
				fmt.Printf("; the Runner reaches the army %s", gameETA(c, t))
			}
			fmt.Println(".")
			fmt.Println("Building begins when the stone and the army are both at the first hex. Check progress with `keryx roads`.")
			return nil
		},
	}
	cmd.Flags().StringVar(&unitID, "unit", "", "army UUID or id prefix (required)")
	cmd.Flags().StringVar(&from, "from", "", "hex the army builds from, as q,r (default: where the army stands)")
	cmd.Flags().IntVar(&fromQ, "from-q", 0, "from-hex Q — alternative to --from")
	cmd.Flags().IntVar(&fromR, "from-r", 0, "from-hex R — alternative to --from")
	cmd.Flags().StringVar(&to, "to", "", "neighbouring hex the road reaches, as q,r — axial coordinates, read them off 'keryx map'")
	cmd.Flags().IntVar(&toQ, "to-q", 0, "to-hex Q — alternative to --to")
	cmd.Flags().IntVar(&toR, "to-r", 0, "to-hex R — alternative to --to")
	_ = cmd.MarkFlagRequired("unit")
	return cmd
}

func short8(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// resolveRoadHex reads a hex given either as --<name> q,r or --<name>-q/--<name>-r.
func resolveRoadHex(cmd *cobra.Command, name, combined string, q, r int) (hq, hr int, given bool, err error) {
	combinedSet := cmd.Flags().Changed(name)
	qSet, rSet := cmd.Flags().Changed(name+"-q"), cmd.Flags().Changed(name+"-r")
	switch {
	case combinedSet && (qSet || rSet):
		return 0, 0, false, fmt.Errorf("give --%s as either q,r or --%s-q/--%s-r, not both", name, name, name)
	case combinedSet:
		hq, hr, err = parseQR(combined)
		return hq, hr, err == nil, err
	case qSet && rSet:
		return q, r, true, nil
	case qSet || rSet:
		return 0, 0, false, fmt.Errorf("--%s-q and --%s-r must both be given (or use --%s q,r)", name, name, name)
	}
	return 0, 0, false, nil
}

// fetchUnitRow reads one of the caller's units from the unit list.
func fetchUnitRow(c *Client, worldID, unitID string) (unitRow, error) {
	data, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/units", worldID))
	if err != nil {
		return unitRow{}, err
	}
	var resp struct {
		Units []unitRow `json:"units"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return unitRow{}, err
	}
	for _, u := range resp.Units {
		if u.ID == unitID {
			return u, nil
		}
	}
	return unitRow{}, fmt.Errorf("unit %s not found among your units", unitID)
}

// roadsCmd lists the caller's unfinished roads — so a road stuck waiting for its
// stone or its army is visible — and, with --edges, the finished edges known to
// the caller.
func roadsCmd() *cobra.Command {
	var edges bool
	cmd := &cobra.Command{
		Use:   "roads",
		Short: "Your unfinished roads, and (--edges) the finished road edges you know of",
		Long: `Lists the roads you ordered that are not finished, and what each is still
waiting for (the stone caravan, the army). A caravan that was intercepted means
that road will not be finished — give a new order. With --edges, also lists the
finished road edges your people have seen or remember.`,
		Example: "  keryx roads\n  keryx roads --edges",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			sitesRaw, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/road-sites", cfg.WorldID))
			if err != nil {
				return err
			}
			var edgesRaw []byte
			if edges {
				if edgesRaw, err = c.get(fmt.Sprintf("/api/v1/worlds/%s/roads", cfg.WorldID)); err != nil {
					return err
				}
			}
			if jsonMode {
				if edges {
					fmt.Printf("{\"road_sites\":%s,\"roads\":%s}\n", sitesRaw, edgesRaw)
					return nil
				}
				printRawJSON(sitesRaw)
				return nil
			}
			var sites struct {
				Sites []struct {
					UnitID      *string  `json:"unit_id"`
					FromQ       int      `json:"from_q"`
					FromR       int      `json:"from_r"`
					ToQ         int      `json:"to_q"`
					ToR         int      `json:"to_r"`
					Stone       float64  `json:"stone"`
					State       string   `json:"state"`
					WaitingFor  []string `json:"waiting_for"`
					BuildDueTic *int     `json:"build_due_tick"`
				} `json:"sites"`
			}
			if err := json.Unmarshal(sitesRaw, &sites); err != nil {
				return err
			}
			if len(sites.Sites) == 0 {
				fmt.Println("No unfinished roads.")
			}
			for _, s := range sites.Sites {
				army := "an army"
				if s.UnitID != nil {
					army = "army " + short8(*s.UnitID)
				}
				line := fmt.Sprintf("(%d,%d) → (%d,%d)  %g stone  %s: ", s.FromQ, s.FromR, s.ToQ, s.ToR, s.Stone, army)
				if s.State == "building" {
					line += "building"
				} else {
					line += "waiting for " + strings.Join(s.WaitingFor, " and ")
				}
				fmt.Println(line)
			}
			if edges {
				var e struct {
					Edges []struct{ Q1, R1, Q2, R2 int } `json:"edges"`
				}
				if err := json.Unmarshal(edgesRaw, &e); err != nil {
					return err
				}
				fmt.Printf("\nKnown road edges: %d\n", len(e.Edges))
				for _, x := range e.Edges {
					fmt.Printf("  (%d,%d) — (%d,%d)\n", x.Q1, x.R1, x.Q2, x.R2)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&edges, "edges", false, "also list the finished road edges you know of")
	return cmd
}
