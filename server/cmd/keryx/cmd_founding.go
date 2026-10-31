package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

// foundingStatusResp mirrors GET /worlds/:id/founding/status — kept as a
// CLI-side type (wire JSON only), matching the convention in cmd_actions.go.
type foundingStatusResp struct {
	HostState       *foundingHostState `json:"host_state"`
	Active          bool               `json:"active"`
	HostUnitID      *string            `json:"host_unit_id"`
	Q               *int               `json:"q"`
	R               *int               `json:"r"`
	Population      int                `json:"population"`
	SpearmenInField int                `json:"spearmen_in_field"`
	CurrentTick     int                `json:"current_tick"`
	TickSeconds     float64            `json:"tick_seconds"`
	Grain           foundingStore      `json:"grain"`
	Silver          foundingStore      `json:"silver"`
}

type foundingHostState struct {
	Q            int    `json:"q"`
	R            int    `json:"r"`
	Status       string `json:"status"`
	CanFoundHere bool   `json:"can_found_here"`
	Reason       string `json:"reason"`
}

func foundingHostStatus(s *foundingHostState) string {
	if s == nil {
		return ""
	}
	label := map[string]string{"marching": "Marching", "waiting": "Waiting", "can_found_here": "Can found here"}[s.Status]
	if label == "" {
		return ""
	}
	if s.Reason != "" {
		label += " — " + s.Reason
	}
	return label
}

type foundingStore struct {
	Amount      float64 `json:"amount"`
	RatePerTick float64 `json:"rate_per_tick"`
	TicksLeft   *int    `json:"ticks_left"`
}

// neverJoined reports whether the player has neither a settlement nor an
// active founder phase in this world — i.e. has never run `keryx join`.
// GET .../founding/status answers {"active":false} for BOTH "never joined"
// and "already founded" (api/handlers/found_metropolis.go FoundingStatus:
// "No active phase — settled (or never a founder)") — the two look
// identical to the server but mean opposite things to the player, and
// telling someone who never joined that their metropolis "is already
// founded" sends them looking for a city that never existed
// (megaron_plan_tva_gate1_slices §1, 2026-09-04). A live province check is
// the only way to tell them apart — cfg.ProvinceID alone would be wrong for
// a stale local config.
func neverJoined(c *Client, worldID string) bool {
	return autoDetectProvince(c, worldID) == ""
}

// fetchFoundingStatus GETs the founder-phase read surface.
func fetchFoundingStatus(c *Client) (*foundingStatusResp, error) {
	data, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/founding/status", cfg.WorldID))
	if err != nil {
		return nil, err
	}
	var fp foundingStatusResp
	if err := json.Unmarshal(data, &fp); err != nil {
		return nil, fmt.Errorf("parse founding status: %w", err)
	}
	return &fp, nil
}

// foundingStoreLine renders one escort-store line: ticks left + a real-time
// ETA, BOTH derived from ticks_left at render time (B2: never a stored wall
// clock). Mirrors the web Host panel's hostStoreLine (render/map.js).
func foundingStoreLine(label string, s foundingStore, tickSeconds float64) string {
	if s.TicksLeft == nil {
		return fmt.Sprintf("%s: %.0f — lasts indefinitely", label, s.Amount)
	}
	ticksLeft := float64(*s.TicksLeft)
	return fmt.Sprintf("%s: %.0f left — %s", label, s.Amount, formatDays(ticksLeft, "%.0f"))
}

