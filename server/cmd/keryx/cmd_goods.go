package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func goodsCmd() *cobra.Command {
	var provinceID string
	cmd := &cobra.Command{
		Use:   "goods",
		Short: "Show goods inventory and local prices (defaults to capital; --province for a colony)",
		Example: `  keryx goods
  keryx goods --province <province-id>   # inspect a colony`,
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
			// labor_pool och idle_citizens är identiska på varje rad — visa en gång.
			if lp, ok := goods[0]["labor_pool"].(float64); ok {
				idle := 0.0
				if ic, ok2 := goods[0]["idle_citizens"].(float64); ok2 {
					idle = ic
				}
				fmt.Printf("Labor pool: %d workers  ·  Idle: %d workers\n\n", int(lp), int(idle))
			}
			fmt.Printf("%-10s  %8s  %8s  %6s  %10s  %8s  %8s\n",
				"Good", "Stock", "Rate/d", "Lvl", "Workers", "Yield/w", "Price")
			fmt.Println("──────────────────────────────────────────────────────────────────────────")
			for _, g := range goods {
				key, _ := g["key"].(string)
				stock, _ := g["amount"].(float64)
				rateT, _ := g["rate_per_tick"].(float64)
				price, _ := g["price"].(float64)
				yieldW, _ := g["yield_per_worker"].(float64)
				producible, _ := g["producible"].(bool)
				employed, _ := g["employed_citizens"].(float64)
				unserved, _ := g["unserved_citizens"].(float64)
				wpLevel, _ := g["workplace_level"].(float64)
				rateD := rateT * 24 // per-tick × 24 ticks/day
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
				fmt.Printf("%-10s  %8.1f  %8.1f  %6s  %10s  %8s  %8.1f\n",
					key, stock, rateD, lvlStr, workersStr, yieldStr, price)
			}
			if sawUnserved {
				fmt.Println("\n! = citizens allocated beyond what the workplace can employ. They produce")
				fmt.Println("    nothing. Raise the building's level (build it again) or move the labor.")
			}
			return nil
		},
	}
	cmd.Flags().SortFlags = false
	cmd.Flags().StringVar(&provinceID, "province", "", "province ID to inspect (default: your capital)")
	return cmd
}

func transferCmd() *cobra.Command {
	var good string
	var qty float64
	var destName string
	var provinceID string

	cmd := &cobra.Command{
		Use:   "transfer",
		Short: "Send goods to one of your own settlements (internal logistics, no loss)",
		Example: `  keryx transfer --good grain --qty 50 --dest Korinth
  keryx transfer --from <colony> --good grain --qty 50 --dest Korinth   # pull a colony's surplus home`,
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

func giftCmd() *cobra.Command {
	var silver, grain float64
	var destName string

	cmd := &cobra.Command{
		Use:   "gift",
		Short: "Send silver/grain from your capital to one of your own colonies (boosts loyalty at 50+ silver-equivalent)",
		Example: `  keryx gift --silver 60 --dest Korinth
  keryx gift --grain 100 --silver 20 --dest Korinth`,
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
				destName, resp.SilverSent, resp.GrainSent, resp.ArrivesAt)
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
