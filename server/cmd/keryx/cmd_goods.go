package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func goodsCmd() *cobra.Command {
	var provinceID string
	cmd := &cobra.Command{
		Use:   "goods",
		Short: "Show goods inventory and base values (defaults to capital; --province for a colony)",
		Example: `  keryx goods
  keryx goods --province <province-id>   # inspect a colony`,
		Args: rejectPositionalArgs("province"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			// Default to the capital; --province lets you inspect any province you own,
			// mirroring `build`/`status --province`.
			prov := cfg.ProvinceID
			if provinceID != "" {
				resolved, err := resolveProvince(c, cfg.WorldID, provinceID)
				if err != nil {
					return err
				}
				prov = resolved
			}
			path := fmt.Sprintf("/api/v1/worlds/%s/provinces/%s/goods", cfg.WorldID, prov)
			data, err := c.get(path)
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var goods []map[string]any
			if err := json.Unmarshal(data, &goods); err != nil {
				return err
			}
			if len(goods) == 0 {
				fmt.Println("No goods available.")
				return nil
			}
			sawUnserved := false
			var capNotes []capNote
			// labor_pool och idle_citizens är identiska på varje rad — visa en gång.
			if lp, ok := goods[0]["labor_pool"].(float64); ok {
				idle := 0.0
				if ic, ok2 := goods[0]["idle_citizens"].(float64); ok2 {
					idle = ic
				}
				fmt.Printf("Labor pool: %d workers  ·  Idle: %d workers\n\n", int(lp), int(idle))
			}
			fmt.Printf("%-10s  %9s  %8s  %6s  %10s  %8s  %8s\n",
				"Good", "Stock", "Rate/t", "Lvl", "Workers", "Yield/w", "Value")
			fmt.Println("──────────────────────────────────────────────────────────────────────────")
			for _, g := range goods {
				key, _ := g["key"].(string)
				stock, _ := g["amount"].(float64)
				capV, _ := g["cap"].(float64)
				percent, _ := g["percent"].(float64)
				rateT, _ := g["rate_per_tick"].(float64)
				baseValue, _ := g["base_value"].(float64)
				yieldW, _ := g["yield_per_worker"].(float64)
				producible, _ := g["producible"].(bool)
				employed, _ := g["employed_citizens"].(float64)
				unserved, _ := g["unserved_citizens"].(float64)
				wpLevel, _ := g["workplace_level"].(float64)
				// Workers reads "employed" normally, "employed+N idle" when the
				// allocation exceeds what the workplace can employ. Before this the
				// overflow was completely silent — a playtester could allocate 100 % of
				// the city to fish behind a level-1 harbour and see no difference from a
				// saturated one (Deiphobos, 2026-07-23).
				workersStr := fmt.Sprintf("%d", int(employed))
				if unserved >= 1 {
					workersStr = fmt.Sprintf("%d+%d!", int(employed), int(unserved))
					sawUnserved = true
				}
				lvlStr := "—"
				if wpLevel > 0 {
					lvlStr = fmt.Sprintf("L%d", int(wpLevel))
				}
				yieldStr := fmt.Sprintf("%.4f", yieldW)
				if !producible {
					// Terrain cannot produce this good — grey it out for the agent.
					workersStr = "—"
					yieldStr = "—"
					lvlStr = "—"
				}
				// Same class of silent waste as the unserved-labor case above, on the
				// other end of the good's pipe: the workplace is fine, but the stock
				// is full, so everything produced past the cap is discarded and the
				// labor sitting on it earns nothing (2026-07-25 DB audit: 38% of the
				// world's labor was on capped goods with no marker anywhere).
				stockStr := fmt.Sprintf("%.1f", stock)
				if atStorageCeiling(stock, capV) {
					stockStr += "*"
					capNotes = append(capNotes, capNote{key: key, percent: percent})
				}
				fmt.Printf("%-10s  %9s  %8.1f  %6s  %10s  %8s  %8.1f\n",
					key, stockStr, rateT, lvlStr, workersStr, yieldStr, baseValue)
			}
			if sawUnserved {
				fmt.Println("\n! = citizens allocated beyond what the workplace can employ. They produce")
				fmt.Println("    nothing. Raise the building's level (build it again) or move the labor.")
			}
			if len(capNotes) > 0 {
				fmt.Println()
				fmt.Println(capFootnote(capNotes))
			}
			return nil
		},
	}
	cmd.Flags().SortFlags = false
	cmd.Flags().StringVar(&provinceID, "province", "", "province ID to inspect (default: your capital)")
	return cmd
}

