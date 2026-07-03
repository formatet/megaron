package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// unitCmd returns the top-level "unit" command with its subcommands.
func unitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unit",
		Short: "Manage discrete military units",
	}
	cmd.AddCommand(
		unitListCmd(),
		unitMarchCmd(),
		unitRecallCmd(),
		unitRedirectCmd(),
		unitStanceCmd(),
		unitLoadCmd(),
		unitUnloadCmd(),
	)
	return cmd
}

// ---- unit list ---------------------------------------------------------------

func unitListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List your units",
		Example: `  poleia unit list
  poleia unit list --json`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/units", cfg.WorldID)
			data, err := c.get(path)
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var resp struct {
				Units []unitRow `json:"units"`
			}
			if err := json.Unmarshal(data, &resp); err != nil {
				return fmt.Errorf("parse response: %w", err)
			}
			if len(resp.Units) == 0 {
				fmt.Println("No units.")
				return nil
			}
			fmt.Printf("%-36s  %-16s  %-8s  %-10s  %-9s  %s\n",
				"ID", "Type", "Size", "Status", "Stance", "Location / ETA")
			fmt.Println(strings.Repeat("─", 110))
			for _, u := range resp.Units {
				fmt.Printf("%-36s  %-16s  %-8s  %-10s  %-9s  %s\n",
					u.ID, u.Type, formatSize(u), u.Status, stanceStr(u.Stance), locationStr(u))
			}
			return nil
		},
	}
}

type unitRow struct {
	ID           string     `json:"id"`
	Type         string     `json:"type"`
	Category     string     `json:"category"`
	Size         int        `json:"size"`
	Crew         int        `json:"crew"`
	Status       string     `json:"status"`
	Stance       *string    `json:"stance"`
	SettlementID *string    `json:"settlement_id"`
	Q            *int       `json:"q"`
	R            *int       `json:"r"`
	TargetQ      *int       `json:"target_q"`
	TargetR      *int       `json:"target_r"`
	ArrivesAt    *time.Time `json:"arrives_at"`
	CargoUnitID  *string    `json:"cargo_unit_id"`
	MarchIntent  *string    `json:"march_intent"`
	ColonyName   *string    `json:"colony_name"`
}

func formatSize(u unitRow) string {
	if u.Status == "forming" {
		// A land unit auto-deploys (forming → garrison) the moment its size reaches
		// 100 men; you grow it by recruiting more of the same type into the same
		// settlement. Spell that out so the unit isn't left stuck at e.g. 40/100.
		return fmt.Sprintf("%d/100 (forming — recruit %d more %s here to deploy)",
			u.Size, 100-u.Size, u.Type)
	}
	if u.Category == "naval" {
		return fmt.Sprintf("1 vessel (crew %d)", u.Crew)
	}
	return fmt.Sprintf("%d men", u.Size)
}

func stanceStr(s *string) string {
	if s == nil || *s == "" {
		return "—"
	}
	return *s
}

func locationStr(u unitRow) string {
	switch u.Status {
	case "marching":
		loc := ""
		// Fas 2i: a colonize march has no settlement row until it arrives — this
		// was the only place its chosen name was visible at all before then.
		if u.MarchIntent != nil && *u.MarchIntent == "colonize" && u.ColonyName != nil && *u.ColonyName != "" {
			loc = fmt.Sprintf("founding %q (pending) — ", *u.ColonyName)
		}
		if u.Q != nil && u.R != nil {
			loc += fmt.Sprintf("(%d,%d)→", *u.Q, *u.R)
		}
		if u.TargetQ != nil && u.TargetR != nil {
			loc += fmt.Sprintf("(%d,%d)", *u.TargetQ, *u.TargetR)
		}
		if u.ArrivesAt != nil {
			loc += " ETA " + u.ArrivesAt.Local().Format("15:04 Jan 2")
		}
		return loc
	case "embarked":
		cargo := ""
		if u.CargoUnitID != nil {
			cargo = " aboard " + (*u.CargoUnitID)[:8] + "…"
		}
		return "embarked" + cargo
	default:
		if u.SettlementID != nil {
			return "settlement " + (*u.SettlementID)[:8] + "…"
		}
		if u.Q != nil && u.R != nil {
			return fmt.Sprintf("hex (%d,%d)", *u.Q, *u.R)
		}
		return "—"
	}
}

// ---- unit march --------------------------------------------------------------

