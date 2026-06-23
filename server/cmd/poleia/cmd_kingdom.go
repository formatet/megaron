package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// resolveCapitalProvinceID looks up a Wanax's capital province ID by settlement
// name. It queries /wanaxes (the same FOW-gated directory the agent reads) so
// the set of resolvable names is always identical to state["wanaxes"].
func resolveCapitalProvinceID(c *Client, name string) (string, error) {
	data, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/wanaxes", cfg.WorldID))
	if err != nil {
		return "", err
	}
	var entries []map[string]any
	if err := json.Unmarshal(data, &entries); err != nil {
		return "", err
	}
	for _, e := range entries {
		eName, _ := e["name"].(string)
		if !strings.EqualFold(eName, name) {
			continue
		}
		own, _ := e["own"].(bool)
		if own {
			return "", fmt.Errorf("%q is your own settlement — kingdom invites go to other Wanaxes; pick a neighbour from state[\"wanaxes\"] (rows without \"own\":true)", name)
		}
		isCapital, _ := e["is_capital"].(bool)
		if !isCapital {
			// Name matched a colony, not a capital — keep looking for the capital row.
			continue
		}
		provID, _ := e["province_id"].(string)
		if provID == "" {
			return "", fmt.Errorf("no visible capital settlement named %q (check spelling — exact name from state[\"wanaxes\"])", name)
		}
		return provID, nil
	}
	return "", fmt.Errorf("no visible capital settlement named %q (check spelling — exact name from state[\"wanaxes\"])", name)
}

func kingdomFoundCmd() *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:     "kingdom-found",
		Short:   "Found a new kingdom (requires an active capital, not already in a kingdom)",
		Example: `  poleia kingdom-found --name "Achaean League"`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if name == "" {
				return fmt.Errorf("--name required")
			}
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/kingdoms", cfg.WorldID)
			data, err := c.post(path, map[string]string{"name": name})
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var resp map[string]any
			if err := json.Unmarshal(data, &resp); err != nil {
				return err
			}
			fmt.Printf("Kingdom founded · %s · %v\n", resp["name"], resp["id"])
			fmt.Printf("Next: it stays 'forming' (inactive) until it reaches 3 members. "+
				"Invite at least 2 other Wanaxes by name now:\n"+
				"  poleia kingdom-invite --kingdom %v --target <WanaxName>\n", resp["id"])
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "kingdom name")
	return cmd
}

func kingdomInviteCmd() *cobra.Command {
	var kingdomID, target string
	cmd := &cobra.Command{
		Use:     "kingdom-invite",
		Short:   "Invite a Wanax (by settlement name) to your kingdom — basileus only",
		Example: `  poleia kingdom-invite --kingdom <id> --target Akhilles`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if kingdomID == "" || target == "" {
				return fmt.Errorf("--kingdom and --target required")
			}
			c := newClient(cfg)
			provinceID, err := resolveCapitalProvinceID(c, target)
			if err != nil {
				return err
			}
			path := fmt.Sprintf("/api/v1/worlds/%s/kingdoms/%s/invite", cfg.WorldID, kingdomID)
			if _, err := c.post(path, map[string]string{"province_id": provinceID}); err != nil {
				return err
			}
			fmt.Printf("Invitation sent to %s · expires in 48h.\n", target)
			return nil
		},
	}
	cmd.Flags().StringVar(&kingdomID, "kingdom", "", "kingdom ID")
	cmd.Flags().StringVar(&target, "target", "", "invitee's settlement/Wanax name (exact spelling)")
	return cmd
}

func kingdomJoinCmd() *cobra.Command {
	var kingdomID string
	cmd := &cobra.Command{
		Use:     "kingdom-join",
		Short:   "Join a kingdom you've been invited to",
		Example: `  poleia kingdom-join --kingdom <id>`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if kingdomID == "" {
				return fmt.Errorf("--kingdom required")
			}
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/kingdoms/%s/join", cfg.WorldID, kingdomID)
			_, err := c.post(path, map[string]any{})
			if err != nil {
				return err
			}
			fmt.Println("Joined kingdom.")
			return nil
		},
	}
	cmd.Flags().StringVar(&kingdomID, "kingdom", "", "kingdom ID")
	return cmd
}

func kingdomInvitationsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "kingdom-invitations",
		Short: "List pending kingdom invitations to your settlement",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/kingdoms/invitations", cfg.WorldID)
			data, err := c.get(path)
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var invs []map[string]any
			if err := json.Unmarshal(data, &invs); err != nil {
				return err
			}
			if len(invs) == 0 {
				fmt.Println("No pending invitations.")
				return nil
			}
			for _, i := range invs {
				fmt.Printf("  %v invites you to %v (kingdom %v) · expires %v\n",
					i["invited_by"], i["kingdom_name"], i["kingdom_id"], i["expires_at"])
			}
			return nil
		},
	}
}

