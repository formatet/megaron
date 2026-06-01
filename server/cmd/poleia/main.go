// poleia — CLI client for the Poleia game server.
//
// Usage:
//
//	poleia login --server http://10.0.1.88:8080 --username alice
//	poleia status
//	poleia march --target Korinth --intent attack --hoplites 50
//	poleia marches
//	poleia recruit --unit hoplites --count 20
//	poleia build --type farm
//	poleia craft --qty 5
//	poleia worlds
//	poleia kingdoms
//	poleia settlements
//	poleia goods
//	poleia trade --good grain --qty 10 --dest Korinth
//	poleia inbox
//
// Environment variables:
//
//	POLEIA_SERVER   override server URL
//	POLEIA_TOKEN    override stored JWT token
//	POLEIA_CONFIG   override config file path
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var cfg *Config

func main() {
	root := &cobra.Command{
		Use:   "poleia",
		Short: "Poleia — Bronze Age grand strategy CLI",
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if cmd.Name() == "login" {
				return nil
			}
			var err error
			cfg, err = loadConfig()
			if err != nil {
				fmt.Fprintln(os.Stderr, "Not logged in — run: poleia login --server <url>")
				os.Exit(2)
			}
			if cfg.WorldID == "" && cmd.Name() != "worlds" {
				fmt.Fprintln(os.Stderr, "No active world — run: poleia worlds")
				os.Exit(2)
			}
			return nil
		},
	}

	root.PersistentFlags().BoolVarP(&jsonMode, "json", "j", false, "output as JSON (for scripts and MCP)")

	root.AddCommand(
		loginCmd(),
		statusCmd(),
		marchCmd(),
		marchesCmd(),
		recallCmd(),
		recruitCmd(),
		buildCmd(),
		craftCmd(),
		worldsCmd(),
		kingdomsCmd(),
		settlementsCmd(),
		wanaxesCmd(),
		goodsCmd(),
		tradeCmd(),
		inboxCmd(),
		replyCmd(),
		tradeAcceptCmd(),
		tradeDeclineCmd(),
		gossipCmd(),
		messengerCmd(),
	)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
