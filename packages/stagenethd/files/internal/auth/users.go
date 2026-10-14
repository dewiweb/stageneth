package auth

import (
	"errors"
	"os"
	"path/filepath"

	"stageneth/internal/ucifile"
)

// Users manages the application user database stored as
// `config user '<name>'` sections in a UCI file (e.g. /etc/config/stagenethd).
//
//	config user 'admin'
//		option role 'tech'
//		option password '$argon2id$...'
type Users struct {
	path string
}

func NewUsers(path string) *Users { return &Users{path: path} }

var (
	ErrUserNotFound = errors.New("user not found")
	ErrBadRole      = errors.New("invalid role")
	ErrBadName      = errors.New("invalid username")
	ErrWeakPassword = errors.New("password too short")
)

// MinPasswordLen is enforced on every password write path.
const MinPasswordLen = 8

// dummyHash is a valid argon2id PHC string verified against when the
// requested user does not exist, so login timing does not leak which
// usernames are registered.
var dummyHash string

func init() {
	h, err := HashPassword("stageneth-dummy")
	if err != nil {
		panic(err)
	}
	dummyHash = h
}

// Roles: operator = read + apply; tech = full access.
func ValidRole(role string) bool { return role == "operator" || role == "tech" }

func validName(name string) bool {
	if name == "" || len(name) > 32 {
		return false
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.') {
			return false
		}
	}
	return true
}

func (u *Users) load() (*ucifile.File, error) {
	f, err := ucifile.Read(u.path)
	if err != nil {
		if os.IsNotExist(err) {
			return &ucifile.File{}, nil
		}
		return nil, err
	}
	return f, nil
}

func (u *Users) find(f *ucifile.File, name string) *ucifile.Section {
	for i := range f.Sections {
		if f.Sections[i].Type == "user" && f.Sections[i].Name == name {
			return &f.Sections[i]
		}
	}
	return nil
}

func (u *Users) save(f *ucifile.File) error {
	tmp := u.path + ".tmp"
	b, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	if err := f.Write(b); err != nil {
		b.Close()
		return err
	}
	if err := b.Sync(); err != nil {
		b.Close()
		return err
	}
	if err := b.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, u.path)
}

// HasUsers reports whether at least one user exists (firstboot detection).
func (u *Users) HasUsers() bool {
	f, err := u.load()
	if err != nil {
		return false
	}
	for _, s := range f.Sections {
		if s.Type == "user" {
			return true
		}
	}
	return false
}

// List returns usernames and roles (no hashes).
func (u *Users) List() [][2]string {
	f, err := u.load()
	if err != nil {
		return nil
	}
	var out [][2]string
	for _, s := range f.Sections {
		if s.Type == "user" && s.Name != "" {
			out = append(out, [2]string{s.Name, s.Get("role")})
		}
	}
	return out
}

// Set creates or updates a user.
func (u *Users) Set(name, role, password string) error {
	if !validName(name) {
		return ErrBadName
	}
	if !ValidRole(role) {
		return ErrBadRole
	}
	if len(password) < MinPasswordLen {
		return ErrWeakPassword
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	f, err := u.load()
	if err != nil {
		return err
	}
	s := u.find(f, name)
	if s == nil {
		f.Sections = append(f.Sections, ucifile.Section{Type: "user", Name: name})
		s = &f.Sections[len(f.Sections)-1]
	}
	s.Set("role", role)
	s.Set("password", hash)
	return u.save(f)
}

// SetPassword updates only the password of an existing user.
func (u *Users) SetPassword(name, password string) error {
	if len(password) < MinPasswordLen {
		return ErrWeakPassword
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	f, err := u.load()
	if err != nil {
		return err
	}
	s := u.find(f, name)
	if s == nil {
		return ErrUserNotFound
	}
	s.Set("password", hash)
	return u.save(f)
}

// Remove deletes a user. Refuses to remove the last remaining user.
func (u *Users) Remove(name string) error {
	f, err := u.load()
	if err != nil {
		return err
	}
	users := 0
	for _, s := range f.Sections {
		if s.Type == "user" {
			users++
		}
	}
	if u.find(f, name) == nil {
		return ErrUserNotFound
	}
	if users <= 1 {
		return errors.New("cannot remove the last user")
	}
	out := f.Sections[:0]
	for _, s := range f.Sections {
		if !(s.Type == "user" && s.Name == name) {
			out = append(out, s)
		}
	}
	f.Sections = out
	return u.save(f)
}

// Verify checks a password and returns the user's role. Always runs a
// full argon2 verification — even for unknown users — so the response
// time does not reveal whether the account exists.
func (u *Users) Verify(name, password string) (string, error) {
	f, err := u.load()
	if err != nil {
		return "", err
	}
	s := u.find(f, name)
	if s == nil {
		// burn a full argon2 verification so timing doesn't leak
		// whether the account exists
		_ = VerifyPassword(password, dummyHash)
		return "", ErrUserNotFound
	}
	if !VerifyPassword(password, s.Get("password")) {
		return "", ErrNoPassword
	}
	role := s.Get("role")
	if !ValidRole(role) {
		role = "tech" // legacy/unknown role: keep permissive
	}
	return role, nil
}

// Path returns the config path (for tests).
func (u *Users) Path() string { return filepath.Clean(u.path) }
