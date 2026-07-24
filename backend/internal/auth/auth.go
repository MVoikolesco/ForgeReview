// Package auth implements the local, server-side session boundary.
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const CookieName = "forgereview_session"

var ErrInvalidSession = errors.New("invalid or expired session")

type Role string

const (
	RoleViewer Role = "viewer"
	RoleEditor Role = "editor"
	RoleAdmin  Role = "admin"
)

type User struct {
	ID     int64  `json:"id"`
	Email  string `json:"email"`
	Role   Role   `json:"role"`
	Active bool   `json:"active"`
}
type SessionStore interface {
	CreateUser(context.Context, string, string, Role) (User, error)
	UserByEmail(context.Context, string) (User, string, error)
	UserByID(context.Context, int64) (User, error)
	Users(context.Context) ([]User, error)
	UpdateUser(context.Context, int64, Role, bool) (User, error)
	DeleteUserSessions(context.Context, int64) error
	CreateSession(context.Context, string, int64, time.Time) error
	DeleteSession(context.Context, string) error
	SessionValid(context.Context, string, int64, time.Time) (bool, error)
	UserCount(context.Context) (int, error)
}
type Manager struct {
	store SessionStore
	key   []byte
	ttl   time.Duration
	now   func() time.Time
}
type Config struct {
	SigningKey string
	TTL        time.Duration
	Now        func() time.Time
}

func New(store SessionStore, config Config) (*Manager, error) {
	if len(config.SigningKey) < 32 {
		return nil, errors.New("FORGEREVIEW_SESSION_SIGNING_KEY must be at least 32 characters")
	}
	if config.TTL == 0 {
		config.TTL = 8 * time.Hour
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &Manager{store: store, key: []byte(config.SigningKey), ttl: config.TTL, now: config.Now}, nil
}

// Bootstrap creates the first administrator only. Once any user exists, the
// supplied configuration is deliberately ignored so a changed environment can
// never replace an existing account.
func (m *Manager) Bootstrap(ctx context.Context, email, password string) error {
	count, err := m.store.UserCount(ctx)
	if err != nil {
		return err
	}
	if count != 0 {
		return nil
	}
	if email == "" || password == "" {
		return errors.New("FORGEREVIEW_BOOTSTRAP_ADMIN_EMAIL and FORGEREVIEW_BOOTSTRAP_ADMIN_PASSWORD are required when the users table is empty")
	}
	if strings.TrimSpace(email) != email || strings.ToLower(email) != email {
		return errors.New("FORGEREVIEW_BOOTSTRAP_ADMIN_EMAIL must be a trimmed, lowercase email address")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = m.store.CreateUser(ctx, email, string(hash), RoleAdmin)
	return err
}
func (m *Manager) CreateUser(ctx context.Context, email, password string, role Role) (User, error) {
	if !validRole(role) || len(password) < 12 || strings.TrimSpace(email) == "" {
		return User{}, errors.New("email, a password of at least 12 characters, and a valid role are required")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, err
	}
	return m.store.CreateUser(ctx, strings.ToLower(strings.TrimSpace(email)), string(hash), role)
}

// UpdateUser changes an account only when it cannot remove the final active
// administrator. Session nonces are revoked whenever authorization changes.
func (m *Manager) UpdateUser(ctx context.Context, id int64, role Role, active bool) (User, error) {
	if !validRole(role) {
		return User{}, errors.New("a valid role is required")
	}
	user, err := m.store.UpdateUser(ctx, id, role, active)
	if err != nil {
		return User{}, err
	}
	if err = m.store.DeleteUserSessions(ctx, id); err != nil {
		return User{}, err
	}
	return user, nil
}
func (m *Manager) Login(ctx context.Context, email, password string) (User, string, time.Time, error) {
	user, hash, err := m.store.UserByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil || !user.Active || bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return User{}, "", time.Time{}, errors.New("invalid credentials")
	}
	expires := m.now().Add(m.ttl)
	nonce, err := randomToken()
	if err != nil {
		return User{}, "", time.Time{}, err
	}
	if err = m.store.CreateSession(ctx, nonce, user.ID, expires); err != nil {
		return User{}, "", time.Time{}, err
	}
	token, err := m.sign(sessionClaims{UserID: user.ID, Nonce: nonce, ExpiresAt: expires.Unix()})
	return user, token, expires, err
}
func (m *Manager) Authenticate(ctx context.Context, token string) (User, error) {
	claims, err := m.verify(token)
	if err != nil || claims.ExpiresAt <= m.now().Unix() {
		return User{}, ErrInvalidSession
	}
	ok, err := m.store.SessionValid(ctx, claims.Nonce, claims.UserID, m.now())
	if err != nil {
		return User{}, fmt.Errorf("validate session: %w", err)
	}
	if !ok {
		return User{}, ErrInvalidSession
	}
	user, err := m.store.UserByID(ctx, claims.UserID)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrInvalidSession
	}
	if err != nil {
		return User{}, fmt.Errorf("load session user: %w", err)
	}
	if !user.Active {
		return User{}, ErrInvalidSession
	}
	return user, nil
}
func (m *Manager) Logout(ctx context.Context, token string) {
	if claims, err := m.verify(token); err == nil {
		_ = m.store.DeleteSession(ctx, claims.Nonce)
	}
}

type sessionClaims struct {
	UserID    int64  `json:"uid"`
	Nonce     string `json:"nonce"`
	ExpiresAt int64  `json:"exp"`
}

func (m *Manager) sign(claims sessionClaims) (string, error) {
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, m.key)
	mac.Write([]byte(encoded))
	return encoded + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}
func (m *Manager) verify(token string) (sessionClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return sessionClaims{}, errors.New("bad token")
	}
	mac := hmac.New(sha256.New, m.key)
	mac.Write([]byte(parts[0]))
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(sig, mac.Sum(nil)) {
		return sessionClaims{}, errors.New("bad signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return sessionClaims{}, err
	}
	var claims sessionClaims
	err = json.Unmarshal(payload, &claims)
	if claims.UserID < 1 || claims.Nonce == "" || claims.ExpiresAt < 1 {
		return sessionClaims{}, fmt.Errorf("invalid claims")
	}
	return claims, err
}
func randomToken() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b), err
}
func validRole(role Role) bool { return role == RoleViewer || role == RoleEditor || role == RoleAdmin }
