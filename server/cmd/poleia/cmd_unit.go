package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/poleia/server/internal/events"
	"github.com/poleia/server/internal/unit"
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
			fmt.Printf("%-36s  %-16s  %-14s  %-8s  %-10s  %-9s  %s\n",
				"ID", "Type", "Name", "Size", "Status", "Stance", "Location / ETA")
			fmt.Println(strings.Repeat("─", 125))
			for _, u := range resp.Units {
				fmt.Printf("%-36s  %-16s  %-14s  %-8s  %-10s  %-9s  %s\n",
					u.ID, unit.DisplayName(u.Type), shipNameStr(u.Name), formatSize(u), u.Status, stanceStr(u.Stance), locationStr(u))
			}
			return nil
		},
	}
}

type unitRow struct {
	ID              string     `json:"id"`
	Type            string     `json:"type"`
	Category        string     `json:"category"`
	Size            int        `json:"size"`
	Crew            int        `json:"crew"`
	Status          string     `json:"status"`
	Name            *string    `json:"name"`
	BuildCompleteAt *time.Time `json:"build_complete_at"`
	Stance          *string    `json:"stance"`
	SettlementID    *string    `json:"settlement_id"`
	Q               *int       `json:"q"`
	R               *int       `json:"r"`
	TargetQ         *int       `json:"target_q"`
	TargetR         *int       `json:"target_r"`
	ArrivesAt       *time.Time `json:"arrives_at"`
	CargoUnitID     *string    `json:"cargo_unit_id"`
	MarchIntent     *string    `json:"march_intent"`
	ColonyName      *string    `json:"colony_name"`
}

func formatSize(u unitRow) string {
	switch u.Status {
	case "forming":
		if u.Category == "naval" {
			// A ship builds as one vessel with a fixed build time (ship-build
			// overhaul 2026-07-09) — not size-based like land, so show the ETA.
			eta := "unknown"
			if u.BuildCompleteAt != nil {
				eta = u.BuildCompleteAt.Local().Format("15:04 Jan 2")
			}
			return fmt.Sprintf("building (crew %d) — ready %s", u.Crew, eta)
		}
		// Land: still gathering men. At 100 it enters training (below); grow it by
		// recruiting more of the same type into the same settlement.
		return fmt.Sprintf("%d/100 (forming — recruit %d more %s here)",
			u.Size, 100-u.Size, unit.DisplayName(u.Type))
	case "training":
		// Land: full at 100, maturing to a deployable garrison at the ready ETA.
		eta := "unknown"
		if u.BuildCompleteAt != nil {
			eta = u.BuildCompleteAt.Local().Format("15:04 Jan 2")
		}
		return fmt.Sprintf("100/100 (training — ready %s)", eta)
	}
	if u.Category == "naval" {
		return fmt.Sprintf("1 vessel (crew %d)", u.Crew)
	}
	return fmt.Sprintf("%d men", u.Size)
}

