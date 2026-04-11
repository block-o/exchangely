package auth

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

// mockUserRepo is an in-memory implementation of UserRepository for testing.
type mockUserRepo struct {
	mu       sync.RWMutex
	byID     map[uuid.UUID]*User
	byEmail  map[string]*User
	byGoogle map[string]*User
}

func newMockUserRepo() *mockUserRepo {
	return &mockUserRepo{
		byID:     make(map[uuid.UUID]*User),
		byEmail:  make(map[string]*User),
		byGoogle: make(map[string]*User),
	}
}

func (r *mockUserRepo) FindByID(_ context.Context, id uuid.UUID) (*User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	u, ok := r.byID[id]
	if !ok {
		return nil, nil
	}
	cp := *u
	return &cp, nil
}

func (r *mockUserRepo) FindByEmail(_ context.Context, email string) (*User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	u, ok := r.byEmail[email]
	if !ok {
		return nil, nil
	}
	cp := *u
	return &cp, nil
}

func (r *mockUserRepo) FindByGoogleID(_ context.Context, googleID string) (*User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	u, ok := r.byGoogle[googleID]
	if !ok {
		return nil, nil
	}
	cp := *u
	return &cp, nil
}

func (r *mockUserRepo) Create(_ context.Context, user *User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *user
	if cp.PasswordHash != nil {
		h := *cp.PasswordHash
		cp.PasswordHash = &h
	}
	if cp.GoogleID != nil {
		g := *cp.GoogleID
		cp.GoogleID = &g
	}
	r.byID[cp.ID] = &cp
	r.byEmail[cp.Email] = &cp
	if cp.GoogleID != nil {
		r.byGoogle[*cp.GoogleID] = &cp
	}
	return nil
}

func (r *mockUserRepo) Update(_ context.Context, user *User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *user
	if cp.PasswordHash != nil {
		h := *cp.PasswordHash
		cp.PasswordHash = &h
	}
	if cp.GoogleID != nil {
		g := *cp.GoogleID
		cp.GoogleID = &g
	}
	r.byID[cp.ID] = &cp
	r.byEmail[cp.Email] = &cp
	if cp.GoogleID != nil {
		r.byGoogle[*cp.GoogleID] = &cp
	}
	return nil
}

func (r *mockUserRepo) UpdatePasswordHash(_ context.Context, userID uuid.UUID, hash string, mustChange bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.byID[userID]
	if !ok {
		return nil
	}
	u.PasswordHash = &hash
	u.MustChangePassword = mustChange
	return nil
}

// mockSessionRepo is an in-memory implementation of SessionRepository for testing.
type mockSessionRepo struct {
	mu       sync.RWMutex
	byID     map[uuid.UUID]*Session
	byHash   map[string]*Session
	byUserID map[uuid.UUID][]*Session
}

func newMockSessionRepo() *mockSessionRepo {
	return &mockSessionRepo{
		byID:     make(map[uuid.UUID]*Session),
		byHash:   make(map[string]*Session),
		byUserID: make(map[uuid.UUID][]*Session),
	}
}

func (r *mockSessionRepo) Create(_ context.Context, session *Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *session
	r.byID[cp.ID] = &cp
	r.byHash[cp.RefreshTokenHash] = &cp
	r.byUserID[cp.UserID] = append(r.byUserID[cp.UserID], &cp)
	return nil
}

func (r *mockSessionRepo) FindByTokenHash(_ context.Context, tokenHash string) (*Session, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.byHash[tokenHash]
	if !ok {
		return nil, nil
	}
	cp := *s
	return &cp, nil
}

func (r *mockSessionRepo) Delete(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.byID[id]
	if !ok {
		return nil
	}
	delete(r.byID, id)
	delete(r.byHash, s.RefreshTokenHash)
	// Remove from byUserID slice.
	sessions := r.byUserID[s.UserID]
	for i, sess := range sessions {
		if sess.ID == id {
			r.byUserID[s.UserID] = append(sessions[:i], sessions[i+1:]...)
			break
		}
	}
	return nil
}

func (r *mockSessionRepo) DeleteAllForUser(_ context.Context, userID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	sessions := r.byUserID[userID]
	for _, s := range sessions {
		delete(r.byID, s.ID)
		delete(r.byHash, s.RefreshTokenHash)
	}
	delete(r.byUserID, userID)
	return nil
}

func (r *mockSessionRepo) DeleteExpired(_ context.Context) (int64, error) {
	return 0, nil
}

// sessionsForUser returns the count of sessions for a given user.
func (r *mockSessionRepo) sessionsForUser(userID uuid.UUID) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.byUserID[userID])
}

// mockAPITokenRepo is an in-memory implementation of APITokenRepository for testing.
type mockAPITokenRepo struct {
	mu     sync.RWMutex
	byID   map[uuid.UUID]*APIToken
	byHash map[string]*APIToken
}

func newMockAPITokenRepo() *mockAPITokenRepo {
	return &mockAPITokenRepo{
		byID:   make(map[uuid.UUID]*APIToken),
		byHash: make(map[string]*APIToken),
	}
}

func (r *mockAPITokenRepo) Create(_ context.Context, token *APIToken) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *token
	r.byID[cp.ID] = &cp
	r.byHash[cp.TokenHash] = &cp
	return nil
}

func (r *mockAPITokenRepo) FindByTokenHash(_ context.Context, tokenHash string) (*APIToken, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.byHash[tokenHash]
	if !ok {
		return nil, nil
	}
	cp := *t
	return &cp, nil
}

func (r *mockAPITokenRepo) ListByUserID(_ context.Context, userID uuid.UUID) ([]APIToken, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var tokens []APIToken
	for _, t := range r.byID {
		if t.UserID == userID {
			tokens = append(tokens, *t)
		}
	}
	sort.Slice(tokens, func(i, j int) bool {
		return tokens[i].CreatedAt.After(tokens[j].CreatedAt)
	})
	return tokens, nil
}

func (r *mockAPITokenRepo) CountActiveByUserID(_ context.Context, userID uuid.UUID) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	now := time.Now()
	count := 0
	for _, t := range r.byID {
		if t.UserID == userID && t.RevokedAt == nil && t.ExpiresAt.After(now) {
			count++
		}
	}
	return count, nil
}

func (r *mockAPITokenRepo) Revoke(_ context.Context, id uuid.UUID, userID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.byID[id]
	if !ok || t.UserID != userID {
		return ErrTokenNotFound
	}
	// Idempotent: if already revoked, do nothing.
	if t.RevokedAt != nil {
		return nil
	}
	now := time.Now()
	t.RevokedAt = &now
	return nil
}

func (r *mockAPITokenRepo) UpdateLastUsedAt(_ context.Context, id uuid.UUID, t time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	tok, ok := r.byID[id]
	if !ok {
		return nil
	}
	tok.LastUsedAt = &t
	return nil
}
