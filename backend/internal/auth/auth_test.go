package auth_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"forgereview/backend/internal/auth"
	"forgereview/backend/internal/store"
)

func TestBootstrapHashesOnlyFirstAdminAndSessionsExpire(t *testing.T) {
	db, err := store.Open("file:" + t.TempDir() + "/auth.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	manager, err := auth.New(db, auth.Config{SigningKey: strings.Repeat("s", 32), TTL: time.Hour, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	if err = manager.Bootstrap(context.Background(), "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	if err = manager.Bootstrap(context.Background(), "another password which must not replace"); err != nil {
		t.Fatal(err)
	}
	user, hash, err := db.UserByEmail(context.Background(), "admin@localhost")
	if err != nil || user.Role != auth.RoleAdmin || hash == "correct horse battery staple" {
		t.Fatalf("bootstrap user = %#v, %q, %v", user, hash, err)
	}
	_, token, _, err := manager.Login(context.Background(), user.Email, "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manager.Authenticate(context.Background(), token+"x"); err == nil {
		t.Fatal("tampered signature authenticated")
	}
	now = now.Add(2 * time.Hour)
	if _, err = manager.Authenticate(context.Background(), token); err == nil {
		t.Fatal("expired session authenticated")
	}
}

var _ = sql.ErrNoRows
