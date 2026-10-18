package events

import "context"

// RetiredSitosTick accepts legacy scheduled work without touching projections,
// emitting new events or scheduling another pass. Migration184 cancels old rows;
// keeping this compatibility handler also makes delayed delivery harmless.
func RetiredSitosTick(context.Context, ScheduledEvent) error { return nil }

// RetiredLoyaltyTick accepts each of the four frozen loyalty queue identifiers.
// Migration 185 cancels existing rows; delayed delivery remains a successful,
// idempotent no-op without projection writes, notifications or recurring work.
func RetiredLoyaltyTick(context.Context, ScheduledEvent) error { return nil }
