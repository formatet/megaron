package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func stormsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "storms",
		Short: "Storms at sea you can see, and where you last saw the others",
		Long: `A storm is three connected sea hexes that drift slowly. You see one only inside your
sight; afterwards it is remembered where you LAST saw it (with the day), not where it is now.
A ship that sails into a storm takes 2 hull damage (of 5), once per storm however long it stays inside.
The first Wanax whose ship a storm strikes may name it, once: keryx storms name <id> "<name>".`,
		Args: noPositionalArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			data, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/storms", cfg.WorldID))
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			out, err := formatStorms(data)
			if err != nil {
				return err
			}
			fmt.Print(out)
			return nil
		},
	}
	cmd.AddCommand(stormsNameCmd(),
		stormsAdminCmd("rename <storm-id> <name>", "Admin: set or change a storm's name (X-Admin-Key)", cobra.ExactArgs(2), "PUT"),
		stormsAdminCmd("unname <storm-id>", "Admin: clear a storm's name; the namer's right stays spent (X-Admin-Key)", cobra.ExactArgs(1), "DELETE"))
	return cmd
}

// formatStorms renders GET /storms for a person: live storms with their heading, then the
// remembered ones with the day they were last seen.
func formatStorms(data []byte) (string, error) {
	var resp struct {
		Storms []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			CanName  bool   `json:"can_name"`
			Tier     string `json:"tier"`
			Heading  string `json:"heading"`
			SeenTick int    `json:"seen_tick"`
			Hexes    []struct {
				Q int `json:"q"`
				R int `json:"r"`
			} `json:"hexes"`
		} `json:"storms"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", err
	}
	if len(resp.Storms) == 0 {
		return "No storms known. A storm shows only inside your sight.\n", nil
	}
	hexes := func(i int) string {
		var parts []string
		for _, h := range resp.Storms[i].Hexes {
			parts = append(parts, fmt.Sprintf("(%d,%d)", h.Q, h.R))
		}
		return strings.Join(parts, " ")
	}
	var b strings.Builder
	// label is "Storm" or "Storm <Name>"; a storm you may still name says how.
	label := func(i int) string {
		s := resp.Storms[i]
		if s.Name != "" {
			return "Storm " + s.Name
		}
		return "Storm"
	}
	note := func(i int) string {
		if resp.Storms[i].CanName {
			return fmt.Sprintf(" — you were first to meet it: keryx storms name %s \"<name>\"", shortID(resp.Storms[i].ID))
		}
		return ""
	}
	for i, s := range resp.Storms {
		if s.Tier != "live" {
			continue
		}
		fmt.Fprintf(&b, "%s in sight, drifting %s: %s [%s]%s\n", label(i), s.Heading, hexes(i), shortID(s.ID), note(i))
	}
	for i, s := range resp.Storms {
		if s.Tier == "live" {
			continue
		}
		fmt.Fprintf(&b, "%s last seen on day %d: %s (it has moved since) [%s]%s\n", label(i), s.SeenTick, hexes(i), shortID(s.ID), note(i))
	}
	return b.String(), nil
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// stormsNameCmd names a storm you were first to meet. The id may be the 8-character prefix
// that `keryx storms` prints.
func stormsNameCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "name <storm-id> <name>",
		Short: "Name a storm you were the first to meet (once, 2-30 characters)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := newClient(cfg)
			list, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/storms", cfg.WorldID))
			if err != nil {
				return err
			}
			id, err := resolveStormID(list, args[0])
			if err != nil {
				return err
			}
			data, err := c.post(fmt.Sprintf("/api/v1/worlds/%s/storms/%s/name", cfg.WorldID, id), map[string]string{"name": args[1]})
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			fmt.Printf("The storm is named %s. Everyone who sees it will know it by that name.\n", args[1])
			return nil
		},
	}
}

// resolveStormID finds the one listed storm whose id starts with prefix.
func resolveStormID(list []byte, prefix string) (string, error) {
	var resp struct {
		Storms []struct {
			ID string `json:"id"`
		} `json:"storms"`
	}
	if err := json.Unmarshal(list, &resp); err != nil {
		return "", err
	}
	var found []string
	for _, s := range resp.Storms {
		if prefix != "" && strings.HasPrefix(s.ID, prefix) {
			found = append(found, s.ID)
		}
	}
	switch len(found) {
	case 0:
		return "", fmt.Errorf("no storm known to you starts with %q (see keryx storms)", prefix)
	case 1:
		return found[0], nil
	}
	return "", fmt.Errorf("%q matches %d storms; give more of the id", prefix, len(found))
}

// stormsAdminCmd moderates names with the admin key (full storm id, no FOW).
func stormsAdminCmd(use, short string, args cobra.PositionalArgs, method string) *cobra.Command {
	var adminKey string
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  args,
		RunE: func(cmd *cobra.Command, a []string) error {
			if adminKey == "" {
				adminKey = os.Getenv("POLEIA_ADMIN_KEY")
			}
			if adminKey == "" {
				return fmt.Errorf("admin key required: set POLEIA_ADMIN_KEY or use --key")
			}
			c := newClient(cfg)
			c.extraHeaders = map[string]string{"X-Admin-Key": adminKey}
			path := fmt.Sprintf("/api/v1/admin/worlds/%s/storms/%s/name", cfg.WorldID, a[0])
			var err error
			if method == "PUT" {
				_, err = c.put(path, map[string]string{"name": a[1]})
			} else {
				_, err = c.delete(path)
			}
			if err != nil {
				return err
			}
			fmt.Println("Done.")
			return nil
		},
	}
	cmd.Flags().StringVar(&adminKey, "key", "", "admin key (overrides POLEIA_ADMIN_KEY env var)")
	return cmd
}
