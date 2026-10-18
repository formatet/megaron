package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// codexIndex mirrors web/static/codex/index.json — the same articles the web
// shows behind "?". keryx reads them from the server's static files so a
// terminal player has the rules too (Haiku round 6: "Rules: ? (Codex)" in the
// opening dispatch pointed at something keryx could not open).
type codexIndex struct {
	Categories []string `json:"categories"`
	Articles   []struct {
		ID       string   `json:"id"`
		Title    string   `json:"title"`
		Category string   `json:"category"`
		Related  []string `json:"related"`
	} `json:"articles"`
}

func codexCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "codex [article]",
		Short: "Read the Codex, the game's rules: no argument lists the articles, an id prints one",
		Example: `  keryx codex
  keryx codex bronze`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := newClient(cfg)
			data, err := c.get("/static/codex/index.json")
			if err != nil {
				return fmt.Errorf("could not load the Codex: %w", err)
			}
			var idx codexIndex
			if err := json.Unmarshal(data, &idx); err != nil {
				return fmt.Errorf("could not read the Codex index: %w", err)
			}
			if len(args) == 0 {
				fmt.Print(formatCodexIndex(idx))
				return nil
			}
			id := strings.ToLower(strings.TrimSuffix(args[0], ".md"))
			title, related, ok := codexArticle(idx, id)
			if !ok {
				return fmt.Errorf("no Codex article %q — run `keryx codex` for the list", args[0])
			}
			body, err := c.get("/static/codex/" + id + ".md")
			if err != nil {
				return fmt.Errorf("could not load Codex article %q: %w", id, err)
			}
			fmt.Printf("%s\n\n%s\n", title, strings.TrimSpace(string(body)))
			if len(related) > 0 {
				fmt.Printf("\nRelated: %s  (keryx codex <id>)\n", strings.Join(related, ", "))
			}
			return nil
		},
	}
}

// formatCodexIndex lists every article under its category, in index order.
func formatCodexIndex(idx codexIndex) string {
	var b strings.Builder
	b.WriteString("The Codex — read one with `keryx codex <id>`\n")
	for _, cat := range idx.Categories {
		var rows []string
		for _, a := range idx.Articles {
			if a.Category == cat {
				rows = append(rows, fmt.Sprintf("  %-18s %s", a.ID, a.Title))
			}
		}
		if len(rows) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n%s\n%s\n", cat, strings.Join(rows, "\n"))
	}
	return b.String()
}

// codexArticle only accepts ids listed in the index, so the id is never a path.
func codexArticle(idx codexIndex, id string) (title string, related []string, ok bool) {
	for _, a := range idx.Articles {
		if a.ID == id {
			return a.Title, a.Related, true
		}
	}
	return "", nil, false
}
