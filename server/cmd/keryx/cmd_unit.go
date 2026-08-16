package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"formatet/megaron/server/internal/unit"
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
		unitSentryCmd(),
		unitRecallCmd(),
		unitRedirectCmd(),
		unitStanceCmd(),
		unitStandingOrdersCmd(),
		unitLoadCmd(),
		unitUnloadCmd(),
		unitRepairCmd(),
	)
	return cmd
}

// ---- unit sentry -------------------------------------------------------------

// unitSentryCmd posts a naval unit on sentry patrol at a coastal_sea hex. It is a
// thin convenience over `unit march --intent sentry`: the ship sails to the hex,
// holds there watching the approaches (fog-of-war + caravan interception) and
// turns for home on its own when the patrol timer runs out. No recall — the timer
// is the only control (self-terminating sea order).
//
// Deliberately coastal_sea only — river is water too, but not a sea patrol's
// hex: a patrol standing in a 1-hex-wide river has no water to project over
// (megaron_floden_plan.md §3, Timothy 2026-07-29; server rejects it in
// combat/march_start.go's sentry gate).
func unitSentryCmd() *cobra.Command {
	var unitID string
	var q, r int
	cmd := &cobra.Command{
		Use:   "sentry",
		Short: "Post a ship on sentry patrol at a coastal-sea hex (auto-returns after its patrol)",
		Long: `Send a naval unit to a shallow-water (coastal_sea) hex you have seen and hold it
there on sentry: it watches the approaches (fog-of-war) and intercepts enemy
caravans passing within reach. There is no recall — the ship turns for home on
its own when the patrol timer runs out.`,
		Example: "  keryx unit sentry --unit <id> --q 8 --r -3",
		Args:    rejectPositionalArgs("unit"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/units/%s/march", cfg.WorldID, unitID)
			data, err := c.post(path, map[string]any{"q": q, "r": r, "intent": "sentry"})
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			fmt.Printf("Sentry order dispatched: ship %s → (%d,%d). It will patrol, then sail home on its own.\n", unitID, q, r)
			return nil
		},
	}
	cmd.Flags().StringVar(&unitID, "unit", "", "unit id (required)")
	cmd.Flags().IntVar(&q, "q", 0, "target hex Q — axial coordinate, read it off 'keryx map' (required)")
	cmd.Flags().IntVar(&r, "r", 0, "target hex R — axial coordinate, read it off 'keryx map' (required)")
	_ = cmd.MarkFlagRequired("unit")
	_ = cmd.MarkFlagRequired("q")
	_ = cmd.MarkFlagRequired("r")
	return cmd
}

// ---- unit list ---------------------------------------------------------------

func unitListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List your units",
		Example: `  keryx unit list
  keryx unit list --json`,
		Args: noPositionalArgs(), // no flags at all — nothing to guess
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
			// Namnkolumnen är namnstandardens display_name från servern
			// ("2nd Spearmen of Knossos", "White Dolphin, Galley of
			// Kydonia"). Keryx formaterar den INTE själv: allt i temenos ska vara
			// synligt och actionabelt i keryx, men grammatiken bor på servern.
			// Äldre servrar utan fältet faller tillbaka på typ + skeppsnamn.
			fmt.Printf("%-36s  %-46s  %-8s  %-10s  %-9s  %s\n",
				"ID", "Name", "Size", "Status", "Stance", "Location / ETA")
			fmt.Println(strings.Repeat("─", 140))
			for _, u := range resp.Units {
				name := u.DisplayName
				if name == "" {
					name = unit.DisplayName(u.Type) + shipNameSuffix(u.Name)
				}
				fmt.Printf("%-36s  %-46s  %-8s  %-10s  %-9s  %s\n",
					u.ID, name, formatSize(u), u.Status, stanceStr(u.Stance), locationStr(u))
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
	Hull            int        `json:"hull"`
	Status          string     `json:"status"`
	Name            *string    `json:"name"`
	DisplayName     string     `json:"display_name"`
	BuildCompleteAt *time.Time `json:"build_complete_at"`
	Stance          *string    `json:"stance"`
	SettlementID    *string    `json:"settlement_id"`
	Q               *int       `json:"q"`
	R               *int       `json:"r"`
	TargetQ         *int       `json:"target_q"`
	TargetR         *int       `json:"target_r"`
	ArrivesAt       *time.Time `json:"arrives_at"`
	CargoUnitID     *string    `json:"cargo_unit_id"`
	CarrierShipID   *string    `json:"carrier_ship_id"`
	CarrierShipName *string    `json:"carrier_ship_name"`
	MarchIntent     *string    `json:"march_intent"`
	ColonyName      *string    `json:"colony_name"`
}

