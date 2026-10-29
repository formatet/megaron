package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// marketBoardCmd shows or writes a city's market board
// (PUT /worlds/{w}/settlements/{s}/market-board, megaron_plan_marknadstavlan).
func marketBoardCmd() *cobra.Command {
	var buys, sells string
	var clear bool
	cmd := &cobra.Command{
		Use:   "market-board <city>",
		Short: "Show or write what your city's market board says it buys and sells",
		Long: `Show or write your city's market board. A city with a Market can put up a
board naming up to three goods it buys and three it sells. Other rulers'
caravans, armies and messengers read it when they pass within reach of the
market (wider at higher market levels) and remember it as it stood then.

The board is your word: the game checks nothing against your stores. Trade
itself still happens in letters — the board tells others whom to ask.

With no flag it shows the board. --buys and --sells replace each line
(comma-separated goods; an empty value clears that line). --clear empties both.`,
		Example: `  keryx market-board Knossos
  keryx market-board Knossos --buys tin,copper --sells wine
  keryx market-board Knossos --sells ""
  keryx market-board Knossos --clear`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := newClient(cfg)
			sid, err := resolveSettlement(c, cfg.WorldID, args[0])
			if err != nil {
				return err
			}
			base := fmt.Sprintf("/api/v1/worlds/%s/settlements/%s", cfg.WorldID, sid)
			current, err := c.get(base)
			if err != nil {
				return err
			}
			var city struct {
				Name  string         `json:"name"`
				Board map[string]any `json:"market_board"`
			}
			if err := json.Unmarshal(current, &city); err != nil {
				return err
			}
			write := clear || cmd.Flags().Changed("buys") || cmd.Flags().Changed("sells")
			if !write {
				if jsonMode {
					printRawJSON(current)
					return nil
				}
				if city.Board == nil {
					fmt.Printf("%s has no market board. Build a Market first: keryx build %s --type market\n", city.Name, city.Name)
					return nil
				}
				fmt.Printf("%s market board: %s\n", city.Name, describeBoard(city.Board))
				if reach, ok := city.Board["reach"].(float64); ok {
					fmt.Printf("Read by others' journeys within %g hexes of the city.\n", reach)
				}
				return nil
			}
			if city.Board == nil {
				return fmt.Errorf("%s has no Market. Build one before putting up a board: keryx build %s --type market", city.Name, city.Name)
			}
			body := map[string][]string{"buys": boardLine(city.Board["buys"]), "sells": boardLine(city.Board["sells"])}
			if clear {
				body["buys"], body["sells"] = []string{}, []string{}
			}
			if cmd.Flags().Changed("buys") {
				body["buys"] = splitGoods(buys)
			}
			if cmd.Flags().Changed("sells") {
				body["sells"] = splitGoods(sells)
			}
			data, err := c.put(base+"/market-board", body)
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var board map[string]any
			if err := json.Unmarshal(data, &board); err != nil {
				return err
			}
			fmt.Printf("%s market board now says: %s\n", city.Name, describeBoard(board))
			return nil
		},
	}
	cmd.Flags().StringVar(&buys, "buys", "", "goods the city buys, comma-separated (max 3)")
	cmd.Flags().StringVar(&sells, "sells", "", "goods the city sells, comma-separated (max 3)")
	cmd.Flags().BoolVar(&clear, "clear", false, "empty the board")
	return cmd
}

func splitGoods(s string) []string {
	out := []string{}
	for _, g := range strings.Split(s, ",") {
		if g = strings.ToLower(strings.TrimSpace(g)); g != "" {
			out = append(out, g)
		}
	}
	return out
}

func boardLine(v any) []string {
	out := []string{}
	if list, ok := v.([]any); ok {
		for _, g := range list {
			if s, ok := g.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

// describeBoard renders a board or a reading as "Buys tin · Sells wine".
func describeBoard(b map[string]any) string {
	buys, sells := boardLine(b["buys"]), boardLine(b["sells"])
	if len(buys) == 0 && len(sells) == 0 {
		return "the board is empty"
	}
	parts := []string{}
	if len(buys) > 0 {
		parts = append(parts, "Buys "+strings.Join(buys, ", "))
	}
	if len(sells) > 0 {
		parts = append(parts, "Sells "+strings.Join(sells, ", "))
	}
	return strings.Join(parts, " · ")
}

// describeReading adds when a foreign board was read: wall clock, plus age in game days.
func describeReading(b map[string]any) string {
	s := describeBoard(b)
	if raw, ok := b["read_at"].(string); ok {
		at, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return s
		}
		s += " (read " + clockTime(at)
		if days, ok := b["read_game_days_ago"].(float64); ok {
			s += fmt.Sprintf(", %g game days ago", days)
		}
		s += ")"
	}
	return s
}