func unitMarchCmd() *cobra.Command {
	var unitID string
	var targetQ, targetR int
	var stance string
	var intent, name string

	cmd := &cobra.Command{
		Use:   "march",
		Short: "Order a unit to march to a hex",
		Long: `Order a unit to march to a target hex (q,r coordinates).

Terrain passability:
  Impassable (all units):  mountain_limestone, mountain_red
  Land units only:         plains, hills, forest_olive_grove, scrub_maquis,
                           semi_desert, river_valley, river_delta
  Naval units only:        coastal_sea, deep_sea
  (Land units cannot enter sea; naval units cannot enter land.)

A land unit must reach 100 men (garrison status) before it can march.
A unit in fortify stance must be cleared (stance none) before marching.

Exploring: marching into fog or unknown territory is fully allowed — the
server does not FOW-gate the destination, only the route (A* over known
terrain). Sending a unit into the fog frontier IS how you explore: the
route it sweeps is revealed and remembered (dimmed on 'poleia map'
thereafter) once the unit arrives. Run 'poleia map' first to see the
frontier coordinates (fog tiles bordering what you already know) — march
a unit there to push the frontier outward.

Ore on mountain terrain (copper, tin, silver):
  Mountains are impassable — you cannot colonize the mountain hex itself.
  Instead, colonize an ADJACENT passable hex: the ore deposit will fall in
  the new colony's catchment and can be mined from there.
  Use 'poleia map' to see which adjacent hexes are passable.`,
		Example: `  poleia unit march --unit <id> --q 5 --r -3
  poleia unit march --unit <id> --q 5 --r -3 --stance fortify
  poleia unit march --unit <id> --q 5 --r -3 --intent colonize --name Thapsos
  # Explore: march toward a frontier coordinate from 'poleia map' to reveal it
  poleia unit march --unit <id> --q 12 --r -8`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body := map[string]any{
				"target_q": targetQ,
				"target_r": targetR,
			}
			if stance != "" {
				body["stance"] = stance
			}
			if intent != "" {
				body["intent"] = intent
			}
			if name != "" {
				body["name"] = name
			}
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/units/%s/march", cfg.WorldID, unitID)
			data, err := c.post(path, body)
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var resp map[string]any
			json.Unmarshal(data, &resp)
			arrivesAt, _ := resp["arrives_at"].(string)
			fmt.Printf("Unit %s marching to (%d,%d)", unitID[:8], targetQ, targetR)
			if arrivesAt != "" {
				if t, err := time.Parse(time.RFC3339, arrivesAt); err == nil {
					fmt.Printf(" — arrives %s", t.Local().Format("15:04 Jan 2"))
				}
			}
			fmt.Println()
			return nil
		},
	}

	cmd.Flags().StringVar(&unitID, "unit", "", "unit UUID (required)")
	cmd.Flags().IntVar(&targetQ, "q", 0, "target hex Q (required)")
	cmd.Flags().IntVar(&targetR, "r", 0, "target hex R (required)")
	cmd.Flags().StringVar(&stance, "stance", "", "stance on arrival: fortify|storm|sentry")
	cmd.Flags().StringVar(&intent, "intent", "", "arrival intent: colonize (found a new colony on arrival — use --name to name it)")
	cmd.Flags().StringVar(&name, "name", "", "colony name (with --intent colonize)")
	_ = cmd.MarkFlagRequired("unit")
	_ = cmd.MarkFlagRequired("q")
	_ = cmd.MarkFlagRequired("r")
	return cmd
}

// ---- unit recall / redirect ---------------------------------------------------

// recallResponse is the shared JSON shape returned by /recall for both modes.
type recallResponse struct {
	UnitID             string    `json:"unit_id"`
	MessengerID        string    `json:"messenger_id"`
	MessengerArrivesAt time.Time `json:"messenger_arrives_at"`
	DueTick            int       `json:"due_tick"`
	Mode               string    `json:"mode"`
}

func unitRecallCmd() *cobra.Command {
	var unitID string

	cmd := &cobra.Command{
		Use:   "recall",
		Short: "Recall a marching unit — turn it home",
		Long: `Send a recall order to a marching unit. The order travels as a visible
messenger; command is never instant — the unit keeps marching on its original
course until the messenger physically catches up with it, then turns for home
(the hex it originally departed from).`,
		Example: `  poleia unit recall --unit <id>`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/units/%s/recall", cfg.WorldID, unitID)
			data, err := c.post(path, map[string]any{})
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var resp recallResponse
			_ = json.Unmarshal(data, &resp)
			fmt.Printf("Recall order sent to unit %s — messenger arrives %s (tick %d); the unit turns home once it catches up.\n",
				unitID[:8], resp.MessengerArrivesAt.Local().Format("15:04 Jan 2"), resp.DueTick)
			return nil
		},
	}

	cmd.Flags().StringVar(&unitID, "unit", "", "unit UUID (required)")
	_ = cmd.MarkFlagRequired("unit")
	return cmd
}

