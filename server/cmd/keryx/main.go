// keryx — CLI client for the Megaron game server.
//
// Usage:
//
//	keryx login --server http://10.0.1.88:8080 --username alice
//	keryx join
//	keryx status
//	keryx recruit --unit spearman --count 20
//	keryx build --type farm
//	keryx staff foundry +1
//	keryx worlds
//	keryx kingdoms
//	keryx settlements
//	keryx goods
//	keryx transfer --good grain --qty 10 --dest Korinth
//	keryx cargo
//	keryx inbox
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
		Use:   "keryx",
		Short: "Keryx — Megaron's Bronze Age grand strategy CLI",
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if cmd.Name() == "login" {
				return nil
			}
			var err error
			cfg, err = loadConfig()
			if err != nil {
				fmt.Fprintln(os.Stderr, "Not logged in — run: keryx login --server <url>")
				os.Exit(2)
			}
			if cfg.WorldID == "" && commandNeedsWorld(cmd) {
				fmt.Fprintln(os.Stderr, "No active world — run: keryx worlds")
				os.Exit(2)
			}
			return nil
		},
	}

	root.PersistentFlags().BoolVarP(&jsonMode, "json", "j", false, "output as JSON (for scripts and MCP)")

	root.AddCommand(
		loginCmd(),
		passwordCmd(),
		agoraCmd(),
		joinCmd(),
		riseCmd(),
		leaveCmd(),
		statusCmd(),
		mapCmd(),
		stormsCmd(),
		sightingsCmd(),
		recruitCmd(),
		disbandCmd(),
		buildCmd(),
		cancelBuildCmd(),
		worldsCmd(),
		// MVP-disabled 2026-07-08: kingdoms är post-MVP — stubs print why instead
		// of cobra's raw "unknown command" (see cmd_kingdoms_stub.go).
		kingdomsCmd(),
		kingdomFoundCmd(),
		kingdomInviteCmd(),
		kingdomJoinCmd(),
		kingdomInvitationsCmd(),
		kingdomCouncilCmd(),
		kingdomElectionCmd(),
		kingdomElectionCallCmd(),
		kingdomVoteCmd(),
		kingdomTreasuryDepositCmd(),
		kingdomBorrowArmyCmd(),
		settlementsCmd(),
		rosterCmd(),
		occupationCmd(),
		citiesCmd(),
		diplomacyCmd(),
		goodsCmd(),
		ticklogCmd(),
		transferCmd(),
		routeCreateCmd(),
		routesCmd(),
		routePauseCmd(),
		routeResumeCmd(),
		routeDeleteCmd(),
		cargoCmd(),
		giftCmd(),
		inboxCmd(),
		outboxCmd(),
		replyCmd(),
		tradeAcceptCmd(),
		tradeDeclineCmd(),
		tradeCancelCmd(),
		passageCmd(),
		callBackCmd(),
		gossipCmd(),
		messengerCmd(),
		messageCmd(),
		tradeOfferCmd(),
		allocateCmd(),
		cityCmd(),
		placementsAliasCmd(),
		placeCmd(),
		staffCmd(),
		slaughterCmd(),
		idleCmd(),
		briefCmd(),
		unitCmd(),
		retreatDefaultCmd(),
		dispatchesCmd(),
		armyAliasCmd(),
		marchAliasCmd(),
		reinforceAliasCmd(),
		recallAliasCmd(),
		redirectAliasCmd(),
		foundingCmd(),
		wantsCmd(),
		riteCmd(),
		abandonCmd(),
		godCmd(),
		reportCmd(),
		reportsCmd(),
		actionsCmd(),
		notificationsCmd(),
		watchCmd(),
	)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

// Agora belongs to the account, surviving world reseeds.
func commandNeedsWorld(cmd *cobra.Command) bool {
	if cmd.Name() == "worlds" || cmd.Name() == "agora" {
		return false
	}
	return !(cmd.Name() == "password" && cmd.Parent() != nil && cmd.Parent().Name() == "agora")
}
