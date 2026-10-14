package auth

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"
)

const (
	sessionTTL = 12 * time.Hour
	CookieName = "stageneth_sess"
)

type Session struct {
	User    string
	Role    string
	Expires time.Time
}

// Store is an in-memory server-side session store.
type Store struct {
	mu   sync.Mutex
	sess map[string]Session
}

func NewStore() *Store { return &Store{sess: map[string]Session{}} }

// Create issues a new session ID for user/role.
func (s *Store) Create(user, role string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	id := hex.EncodeToString(b)
	s.mu.Lock()
	s.sess[id] = Session{User: user, Role: role, Expires: time.Now().Add(sessionTTL)}
	// opportunistic GC
	for k, v := range s.sess {
		if time.Now().After(v.Expires) {
			delete(s.sess, k)
		}
	}
	s.mu.Unlock()
	return id, nil
}

// Get validates a session and slides its expiry.
func (s *Store) Get(id string) (Session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	se, ok := s.sess[id]
	if !ok || time.Now().After(se.Expires) {
		delete(s.sess, id)
		return Session{}, false
	}
	se.Expires = time.Now().Add(sessionTTL)
	s.sess[id] = se
	return se, true
}

// Delete invalidates a session (logout).
func (s *Store) Delete(id string) {
	s.mu.Lock()
	delete(s.sess, id)
	s.mu.Unlock()
}

// DeleteForUser invalidates every session of a user except keepID.
// Used on password change and user deletion.
func (s *Store) DeleteForUser(user, keepID string) {
	s.mu.Lock()
	for id, se := range s.sess {
		if se.User == user && id != keepID {
			delete(s.sess, id)
		}
	}
	s.mu.Unlock()
}

// SetCookie writes the session cookie.
func SetCookie(w http.ResponseWriter, id string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(ttl.Seconds()),
	})
}

// ClearCookie expires the session cookie.
func ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
}

// SessionFromRequest extracts the session id cookie value.
func SessionFromRequest(r *http.Request) string {
	c, err := r.Cookie(CookieName)
	if err != nil {
		return ""
	}
	return c.Value
}
