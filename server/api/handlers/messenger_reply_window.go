package handlers

// The saved stay-end timer owns a delivered letter's response window. Reading
// it also preserves stays already issued before tuning ReplyStayTicks. Missing
// legacy timers retain the existing physical 'delivered' permission; no arrival
// time is guessed from arrives_at (which is overwritten on the return leg).
const replyDeadlineSQL = `(SELECT MIN(se.due_tick) FROM scheduled_events se
 WHERE se.world_id=m.world_id AND se.event_type='MessengerStayEnd'
 AND se.payload->>'messenger_id'=m.id::text)`
