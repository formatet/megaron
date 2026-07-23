package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// kingdomsDisabledMsg explains why every kingdom verb below is a stub. Kingdoms
// were struck from the MVP chain (Timothy 2026-07-08, see megaron_web_spelbar_plan.md):
// the server capability/CLI surface is disabled while the server-side code stays
// gated for a future reactivation.
const kingdomsDisabledMsg = "Rikesytan är avstängd i denna värld (kingdoms är post-MVP, beslut 2026-07-08)."

// kingdomStubCmd builds a disabled placeholder command for a kingdom verb, so
// `keryx <verb>` prints a clear reason instead of cobra's raw "unknown command".
func kingdomStubCmd(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short + " (disabled — post-MVP)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintln(os.Stderr, kingdomsDisabledMsg)
			os.Exit(1)
			return nil
		},
	}
}

func kingdomsCmd() *cobra.Command {
	return kingdomStubCmd("kingdoms", "List kingdoms")
}

func kingdomFoundCmd() *cobra.Command {
	return kingdomStubCmd("kingdom-found", "Found a new kingdom")
}

func kingdomInviteCmd() *cobra.Command {
	return kingdomStubCmd("kingdom-invite", "Invite a Wanax to your kingdom")
}

func kingdomJoinCmd() *cobra.Command {
	return kingdomStubCmd("kingdom-join", "Join a kingdom you've been invited to")
}

func kingdomInvitationsCmd() *cobra.Command {
	return kingdomStubCmd("kingdom-invitations", "List pending kingdom invitations")
}

func kingdomCouncilCmd() *cobra.Command {
	return kingdomStubCmd("kingdom-council", "List a kingdom's council")
}

func kingdomElectionCmd() *cobra.Command {
	return kingdomStubCmd("kingdom-election", "Show the active election for a kingdom")
}

func kingdomElectionCallCmd() *cobra.Command {
	return kingdomStubCmd("kingdom-election-call", "Call an election for basileus")
}

func kingdomVoteCmd() *cobra.Command {
	return kingdomStubCmd("kingdom-vote", "Cast your vote in an open basileus election")
}

func kingdomTreasuryDepositCmd() *cobra.Command {
	return kingdomStubCmd("kingdom-treasury-deposit", "Send silver to the kingdom treasury")
}

func kingdomBorrowArmyCmd() *cobra.Command {
	return kingdomStubCmd("kingdom-borrow-army", "Borrow units from a kingdom member's settlement")
}
