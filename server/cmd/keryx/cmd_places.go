package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

func placeKindLabel(kind string) string {
	switch kind {
	case "river":
		return "river"
	case "massif":
		return "mountain massif"
	case "desert":
		return "desert"
	case "forest":
		return "cedar forest"
	default:
		return "place"
	}
}

func placesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use: "places", Short: "Known rivers, mountain massifs, deserts and cedar forests",
		Long: `Places you can see or remember. The first Wanax whose own physical unit reaches a place may name it, once.
Seeing it nearby gives no naming right. Use: keryx places name <kind> <q,r> "<name>".`,
		Args: noPositionalArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			data, err := newClient(cfg).get(fmt.Sprintf("/api/v1/worlds/%s/places", cfg.WorldID))
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			out, err := formatPlaces(data)
			if err != nil {
				return err
			}
			fmt.Print(out)
			return nil
		},
	}
	cmd.AddCommand(placesNameCmd(), placesAdminCmd("PUT"), placesAdminCmd("DELETE"))
	return cmd
}

func formatPlaces(data []byte) (string, error) {
	var resp struct {
		Places []struct {
			Kind    string `json:"kind"`
			ID      string `json:"id"`
			Name    string `json:"name"`
			CanName bool   `json:"can_name"`
			Hexes   []struct {
				Q int `json:"q"`
				R int `json:"r"`
			} `json:"hexes"`
			ToldHexes []struct {
				Q int `json:"q"`
				R int `json:"r"`
			} `json:"told_hexes"`
			ToldBy   []string `json:"told_by"`
			Tellable bool     `json:"tellable"`
		} `json:"places"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", err
	}
	if len(resp.Places) == 0 {
		return "No places known. March or sail to discover the land.\n", nil
	}
	var out strings.Builder
	for _, p := range resp.Places {
		name := p.Name
		if name == "" {
			name = "unnamed"
		}
		var hexes []string
		for _, h := range p.Hexes {
			hexes = append(hexes, fmt.Sprintf("(%d,%d)", h.Q, h.R))
		}
		fmt.Fprintf(&out, "%s: %s [%s %s] — known hexes: %s", placeKindLabel(p.Kind), name, p.Kind, p.ID, strings.Join(hexes, " "))
		if len(p.ToldHexes) > 0 {
			fmt.Fprintf(&out, " — %d more hexes told by %s", len(p.ToldHexes), strings.Join(p.ToldBy, ", "))
		}
		if p.CanName {
			fmt.Fprintf(&out, " — you were first to reach it: keryx places name %s %s \"<name>\"", p.Kind, p.ID)
		}
		if p.Tellable {
			fmt.Fprintf(&out, " — tell of it: keryx message --to <city> --text \"…\" --tell-place %q", p.Name)
		}
		out.WriteByte('\n')
	}
	return out.String(), nil
}

func validatePlaceArgs(kind, id string) error {
	switch kind {
	case "river", "massif", "desert", "forest":
	default:
		return fmt.Errorf("kind must be river, massif, desert or forest")
	}
	if !regexp.MustCompile(`^-?\d+,-?\d+$`).MatchString(id) {
		return fmt.Errorf("place id must be its q,r anchor (see keryx places)")
	}
	return nil
}

func placesNameCmd() *cobra.Command {
	return &cobra.Command{Use: "name <kind> <q,r> <name>", Short: "Name a place your unit was first to reach (once, 2–30 characters)", Args: cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validatePlaceArgs(args[0], args[1]); err != nil {
				return err
			}
			data, err := newClient(cfg).post(fmt.Sprintf("/api/v1/worlds/%s/places/%s/%s/name", cfg.WorldID, args[0], args[1]), map[string]string{"name": args[2]})
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var result struct {
				Name string `json:"name"`
			}
			if err := json.Unmarshal(data, &result); err != nil {
				return err
			}
			fmt.Printf("The %s is named %s. Everyone who knows it will see that name.\n", placeKindLabel(args[0]), result.Name)
			return nil
		}}
}

func placesAdminCmd(method string) *cobra.Command {
	use, short, n := "rename <kind> <q,r> <name>", "Admin: set or change a place's name (X-Admin-Key)", 3
	if method == "DELETE" {
		use, short, n = "unname <kind> <q,r>", "Admin: clear a place's name; the naming right stays spent (X-Admin-Key)", 2
	}
	var key string
	cmd := &cobra.Command{Use: use, Short: short, Args: cobra.ExactArgs(n), RunE: func(cmd *cobra.Command, args []string) error {
		if err := validatePlaceArgs(args[0], args[1]); err != nil {
			return err
		}
		if key == "" {
			key = os.Getenv("POLEIA_ADMIN_KEY")
		}
		if key == "" {
			return fmt.Errorf("admin key required: set POLEIA_ADMIN_KEY or use --key")
		}
		c := newClient(cfg)
		c.extraHeaders = map[string]string{"X-Admin-Key": key}
		path := fmt.Sprintf("/api/v1/admin/worlds/%s/places/%s/%s/name", cfg.WorldID, args[0], args[1])
		var data []byte
		var err error
		if method == "PUT" {
			data, err = c.put(path, map[string]string{"name": args[2]})
		} else {
			data, err = c.delete(path)
		}
		if err != nil {
			return err
		}
		if jsonMode {
			printRawJSON(data)
		} else {
			fmt.Println("Done.")
		}
		return nil
	}}
	cmd.Flags().StringVar(&key, "key", "", "admin key (overrides POLEIA_ADMIN_KEY env var)")
	return cmd
}

// resolveTellPlace finds what --tell-place names: a tellable named place
// from /places, else a city on the map (megaron_plan_vagbeskrivningar v1).
func resolveTellPlace(c *Client, markers []map[string]any, name string) (map[string]any, error) {
	data, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/places", cfg.WorldID))
	if err != nil {
		return nil, err
	}
	var resp struct {
		Places []struct {
			Kind     string `json:"kind"`
			ID       string `json:"id"`
			Name     string `json:"name"`
			Tellable bool   `json:"tellable"`
		} `json:"places"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}
	for _, p := range resp.Places {
		if strings.EqualFold(p.Name, name) {
			if !p.Tellable {
				return nil, fmt.Errorf("you know %s only from what others told you — you can tell only of what your own people saw", p.Name)
			}
			var q, r int
			fmt.Sscanf(p.ID, "%d,%d", &q, &r)
			return map[string]any{"kind": p.Kind, "anchor_q": q, "anchor_r": r}, nil
		}
	}
	for _, m := range markers {
		if n, _ := m["name"].(string); strings.EqualFold(n, name) {
			q, _ := m["q"].(float64)
			r, _ := m["r"].(float64)
			return map[string]any{"kind": "city", "anchor_q": int(q), "anchor_r": int(r)}, nil
		}
	}
	return nil, fmt.Errorf("no place or city called %q that you know — see keryx places and keryx cities", name)
}
