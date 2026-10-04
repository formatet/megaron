package main

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"formatet/megaron/server/api/handlers"
	"formatet/megaron/server/internal/agora"
	"formatet/megaron/server/internal/notify"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type agoraRemote interface {
	Initialize(context.Context) error
	Homeserver() string
	UserID(string) string
	Create(context.Context, string, string, string) error
	ResetPassword(context.Context, string, string, string) error
	SetDisplayName(context.Context, string, string, string) error
}

type agoraService struct {
	pool   *pgxpool.Pool
	remote agoraRemote
	room   string
	hub    *notify.Hub
}

func (s *agoraService) Account(ctx context.Context, playerID uuid.UUID) (handlers.AgoraAccount, error) {
	var state, localpart string
	if err := s.pool.QueryRow(ctx, `SELECT COALESCE(agora_state,''),COALESCE(agora_localpart,'') FROM players WHERE id=$1`, playerID).Scan(&state, &localpart); err != nil {
		return handlers.AgoraAccount{}, agora.ErrUnavailable
	}
	account := handlers.AgoraAccount{Enabled: true, State: "pending", Homeserver: s.remote.Homeserver()}
	if state == "creating" {
		account.State = "provisioning"
	}
	if state == "ready" {
		if err := s.remote.Initialize(ctx); err != nil {
			return handlers.AgoraAccount{}, err
		}
		account.State = "ready"
		account.Localpart = localpart
		account.UserID = s.remote.UserID(localpart)
	}
	return account, nil
}

func agoraLockKey(value string) int64 {
	hash := sha256.Sum256([]byte("megaron-agora:" + value))
	return int64(binary.BigEndian.Uint64(hash[:8]))
}

// Session locks cover external I/O without holding a database transaction. The
// dedicated connection is never returned to the pool until every lock is gone.
func (s *agoraService) lock(ctx context.Context, playerID uuid.UUID) (*pgxpool.Conn, error) {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return nil, agora.ErrUnavailable
	}
	for _, key := range []int64{agoraLockKey("player:" + playerID.String()), agoraLockKey("room:" + s.room)} {
		if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, key); err != nil {
			s.unlock(conn)
			return nil, agora.ErrUnavailable
		}
	}
	return conn, nil
}