func formatSize(u unitRow) string {
	// The host is a size-1 map token; the people it represents live in
	// founder_phase.population, never in units.size — "1 men" would be a lie
	// (4.5 displayfällan). The real numbers live in `keryx founding status`.
	if u.Type == string(unit.TypeNomadicHost) {
		return "a people on the move"
	}
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
		hull := ""
		if u.Hull < 5 {
			hull = fmt.Sprintf(", hull %d/5 damaged", u.Hull)
		}
		return fmt.Sprintf("1 vessel (crew %d%s)", u.Crew, hull)
	}
	return fmt.Sprintf("%d men", u.Size)
}

// Fallback när servern är äldre än namnstandarden och inte skickar display_name.
func shipNameSuffix(name *string) string {
	if name == nil || *name == "" {
		return ""
	}
	return " \"" + *name + "\""
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
		// An embarked land unit is cargo aboard a ship — name the carrier (the ship
		// whose cargo_unit_id points back at this unit), so a unit stranded at sea
		// (assault target vanished) is legible instead of an orphaned "embarked".
		if u.CarrierShipName != nil && *u.CarrierShipName != "" {
			return "embarked on " + *u.CarrierShipName
		}
		if u.CarrierShipID != nil {
			return "embarked on ship " + (*u.CarrierShipID)[:8] + "…"
		}
		return "embarked"
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
  Land units only:         plains, hills, forest_olive_grove, forest_cedar,
                           scrub_maquis, semi_desert, river_valley, river_delta
  Naval units only:        coastal_sea, deep_sea, river
  Both land AND naval:     river_ford (the river's port — steep move cost for
                           either, but the one hex where a land unit can
                           cross and a ship can still sail through)
  (Land units cannot enter sea or river; naval units cannot enter land.
  river is impassable to land units — it is a wall, not a crossing. Every
  ~10 hexes of a river's length carries one river_ford instead.)

A land unit must reach 100 men (garrison status) before it can march.
A unit in fortify stance must be cleared (stance none) before marching.

Exploring: any march into fog or unknown territory reveals the route it
sweeps (dimmed on 'keryx map' thereafter) once the unit arrives — the
server does not FOW-gate the destination, only the route (A* over known
terrain). Run 'keryx map' first to see the frontier coordinates (fog
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
  Use 'keryx map' to see which adjacent hexes are passable.

Conquest choice (--mode, only matters when the target is an enemy settlement):
  sack (default) — loot goods (silver + a share of the rest, weighted by
    portability) and raze the settlement; the loot is carried home as a
    physical, interceptable caravan. annex — keep today's behaviour: take
    the settlement outright (a captured capital becomes an ordinary colony).`,
		Example: `  keryx unit march --unit <id> --q 5 --r -3
  keryx unit march --unit <id> --q 5 --r -3 --stance fortify
  keryx unit march --unit <id> --q 5 --r -3 --intent colonize --name Thapsos
  # Colonize the hex the unit already stands on (no coords needed):
  keryx unit march --unit <id> --intent colonize --name Thapsos
  # Any march reveals fog along its route toward a frontier coordinate:
  keryx unit march --unit <id> --q 12 --r -8
  # Explore: sails/marches to the target then automatically returns home
  keryx unit march --unit <id> --q 12 --r -8 --intent explore
  # Attack an enemy settlement and annex it instead of the sack default:
  keryx unit march --unit <id> --q 5 --r -3 --mode annex`,
		Args: rejectPositionalArgs("unit"),
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
			// but does not block on the y/N — same pattern as `keryx rite`.
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
			// Field unit: the order travels by runner and executes on
			// delivery (temenos_orderlopare_plan.md Fas 5) — the 202 is a
			// dispatch receipt with the COURIER's ETA, not a march start.
			if status, _ := resp["status"].(string); status == "order_dispatched" {
				fmt.Printf("A Runner carries your march order to unit %s — target (%d,%d)", unitID[:8], targetQ, targetR)
				if courierAt, _ := resp["courier_arrives_at"].(string); courierAt != "" {
					if t, err := time.Parse(time.RFC3339, courierAt); err == nil {
						fmt.Printf("; the runner reaches it %s", t.Local().Format("15:04 Jan 2"))
					}
				}
				fmt.Println(" — the march begins on delivery.")
				return nil
			}
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
			// The colonist purse (mig 107): a founding no longer mints the colony's
			// silver, the column carries it from home. Printed at dispatch because
			// this is the last moment the Wanax can recall the expedition and fund
			// it properly — after it lands, the colony is simply poor.
			if intent == "colonize" {
				carried, _ := resp["carried_silver"].(float64)
				short, _ := resp["purse_shortfall"].(float64)
				switch {
				case short > 0 && carried > 0:
					fmt.Printf("  Bär med sig %.0f silver hemifrån — %.0f mindre än kolonin behöver. Den grundas fattig.\n", carried, short)
				case short > 0:
					fmt.Printf("  ⚠ Bär INGET silver — moderstaden hade inget att skicka med. Kolonin grundas på 0 och kan inte betala sold.\n")
				case carried > 0:
					fmt.Printf("  Bär med sig %.0f silver hemifrån (dras ur moderstadens kassa nu, inte myntat vid grundningen).\n", carried)
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&unitID, "unit", "", "unit UUID (required)")
	cmd.Flags().IntVar(&targetQ, "q", 0, "target hex Q — axial coordinate, read it off 'keryx map' (required, unless colonizing in place)")
	cmd.Flags().IntVar(&targetR, "r", 0, "target hex R — axial coordinate, read it off 'keryx map' (required, unless colonizing in place)")
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
		TicksUntilEmpty *float64 `json:"ticks_until_empty"`
		WithFarmPerTick float64  `json:"with_farm_per_tick"`
	} `json:"grain"`
	UnknownHexes    int    `json:"unknown_hexes"`
	IsolatedWarning string `json:"isolated_warning,omitempty"`
	// FoundingGifts is only sent for a metropolis founding (?starter_farm=1) —
	// the gifts this site earns (Demeter's farm, Poseidon's galley). Empty for
	// colonies, which are owed nothing.
	FoundingGifts []struct {
		Key    string `json:"key"`
		Label  string `json:"label"`
		Detail string `json:"detail"`
	} `json:"founding_gifts,omitempty"`
	// CatchmentConflict is set when this site's 7-hex catchment overlaps an
	// existing settlement's — the delad-catchment-grind invariant (Timothy
	// 2026-07-27/28). The march/settle call will reject this site with the
	// same message; showing it here lets the Wanax pick another site before
	// walking there.
	CatchmentConflict *struct {
		Blocked        bool   `json:"blocked"`
		MinMoveHexes   int    `json:"min_move_hexes"`
		Message        string `json:"message"`
		SettlementName string `json:"settlement_name,omitempty"`
	} `json:"catchment_conflict,omitempty"`
}

// fetchColonizePreview GETs the grain/goods forecast for founding a colony at (q,r).
func fetchColonizePreview(c *Client, worldID string, q, r int) (*colonizePreview, error) {
	return fetchPreviewPath(c, fmt.Sprintf("/api/v1/worlds/%s/colonize-preview?q=%d&r=%d", worldID, q, r))
}

// fetchColonizePreviewParams is the founder-phase variant: ?pop=&seed= makes the
// SAME endpoint forecast the metropolis (its population, the host's carried
// grain as stock) instead of a colony — temenos_nomadic_host_fas4_plan.md 4.3.
// starter_farm=1 tells the endpoint a metropolis founding seeds a starter farm
// (createMetropolis), unlike a plain colony — see founding-forecast-fix-plan.
func fetchColonizePreviewParams(c *Client, worldID string, q, r, pop, seed int) (*colonizePreview, error) {
	return fetchPreviewPath(c, fmt.Sprintf("/api/v1/worlds/%s/colonize-preview?q=%d&r=%d&pop=%d&seed=%d&starter_farm=1",
		worldID, q, r, pop, seed))
}

func fetchPreviewPath(c *Client, path string) (*colonizePreview, error) {
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

// renderColonizePreview prints the founding grain balance per tick, so a
// Wanax sees whether a target hex can feed a colony before committing the march.
func renderColonizePreview(p *colonizePreview, q, r int) {
	renderCatchmentForecast(fmt.Sprintf("Colonize (%d,%d)", q, r), p)
	// The march order is one-way for the unit itself: foundColony folds the
	// colonists into the new city's populace and disbands the unit. Playtesters
	// twice ordered a colonize expecting the cohort back, then hit "that unit no
	// longer exists" on the next order — so say it here, where the decision is
	// still open. Printed before the confirmation prompt AND on the --yes path,
	// so an agent that skips the prompt still reads it.
	fmt.Println("OBS: enheten förbrukas — kolonisterna blir kolonins befolkning och enheten upplöses. Den kan inte beordras igen.")
	fmt.Println("En koloni försörjer inte sig själv automatiskt — bygg farm om terrängen bär, annars ordna grain via intern transfer (keryx transfer --good grain --qty <n> --dest <koloni>).")
}

// renderCatchmentForecast is the shared forecast body — colonization and the
// founder-phase settle print the same numbers under different headers.
// All rates are per-tick from the server, so these print directly with no
// conversion.
func renderCatchmentForecast(title string, p *colonizePreview) {
	known := len(p.Catchment) - p.UnknownHexes

	fmt.Printf("%s — catchment-prognos (%d/%d hexar kända, %d okända):\n",
		title, known, len(p.Catchment), p.UnknownHexes)

	// Catchment overlap is checked BEFORE the grain math below (AK3: the
	// blockage must be visible before the Wanax walks/settles there) — the
	// march/settle call will refuse this exact site with the same message.
	if p.CatchmentConflict != nil && p.CatchmentConflict.Blocked {
		fmt.Printf("  ⛔ BLOCKERAD: %s\n", p.CatchmentConflict.Message)
	}

	prodPerTick := p.Grain.BasePerTick
	netPerTick := p.Grain.EstNetPerTick
	consPerTick := prodPerTick - netPerTick
	fmt.Printf("  Grain: produktion ~%.0f/tick − konsumtion ~%.0f/tick = NETTO %+.0f/tick\n",
		prodPerTick, consPerTick, netPerTick)

	if netPerTick < 0 {
		reach := ""
		if p.Grain.TicksUntilEmpty != nil {
			reach = fmt.Sprintf(" → räcker ~%.0f tick", *p.Grain.TicksUntilEmpty)
		}
		farmNetPerTick := p.Grain.WithFarmPerTick - consPerTick
		farmNote := ""
		if p.Grain.WithFarmPerTick <= p.Grain.BasePerTick {
			farmNote = " (ingen jordbruksterräng i känd catchment — en farm hjälper inte här)"
		}
		fmt.Printf("  Startlager %.0f grain%s. Med farm: ~%+.0f/tick%s\n",
			p.Grain.Seed, reach, farmNetPerTick, farmNote)
	} else {
		fmt.Printf("  Startlager %.0f grain — staden är självförsörjande.\n", p.Grain.Seed)
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
		if rate := p.Goods[g]; rate > 0 {
			extras = append(extras, fmt.Sprintf("%s ~%.0f/tick", g, rate))
		}
	}
	if len(extras) > 0 {
		fmt.Printf("  Övrigt: %s\n", strings.Join(extras, ", "))
	}

	// Gudagåvorna hör till platsen, inte till grundandet: de faller ut ur samma
	// geografi som siffrorna ovan, så de hör hemma i prognosen — inte som en
	// överraskning efter det oåterkalleliga settlet.
	for _, gift := range p.FoundingGifts {
		fmt.Printf("  Gåva: %s — %s\n", gift.Label, gift.Detail)
	}

	// P8 (soak 2026-07-18): make an isolated founding site visible BEFORE the
	// Wanax commits — server-computed conservative heuristic (no hills, no
	// ore, no neighbour within reach; see isolationWarningText in
	// api/handlers/world.go). Heads-up, not a gate — founding still proceeds.
	if p.IsolatedWarning != "" {
		fmt.Printf("  ⚠ %s\n", p.IsolatedWarning)
	}
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

func unitRecallCmd() *cobra.Command {
	var unitID string

	cmd := &cobra.Command{
		Use:   "recall",
		Short: "Recall a marching unit — turn it home",
		Long: `Send a recall order to a marching unit. The order travels as a visible
Runner; command is never instant — the unit keeps marching on its original
course until the runner physically catches up with it, then turns for home
(the hex it originally departed from).`,
		Example: `  keryx unit recall --unit <id>`,
		Args:    rejectPositionalArgs("unit"),
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
			var resp map[string]any
			json.Unmarshal(data, &resp)
			fmt.Printf("Recall order sent to unit %s", unitID[:8])
			if courierAt, _ := resp["courier_arrives_at"].(string); courierAt != "" {
				if t, err := time.Parse(time.RFC3339, courierAt); err == nil {
					fmt.Printf(" — Runner arrives %s", t.Local().Format("15:04 Jan 2"))
				}
			}
			fmt.Println("; the unit turns home once it catches up.")
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
the order's Runner physically catches up with it, then turns onto the new course.`,
		Example: `  keryx unit redirect --unit <id> --target 5,-3`,
		Args:    rejectPositionalArgs("unit"),
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
			var resp map[string]any
			json.Unmarshal(data, &resp)
			fmt.Printf("Redirect order sent to unit %s (new course %d,%d)", unitID[:8], q, r)
			if courierAt, _ := resp["courier_arrives_at"].(string); courierAt != "" {
				if t, err := time.Parse(time.RFC3339, courierAt); err == nil {
					fmt.Printf(" — Runner arrives %s", t.Local().Format("15:04 Jan 2"))
				}
			}
			fmt.Println(".")
			return nil
		},
	}

	cmd.Flags().StringVar(&unitID, "unit", "", "unit UUID (required)")
	cmd.Flags().StringVar(&target, "target", "", "new target hex as q,r — axial coordinates, read them off 'keryx map' (required)")
	_ = cmd.MarkFlagRequired("unit")
	_ = cmd.MarkFlagRequired("target")
	return cmd
}