// atStorageCeiling reports whether a good's stock has effectively filled its
// cap. >=99% rather than the exact `>= cap`: some rows land slightly over cap
// via delivery paths that arrive after the server's cap check runs, and a
// good sitting at 99% is full within the hour regardless — one state to
// detect, not two.
func atStorageCeiling(amount, capV float64) bool {
	return capV > 0 && amount >= capV*0.99
}

// capNote is one good at its storage ceiling, carried through to the
// footnote so it can call out the labor share actually being wasted.
type capNote struct {
	key     string
	percent float64
}

// capFootnote builds the "* = at the storage ceiling" note for `keryx
// goods`. Goods with labor still allocated to them (percent > 0) get their
// share called out — that labor is the part actually producing nothing;
// goods at cap with no labor on them are just named.
func capFootnote(notes []capNote) string {
	if len(notes) == 0 {
		return ""
	}
	parts := make([]string, 0, len(notes))
	for _, n := range notes {
		if n.percent > 0 {
			parts = append(parts, fmt.Sprintf("%s (%.0f%% of your labor)", n.key, n.percent))
		} else {
			parts = append(parts, n.key)
		}
	}
	return "* = at the storage ceiling. Everything produced above it is discarded — the citizens\n" +
		"    working it produce nothing. At the ceiling now: " + strings.Join(parts, ", ") + ".\n" +
		"    Move that labor to a good with room, or spend the stock (build, staff a foundry/press, trade it away)."
}

func transferCmd() *cobra.Command {
	var good string
	var qty float64
	var destName string
	var provinceID string

	cmd := &cobra.Command{
		Use: "transfer",
		// "no loss" here means no storm/pirates dice roll (internal logistics never
		// rolls it, unlike a negotiated trade) — it is still a physical caravan that
		// can be intercepted and seized. Don't shorten this back to a bare "no loss";
		// that reads as risk-free, which it isn't (see `keryx cargo`).
		Short: "Send goods to one of your own settlements (no consent needed, no storm/pirates roll — but still a physical, seizable caravan)",
		Example: `  keryx transfer --good grain --qty 50 --dest Korinth
  keryx transfer --from <colony> --good grain --qty 50 --dest Korinth   # pull a colony's surplus home`,
		// --good, --qty and --dest are all required — no single one is the
		// obvious guess for a stray positional.
		Args: noPositionalArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			destID, err := resolveSettlement(c, cfg.WorldID, destName)
			if err != nil {
				return fmt.Errorf("resolve destination %q: %w", destName, err)
			}
			// Default source is the capital; --from/--province lets you pull a
			// colony's surplus home instead, mirroring `goods`/`build --province`.
			src := cfg.ProvinceID
			if provinceID != "" {
				resolved, err := resolveProvince(c, cfg.WorldID, provinceID)
				if err != nil {
					return err
				}
				src = resolved
			}
			path := fmt.Sprintf("/api/v1/worlds/%s/provinces/%s/trade", cfg.WorldID, src)
			data, err := c.post(path, map[string]any{
				"good_key":       good,
				"quantity":       qty,
				"destination_id": destID,
			})
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var resp map[string]any
			if err := json.Unmarshal(data, &resp); err != nil {
				return err
			}
			mins, _ := resp["travel_min"].(float64)
			fmt.Printf("Transfer dispatched: %.1f %s → %s · arrives in %.0f min\n", qty, good, destName, mins)
			return nil
		},
	}

	cmd.Flags().StringVarP(&good, "good", "g", "", "good key (e.g. grain, timber, silver)")
	cmd.Flags().Float64VarP(&qty, "qty", "q", 0, "quantity to send")
	cmd.Flags().StringVarP(&destName, "dest", "d", "", "destination settlement name")
	cmd.Flags().StringVar(&provinceID, "province", "", "source province/settlement (default: your capital)")
	cmd.Flags().StringVar(&provinceID, "from", "", "alias for --province")
	_ = cmd.MarkFlagRequired("good")
	_ = cmd.MarkFlagRequired("qty")
	_ = cmd.MarkFlagRequired("dest")
	return cmd
}

