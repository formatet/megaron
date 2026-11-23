package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// stepsCmd prints the Lawagetas' first steps (GET /worlds/{w}/first-steps).
// The rules and texts live on the server (api/handlers/first_steps.go); this
// only renders them. --hide / --show flip the per-player flag the web card uses.
func stepsCmd() *cobra.Command {
	var hide, show bool
	cmd := &cobra.Command{
		Use:   "steps",
		Short: "The Lawagetas' first steps — what to do next at the start",
		Long: `Show the four first steps the Lawagetas, your war-leader, sets before a new
Wanax: found a city, put the people to work, order a building, send people out
to look. Each is marked when done. The advice ends when the steps are done.

  --hide   stop showing the Lawagetas' card on the map (it is only hidden, not lost)
  --show   bring it back`,
		Example: `  keryx steps
  keryx steps --hide
  keryx steps --show`,
		Args: noPositionalArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if hide && show {
				return fmt.Errorf("choose either --hide or --show, not both")
			}
			c := newClient(cfg)
			base := fmt.Sprintf("/api/v1/worlds/%s/first-steps", cfg.WorldID)
			if hide || show {
				verb, msg := "hide", "The Lawagetas' card is hidden on the map. `keryx steps --show` brings it back."
				if show {
					verb, msg = "show", "The Lawagetas' card is shown on the map again."
				}
				data, err := c.post(base+"/"+verb, nil)
				if err != nil {
					return err
				}
				if jsonMode {
					printRawJSON(data)
					return nil
				}
				fmt.Println(msg)
				return nil
			}
			data, err := c.get(base)
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			out, err := renderFirstSteps(c, data)
			if err != nil {
				return err
			}
			fmt.Print(out)
			return nil
		},
	}
	cmd.Flags().BoolVar(&hide, "hide", false, "hide the Lawagetas' card on the map")
	cmd.Flags().BoolVar(&show, "show", false, "show the Lawagetas' card on the map again")
	return cmd
}

// renderFirstSteps turns the server body into the text keryx prints. The
// server's {when} token is filled with the wall-clock time of ready_tick.
func renderFirstSteps(c *Client, data []byte) (string, error) {
	var r struct {
		Hidden      bool   `json:"hidden"`
		DoneCount   int    `json:"done_count"`
		Total       int    `json:"total"`
		Next        string `json:"next"`
		ClosingText string `json:"closing_text"`
		Steps       []struct {
			Key       string `json:"key"`
			Done      bool   `json:"done"`
			Text      string `json:"text"`
			ReadyTick *int   `json:"ready_tick"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "The Lawagetas' first steps (%d/%d)\n", r.DoneCount, r.Total)
	for _, s := range r.Steps {
		mark := "[ ]"
		if s.Done {
			mark = "[x]"
		}
		text := s.Text
		if s.ReadyTick != nil {
			when := formatWorldTime(c, *s.ReadyTick)
			if when == "time unavailable" {
				when = "soon"
			}
			text = strings.ReplaceAll(text, "{when}", when)
		}
		fmt.Fprintf(&b, "%s %-8s %s\n", mark, s.Key, text)
	}
	if r.Next == "" && r.ClosingText != "" {
		fmt.Fprintf(&b, "\n%s\n", r.ClosingText)
	}
	if r.Hidden {
		b.WriteString("\n(The card is hidden on the map. `keryx steps --show` brings it back.)\n")
	}
	return b.String(), nil
}
