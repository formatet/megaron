package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// richOfferingCmd turns a temple's rich offering on or off: a per-temple gift on top of
// the daily tithe that gives +50% effect while it is paid in full (Codex: Temples).
func richOfferingCmd() *cobra.Command {
	var god, provinceID, good string
	var off bool
	cmd := &cobra.Command{
		Use:   "rich-offering",
		Short: "Give a temple a rich offering on top of its tithe (+50% effect while paid in full)",
		Example: `  keryx rich-offering --god potnia --good fish
  keryx rich-offering --god potnia --off
  keryx rich-offering --god velchanos --good livestock --province <province-id>`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if god == "" {
				return fmt.Errorf("name the temple's god with --god")
			}
			if off == (good != "") {
				return fmt.Errorf("say either --good <good> or --off")
			}
			c := newClient(cfg)
			view, err := loadRealmCult(c)
			if err != nil {
				return err
			}
			if provinceID != "" {
				if provinceID, err = resolveProvince(c, cfg.WorldID, provinceID); err != nil {
					return err
				}
			}
			id := ""
			for _, t := range view.Temples {
				if t.GodID != god || (provinceID != "" && t.ProvinceID != provinceID) {
					continue
				}
				if id != "" {
					return fmt.Errorf("you have more than one temple of %s — name the city with --province", god)
				}
				id = t.ID
			}
			if id == "" {
				return fmt.Errorf("no temple of %s matches — `keryx status` lists them", god)
			}
			path := fmt.Sprintf("/api/v1/worlds/%s/temples/%s/rich-offering", cfg.WorldID, id)
			var raw []byte
			if off {
				raw, err = c.delete(path)
			} else {
				raw, err = c.put(path, map[string]string{"good": good})
			}
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(raw)
				return nil
			}
			if off {
				fmt.Println("Rich offering off. The temple gives its tithe only.")
			} else {
				fmt.Printf("Rich offering: %s each game day after the tithe. It is paid in full or not at all; the Temples list shows whether it was.\n", good)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&god, "god", "", "god of the temple (required)")
	cmd.Flags().StringVar(&provinceID, "province", "", "city, when you hold two temples of the same god")
	cmd.Flags().StringVar(&good, "good", "", "good to offer from the city's store")
	cmd.Flags().BoolVar(&off, "off", false, "turn the rich offering off")
	return cmd
}
