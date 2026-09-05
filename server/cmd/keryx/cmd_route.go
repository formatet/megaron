package main

// Standing orders (megaron_plan_staende_leverans.md) — caravans that run
// themselves between two of a Wanax's own settlements. Keryx-yta (megaron_moc):
// everything visible in the web economy tab must be actionable here too.

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// parseGoodAmountPairs parses "grain:200,fish:50" into good→amount. Used for
// both --out (threshold to maintain at the destination) and --home (floor to
// leave behind at the destination when loading the return leg) — same shape,
// different meaning, so this is shared and the caller names the field.
func parseGoodAmountPairs(spec string) ([]struct {
	Good   string
	Amount float64
}, error) {
	var out []struct {
		Good   string
		Amount float64
	}
	if spec == "" {
		return out, nil
	}
	for _, part := range strings.Split(spec, ",") {
		kv := strings.SplitN(strings.TrimSpace(part), ":", 2)
		if len(kv) != 2 {
			return nil, fmt.Errorf("invalid good:amount pair %q — expected e.g. grain:200", part)
		}
		amount, err := strconv.ParseFloat(strings.TrimSpace(kv[1]), 64)
		if err != nil {
			return nil, fmt.Errorf("invalid amount in %q: %w", part, err)
		}
		out = append(out, struct {
			Good   string
			Amount float64
		}{Good: strings.TrimSpace(kv[0]), Amount: amount})
	}
	return out, nil
}

// resolveOwnSettlement resolves a --from/--crew value to a settlement ID,
// defaulting to the caller's capital settlement when nameOrID is empty —
// mirrors transferCmd's --from/--province default-to-capital convention.
func resolveOwnSettlement(c *Client, worldID, nameOrID string) (string, error) {
	if nameOrID != "" {
		return resolveSettlement(c, worldID, nameOrID)
	}
	data, err := c.get("/api/v1/worlds/" + worldID + "/provinces")
	if err != nil {
		return "", err
	}
	var markers []map[string]any
	if err := json.Unmarshal(data, &markers); err != nil {
		return "", err
	}
	for _, m := range markers {
		if capital, _ := m["is_capital"].(bool); capital {
			if sid, _ := m["settlement_id"].(string); sid != "" {
				return sid, nil
			}
		}
	}
	return "", fmt.Errorf("could not find your capital settlement — pass --from explicitly")
}

