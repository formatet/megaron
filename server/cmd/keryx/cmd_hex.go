package main

import (
	"encoding/json"
	"fmt"
	"github.com/spf13/cobra"
	"strconv"
	"strings"
)

// hex reads existing projections only; it grants no new sight or foreign fields.
func hexCmd() *cobra.Command {
	return &cobra.Command{Use: "hex <q,r>", Short: "Inspect known terrain and forces at their current hex", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		parts := strings.Split(args[0], ",")
		if len(parts) != 2 {
			return fmt.Errorf("use hex <q,r>")
		}
		q, err := strconv.Atoi(parts[0])
		if err != nil {
			return fmt.Errorf("invalid q")
		}
		r, err := strconv.Atoi(parts[1])
		if err != nil {
			return fmt.Errorf("invalid r")
		}
		c := newClient(cfg)
		prefix := "/api/v1/worlds/" + cfg.WorldID
		var tiles []map[string]any
		data, err := c.get(prefix + "/map")
		if err != nil {
			return err
		}
		if err = json.Unmarshal(data, &tiles); err != nil {
			return err
		}
		var tile map[string]any
		for _, t := range tiles {
			if t["q"] == float64(q) && t["r"] == float64(r) {
				tile = t
				break
			}
		}
		if tile == nil {
			return fmt.Errorf("hex outside the map")
		}
		own := []unitRow{}
		foreign := []sightingUnit{}
		var roster struct {
			Units []unitRow `json:"units"`
		}
		data, err = c.get(prefix + "/units")
		if err != nil {
			return err
		}
		if err = json.Unmarshal(data, &roster); err != nil {
			return err
		}
		var trades []struct {
			ShipUnitID string `json:"ship_unit_id"`
			Mine       bool   `json:"mine"`
			CurrentQ   *int   `json:"current_q"`
			CurrentR   *int   `json:"current_r"`
			Items      []struct {
				GoodKey  string  `json:"good_key"`
				Quantity float64 `json:"quantity"`
			} `json:"items"`
		}
		data, err = c.get(prefix + "/trades")
		if err != nil {
			return err
		}
		if err = json.Unmarshal(data, &trades); err != nil {
			return err
		}
		goods := map[string]any{}
		for _, u := range roster.Units {
			uq, ur := u.Q, u.R
			if u.Status == "freighting" {
				uq, ur = nil, nil
			}
			for _, t := range trades {
				if t.Mine && t.ShipUnitID == u.ID {
					if u.Status == "freighting" {
						uq, ur = t.CurrentQ, t.CurrentR
					}
				}
			}
			if u.Status == "marching" && u.CurrentQ != nil && u.CurrentR != nil {
				uq, ur = u.CurrentQ, u.CurrentR
			}
			if u.Status != "embarked" && u.Status != "disbanded" && uq != nil && ur != nil && *uq == q && *ur == r {
				own = append(own, u)
				for _, t := range trades {
					if t.Mine && t.ShipUnitID == u.ID {
						goods[u.ID] = t.Items
					}
				}
			}
		}
		var visible []sightingUnit
		data, err = c.get(prefix + "/foreign-units")
		if err != nil {
			return err
		}
		if err = json.Unmarshal(data, &visible); err != nil {
			return err
		}
		for _, u := range visible {
			if u.Q == q && u.R == r {
				foreign = append(foreign, u)
			}
		}
		if jsonMode {
			printJSON(map[string]any{"hex": tile, "own": own, "foreign": foreign, "ship_goods": goods})
			return nil
		}
		fmt.Printf("Hex (%d,%d): %v\n", q, r, tile["terrain"])
		for _, u := range own {
			name := u.DisplayName
			if name == "" {
				name = u.Type
			}
			fmt.Printf("  %s [%s] · %s ×%d · %s\n", name, u.ID, u.Type, u.Size, u.Status)
			fmt.Println("    " + locationStr(c, u, nil))
			if u.Category == "naval" {
				fmt.Printf("    Hull %d/5 · crew %d · food for %s\n", u.Hull, u.Crew, formatDays(u.ProvisionDays, "%d"))
				for _, p := range u.CargoUnits {
					fmt.Printf("    Passengers: %s ×%d\n", p.DisplayName, p.Size)
				}
			}
			for _, t := range trades {
				if t.Mine && t.ShipUnitID == u.ID {
					for _, item := range t.Items {
						fmt.Printf("    Goods: %s %g\n", item.GoodKey, item.Quantity)
					}
				}
			}
			for good, amount := range u.ProvisionsByGood {
				fmt.Printf("    Provisions: %s %g\n", good, amount)
			}
			for _, o := range u.OrdersEnRoute {
				fmt.Println("    " + ordersEnRouteLine(c, o))
			}
			if u.Status == "marching" && u.Category != "naval" {
				fmt.Printf("    A messenger carries the order: keryx unit redirect --unit %s --target <q,r>; keryx unit recall --unit %s\n", u.ID, u.ID)
			}
			if u.Status == "marching" && u.Category == "naval" {
				fmt.Println("    No orders reach it at sea.")
			}
		}
		for _, u := range foreign {
			fmt.Printf("  %s · %s's %s ×%d · %s\n", u.Name, u.Owner, u.Type, u.Size, u.Status)
			if u.Heading != "" {
				fmt.Println("    Heading " + u.Heading)
			}
			if u.Toward != nil {
				fmt.Printf("    Seems bound toward %s — if so, %s\n", u.Toward.Name, clockTime(u.Toward.EtaAt))
			}
			if u.Category == "naval" {
				fmt.Println("    Hull: unknown · Provisions: unknown")
				if u.Cargo != nil {
					fmt.Printf("    Passengers: %s ×%d\n", u.Cargo.Type, u.Cargo.Size)
				} else {
					fmt.Println("    Passengers: unknown")
				}
			}
			fmt.Println("    Orders and destination: unknown")
		}
		if len(own)+len(foreign) == 0 {
			fmt.Println("  No forces seen here.")
		}
		return nil
	}}
}