// unitRepairCmd starts a hull repair job (megaron_plan_skeppsreparation.md
// Slice C) on a damaged ship standing in garrison at an own shipyard city.
func unitRepairCmd() *cobra.Command {
	var unitID string

	cmd := &cobra.Command{
		Use:   "repair",
		Short: "Repair a damaged ship's hull at a shipyard",
		Long: `Start a hull repair job on a naval unit whose hull is below full strength.
The ship must be in garrison at one of your own settlements with a shipyard.
Timber (cedar for war galleys) is deducted up front, in proportion to the
hull points being restored; the ship is unavailable to march until the
repair completes and its hull returns to full.`,
		Example: `  keryx unit repair --unit <id>`,
		Args:    rejectPositionalArgs("unit"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/units/%s/repair", cfg.WorldID, unitID)
			data, err := c.post(path, map[string]any{})
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var resp struct {
				HullBefore    int     `json:"hull_before"`
				HullTarget    int     `json:"hull_target"`
				Good          string  `json:"good"`
				Amount        float64 `json:"amount"`
				DurationTicks int     `json:"duration_ticks"`
				CompleteAt    string  `json:"complete_at"`
			}
			_ = json.Unmarshal(data, &resp)
			fmt.Printf("Repair started on unit %s (hull %d→%d), costing %.1f %s over %d ticks",
				unitID[:8], resp.HullBefore, resp.HullTarget, resp.Amount, resp.Good, resp.DurationTicks)
			if t, err := time.Parse(time.RFC3339, resp.CompleteAt); err == nil {
				fmt.Printf(" — ready %s", t.Local().Format("15:04 Jan 2"))
			}
			fmt.Println(".")
			return nil
		},
	}

	cmd.Flags().StringVar(&unitID, "unit", "", "unit UUID (required)")
	_ = cmd.MarkFlagRequired("unit")
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
	var unitID, stance, reaction string

	cmd := &cobra.Command{
		Use:   "stance",
		Short: "Set or clear a unit's stance",
		Example: `  keryx unit stance --unit <id> --stance fortify
  keryx unit stance --unit <id> --stance sentry --reaction ignore
  keryx unit stance --unit <id> --stance none`,
		Args: rejectPositionalArgs("unit"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/units/%s/stance", cfg.WorldID, unitID)
			body := map[string]any{"stance": stance}
			if reaction != "" {
				body["reaction_foreign"] = reaction
			}
			data, err := c.post(path, body)
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var stanceResp map[string]any
			json.Unmarshal(data, &stanceResp)
			if status, _ := stanceResp["status"].(string); status == "order_dispatched" {
				fmt.Printf("A Runner carries your stance order (%s) to unit %s", stance, unitID[:8])
				if courierAt, _ := stanceResp["courier_arrives_at"].(string); courierAt != "" {
					if t, err := time.Parse(time.RFC3339, courierAt); err == nil {
						fmt.Printf("; the runner reaches it %s", t.Local().Format("15:04 Jan 2"))
					}
				}
				fmt.Println(" — it applies on delivery.")
				return nil
			}
			if stance == "none" {
				fmt.Printf("Unit %s stance cleared\n", unitID[:8])
			} else if reactionApplied, _ := stanceResp["reaction_foreign"].(string); stance == "sentry" && reactionApplied != "" {
				fmt.Printf("Unit %s stance → %s (reacts to foreign units: %s)\n", unitID[:8], stance, reactionApplied)
			} else {
				fmt.Printf("Unit %s stance → %s\n", unitID[:8], stance)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&unitID, "unit", "", "unit UUID (required)")
	cmd.Flags().StringVar(&stance, "stance", "", "stance: fortify|storm|sentry|none (required)")
	cmd.Flags().StringVar(&reaction, "reaction", "",
		"reaction to foreign units when --stance sentry: intercept|escort|ignore|alert (default intercept; all four wired for unit-vs-unit, escort/alert still stubs against caravans)")
	_ = cmd.MarkFlagRequired("unit")
	_ = cmd.MarkFlagRequired("stance")
	return cmd
}

// ---- unit retreat-order -------------------------------------------------------

// unitRetreatOrderCmd sends KR3 §5's mid-battle retreat order: change the
// rout threshold (or hold-to-last-man) of a unit that is CURRENTLY fighting
// in an active battle. There is no pre-battle preset — the unit must already
// be a battle participant, same as the server-side scope (megaron_todo.md
// KR3 loose end (c)).
func unitStandingOrdersCmd() *cobra.Command {
	var unitID string
	var retreatAtLoss float64
	var holdToLastMan bool

	cmd := &cobra.Command{
		Use:   "retreat-order",
		Short: "Change a unit's mid-battle retreat threshold",
		Example: `  keryx unit retreat-order --unit <id> --retreat-at-loss 0.5
  keryx unit retreat-order --unit <id> --hold-to-last-man`,
		Args: rejectPositionalArgs("unit"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !cmd.Flags().Changed("retreat-at-loss") && !cmd.Flags().Changed("hold-to-last-man") {
				return fmt.Errorf("set at least one of --retreat-at-loss or --hold-to-last-man")
			}
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/units/%s/standing-orders", cfg.WorldID, unitID)
			body := map[string]any{}
			if cmd.Flags().Changed("retreat-at-loss") {
				body["retreat_at_loss"] = retreatAtLoss
			}
			if cmd.Flags().Changed("hold-to-last-man") {
				body["hold_to_last_man"] = holdToLastMan
			}
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
			if status, _ := resp["status"].(string); status == "order_dispatched" {
				fmt.Printf("A Runner carries your retreat order to unit %s", unitID[:8])
				if courierAt, _ := resp["courier_arrives_at"].(string); courierAt != "" {
					if t, err := time.Parse(time.RFC3339, courierAt); err == nil {
						fmt.Printf("; the runner reaches it %s", t.Local().Format("15:04 Jan 2"))
					}
				}
				fmt.Println(" — it applies on delivery.")
				return nil
			}
			fmt.Printf("Unit %s standing orders updated (retreat_at_loss=%v, hold_to_last_man=%v)\n",
				unitID[:8], resp["retreat_at_loss"], resp["hold_to_last_man"])
			return nil
		},
	}

	cmd.Flags().StringVar(&unitID, "unit", "", "unit UUID (required)")
	cmd.Flags().Float64Var(&retreatAtLoss, "retreat-at-loss", 0,
		"fraction of starting strength (0-1) at which this unit's side breaks and retreats")
	cmd.Flags().BoolVar(&holdToLastMan, "hold-to-last-man", false, "disable rout for this unit's side — fight to annihilation")
	_ = cmd.MarkFlagRequired("unit")
	return cmd
}

// ---- unit load ---------------------------------------------------------------

func unitLoadCmd() *cobra.Command {
	var shipID, landUnitID string

	cmd := &cobra.Command{
		Use:     "load",
		Short:   "Embark a land unit onto a ship",
		Example: `  keryx unit load --ship <ship-id> --unit <land-unit-id>`,
		// --ship and --unit are both required and equally plausible for a
		// stray positional — no single one is the obvious guess.
		Args: noPositionalArgs(),
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
		Example: `  keryx unit unload --ship <ship-id>`,
		Args:    rejectPositionalArgs("ship"),
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
