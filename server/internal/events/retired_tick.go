package events

import "context"

// RetiredSitosTick accepts legacy scheduled work without touching projections,
// emitting new events or scheduling another pass. Migration184 cancels old rows;
// keeping this compatibility handler also makes delayed delivery harmless.
func RetiredSitosTick(context.Context, ScheduledEvent) error { return nil }