func (s *agoraService) unlock(conn *pgxpool.Conn) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_unlock_all()`); err != nil {
		// A failed unlock must destroy the session rather than leak a held lock
		// into a later pool user's connection.
		_ = conn.Conn().Close(ctx)
	}
	conn.Release()
}

func (s *agoraService) Password(ctx context.Context, playerID uuid.UUID) (handlers.AgoraPassword, error) {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	conn, err := s.lock(ctx, playerID)
	if err != nil {
		return handlers.AgoraPassword{}, err
	}
	defer s.unlock(conn)
	var localpart string
	if err := conn.QueryRow(ctx, `SELECT COALESCE(agora_localpart,'') FROM players WHERE id=$1 AND agora_state='ready'`, playerID).Scan(&localpart); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return handlers.AgoraPassword{}, handlers.ErrAgoraPending
		}
		return handlers.AgoraPassword{}, agora.ErrUnavailable
	}
	password, err := agora.NewPassword()
	if err != nil {
		return handlers.AgoraPassword{}, err
	}
	// Every explicit player request is a new reset, unlike the durable CREATE.
	if err := s.remote.ResetPassword(ctx, "password-"+uuid.NewString(), localpart, password); err != nil {
		return handlers.AgoraPassword{}, err
	}
	return handlers.AgoraPassword{Password: password, UserID: s.remote.UserID(localpart), Homeserver: s.remote.Homeserver()}, nil
}

func (s *agoraService) run(ctx context.Context) {
	s.pass(ctx)
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.pass(ctx)
		}
	}
}

func (s *agoraService) pass(ctx context.Context) {
	rows, err := s.pool.Query(ctx, `SELECT p.id FROM players p
		WHERE p.agora_localpart IS NULL AND p.wanax_name IS NOT NULL
		AND (p.agora_claim_localpart IS NOT NULL OR EXISTS(SELECT 1 FROM settlements s WHERE s.owner_id=p.id))
		ORDER BY p.id`)
	if err != nil {
		slog.Warn("Agora reconciliation query failed")
		return
	}
	var players []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if rows.Scan(&id) != nil {
			rows.Close()
			slog.Warn("Agora reconciliation scan failed")
			return
		}
		players = append(players, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		slog.Warn("Agora reconciliation query failed")
		return
	}
	for _, id := range players {
		if ctx.Err() != nil {
			return
		}
		playerCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err := s.reconcile(playerCtx, id)
		cancel()
		if err != nil {
			slog.Warn("Agora account awaits reconciliation", "player", id.String())
		}
	}
}

func (s *agoraService) reconcile(ctx context.Context, playerID uuid.UUID) error {
	conn, err := s.lock(ctx, playerID)
	if err != nil {
		return err
	}
	var afterUnlock func()
	defer func() {
		s.unlock(conn)
		// DeliverPersisted obtains a separate pool connection for the mute
		// check. Release ours first so waiting room-lock holders cannot starve
		// this delivery of every available database connection.
		if afterUnlock != nil {
			afterUnlock()
		}
	}()
	var name, claim, localpart string
	var eligible bool
	if err := conn.QueryRow(ctx, `SELECT COALESCE(wanax_name,''),COALESCE(agora_claim_localpart,''),COALESCE(agora_localpart,''),
		EXISTS(SELECT 1 FROM settlements WHERE owner_id=$1) FROM players WHERE id=$1`, playerID).Scan(&name, &claim, &localpart, &eligible); err != nil {
		return agora.ErrUnavailable
	}
	if localpart != "" {
		return nil
	}
	if claim == "" && !eligible {
		return nil
	}
	base, err := agora.Localpart(name)
	if err != nil {
		return err
	}
	// Bounded work per pass; repeated passes retain the exact current candidate.
	for attempts := 0; attempts < 100; attempts++ {
		if claim == "" {
			claim, err = s.reserve(ctx, conn, playerID, base, 1)
			if err != nil {
				return err
			}
		}
		password, err := agora.NewPassword()
		if err != nil {
			return err
		}
		// This stable transaction ID plus its ORIGINAL correlated successful
		// reply is the sole proof that this game account owns the remote account.
		err = s.remote.Create(ctx, "create-"+playerID.String()+"-"+claim, claim, password)
		if errors.Is(err, agora.ErrOccupied) {
			next := 2
			if claim != base {
				if n, parseErr := strconv.Atoi(claim[len(base):]); parseErr == nil {
					next = n + 1
				}
			}
			claim, err = s.reserve(ctx, conn, playerID, base, next)
			if err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		// Cached CREATE may have used another ephemeral password before a crash.
		// A new reset enables profile setup without retaining that earlier secret.
		if err := s.remote.ResetPassword(ctx, "profile-"+uuid.NewString(), claim, password); err != nil {
			return err
		}
		if err := s.remote.SetDisplayName(ctx, claim, password, name); err != nil {
			return err
		}
		return s.ready(ctx, conn, playerID, claim, &afterUnlock)
	}
	return agora.ErrUnavailable
}

func (s *agoraService) reserve(ctx context.Context, conn *pgxpool.Conn, playerID uuid.UUID, base string, start int) (string, error) {
	for suffix := start; suffix < start+1000; suffix++ {
		candidate := base
		if suffix > 1 {
			candidate += strconv.Itoa(suffix)
		}
		result, err := conn.Exec(ctx, `UPDATE players SET agora_claim_localpart=$2,agora_state='creating' WHERE id=$1 AND agora_localpart IS NULL`, playerID, candidate)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				continue
			}
			return "", agora.ErrUnavailable
		}
		if result.RowsAffected() != 1 {
			return "", agora.ErrUnavailable
		}
		return candidate, nil
	}
	return "", agora.ErrUnavailable
}

func (s *agoraService) ready(ctx context.Context, conn *pgxpool.Conn, playerID uuid.UUID, claim string, afterUnlock *func()) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return agora.ErrUnavailable
	}
	defer tx.Rollback(ctx)
	var worldID uuid.UUID
	// Ownership can change during remote I/O. Lock an actually owned settlement
	// only for the short final transaction; a lost last city retains the claim
	// for recovery without emitting a notice into a guessed world.
	if err := tx.QueryRow(ctx, `SELECT world_id FROM settlements WHERE owner_id=$1 ORDER BY world_id,id LIMIT 1 FOR UPDATE`, playerID).Scan(&worldID); err != nil {
		return agora.ErrUnavailable
	}
	result, err := tx.Exec(ctx, `UPDATE players SET agora_localpart=$2,agora_state='ready' WHERE id=$1 AND agora_state='creating' AND agora_claim_localpart=$2 AND agora_localpart IS NULL`, playerID, claim)
	if err != nil || result.RowsAffected() != 1 {
		return agora.ErrUnavailable
	}
	body, err := json.Marshal(map[string]string{"user_id": s.remote.UserID(claim), "homeserver": s.remote.Homeserver(), "text": "Your community chat account is ready. Get your chat password in the account window."})
	if err != nil {
		return agora.ErrUnavailable
	}
	var notificationID uuid.UUID
	if err := tx.QueryRow(ctx, `INSERT INTO notifications(world_id,player_id,kind,level,body_json) VALUES($1,$2,'agora_ready',4,$3) RETURNING id`, worldID, playerID, body).Scan(&notificationID); err != nil {
		return agora.ErrUnavailable
	}
	if tx.Commit(ctx) != nil {
		return agora.ErrUnavailable
	}
	if s.hub != nil {
		*afterUnlock = func() {
			s.hub.DeliverPersisted(ctx, worldID, playerID, notify.Msg{Kind: "agora_ready", WorldID: worldID.String(), ID: notificationID.String(), Level: 4, Payload: json.RawMessage(body)})
		}
	}
	return nil
}
