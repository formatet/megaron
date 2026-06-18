package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func godCmd() *cobra.Command {
	var adminKey string

	cmd := &cobra.Command{
		Use:   "god",
		Short: "God-mode view — full world map and all settlements without FOW (admin only)",
		Example: `  POLEIA_ADMIN_KEY=secret poleia god
  poleia god --key secret --json`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if adminKey == "" {
				adminKey = os.Getenv("POLEIA_ADMIN_KEY")
			}
			if adminKey == "" {
				return fmt.Errorf("admin key required: set POLEIA_ADMIN_KEY or use --key")
			}

			c := newClient(cfg)
			c.extraHeaders = map[string]string{"X-Admin-Key": adminKey}

			path := fmt.Sprintf("/api/v1/admin/worlds/%s/god-view", cfg.WorldID)
			data, err := c.get(path)
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}

			var resp struct {
				Settlements []struct {
					ID        string  `json:"id"`
					Name      string  `json:"name"`
					State     string  `json:"state"`
					Owner     string  `json:"owner"`
					Q         *int    `json:"q"`
					R         *int    `json:"r"`
					Population float64 `json:"population"`
					ArmyTotal int     `json:"army_total"`
					Kingdom   string  `json:"kingdom"`
					Kharis    float64 `json:"kharis"`
					Frenzied  bool    `json:"frenzied"`
				} `json:"settlements"`
				Tiles []struct{} `json:"tiles"`
			}
			if err := json.Unmarshal(data, &resp); err != nil {
				return fmt.Errorf("parse response: %w", err)
			}

			fmt.Printf("%-20s  %-14s  %-8s  %-6s  %-6s  %-18s  %s\n",
				"Settlement", "Owner", "Pop", "Army", "Kharis", "Kingdom", "Pos")
			fmt.Println(strings.Repeat("─", 100))
			for _, s := range resp.Settlements {
				pos := "—"
				if s.Q != nil && s.R != nil {
					pos = fmt.Sprintf("(%d,%d)", *s.Q, *s.R)
				}
				frenzy := ""
				if s.Frenzied {
					frenzy = " ⚡"
				}
				fmt.Printf("%-20s  %-14s  %-8.0f  %-6d  %-6.0f  %-18s  %s%s\n",
					s.Name, s.Owner, s.Population, s.ArmyTotal, s.Kharis, s.Kingdom, pos, frenzy)
			}
			fmt.Printf("\n%d settlements, %d tiles\n", len(resp.Settlements), len(resp.Tiles))
			return nil
		},
	}

	cmd.Flags().StringVar(&adminKey, "key", "", "admin key (overrides POLEIA_ADMIN_KEY env var)")
	return cmd
}