func kingdomCouncilCmd() *cobra.Command {
	var kingdomID string
	cmd := &cobra.Command{
		Use:     "kingdom-council",
		Short:   "List a kingdom's council — members, roles, settlement strength",
		Example: `  poleia kingdom-council --kingdom <id>`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if kingdomID == "" {
				return fmt.Errorf("--kingdom required")
			}
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/kingdoms/%s/council", cfg.WorldID, kingdomID)
			data, err := c.get(path)
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var members []map[string]any
			if err := json.Unmarshal(data, &members); err != nil {
				return err
			}
			for _, m := range members {
				fmt.Printf("  %-9s %-16s %-16s DP %v · walls %v\n",
					m["role"], m["username"], m["settlement_name"], m["army_dp"], m["walls"])
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&kingdomID, "kingdom", "", "kingdom ID")
	return cmd
}

func kingdomElectionCmd() *cobra.Command {
	var kingdomID string
	cmd := &cobra.Command{
		Use:     "kingdom-election",
		Short:   "Show the active election for a kingdom, if any",
		Example: `  poleia kingdom-election --kingdom <id>`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if kingdomID == "" {
				return fmt.Errorf("--kingdom required")
			}
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/kingdoms/%s/election", cfg.WorldID, kingdomID)
			data, err := c.get(path)
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var resp map[string]any
			if err := json.Unmarshal(data, &resp); err != nil {
				return err
			}
			if active, _ := resp["active"].(bool); !active {
				fmt.Println("No active election.")
				return nil
			}
			fmt.Printf("Election for %s · candidate %s · votes %v/%v · closes %v\n",
				resp["candidate_name"], resp["candidate_id"], resp["vote_count"], resp["member_count"], resp["closes_at"])
			return nil
		},
	}
	cmd.Flags().StringVar(&kingdomID, "kingdom", "", "kingdom ID")
	return cmd
}

func kingdomElectionCallCmd() *cobra.Command {
	var kingdomID, candidateID string
	cmd := &cobra.Command{
		Use:     "kingdom-election-call",
		Short:   "Call an election for basileus (Sundays only, kingdom must be unlocked)",
		Example: `  poleia kingdom-election-call --kingdom <id> --candidate <player-id>`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if kingdomID == "" || candidateID == "" {
				return fmt.Errorf("--kingdom and --candidate required")
			}
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/kingdoms/%s/election", cfg.WorldID, kingdomID)
			data, err := c.post(path, map[string]string{"candidate_id": candidateID})
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var resp map[string]any
			if err := json.Unmarshal(data, &resp); err != nil {
				return err
			}
			fmt.Printf("Election called · candidate %v · closes %v\n", resp["candidate_id"], resp["closes_at"])
			return nil
		},
	}
	cmd.Flags().StringVar(&kingdomID, "kingdom", "", "kingdom ID")
	cmd.Flags().StringVar(&candidateID, "candidate", "", "candidate player ID")
	return cmd
}

func kingdomVoteCmd() *cobra.Command {
	var kingdomID, candidateID string
	cmd := &cobra.Command{
		Use:     "kingdom-vote",
		Short:   "Cast your vote in an open basileus election",
		Example: `  poleia kingdom-vote --kingdom <id> --candidate <player-id>`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if kingdomID == "" || candidateID == "" {
				return fmt.Errorf("--kingdom and --candidate required")
			}
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/kingdoms/%s/vote", cfg.WorldID, kingdomID)
			data, err := c.post(path, map[string]string{"candidate_id": candidateID})
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var resp map[string]any
			if err := json.Unmarshal(data, &resp); err != nil {
				return err
			}
			if resolved, _ := resp["resolved"].(bool); resolved {
				fmt.Println("Vote cast · election resolved — a new basileus has been crowned.")
			} else {
				fmt.Println("Vote cast.")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&kingdomID, "kingdom", "", "kingdom ID")
	cmd.Flags().StringVar(&candidateID, "candidate", "", "candidate player ID")
	return cmd
}

func kingdomTreasuryDepositCmd() *cobra.Command {
	var kingdomID string
	var amount float64
	cmd := &cobra.Command{
		Use:     "kingdom-treasury-deposit",
		Short:   "Send silver from your capital to the kingdom treasury (travels as a caravan)",
		Example: `  poleia kingdom-treasury-deposit --kingdom <id> --amount 100`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if kingdomID == "" || amount <= 0 {
				return fmt.Errorf("--kingdom required and --amount must be positive")
			}
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/kingdoms/%s/treasury/deposit", cfg.WorldID, kingdomID)
			data, err := c.post(path, map[string]float64{"amount": amount})
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var resp map[string]any
			if err := json.Unmarshal(data, &resp); err != nil {
				return err
			}
			fmt.Printf("Tribute caravan dispatched · %.0f silver · arrives %v\n", amount, resp["arrives_at"])
			return nil
		},
	}
	cmd.Flags().StringVar(&kingdomID, "kingdom", "", "kingdom ID")
	cmd.Flags().Float64Var(&amount, "amount", 0, "silver to deposit")
	return cmd
}

func kingdomBorrowArmyCmd() *cobra.Command {
	var kingdomID, lenderID string
	var infantry, chariot, priest, ship int
	cmd := &cobra.Command{
		Use:     "kingdom-borrow-army",
		Short:   "Borrow units from a kingdom member's settlement — basileus only",
		Example: `  poleia kingdom-borrow-army --kingdom <id> --lender <player-id> --infantry 20`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if kingdomID == "" || lenderID == "" {
				return fmt.Errorf("--kingdom and --lender required")
			}
			if infantry+chariot+priest+ship == 0 {
				return fmt.Errorf("specify at least one unit type to borrow")
			}
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/kingdoms/%s/borrow-army", cfg.WorldID, kingdomID)
			_, err := c.post(path, map[string]any{
				"lender_player_id": lenderID,
				"spearman":         infantry,
				"war_chariot":      chariot,
				"priest":           priest,
				"ship":             ship,
			})
			if err != nil {
				return err
			}
			fmt.Println("Army borrowed.")
			return nil
		},
	}
	cmd.Flags().StringVar(&kingdomID, "kingdom", "", "kingdom ID")
	cmd.Flags().StringVar(&lenderID, "lender", "", "lender's player ID")
	cmd.Flags().IntVar(&infantry, "spearman", 0, "spearmen to borrow")
	cmd.Flags().IntVar(&chariot, "war-chariot", 0, "war chariots to borrow")
	cmd.Flags().IntVar(&priest, "priest", 0, "priests to borrow")
	cmd.Flags().IntVar(&ship, "ship", 0, "ships to borrow")
	return cmd
}
