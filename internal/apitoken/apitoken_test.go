package apitoken

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/cristian/holocron/internal/db"
	"github.com/cristian/holocron/internal/settings"
)

func newStore(t *testing.T) (*Store, *sql.DB) {
	t.Helper()
	database, err := db.Open(context.Background(), t.TempDir()+"/test.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return NewStore(settings.NewStore(database)), database
}

func TestVerifyRejectsWhenNoTokenExists(t *testing.T) {
	t.Parallel()
	s, _ := newStore(t)
	ctx := context.Background()

	if s.Configured(ctx) {
		t.Error("a fresh store should have no token")
	}
	if err := s.Verify(ctx, "anything"); !errors.Is(err, ErrNoToken) {
		t.Errorf("Verify error = %v, want ErrNoToken", err)
	}
}

func TestGenerateThenVerify(t *testing.T) {
	t.Parallel()
	s, _ := newStore(t)
	ctx := context.Background()

	token, err := s.Generate(ctx)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(token) < 40 {
		t.Errorf("token looks too short: %d chars", len(token))
	}
	if !s.Configured(ctx) {
		t.Error("Configured should be true after Generate")
	}
	if err := s.Verify(ctx, token); err != nil {
		t.Errorf("Verify with the right token: %v", err)
	}
	if err := s.Verify(ctx, token+"x"); err == nil {
		t.Error("Verify accepted a wrong token")
	}
}

// The plaintext must not be recoverable from storage: only its digest is kept.
func TestPlaintextIsNotStored(t *testing.T) {
	t.Parallel()
	s, database := newStore(t)
	ctx := context.Background()

	token, err := s.Generate(ctx)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	var stored string
	if err := database.QueryRowContext(ctx,
		`SELECT value FROM settings WHERE key = ?`, settings.KeyAPITokenHash).Scan(&stored); err != nil {
		t.Fatalf("read stored value: %v", err)
	}
	if stored == token {
		t.Fatal("the plaintext token was stored; only its digest should be")
	}
}

func TestGenerateReplacesThePreviousToken(t *testing.T) {
	t.Parallel()
	s, _ := newStore(t)
	ctx := context.Background()

	first, err := s.Generate(ctx)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	second, err := s.Generate(ctx)
	if err != nil {
		t.Fatalf("Generate again: %v", err)
	}
	if first == second {
		t.Fatal("two generated tokens are identical")
	}
	if err := s.Verify(ctx, first); err == nil {
		t.Error("the previous token still verifies after regenerating")
	}
	if err := s.Verify(ctx, second); err != nil {
		t.Errorf("the new token does not verify: %v", err)
	}
}

func TestRevokeClosesTheAPI(t *testing.T) {
	t.Parallel()
	s, _ := newStore(t)
	ctx := context.Background()

	token, err := s.Generate(ctx)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if err := s.Revoke(ctx); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if s.Configured(ctx) {
		t.Error("Configured should be false after Revoke")
	}
	if err := s.Verify(ctx, token); !errors.Is(err, ErrNoToken) {
		t.Errorf("Verify after revoke = %v, want ErrNoToken", err)
	}
}

func TestBearerToken(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"Bearer abc123":  "abc123",
		"bearer abc123":  "abc123", // scheme is case-insensitive
		"Bearer  spaced": "spaced",
		"Basic abc123":   "",
		"abc123":         "",
		"":               "",
		"Bearer":         "",
	}
	for header, want := range cases {
		if got := BearerToken(header); got != want {
			t.Errorf("BearerToken(%q) = %q, want %q", header, got, want)
		}
	}
}
