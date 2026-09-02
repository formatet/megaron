package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

// foundingStatusResp mirrors GET /worlds/:id/founding/status — kept as a
// CLI-side type (wire JSON only), matching the convention in cmd_actions.go.
type foundingStatusResp struct {
	Active          bool          `json:"active"`
	HostUnitID      *string       `json:"host_unit_id"`
	Q               *int          `json:"q"`
	R               *int          `json:"r"`
	Population      int           `json:"population"`
	SpearmenInField int           `json:"spearmen_in_field"`
	CurrentTick     int           `json:"current_tick"`
	TickSeconds     float64       `json:"tick_seconds"`
	Grain           foundingStore `json:"grain"`
	Silver          foundingStore `json:"silver"`
}

type foundingStore struct {
	Amount      float64 `json:"amount"`
	RatePerTick float64 `json:"rate_per_tick"`
	TicksLeft   *int    `json:"ticks_left"`
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
	realH := float64(*s.TicksLeft) * tickSeconds / 3600
	real := fmt.Sprintf("≈ %.0f h", realH)
	if realH >= 48 {
		real = fmt.Sprintf("≈ %.0f days", realH/24)
	}
	return fmt.Sprintf("%s: %.0f left — %.0f tick (%s real time)", label, s.Amount, ticksLeft, real)
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
	fmt.Printf("  %s\n", foundingStoreLine("Silver (escort's pay)", fp.Silver, fp.TickSeconds))
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
		Short: "Show your wandering host: people, escort stores, position",
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
				fmt.Println("No active founder phase — the metropolis is already founded (see: keryx status).")
				return nil
			}
			return printFoundingStatus(fp)
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
remaining grain and silver are carried into the city's stores, and a coastal
founding is gifted Poseidon's galley.

The founding forecast (same surface as colonization) is shown before the
confirmation in interactive mode. In --json mode (machine caller) the
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
				return fmt.Errorf("you have no wandering host — the metropolis is already founded (see: keryx status)")
			}
			if fp.Q == nil || fp.R == nil {
				return fmt.Errorf("the host has no position on the map — cannot found")
			}
			q, r := *fp.Q, *fp.R

			if jsonMode {
				// Machine caller: no interactive prompt possible, and the act is
				// irreversible — demand the explicit flag instead of proceeding.
				if !yes {
					return fmt.Errorf("--yes required in --json mode: the founding is irreversible")
				}
			} else {
				// The forecast for the hex the host STANDS on — settle founds here,
				// nowhere else. Same endpoint + params as the web Host panel: the
				// metropolis's population and the host's carried grain as stock.
				seed := int(fp.Grain.Amount)
				if seed < 0 {
					seed = 0
				}
				preview, perr := fetchColonizePreviewParams(c, cfg.WorldID, q, r, fp.Population, seed)
				if perr != nil {
					fmt.Printf("(could not fetch founding forecast: %v)\n", perr)
				} else {
					renderCatchmentForecast(fmt.Sprintf("Founding (%d,%d) — metropolis of %d people", q, r, fp.Population), preview)
				}
				if !yes {
					if !stdinIsTerminal() {
						return fmt.Errorf("non-interactive run: add --yes to confirm the irreversible founding")
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
			}
			_ = json.Unmarshal(data, &resp)
			fmt.Printf("⚒ Metropolis founded at (%d,%d)! The host is dissolved — the people have a home.\n",
				resp.Tile.Q, resp.Tile.R)
			fmt.Printf("  Carried into the city: %.0f grain, %.0f silver\n", resp.GrainCarried, resp.SilverCarried)
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
