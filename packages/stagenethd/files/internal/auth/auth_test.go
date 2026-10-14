package auth

import (
	"path/filepath"
	"testing"
	"time"
)

func TestPasswordRoundTrip(t *testing.T) {
	h, err := HashPassword("showtime")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword("showtime", h) {
		t.Error("correct password rejected")
	}
	if VerifyPassword("wrong", h) {
		t.Error("wrong password accepted")
	}
	if VerifyPassword("showtime", "garbage") {
		t.Error("malformed hash accepted")
	}
}

func TestUsersLifecycle(t *testing.T) {
	u := NewUsers(filepath.Join(t.TempDir(), "stagenethd"))
	if u.HasUsers() {
		t.Fatal("fresh db should be empty")
	}
	if err := u.Set("admin", "tech", "pw12345678"); err != nil {
		t.Fatal(err)
	}
	if err := u.Set("viewer", "operator", "pw12345678"); err != nil {
		t.Fatal(err)
	}
	if err := u.Set("bad name!", "tech", "pw12345678"); err != ErrBadName {
		t.Error("invalid name accepted")
	}
	if err := u.Set("x", "root", "pw12345678"); err != ErrBadRole {
		t.Error("invalid role accepted")
	}
	if err := u.Set("weak", "tech", "short"); err != ErrWeakPassword {
		t.Error("short password accepted")
	}
	role, err := u.Verify("admin", "pw12345678")
	if err != nil || role != "tech" {
		t.Errorf("verify admin: %v %q", err, role)
	}
	if _, err := u.Verify("admin", "nope"); err == nil {
		t.Error("wrong password verified")
	}
	if _, err := u.Verify("ghost", "pw12345678"); err != ErrUserNotFound {
		t.Error("missing user verified")
	}
	if err := u.SetPassword("viewer", "newpw5678"); err != nil {
		t.Fatal(err)
	}
	if _, err := u.Verify("viewer", "newpw5678"); err != nil {
		t.Error("password update failed")
	}
	if err := u.Remove("viewer"); err != nil {
		t.Fatal(err)
	}
	if err := u.Remove("admin"); err == nil {
		t.Error("last user removal allowed")
	}
	if got := u.List(); len(got) != 1 || got[0][0] != "admin" {
		t.Errorf("list: %v", got)
	}
}

func TestSessions(t *testing.T) {
	s := NewStore()
	id, err := s.Create("admin", "tech")
	if err != nil {
		t.Fatal(err)
	}
	se, ok := s.Get(id)
	if !ok || se.User != "admin" || se.Role != "tech" {
		t.Fatal("session not found")
	}
	if _, ok := s.Get("bogus"); ok {
		t.Error("bogus session valid")
	}
	s.Delete(id)
	if _, ok := s.Get(id); ok {
		t.Error("deleted session still valid")
	}
}

func TestDeleteForUser(t *testing.T) {
	s := NewStore()
	a1, _ := s.Create("alice", "tech")
	a2, _ := s.Create("alice", "tech")
	b1, _ := s.Create("bob", "operator")
	s.DeleteForUser("alice", a2)
	if _, ok := s.Get(a1); ok {
		t.Error("a1 should be invalidated")
	}
	if _, ok := s.Get(a2); !ok {
		t.Error("a2 should be kept")
	}
	if _, ok := s.Get(b1); !ok {
		t.Error("bob's session must survive")
	}
	s.DeleteForUser("alice", "")
	if _, ok := s.Get(a2); ok {
		t.Error("a2 should be invalidated with empty keepID")
	}
}

func TestSessionExpiry(t *testing.T) {
	s := NewStore()
	id, _ := s.Create("a", "operator")
	s.mu.Lock()
	se := s.sess[id]
	se.Expires = time.Now().Add(-time.Hour)
	s.sess[id] = se
	s.mu.Unlock()
	if _, ok := s.Get(id); ok {
		t.Error("expired session still valid")
	}
}