func shipNameStr(name *string) string {
	if name == nil || *name == "" {
		return "—"
	}
	return *name
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
		// Explore order: exploring the target, then automatically turns for
		// home (explore_return) — no recall needed, spell that out so it isn't
		// mistaken for a stranded unit.
		if u.MarchIntent != nil && *u.MarchIntent == "explore" {
			loc = "exploring (auto-returns home) — "
		}
		if u.MarchIntent != nil && *u.MarchIntent == "explore_return" {
			loc = "returning home from explore — "
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
	var mode string
	var yes bool

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

Exploring: any march into fog or unknown territory reveals the route it
sweeps (dimmed on 'poleia map' thereafter) once the unit arrives — the
server does not FOW-gate the destination, only the route (A* over known
terrain). Run 'poleia map' first to see the frontier coordinates (fog
tiles bordering what you already know).

--intent explore sends the unit there AND automatically marches it back
home afterwards — no recall needed. The unit must currently be garrisoned
at a settlement (it needs a home to return to). Works for land or naval
units; its main use is sending a ship out to sweep fog and sail home on
its own.

Ore on mountain terrain (copper, tin, silver):
  Mountains are impassable — you cannot colonize the mountain hex itself.
  Instead, colonize an ADJACENT passable hex: the ore deposit will fall in
  the new colony's catchment and can be mined from there.
  Use 'poleia map' to see which adjacent hexes are passable.

Conquest choice (--mode, only matters when the target is an enemy settlement):
  sack (default) — loot goods (silver + a share of the rest, weighted by
    portability) and raze the settlement; the loot is carried home as a
    physical, interceptable caravan. annex — keep today's behaviour: take
    the settlement outright (a captured capital becomes an ordinary colony).`,
		Example: `  poleia unit march --unit <id> --q 5 --r -3
  poleia unit march --unit <id> --q 5 --r -3 --stance fortify
  poleia unit march --unit <id> --q 5 --r -3 --intent colonize --name Thapsos
  # Colonize the hex the unit already stands on (no coords needed):
  poleia unit march --unit <id> --intent colonize --name Thapsos
  # Any march reveals fog along its route toward a frontier coordinate:
  poleia unit march --unit <id> --q 12 --r -8
  # Explore: sails/marches to the target then automatically returns home
  poleia unit march --unit <id> --q 12 --r -8 --intent explore
  # Attack an enemy settlement and annex it instead of the sack default:
  poleia unit march --unit <id> --q 5 --r -3 --mode annex`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			qSet, rSet := cmd.Flags().Changed("q"), cmd.Flags().Changed("r")
			// Fas 2f: colonize the hex you already stand on. Omit --q/--r together
			// with --intent colonize and we resolve the unit's current field
			// position, so you never have to look the coordinates up.
			if intent == "colonize" && !qSet && !rSet {
				cq, cr, err := currentHex(c, cfg.WorldID, unitID)
				if err != nil {
					return err
				}
				targetQ, targetR = cq, cr
			} else if !qSet || !rSet {
				return fmt.Errorf("--q and --r are required (or use --intent colonize alone to found a colony on the hex your unit already occupies)")
			}

			// Colonize catchment forecast (DEL A, megaron_koloni_legibilitet_plan.md):
			// show the grain balance the new colony would start with BEFORE the march
			// is dispatched, then confirm. Skipped in --json mode (machine caller).
			// --yes or a non-interactive stdin (the agent harness) prints the forecast
			// but does not block on the y/N — same pattern as `poleia rite`.
			if intent == "colonize" && !jsonMode {
				preview, perr := fetchColonizePreview(c, cfg.WorldID, targetQ, targetR)
				if perr != nil {
					// Never block colonization on a forecast failure — warn and proceed.
					fmt.Printf("(kunde inte hämta catchment-prognos: %v)\n", perr)
				} else {
					renderColonizePreview(preview, targetQ, targetR)
					if !yes && stdinIsTerminal() {
						ok, aerr := askYesNo("Grunda kolonin?")
						if aerr != nil {
							return aerr
						}
						if !ok {
							fmt.Println("Avbröt — ingen koloni grundad.")
							return nil
						}
					}
				}
			}

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
			if mode != "" {
				body["mode"] = mode
			}
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
			verb := "marching to"
			if intent == "colonize" {
				verb = "colonizing"
			} else if intent == "explore" {
				verb = "exploring"
			}
			fmt.Printf("Unit %s %s (%d,%d)", unitID[:8], verb, targetQ, targetR)
			if arrivesAt != "" {
				if t, err := time.Parse(time.RFC3339, arrivesAt); err == nil {
					fmt.Printf(" — arrives %s", t.Local().Format("15:04 Jan 2"))
				}
			}
			if intent == "explore" {
				fmt.Print(" — it will sail/march home automatically once it arrives")
			}
			fmt.Println()
			return nil
		},
	}

	cmd.Flags().StringVar(&unitID, "unit", "", "unit UUID (required)")
	cmd.Flags().IntVar(&targetQ, "q", 0, "target hex Q (required, unless colonizing in place)")
	cmd.Flags().IntVar(&targetR, "r", 0, "target hex R (required, unless colonizing in place)")
	cmd.Flags().StringVar(&stance, "stance", "", "stance on arrival: fortify|storm|sentry")
	cmd.Flags().StringVar(&intent, "intent", "", "arrival intent: colonize (found a new colony — use --name to name it; omit --q/--r to colonize the hex the unit is on) | explore (auto-returns home after reaching the target; unit must be garrisoned at a settlement)")
	cmd.Flags().StringVar(&name, "name", "", "colony name (with --intent colonize)")
	cmd.Flags().StringVar(&mode, "mode", "", "conquest choice when attacking a settlement: sack (default, loot+raze) | annex (take the city)")
	cmd.Flags().BoolVar(&yes, "yes", false, "skip the colonize catchment-forecast confirmation (required for non-interactive/agent use)")
	_ = cmd.MarkFlagRequired("unit")
	return cmd
}

// colonizePreview mirrors the /colonize-preview endpoint's JSON (DEL A).
type colonizePreview struct {
	Catchment []struct {
		Q             int    `json:"q"`
		R             int    `json:"r"`
		Known         bool   `json:"known"`
		Terrain       string `json:"terrain"`
		CopperDeposit bool   `json:"copper_deposit"`
		TinDeposit    bool   `json:"tin_deposit"`
		SilverDeposit bool   `json:"silver_deposit"`
		CedarDeposit  bool   `json:"cedar_deposit"`
	} `json:"catchment"`
	Goods map[string]float64 `json:"goods"`
	Grain struct {
		BasePerTick     float64  `json:"base_per_tick"`
		EstNetPerTick   float64  `json:"est_net_per_tick"`
		Seed            float64  `json:"seed"`
		DaysUntilEmpty  *float64 `json:"days_until_empty"`
		WithFarmPerTick float64  `json:"with_farm_per_tick"`
	} `json:"grain"`
	UnknownHexes int `json:"unknown_hexes"`
}

// fetchColonizePreview GETs the grain/goods forecast for founding a colony at (q,r).
func fetchColonizePreview(c *Client, worldID string, q, r int) (*colonizePreview, error) {
	path := fmt.Sprintf("/api/v1/worlds/%s/colonize-preview?q=%d&r=%d", worldID, q, r)
	data, err := c.get(path)
	if err != nil {
		return nil, err
	}
	var p colonizePreview
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parse preview: %w", err)
	}
	return &p, nil
}

// renderColonizePreview prints the founding grain balance per game-day, so a
// Wanax sees whether a target hex can feed a colony before committing the march.
// All rates are per-tick from the server; ×TicksPerDay converts to per-day.
func renderColonizePreview(p *colonizePreview, q, r int) {
	td := float64(events.TicksPerDay)
	known := len(p.Catchment) - p.UnknownHexes

	fmt.Printf("Colonize (%d,%d) — catchment-prognos (%d/%d hexar kända, %d okända):\n",
		q, r, known, len(p.Catchment), p.UnknownHexes)

	prodPerDay := p.Grain.BasePerTick * td
	netPerDay := p.Grain.EstNetPerTick * td
	consPerDay := prodPerDay - netPerDay
	fmt.Printf("  Grain: produktion ~%.0f/dygn − konsumtion ~%.0f/dygn = NETTO %+.0f/dygn\n",
		prodPerDay, consPerDay, netPerDay)

	if netPerDay < 0 {
		reach := ""
		if p.Grain.DaysUntilEmpty != nil {
			reach = fmt.Sprintf(" → räcker ~%.0f speldygn", *p.Grain.DaysUntilEmpty)
		}
		farmNetPerDay := p.Grain.WithFarmPerTick*td - consPerDay
		farmNote := ""
		if p.Grain.WithFarmPerTick <= p.Grain.BasePerTick {
			farmNote = " (ingen jordbruksterräng i känd catchment — en farm hjälper inte här)"
		}
		fmt.Printf("  Startlager %.0f grain%s. Med farm: ~%+.0f/dygn%s\n",
			p.Grain.Seed, reach, farmNetPerDay, farmNote)
	} else {
		fmt.Printf("  Startlager %.0f grain — kolonin är självförsörjande.\n", p.Grain.Seed)
	}

	// "Övrigt": deposits present in the known catchment + any building-free
	// non-grain production. Sorted for stable output.
	var extras []string
	dep := map[string]bool{}
	for _, ce := range p.Catchment {
		if !ce.Known {
			continue
		}
		if ce.CopperDeposit {
			dep["copper"] = true
		}
		if ce.TinDeposit {
			dep["tin"] = true
		}
		if ce.SilverDeposit {
			dep["silver"] = true
		}
		if ce.CedarDeposit {
			dep["cedar"] = true
		}
	}
	for _, d := range []string{"copper", "tin", "silver", "cedar"} {
		if dep[d] {
			extras = append(extras, d+"-deposit ✓")
		}
	}
	goodKeys := make([]string, 0, len(p.Goods))
	for g := range p.Goods {
		goodKeys = append(goodKeys, g)
	}
	sort.Strings(goodKeys)
	for _, g := range goodKeys {
		if g == "grain" {
			continue
		}
		if rate := p.Goods[g] * td; rate > 0 {
			extras = append(extras, fmt.Sprintf("%s ~%.0f/dygn", g, rate))
		}
	}
	if len(extras) > 0 {
		fmt.Printf("  Övrigt: %s\n", strings.Join(extras, ", "))
	}

	fmt.Println("En koloni försörjer inte sig själv automatiskt — bygg farm om terrängen bär, annars ordna grain via intern transfer (poleia transfer --good grain --qty <n> --dest <koloni>).")
}

// currentHex resolves a field-positioned unit's current (q,r) so the
// colonize-in-place shortcut can found a colony where the unit already stands
// without the Wanax looking up coordinates.
func currentHex(c *Client, worldID, unitID string) (int, int, error) {
	data, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/units", worldID))
	if err != nil {
		return 0, 0, err
	}
	var resp struct {
		Units []unitRow `json:"units"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return 0, 0, fmt.Errorf("parse units: %w", err)
	}
	for _, u := range resp.Units {
		if u.ID != unitID {
			continue
		}
		if u.SettlementID != nil {
			return 0, 0, fmt.Errorf("unit is garrisoned in a settlement, not standing on an open hex — march it to the hex you want to colonize, or pass --q/--r")
		}
		if u.Q == nil || u.R == nil {
			return 0, 0, fmt.Errorf("unit has no map position yet; pass --q/--r")
		}
		return *u.Q, *u.R, nil
	}
	return 0, 0, fmt.Errorf("unit %s not found among your units", unitID)
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