// printFoundingStatus renders the wandering host's status block — shared by
// `founding status` and the founder-fallback in `status`/`map` (a Wanax
// without a settlement must still see and act, feedback_keryx_surface).
func printFoundingStatus(fp *foundingStatusResp) error {
	pos := "unknown"
	if fp.Q != nil && fp.R != nil {
		pos = fmt.Sprintf("(%d,%d)", *fp.Q, *fp.R)
	}
	hostID := ""
	if fp.HostUnitID != nil {
		hostID = *fp.HostUnitID
	}
	fmt.Println("Nomadic Host — your people on the move")
	if line := foundingHostStatus(fp.HostState); line != "" {
		fmt.Printf("  %s · physical position %s\n", line, pos)
	}
	// Synradien är 2 sedan Timothy 2026-08-22 ("synradie för alla landenheter är
	// två"), med hostets två gamla undantag kvar: vid vatten 4, på berg 2+2
	// (province.LiveRadius, nomadic_host_vision_test.go). Raden sa "syn 1 hex"
	// ända till 2026-08-28 — en siffra som gjorde hostet blindare än det är, och
	// som just här kostar: 2 är exakt hexgrid.CatchmentRadius, alltså SER hostet
	// hela det upptagningsområde det skulle få om det grundade där det står.
	// Grundningsprognosen visar bara KÄNDA hexar, så den skillnaden är hela
	// skälet till att prognosens fyndighetsrad ("Övrigt: silver-deposit ✓") är
	// ifylld i stället för tom.
	fmt.Printf("  %d people · cannot fight · sight 2 hexes (4 by water or on mountains) · position %s\n", fp.Population, pos)
	fmt.Printf("  %s\n", foundingStoreLine("Grain (escort's ration)", fp.Grain, fp.TickSeconds))
	fmt.Printf("  %s\n", foundingStoreLine("Silver reserve", fp.Silver, fp.TickSeconds))
	kohort := "cohorts"
	if fp.SpearmenInField == 1 {
		kohort = "cohort"
	}
	fmt.Printf("  %d Spearmen-%s in the field · messengers free to send\n", fp.SpearmenInField, kohort)
	fmt.Println("\nNext steps:")
	fmt.Printf("  keryx unit march --unit %s --q <q> --r <r>   # travel\n", hostID)
	fmt.Println("  keryx founding settle                       # found the metropolis where the host stands")
	fmt.Println("  keryx message --from-host --to <city> --text \"...\"")
	return nil
}

// printFoundingForecast prints the catchment forecast for the hex the host
// STANDS on — `settle` founds here, nowhere else. Same endpoint + params as the
// web Host panel: the metropolis's population and the host's carried grain as
// stock. Shared by `founding status` and `founding settle`.
func printFoundingForecast(c *Client, fp *foundingStatusResp, q, r int) {
	seed := fp.Grain.Amount
	if seed < 0 {
		seed = 0
	}
	preview, perr := fetchColonizePreviewParams(c, cfg.WorldID, q, r, fp.Population, seed)
	if perr != nil {
		fmt.Printf("(could not fetch founding forecast: %v)\n", perr)
		return
	}
	renderCatchmentForecast(fmt.Sprintf("Founding (%d,%d) — metropolis of %d people", q, r, fp.Population), preview)
}

func foundingCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "founding",
		Short: "The Nomadic Host: your people before the metropolis (status, settle)",
	}
	cmd.AddCommand(foundingStatusCmd(), foundingSettleCmd())
	return cmd
}

// ---- founding status -----------------------------------------------------

func foundingStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show your wandering host: people, escort stores, position, and the founding forecast for its hex",
		Example: `  keryx founding status
  keryx founding status --json`,
		Args: noPositionalArgs(), // no flags at all — nothing to guess
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			if jsonMode {
				data, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/founding/status", cfg.WorldID))
				if err != nil {
					return err
				}
				printRawJSON(data)
				return nil
			}
			fp, err := fetchFoundingStatus(c)
			if err != nil {
				return err
			}
			if !fp.Active {
				if neverJoined(c, cfg.WorldID) {
					fmt.Println("No active founder phase — you haven't joined this world yet. Run: keryx join")
					return nil
				}
				fmt.Println("No active founder phase — the metropolis is already founded (see: keryx status).")
				return nil
			}
			if err := printFoundingStatus(fp); err != nil {
				return err
			}
			if fp.Q != nil && fp.R != nil {
				fmt.Println()
				printFoundingForecast(c, fp, *fp.Q, *fp.R)
			}
			return nil
		},
	}
}

// ---- founding settle -------------------------------------------------------

