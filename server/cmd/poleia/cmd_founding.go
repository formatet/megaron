package main

import (
	"encoding/json"
	"fmt"

	"github.com/poleia/server/internal/events"
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

// foundingStoreLine renders one escort-store line: game-days left + a real-time
// ETA, BOTH derived from ticks_left at render time (B2: never a stored wall
// clock). Mirrors the web Host panel's hostStoreLine (render/map.js).
func foundingStoreLine(label string, s foundingStore, tickSeconds float64) string {
	if s.TicksLeft == nil {
		return fmt.Sprintf("%s: %.0f — räcker tills vidare", label, s.Amount)
	}
	gameDays := float64(*s.TicksLeft) / float64(events.TicksPerDay)
	realH := float64(*s.TicksLeft) * tickSeconds / 3600
	real := fmt.Sprintf("≈ %.0f h", realH)
	if realH >= 48 {
		real = fmt.Sprintf("≈ %.0f dygn", realH/24)
	}
	return fmt.Sprintf("%s: %.0f kvar — %.0f speldygn (%s verklig tid)", label, s.Amount, gameDays, real)
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
		Example: `  poleia founding status
  poleia founding status --json`,
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
				fmt.Println("Ingen aktiv founder-fas — huvudstaden är grundad (se: poleia status).")
				return nil
			}
			pos := "okänd"
			if fp.Q != nil && fp.R != nil {
				pos = fmt.Sprintf("(%d,%d)", *fp.Q, *fp.R)
			}
			hostID := ""
			if fp.HostUnitID != nil {
				hostID = *fp.HostUnitID
			}
			fmt.Println("Nomadic Host — ditt folk på vandring")
			fmt.Printf("  %d folk · kan inte strida · syn 1 hex · position %s\n", fp.Population, pos)
			fmt.Printf("  %s\n", foundingStoreLine("Grain (eskortens ranson)", fp.Grain, fp.TickSeconds))
			fmt.Printf("  %s\n", foundingStoreLine("Silver (eskortens sold)", fp.Silver, fp.TickSeconds))
			kohort := "kohorter"
			if fp.SpearmenInField == 1 {
				kohort = "kohort"
			}
			fmt.Printf("  %d Spearmen-%s i fält · budbärare fria att sända\n", fp.SpearmenInField, kohort)
			fmt.Println("\nNästa steg:")
			fmt.Printf("  poleia unit march --unit %s --q <q> --r <r>   # vandra\n", hostID)
			fmt.Println("  poleia founding settle                       # grunda huvudstaden där hostet står")
			fmt.Println("  poleia message --from-host --to <stad> --text \"...\"")
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
remaining grain and silver are carried into the city's stores, and a coastal
founding is gifted Poseidon's galley.

The founding forecast (same surface as colonization) is ALWAYS shown before
the confirmation. To found somewhere else: march the host there first
(poleia unit march), then settle.`,
		Example: `  poleia founding settle
  poleia founding settle --name Thapsos --yes`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			fp, err := fetchFoundingStatus(c)
			if err != nil {
				return err
			}
			if !fp.Active {
				return fmt.Errorf("du har ingen vandrande host — huvudstaden är redan grundad (se: poleia status)")
			}
			if fp.Q == nil || fp.R == nil {
				return fmt.Errorf("hostet har ingen position på kartan — kan inte grunda")
			}
			q, r := *fp.Q, *fp.R

			if jsonMode {
				// Machine caller: no interactive prompt possible, and the act is
				// irreversible — demand the explicit flag instead of proceeding.
				if !yes {
					return fmt.Errorf("--yes krävs i --json-läge: grundningen är oåterkallelig")
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
					fmt.Printf("(kunde inte hämta grundningsprognos: %v)\n", perr)
				} else {
					renderCatchmentForecast(fmt.Sprintf("Grundning (%d,%d) — metropolis om %d folk", q, r, fp.Population), preview)
				}
				if !yes {
					if !stdinIsTerminal() {
						return fmt.Errorf("icke-interaktiv körning: lägg till --yes för att bekräfta den oåterkalleliga grundningen")
					}
					ok, aerr := askYesNo("Grunda huvudstaden här? Hostet upplöses — för alltid.")
					if aerr != nil {
						return aerr
					}
					if !ok {
						fmt.Println("Avbröt — hostet vandrar vidare.")
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
			fmt.Printf("⚒ Metropolis grundad på (%d,%d)! Hostet är upplöst — folket har ett hem.\n",
				resp.Tile.Q, resp.Tile.R)
			fmt.Printf("  Buret in i staden: %.0f grain, %.0f silver\n", resp.GrainCarried, resp.SilverCarried)
			if resp.PoseidonGift != nil {
				fmt.Println("  Poseidons gåva: en galär ligger i hamnen.")
			}
			fmt.Println("  Kör: poleia status")
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "metropolis name (default: culture-appropriate)")
	cmd.Flags().StringVar(&culture, "culture", "", "culture (default: akhaier)")
	cmd.Flags().BoolVar(&yes, "yes", false, "skip the interactive confirmation (required for non-interactive/agent use); the forecast is still printed")
	return cmd
}