func unitRedirectCmd() *cobra.Command {
	var unitID, target string

	cmd := &cobra.Command{
		Use:   "redirect",
		Short: "Redirect a marching unit to a new hex",
		Long: `Send a redirect order to a marching unit, giving it a new destination.
Command is never instant — the unit keeps marching on its original course until
the order messenger physically catches up with it, then turns onto the new course.`,
		Example: `  poleia unit redirect --unit <id> --target 5,-3`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q, r, err := parseQR(target)
			if err != nil {
				return err
			}
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/units/%s/recall", cfg.WorldID, unitID)
			data, err := c.post(path, map[string]any{"target_q": q, "target_r": r})
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var resp recallResponse
			_ = json.Unmarshal(data, &resp)
			fmt.Printf("Redirect order sent to unit %s (new course %d,%d) — messenger arrives %s (tick %d).\n",
				unitID[:8], q, r, resp.MessengerArrivesAt.Local().Format("15:04 Jan 2"), resp.DueTick)
			return nil
		},
	}

	cmd.Flags().StringVar(&unitID, "unit", "", "unit UUID (required)")
	cmd.Flags().StringVar(&target, "target", "", "new target hex as q,r (required)")
	_ = cmd.MarkFlagRequired("unit")
	_ = cmd.MarkFlagRequired("target")
	return cmd
}

// parseQR parses a "q,r" flag value into two ints.
func parseQR(s string) (int, int, error) {
	parts := strings.Split(s, ",")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid --target %q: expected \"q,r\"", s)
	}
	q, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid --target %q: q is not an integer", s)
	}
	r, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid --target %q: r is not an integer", s)
	}
	return q, r, nil
}

// ---- unit stance -------------------------------------------------------------

func unitStanceCmd() *cobra.Command {
	var unitID, stance string

	cmd := &cobra.Command{
		Use:   "stance",
		Short: "Set or clear a unit's stance",
		Example: `  poleia unit stance --unit <id> --stance fortify
  poleia unit stance --unit <id> --stance none`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/units/%s/stance", cfg.WorldID, unitID)
			data, err := c.post(path, map[string]any{"stance": stance})
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			if stance == "none" {
				fmt.Printf("Unit %s stance cleared\n", unitID[:8])
			} else {
				fmt.Printf("Unit %s stance → %s\n", unitID[:8], stance)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&unitID, "unit", "", "unit UUID (required)")
	cmd.Flags().StringVar(&stance, "stance", "", "stance: fortify|storm|sentry|none (required)")
	_ = cmd.MarkFlagRequired("unit")
	_ = cmd.MarkFlagRequired("stance")
	return cmd
}

// ---- unit load ---------------------------------------------------------------

func unitLoadCmd() *cobra.Command {
	var shipID, landUnitID string

	cmd := &cobra.Command{
		Use:     "load",
		Short:   "Embark a land unit onto a ship",
		Example: `  poleia unit load --ship <ship-id> --unit <land-unit-id>`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/units/%s/load", cfg.WorldID, shipID)
			data, err := c.post(path, map[string]any{"unit_id": landUnitID})
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			fmt.Printf("Unit %s embarked on ship %s\n", landUnitID[:8], shipID[:8])
			return nil
		},
	}

	cmd.Flags().StringVar(&shipID, "ship", "", "ship unit UUID (required)")
	cmd.Flags().StringVar(&landUnitID, "unit", "", "land unit UUID to embark (required)")
	_ = cmd.MarkFlagRequired("ship")
	_ = cmd.MarkFlagRequired("unit")
	return cmd
}

// ---- unit unload -------------------------------------------------------------

func unitUnloadCmd() *cobra.Command {
	var shipID string

	cmd := &cobra.Command{
		Use:     "unload",
		Short:   "Disembark the cargo unit from a ship",
		Example: `  poleia unit unload --ship <ship-id>`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/units/%s/unload", cfg.WorldID, shipID)
			data, err := c.post(path, nil)
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			fmt.Printf("Cargo unit disembarked from ship %s\n", shipID[:8])
			return nil
		},
	}

	cmd.Flags().StringVar(&shipID, "ship", "", "ship unit UUID (required)")
	_ = cmd.MarkFlagRequired("ship")
	return cmd
}