func foundingSettleCmd() *cobra.Command {
	var name, culture string
	var yes bool

	cmd := &cobra.Command{
		Use:   "settle",
		Short: "Found the metropolis on the hex the host stands on — irreversible",
		Long: `Turn the wandering host into your first and only city — a metropolis — on
the hex it currently occupies. The host dissolves permanently in the act; its
remaining grain and silver are carried into the city's stores. The founding
is owed ONE gift: a coastal city Poseidon's galley, an inland city with
grain land in its catchment Demeter's farm — never both.

The founding forecast (same surface as colonization) is shown before the
confirmation. 'keryx founding status' shows the same forecast for the hex the
host stands on, so look there first. Run without --yes in a non-interactive shell and
settle prints the forecast and stops without founding anything. In --json mode (machine caller) the
forecast is skipped, same convention as 'unit march --intent colonize' —
query GET /worlds/:id/colonize-preview yourself first if you need it before
settling. To found somewhere else: march the host there first
(keryx unit march), then settle.`,
		Example: `  keryx founding settle
  keryx founding settle --name Thapsos --yes`,
		Args: rejectPositionalArgs("name"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			fp, err := fetchFoundingStatus(c)
			if err != nil {
				return err
			}
			if !fp.Active {
				if neverJoined(c, cfg.WorldID) {
					return fmt.Errorf("you haven't joined this world yet — run: keryx join")
				}
				return fmt.Errorf("you have no wandering host — the metropolis is already founded (see: keryx status)")
			}
			if fp.Q == nil || fp.R == nil {
				return fmt.Errorf("the host has no position on the map — cannot found")
			}
			q, r := *fp.Q, *fp.R
			if fp.HostState != nil && !fp.HostState.CanFoundHere {
				return fmt.Errorf("cannot found here: %s", fp.HostState.Reason)
			}

			if jsonMode {
				// Machine caller: no interactive prompt possible, and the act is
				// irreversible — demand the explicit flag instead of proceeding.
				if !yes {
					return fmt.Errorf("--yes required in --json mode: the founding is irreversible")
				}
			} else {
				printFoundingForecast(c, fp, q, r)
				if !yes {
					if !stdinIsTerminal() {
						return errNeedYes("keryx founding settle", "the irreversible founding")
					}
					ok, aerr := askYesNo("Found the metropolis here? The host dissolves — forever.")
					if aerr != nil {
						return aerr
					}
					if !ok {
						fmt.Println("Aborted — the host travels on.")
						return nil
					}
				}
			}

			body := map[string]any{}
			if name != "" {
				body["name"] = name
			}
			if culture != "" {
				body["culture"] = culture
			}
			body["expected_q"], body["expected_r"] = q, r
			data, err := c.post(fmt.Sprintf("/api/v1/worlds/%s/founding/settle", cfg.WorldID), body)
			if err != nil {
				return err
			}

			// The world changed shape: the player now has a province. Re-resolve it
			// into the config so every province-scoped verb works immediately.
			if pid := autoDetectProvince(c, cfg.WorldID); pid != "" {
				cfg.ProvinceID = pid
				_ = saveConfig(cfg)
			}

			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var resp struct {
				SettlementID string `json:"settlement_id"`
				Tile         struct {
					Q int `json:"q"`
					R int `json:"r"`
				} `json:"tile"`
				Coastal       bool    `json:"coastal"`
				PoseidonGift  *string `json:"poseidon_gift"`
				GrainCarried  float64 `json:"grain_carried"`
				SilverCarried float64 `json:"silver_carried"`
				GrainTotal    float64 `json:"grain_total"`
				SilverTotal   float64 `json:"silver_total"`
			}
			_ = json.Unmarshal(data, &resp)
			fmt.Printf("⚒ Metropolis founded at (%d,%d)! The host is dissolved — the people have a home.\n",
				resp.Tile.Q, resp.Tile.R)
			fmt.Println(foundingStoresLine(resp.GrainCarried, resp.SilverCarried, resp.GrainTotal, resp.SilverTotal))
			if resp.PoseidonGift != nil {
				fmt.Println("  Poseidon's gift: a galley lies in the harbor.")
			}
			fmt.Println("  Run: keryx status")
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "metropolis name (default: culture-appropriate)")
	cmd.Flags().StringVar(&culture, "culture", "", "culture (MVP: minoan only)")
	cmd.Flags().BoolVar(&yes, "yes", false, "skip the interactive confirmation (required for non-interactive/agent use); the forecast is only printed in non-json mode")
	return cmd
}

// foundingStoresLine says what the host carried in AND what the city holds now
// Founding adds no extra grain or silver; the live receipt reports both.
func foundingStoresLine(grainCarried, silverCarried, grainTotal, silverTotal float64) string {
	return fmt.Sprintf("  Carried in by the host: %.0f grain, %.0f silver. The city now holds %.0f grain, %.0f silver (founding adds no extra grain or silver).",
		grainCarried, silverCarried, grainTotal, silverTotal)
}

// errNeedYes is the refusal for a non-interactive run: it names the command to re-run.
func errNeedYes(cmdline, what string) error {
	return fmt.Errorf("Nothing sent — add --yes to confirm %s (%s ... --yes)", what, cmdline)
}