func routeCreateCmd() *cobra.Command {
	var fromName, toName, crewSide, outSpec, homeSpec string
	cmd := &cobra.Command{
		Use:   "route",
		Short: "Create a standing order — a caravan that keeps a destination's stock topped up on its own",
		Example: `  keryx route --to Colony --out grain:200
  keryx route --from Petras --to Colony --out grain:200,fish:50 --home silver:0,stone:20 --crew from`,
		Args: noPositionalArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			fromID, err := resolveOwnSettlement(c, cfg.WorldID, fromName)
			if err != nil {
				return fmt.Errorf("resolve --from: %w", err)
			}
			toID, err := resolveSettlement(c, cfg.WorldID, toName)
			if err != nil {
				return fmt.Errorf("resolve --to: %w", err)
			}
			crewID := fromID
			if crewSide == "to" {
				crewID = toID
			} else if crewSide != "" && crewSide != "from" {
				return fmt.Errorf("--crew must be \"from\" or \"to\", got %q", crewSide)
			}

			outPairs, err := parseGoodAmountPairs(outSpec)
			if err != nil {
				return fmt.Errorf("--out: %w", err)
			}
			if len(outPairs) == 0 {
				return fmt.Errorf("--out must name at least one good:threshold, e.g. grain:200")
			}
			homePairs, err := parseGoodAmountPairs(homeSpec)
			if err != nil {
				return fmt.Errorf("--home: %w", err)
			}

			outbound := make([]map[string]any, 0, len(outPairs))
			for _, p := range outPairs {
				outbound = append(outbound, map[string]any{"good_key": p.Good, "threshold": p.Amount})
			}
			ret := make([]map[string]any, 0, len(homePairs))
			for _, p := range homePairs {
				ret = append(ret, map[string]any{"good_key": p.Good, "floor": p.Amount})
			}

			path := fmt.Sprintf("/api/v1/worlds/%s/standing-orders", cfg.WorldID)
			data, err := c.post(path, map[string]any{
				"from_settlement_id":      fromID,
				"to_settlement_id":        toID,
				"crewed_by_settlement_id": crewID,
				"outbound":                outbound,
				"return":                  ret,
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
			fmt.Printf("Standing order created: %s → %s (id %v)\n", fromName, toName, resp["id"])
			return nil
		},
	}
	cmd.Flags().SortFlags = false
	cmd.Flags().StringVar(&fromName, "from", "", "source settlement name (default: your capital)")
	cmd.Flags().StringVar(&toName, "to", "", "destination settlement name")
	cmd.Flags().StringVar(&crewSide, "crew", "from", `which end supplies the gubbe: "from" or "to"`)
	cmd.Flags().StringVar(&outSpec, "out", "", "outbound goods to keep topped up at the destination, e.g. grain:200,fish:50")
	cmd.Flags().StringVar(&homeSpec, "home", "", "return goods to bring home, leaving at least this much behind, e.g. silver:0,stone:20")
	_ = cmd.MarkFlagRequired("to")
	_ = cmd.MarkFlagRequired("out")
	return cmd
}

func routesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "routes",
		Short:   "List your standing orders",
		Example: `  keryx routes`,
		Args:    noPositionalArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			data, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/standing-orders", cfg.WorldID))
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var orders []map[string]any
			if err := json.Unmarshal(data, &orders); err != nil {
				return err
			}
			if len(orders) == 0 {
				fmt.Println("No standing orders.")
				return nil
			}
			for _, o := range orders {
				status, _ := o["status"].(string)
				line := fmt.Sprintf("%v  %v → %v  [%s]", o["id"], o["from_name"], o["to_name"], status)
				if reason, _ := o["pause_reason"].(string); reason != "" {
					line += " — " + reason
				}
				fmt.Println(line)
			}
			return nil
		},
	}
	return cmd
}

func routeSetStatusCmd(use, short, verb string) *cobra.Command {
	var orderID string
	cmd := &cobra.Command{
		Use:     use,
		Short:   short,
		Example: fmt.Sprintf(`  keryx %s --id <standing-order-id>`, use),
		Args:    noPositionalArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/standing-orders/%s/%s", cfg.WorldID, orderID, verb)
			data, err := c.post(path, nil)
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			fmt.Printf("Standing order %s: %s\n", orderID, verb)
			return nil
		},
	}
	cmd.Flags().StringVar(&orderID, "id", "", "standing order ID (see `keryx routes`)")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func routePauseCmd() *cobra.Command {
	return routeSetStatusCmd("route-pause", "Pause a standing order (an in-flight caravan still completes)", "pause")
}

func routeResumeCmd() *cobra.Command {
	return routeSetStatusCmd("route-resume", "Resume a paused standing order", "resume")
}

func routeDeleteCmd() *cobra.Command {
	var orderID string
	cmd := &cobra.Command{
		Use:     "route-delete",
		Short:   "Delete a standing order (an in-flight caravan still completes, as an ordinary transfer)",
		Example: `  keryx route-delete --id <standing-order-id>`,
		Args:    noPositionalArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/standing-orders/%s", cfg.WorldID, orderID)
			if _, err := c.delete(path); err != nil {
				return err
			}
			fmt.Printf("Standing order %s deleted\n", orderID)
			return nil
		},
	}
	cmd.Flags().StringVar(&orderID, "id", "", "standing order ID (see `keryx routes`)")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}
