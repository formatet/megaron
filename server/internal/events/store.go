// Package events implements the event sourcing core for Poleia.
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

// Store is the append-only event log.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a Store backed by the given connection pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
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