// cargoCmd is "last i rörelse" for keryx (web/last-i-rorelse, 2026-08): before
// this, `keryx transfer` gave a fire-and-forget dispatch confirmation and then
// the cargo vanished from view — no ETA, no list of what's still moving.
// GET /trades carries every physical mover visible to the caller (internal
// transfers AND trade legs); this filters to the caller's own and prints it.
//
// It filters on `role`, not `mine` (2026-08-24). `mine` is true only for the
// side that DISPATCHED the shipment, so a Wanax who accepted a trade offer,
// paid escrow, and was waiting on the goods saw "No cargo of yours currently
// in transit" for the whole voyage — the one row that mattered to them was in
// the payload the entire time, marked as someone else's. The buyer could not
// watch the thing they had already paid for.
func cargoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cargo",
		Short: "Show goods in transit to or from you (internal transfers and trade deliveries)",
		Args:  noPositionalArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/trades", cfg.WorldID)
			data, err := c.get(path)
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var markers []map[string]any
			if err := json.Unmarshal(data, &markers); err != nil {
				return err
			}
			var mine []map[string]any
			for _, m := range markers {
				switch role, _ := m["role"].(string); role {
				case "sender", "recipient":
					mine = append(mine, m)
				}
			}
			if len(mine) == 0 {
				fmt.Println("No cargo of yours currently in transit.")
				return nil
			}
			fmt.Printf("%-10s  %9s  %12s  %12s  %10s  %s\n", "Good", "Qty", "From", "To", "ETA", "Direction")
			fmt.Println("─────────────────────────────────────────────────────────────────────────────")
			for _, m := range mine {
				good, _ := m["good_key"].(string)
				qty, _ := m["quantity"].(float64)
				oq, _ := m["origin_q"].(float64)
				orr, _ := m["origin_r"].(float64)
				dq, _ := m["dest_q"].(float64)
				dr, _ := m["dest_r"].(float64)
				etaStr := "—"
				if arrivesStr, ok := m["arrives_at"].(string); ok {
					if t, err := time.Parse(time.RFC3339, arrivesStr); err == nil {
						etaStr = gameETA(c, t)
					}
				}
				direction := "outgoing"
				if role, _ := m["role"].(string); role == "recipient" {
					direction = "incoming"
				}
				fmt.Printf("%-10s  %9.0f  %12s  %12s  %10s  %s\n",
					good, qty,
					fmt.Sprintf("(%d,%d)", int(oq), int(orr)),
					fmt.Sprintf("(%d,%d)", int(dq), int(dr)),
					etaStr, direction)
			}
			fmt.Println("\nPhysical cargo, not a promise — it can be intercepted and seized in transit.")
			fmt.Println("Internal transfers never roll the storm/pirates loss die (that's only for")
			fmt.Println("negotiated trade deliveries) — but interception is a separate risk.")
			return nil
		},
	}
}

func giftCmd() *cobra.Command {
	var silver, grain float64
	var destName string

	cmd := &cobra.Command{
		Use:   "gift",
		Short: "Send silver/grain from your capital to one of your own colonies (boosts loyalty at 50+ silver-equivalent)",
		Example: `  keryx gift --silver 60 --dest Korinth
  keryx gift --grain 100 --silver 20 --dest Korinth`,
		// --silver/--grain/--dest are all plausible — no single one is the
		// obvious guess for a stray positional.
		Args: noPositionalArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if silver <= 0 && grain <= 0 {
				return fmt.Errorf("gift must include --silver or --grain")
			}
			c := newClient(cfg)
			destID, err := resolveSettlement(c, cfg.WorldID, destName)
			if err != nil {
				return fmt.Errorf("resolve destination %q: %w", destName, err)
			}
			path := fmt.Sprintf("/api/v1/worlds/%s/settlements/%s/gift", cfg.WorldID, destID)
			data, err := c.post(path, map[string]any{
				"silver": silver,
				"grain":  grain,
			})
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var resp struct {
				LoyaltyDelta int     `json:"loyalty_delta"`
				SilverSent   float64 `json:"silver_sent"`
				GrainSent    float64 `json:"grain_sent"`
				ArrivesAt    string  `json:"arrives_at"`
			}
			if err := json.Unmarshal(data, &resp); err != nil {
				return err
			}
			fmt.Printf("Gift dispatched to %s: silver %.0f, grain %.0f · arrives %s\n",
				destName, resp.SilverSent, resp.GrainSent, arrivalETA(c, resp.ArrivesAt))
			if resp.LoyaltyDelta > 0 {
				fmt.Printf("Loyalty +%d on arrival.\n", resp.LoyaltyDelta)
			} else {
				fmt.Println("Below the 50 silver-equivalent threshold — no loyalty gain.")
			}
			return nil
		},
	}

	cmd.Flags().Float64Var(&silver, "silver", 0, "silver to send")
	cmd.Flags().Float64Var(&grain, "grain", 0, "grain to send")
	cmd.Flags().StringVarP(&destName, "dest", "d", "", "destination settlement name (must be your own)")
	_ = cmd.MarkFlagRequired("dest")
	return cmd
}
