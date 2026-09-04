package auth

import (
	"context"
	"testing"
)

// Proves the documented consequence for pre-existing accounts: their
// password_hash was bcrypt(username) (the old placeholder), so once real
// checking ships their working password is their own username, and nothing
// else, until they set a real one. Not designed UX — a mathematical fact
// worth having a test pin down.
func TestLegacyAccountPasswordIsItsOwnUsername(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	if _, _, err := svc.Login(ctx, "legacyuser", "legacyuser"); err != nil {
		t.Errorf("legacy account login with username-as-password: got %v, want nil", err)
	}
	if _, _, err := svc.Login(ctx, "legacyuser", "wrong"); err != ErrInvalidPassword {
		t.Errorf("legacy account login with wrong password: got %v, want ErrInvalidPassword", err)
	}
}
