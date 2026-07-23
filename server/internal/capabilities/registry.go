package capabilities

import (
	"context"
	"os"

	"formatet/megaron/server/internal/clock"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// checkers lists every mutating verb's checker, in the fixed display order
// used by `keryx actions <category>` and the --json output. Keep this in
// sync with temenos_capabilities.md's verb registry when a new mutating verb
// is added to cmd/keryx or main.go's routes.
var checkers = []func(checkContext) Verb{
	// province
	canBuild,
	canCancelBuild,
	canAllocate,
	canCraft,
	canRecruit,
	canAbandon,
	// military
	canMarch,
	canRecall,
	canRedirect,
	canStance,
	canLoad,
	canUnload,
	canDisband,
	canColonize,
	// trade
	canTradeOffer,
	canSell,
	canTradeAccept,
	canTradeDecline,
	canTradeCancel,
	canTransfer,
	canGift,
	// diplomacy
	canMessage,
	canReply,
	canMessenger,
	// kingdom
	canKingdomFound,
	canKingdomInvite,
	canKingdomJoin,
	canKingdomVote,
	canKingdomElectionCall,
	canKingdomTreasuryDeposit,
	canKingdomBorrowArmy,
	canKingdomCouncil,
	// cult
	canRite,
}

// List returns every mutating verb's capability for the given province, in
// registry order. settlementID may be uuid.Nil if the province has no
// settlement — settlement-scoped verbs then simply report themselves
// unsatisfied rather than erroring.
func List(ctx context.Context, pool *pgxpool.Pool, clk clock.Clock, worldID, provinceID, playerID, settlementID uuid.UUID) []Verb {
	cc := checkContext{
		ctx:          ctx,
		pool:         pool,
		clk:          clk,
		worldID:      worldID,
		provinceID:   provinceID,
		playerID:     playerID,
		settlementID: settlementID,
	}
	// Kingdoms are post-MVP (Timothy 2026-07-08) — the category vanishes from
	// the actions response entirely unless explicitly re-enabled, so keryx and
	// agent.py never see it (server = enda sanning).
	kingdomsEnabled := os.Getenv("KINGDOMS_ENABLED") != ""

	verbs := make([]Verb, 0, len(checkers))
	for _, fn := range checkers {
		v := fn(cc)
		if v.Category == CategoryKingdom && !kingdomsEnabled {
			continue
		}
		verbs = append(verbs, v)
	}
	return verbs
}
