// Package auth handles player registration, login, and JWT issuance.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrUserNotFound    = errors.New("user not found")
	ErrInvalidPassword = errors.New("invalid password")
	ErrUserExists      = errors.New("username or email already registered")
	ErrInvalidToken    = errors.New("invalid or expired token")
)

// Player is the auth view of a registered user.
type Player struct {
	ID           uuid.UUID
	Username     string
	Email        string
	PasswordHash string
	EraCount     int
	CreatedAt    time.Time
}

// Claims are the JWT payload fields.
type Claims struct {
	PlayerID uuid.UUID `json:"pid"`
	Username string    `json:"username"`
	jwt.RegisteredClaims
}

// Service handles authentication operations.
type Service struct {
	pool      *pgxpool.Pool
	jwtSecret []byte
	accessTTL time.Duration
	refreshTTL time.Duration
}

// NewService creates a Service with the given pool and JWT secret.
func NewService(pool *pgxpool.Pool, jwtSecret string) *Service {
	return &Service{
		pool:       pool,
		jwtSecret:  []byte(jwtSecret),
		accessTTL:  24 * time.Hour,
		refreshTTL: 7 * 24 * time.Hour,
	}
}

// Register creates a new player and returns access + refresh tokens.
// Password is not required in this stage — column kept for future use.
func (s *Service) Register(ctx context.Context, username, email, _ string) (accessToken, refreshToken string, err error) {
	// Store a placeholder hash so the column constraint is satisfied.
	hash, _ := bcrypt.GenerateFromPassword([]byte(username), bcrypt.MinCost)

	var id uuid.UUID
	err = s.pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash)
		 VALUES ($1, $2, $3)
		 RETURNING id`,
		username, email, string(hash),
	).Scan(&id)
	if err != nil {
		return "", "", ErrUserExists
	}

	slog.Info("player registered", "id", id, "username", username)
	return s.issueTokenPair(ctx, id, username)
}

// Login issues a token for any known username — no password check.
func (s *Service) Login(ctx context.Context, usernameOrEmail, _ string) (accessToken, refreshToken string, err error) {
	var p Player
	err = s.pool.QueryRow(ctx,
		`SELECT id, username, email, password_hash, era_count, created_at
		 FROM players WHERE username = $1 OR email = $1`,
		usernameOrEmail,
	).Scan(&p.ID, &p.Username, &p.Email, &p.PasswordHash, &p.EraCount, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrUserNotFound
	}
	if err != nil {
		return "", "", fmt.Errorf("lookup player: %w", err)
	}

	slog.Info("player logged in", "id", p.ID, "username", p.Username)
	return s.issueTokenPair(ctx, p.ID, p.Username)
}

// Refresh validates a refresh token and issues a new token pair.
func (s *Service) Refresh(ctx context.Context, refreshToken string) (newAccess, newRefresh string, err error) {
	hash := hashToken(refreshToken)

	var playerID uuid.UUID
	var username string
	var expiresAt time.Time

	err = s.pool.QueryRow(ctx,
		`SELECT rt.player_id, p.username, rt.expires_at
		 FROM refresh_tokens rt
		 JOIN players p ON p.id = rt.player_id
		 WHERE rt.token_hash = $1`,
		hash,
	).Scan(&playerID, &username, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrInvalidToken
	}
	if err != nil {
		return "", "", fmt.Errorf("lookup refresh token: %w", err)
	}
	if time.Now().After(expiresAt) {
		return "", "", ErrInvalidToken
	}

	// Invalidate old refresh token (rotation).
	if _, err := s.pool.Exec(ctx, `DELETE FROM refresh_tokens WHERE token_hash = $1`, hash); err != nil {
		return "", "", fmt.Errorf("revoke old token: %w", err)
	}

	return s.issueTokenPair(ctx, playerID, username)
}

// ValidateAccessToken parses and validates a JWT access token, returning its claims.
func (s *Service) ValidateAccessToken(tokenStr string) (*Claims, error) {
	t, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.jwtSecret, nil
	})
	if err != nil || !t.Valid {
		return nil, ErrInvalidToken
	}
	claims, ok := t.Claims.(*Claims)
	if !ok {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

// Me returns the player record for a given player ID.
func (s *Service) Me(ctx context.Context, playerID uuid.UUID) (*Player, error) {
	var p Player
	err := s.pool.QueryRow(ctx,
		`SELECT id, username, email, password_hash, era_count, created_at
		 FROM players WHERE id = $1`,
		playerID,
	).Scan(&p.ID, &p.Username, &p.Email, &p.PasswordHash, &p.EraCount, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lookup player: %w", err)
	}
	return &p, nil
}

func (s *Service) issueTokenPair(ctx context.Context, playerID uuid.UUID, username string) (accessToken, refreshToken string, err error) {
	now := time.Now()

	claims := Claims{
		PlayerID: playerID,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.accessTTL)),
			Subject:   playerID.String(),
		},
	}
	accessToken, err = jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.jwtSecret)
	if err != nil {
		return "", "", fmt.Errorf("sign access token: %w", err)
	}

	rawRefresh := make([]byte, 32)
	if _, err := rand.Read(rawRefresh); err != nil {
		return "", "", fmt.Errorf("generate refresh token: %w", err)
	}
	refreshToken = hex.EncodeToString(rawRefresh)
	tokenHash := hashToken(refreshToken)
	expiresAt := now.Add(s.refreshTTL)

	if _, err := s.pool.Exec(ctx,
		`INSERT INTO refresh_tokens (player_id, token_hash, expires_at) VALUES ($1, $2, $3)`,
		playerID, tokenHash, expiresAt,
	); err != nil {
		return "", "", fmt.Errorf("store refresh token: %w", err)
	}

	return accessToken, refreshToken, nil
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
