package handlers

// Count vessels, not timers: land training never occupies a naval build slot.
// A converted ship may still have a legacy TrainComplete timer, so remaining
// shipyard work and the old timer are alternative witnesses for the SAME job.
// Both the early check and the settlement-locked check use this query.
const navalBuildQueueCountSQL = `SELECT COUNT(*) FROM units u
 WHERE u.world_id=$1 AND u.settlement_id=$2
   AND u.category='naval' AND u.status='forming'
   AND (u.shipyard_work_remaining IS NOT NULL OR EXISTS (
     SELECT 1 FROM scheduled_events e
     WHERE e.world_id=$1 AND e.event_type='TrainComplete'
       AND e.processed_at IS NULL AND e.failed_at IS NULL
       AND e.payload->>'settlement_id'=$2::text
       AND e.payload->>'unit_id'=u.id::text
   ))`
