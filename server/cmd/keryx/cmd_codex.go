package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// codexIndex mirrors web/static/codex/index.json — the same articles the web
// shows behind "?". keryx reads them from the server's static files so a
// terminal player has the rules too (Haiku round 6: "Rules: ? (Codex)" in the
// opening dispatch pointed at something keryx could not open).
type codexArticleEntry struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Category string   `json:"category"`
	Related  []string `json:"related"`
	Aliases  []string `json:"aliases"`
	Future   bool     `json:"future"`
}
type codexIndex struct {
	Categories []string            `json:"categories"`
	Articles   []codexArticleEntry `json:"articles"`
}

func codexCmd() *cobra.Command {
	var query string
	cmd := &cobra.Command{
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
			if query != "" {
				if len(args) > 0 {
					return fmt.Errorf("choose an article or --search, not both")
				}
				raw, err := c.get("/static/codex/articles.json")
				if err != nil {
					return err
				}
				var bodies map[string]string
				if err := json.Unmarshal(raw, &bodies); err != nil {
					return err
				}
				hits := searchCodexArticles(query, idx, bodies)
				if jsonMode {
					data, _ := json.Marshal(hits)
					printRawJSON(data)
					return nil
				}
				if len(hits) == 0 {
					fmt.Println("No matching Codex articles.")
				}
				for _, a := range hits {
					future := ""
					if a.Future {
						future = " (not yet in the world)"
					}
					fmt.Printf("%-26s %s%s\n", a.ID, a.Title, future)
				}
				return nil
			}
			if len(args) == 0 {
				fmt.Print(formatCodexIndex(idx))
				return nil
			}
			input := strings.TrimSuffix(args[0], ".md")
			article, ok := resolveCodexArticle(idx, input)
			id, title, related := article.ID, article.Title, article.Related
			if !ok {
				return fmt.Errorf("no Codex article %q — run `keryx codex --search QUERY` or `keryx codex` for the list", args[0])
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
	cmd.Flags().StringVar(&query, "search", "", "search titles, aliases and article contents")
	return cmd
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

func normaliseCodexSearch(value string) string {
	return strings.Join(strings.Fields(strings.NewReplacer("_", " ", "-", " ").Replace(strings.ToLower(value))), " ")
}
func resolveCodexArticle(idx codexIndex, input string) (codexArticleEntry, bool) {
	needle := normaliseCodexSearch(input)
	for _, a := range idx.Articles {
		if normaliseCodexSearch(a.ID) == needle || normaliseCodexSearch(a.Title) == needle {
			return a, true
		}
	}
	var found codexArticleEntry
	for _, a := range idx.Articles {
		for _, alias := range a.Aliases {
			if normaliseCodexSearch(alias) == needle {
				if found.ID != "" && found.ID != a.ID {
					return codexArticleEntry{}, false
				}
				found = a
				break
			}
		}
	}
	if found.ID != "" {
		return found, true
	}
	return codexArticleEntry{}, false
}
func searchCodexArticles(query string, idx codexIndex, bodies map[string]string) []codexArticleEntry {
	needle := normaliseCodexSearch(query)
	type scored struct {
		a     codexArticleEntry
		score int
	}
	rows := []scored{}
	if needle == "" {
		return []codexArticleEntry{}
	}
	for _, a := range idx.Articles {
		title := normaliseCodexSearch(a.Title)
		score := 4
		if title == needle {
			score = 0
		} else if strings.Contains(title, needle) {
			score = 1
		} else {
			for _, alias := range a.Aliases {
				if strings.Contains(normaliseCodexSearch(alias), needle) {
					score = 2
					break
				}
			}
			if score == 4 && strings.Contains(normaliseCodexSearch(bodies[a.ID]), needle) {
				score = 3
			}
		}
		if score < 4 {
			rows = append(rows, scored{a, score})
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].score != rows[j].score {
			return rows[i].score < rows[j].score
		}
		return strings.ToLower(rows[i].a.Title) < strings.ToLower(rows[j].a.Title)
	})
	result := make([]codexArticleEntry, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.a)
	}
	return result
}
