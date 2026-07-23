// Package events implements the event sourcing core for Megaron.
// All game state changes are recorded as immutable events; projections are derived from the log.
package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// StreamType identifies what entity an event belongs to.
type StreamType string

const (
	StreamProvince StreamType = "province"
	StreamKingdom  StreamType = "kingdom"
	StreamWorld    StreamType = "world"
	StreamCombat   StreamType = "combat"
	StreamReligion StreamType = "religion"
)

// Event is a persisted domain event.
type Event struct {
	ID         int64
	StreamID   uuid.UUID
	StreamType StreamType
	EventType  string
	Payload    json.RawMessage
	Causation  *int64
	WorldID    uuid.UUID
	CreatedAt  time.Time
}

// Sink receives a copy of every event after successful Append. Sinks must not
// panic or block — they run synchronously on the writer's goroutine. Use them
// for logging, chronicling, or fan-out to side channels. A Sink that returns
// an error is logged but never breaks the event write.
type Sink interface {
	Record(ctx context.Context, e SinkEvent)
}

// SinkEvent is the shape passed to sinks. Mirrors Event but lives here so
// downstream packages don't need to import events just to consume them.
type SinkEvent struct {
	ID         int64
	StreamID   uuid.UUID
	StreamType string
	EventType  string
	Payload    json.RawMessage
	WorldID    uuid.UUID
	CreatedAt  time.Time
}

// Store is the append-only event log.
type Store struct {
	pool  *pgxpool.Pool
	sinks []Sink
}

// NewStore creates a Store backed by the given connection pool. Optional sinks
// receive every event after a successful Append.
func NewStore(pool *pgxpool.Pool, sinks ...Sink) *Store {
	return &Store{pool: pool, sinks: sinks}
}

// Append writes a single event to the log and returns it with its assigned ID.
func (s *Store) Append(ctx context.Context, streamID uuid.UUID, streamType StreamType, eventType string, payload any, worldID uuid.UUID, causation *int64) (*Event, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	var e Event
	err = s.pool.QueryRow(ctx,
		`INSERT INTO events (stream_id, stream_type, event_type, payload, causation, world_id)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, stream_id, stream_type, event_type, payload, causation, world_id, created_at`,
		streamID, string(streamType), eventType, raw, causation, worldID,
	).Scan(&e.ID, &e.StreamID, &e.StreamType, &e.EventType, &e.Payload, &e.Causation, &e.WorldID, &e.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert event: %w", err)
	}

	slog.Debug("event appended", "type", eventType, "stream", streamID, "world", worldID)

	if len(s.sinks) > 0 {
		se := SinkEvent{
			ID:         e.ID,
			StreamID:   e.StreamID,
			StreamType: string(e.StreamType),
			EventType:  e.EventType,
			Payload:    e.Payload,
			WorldID:    e.WorldID,
			CreatedAt:  e.CreatedAt,
		}
		for _, sink := range s.sinks {
			func() {
				defer func() {
					if r := recover(); r != nil {
						slog.Error("event sink panic", "err", r, "type", eventType, "event_id", e.ID)
					}
				}()
				sink.Record(ctx, se)
			}()
		}
	}

	return &e, nil
}

// LoadStream returns all events for a stream in order.
func (s *Store) LoadStream(ctx context.Context, streamID uuid.UUID) ([]Event, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, stream_id, stream_type, event_type, payload, causation, world_id, created_at
		 FROM events WHERE stream_id = $1 ORDER BY id`,
		streamID,
	)
	if err != nil {
		return nil, fmt.Errorf("query stream: %w", err)
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.StreamID, &e.StreamType, &e.EventType, &e.Payload, &e.Causation, &e.WorldID, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// LoadWorldEvents returns all events for a world since a given event ID (for streaming).
func (s *Store) LoadWorldEvents(ctx context.Context, worldID uuid.UUID, afterID int64, limit int) ([]Event, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, stream_id, stream_type, event_type, payload, causation, world_id, created_at
		 FROM events WHERE world_id = $1 AND id > $2 ORDER BY id LIMIT $3`,
		worldID, afterID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query world events: %w", err)
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.StreamID, &e.StreamType, &e.EventType, &e.Payload, &e.Causation, &e.WorldID, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		events = append(events, e)
	}
	return events, rows.Err()
}
