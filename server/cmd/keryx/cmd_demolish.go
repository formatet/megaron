package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

// demolishCmd tears down a finished building so its hex/slot is free for a new
// build at once. A hex building is addressed by --hex q,r (as `build` does), a
// city building by --type. Placed citizens are lifted off to idle; no refund.
func demolishCmd() *cobra.Command {
	var buildingType, hexFlag, provinceID string

	cmd := &cobra.Command{
		Use:   "demolish [city]",
		Short: "Tear down a finished building so the hex/slot is free to rebuild (no refund; its workers become idle)",
		Example: `  keryx demolish --hex 3,-1                 # the farm/mine/lumbermill/stonequarry standing on that hex
  keryx demolish --type market               # a city building
  keryx demolish --hex 5,2 --province <province-id>`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := newClient(cfg)
			if len(args) > 0 {
				if provinceID != "" {
					return fmt.Errorf("choose a city either positionally or with --province")
				}
				provinceID = args[0]
			}
			if hexFlag == "" && buildingType == "" {
				return fmt.Errorf("say what to demolish: --hex q,r for a hex building, or --type <building> for a city building")
			}
			prov := cfg.ProvinceID
			if provinceID != "" {
				resolved, err := resolveProvince(c, cfg.WorldID, provinceID)
				if err != nil {
					return err
				}
				prov = resolved
			}
			base := fmt.Sprintf("/api/v1/worlds/%s/provinces/%s/buildings", cfg.WorldID, prov)
			data, err := c.get(base)
			if err != nil {
				return err
			}
			var buildings []struct {
				ID   string `json:"id"`
				Type string `json:"type"`
				HexQ *int   `json:"hex_q"`
				HexR *int   `json:"hex_r"`
			}
			if err := json.Unmarshal(data, &buildings); err != nil {
				return err
			}
			var wantQ, wantR int
			if hexFlag != "" {
				if wantQ, wantR, err = parseHexFlag(hexFlag); err != nil {
					return err
				}
			}
			var id string
			for _, b := range buildings {
				if hexFlag != "" {
					if b.HexQ == nil || b.HexR == nil || *b.HexQ != wantQ || *b.HexR != wantR {
						continue
					}
				} else if b.HexQ != nil {
					continue
				}
				if buildingType != "" && b.Type != buildingType {
					continue
				}
				id = b.ID
				break
			}
			if id == "" {
				return fmt.Errorf("no finished building matches — `keryx city` shows what stands where (something still under construction is stopped with `keryx cancel-build`)")
			}
			out, err := deleteBuilding(c, cfg.WorldID, prov, id)
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(out)
				return nil
			}
			var resp struct {
				Type   string `json:"building_type"`
				Level  int    `json:"level"`
				HexQ   *int   `json:"hex_q"`
				HexR   *int   `json:"hex_r"`
				Lifted int    `json:"citizens_lifted"`
			}
			if err := json.Unmarshal(out, &resp); err != nil {
				return err
			}
			where := "in the city"
			if resp.HexQ != nil && resp.HexR != nil {
				where = fmt.Sprintf("on hex %d,%d", *resp.HexQ, *resp.HexR)
			}
			fmt.Printf("Demolished: %s (level %d) %s — the place is free to build on. %d citizen(s) lifted off their work and are idle now.\n",
				resp.Type, resp.Level, where, resp.Lifted)
			return nil
		},
	}
	cmd.Flags().StringVar(&buildingType, "type", "", "city building to demolish (market, temple, harbour, ...)")
	cmd.Flags().StringVar(&hexFlag, "hex", "", "hex building to demolish, as \"q,r\"")
	cmd.Flags().StringVar(&provinceID, "province", "", "city to demolish in (default: your capital)")
	return cmd
}

func deleteBuilding(c *Client, worldID, provinceID, buildingID string) ([]byte, error) {
	return c.delete(fmt.Sprintf("/api/v1/worlds/%s/provinces/%s/buildings/%s", worldID, provinceID, buildingID))
}
