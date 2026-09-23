package auth

import (
	"context"
	"errors"
	"testing"
)

func TestChangePassword(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	username := "pwchange-" + uuid8()
	access, refresh, err := svc.Register(ctx, username, "old-secret")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	claims, err := svc.ValidateAccessToken(access)
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.ChangePassword(ctx, claims.PlayerID, "wrong", "new-secret"); !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("change with wrong current password: err = %v, want ErrInvalidPassword", err)
	}
	if _, _, err := svc.Login(ctx, username, "old-secret"); err != nil {
		t.Fatalf("a refused change must leave the old password working: %v", err)
	}

	if err := svc.ChangePassword(ctx, claims.PlayerID, "old-secret", "new-secret"); err != nil {
		t.Fatalf("change: %v", err)
	}
	if _, _, err := svc.Login(ctx, username, "old-secret"); !errors.Is(err, ErrInvalidPassword) {
		t.Errorf("login with the OLD password after change: err = %v, want ErrInvalidPassword", err)
	}
	if _, _, err := svc.Login(ctx, username, "new-secret"); err != nil {
		t.Errorf("login with the new password: %v", err)
	}
	if _, _, err := svc.Refresh(ctx, refresh); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("refresh with a pre-change token: err = %v, want ErrInvalidToken — old sessions must not renew themselves", err)
	}
}
