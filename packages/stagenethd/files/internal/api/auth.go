package api

import (
	json "encoding/json"
	http "net/http"
	sync "sync"
	time "time"

	"stageneth/internal/auth"
)

// --- login rate limiting -----------------------------------------------------

const (
	loginMaxFails   = 8
	loginFailWindow = 10 * time.Minute
)

var loginLimiter = struct {
	sync.Mutex
	fails map[string][]time.Time
}{fails: map[string][]time.Time{}}

func clientIP(r *http.Request) string {
	// Only X-Real-IP is trusted: nginx overwrites it with $remote_addr.
	// X-Forwarded-For is client-controlled (nginx doesn't set it), so
	// honoring it would let an attacker rotate fake IPs past the limiter.
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	return r.RemoteAddr
}

func loginThrottled(ip string) bool {
	loginLimiter.Lock()
	defer loginLimiter.Unlock()
	now := time.Now()
	fails := loginLimiter.fails[ip]
	keep := fails[:0]
	for _, t := range fails {
		if now.Sub(t) < loginFailWindow {
			keep = append(keep, t)
		}
	}
	if len(keep) == 0 {
		delete(loginLimiter.fails, ip)
		return false
	}
	loginLimiter.fails[ip] = keep
	return len(keep) >= loginMaxFails
}

func loginFailed(ip string) {
	loginLimiter.Lock()
	// hard cap: unique-IP scanning must not grow the map forever —
	// evict stale entries first, then arbitrary ones until under cap
	if len(loginLimiter.fails) >= 1024 {
		for k, v := range loginLimiter.fails {
			if len(v) == 0 || time.Since(v[len(v)-1]) > loginFailWindow {
				delete(loginLimiter.fails, k)
			}
		}
		for k := range loginLimiter.fails {
			if len(loginLimiter.fails) < 1024 {
				break
			}
			delete(loginLimiter.fails, k)
		}
	}
	loginLimiter.fails[ip] = append(loginLimiter.fails[ip], time.Now())
	loginLimiter.Unlock()
}

// --- session endpoints -------------------------------------------------------

func sessionLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respond(w, 405, nil, "method not allowed")
		return
	}
	ip := clientIP(r)
	if loginThrottled(ip) {
		respond(w, 429, nil, "too many login attempts")
		return
	}
	var req loginRequest
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.Username == "" || req.Password == "" {
		respond(w, 400, nil, "invalid login request")
		return
	}
	role, err := usersDB.Verify(req.Username, req.Password)
	if err != nil {
		loginFailed(ip)
		audit("login_failed", req.Username, r)
		respond(w, 401, nil, "invalid credentials")
		return
	}
	id, err := sessions.Create(req.Username, role)
	if err != nil {
		respond(w, 500, nil, "session error")
		return
	}
	auth.SetCookie(w, id, 12*time.Hour)
	audit("login", req.Username+" role="+role, r)
	respond(w, 200, map[string]string{"user": req.Username, "role": role}, "login success")
}

func sessionLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		respond(w, 405, nil, "method not allowed")
		return
	}
	if id := auth.SessionFromRequest(r); id != "" {
		sessions.Delete(id)
	}
	auth.ClearCookie(w)
	respond(w, 200, nil, "logged out")
}

func sessionStatus(w http.ResponseWriter, r *http.Request) {
	se, ok := sessionOf(r)
	respond(w, 200, map[string]interface{}{
		"authenticated": ok,
		"user":          se.User,
		"role":          se.Role,
	}, "ok")
}

// sessionDispatch routes /api/session by method.
func sessionDispatch(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		sessionStatus(w, r)
	case http.MethodPost:
		sessionLogin(w, r)
	case http.MethodDelete:
		sessionLogout(w, r)
	default:
		respond(w, 405, nil, "method not allowed")
	}
}

// --- password management -----------------------------------------------------

func changePassword(w http.ResponseWriter, r *http.Request) {
	se, ok := sessionOf(r)
	if !ok {
		respond(w, 401, nil, "authentication required")
		return
	}
	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
		ConfirmPassword string `json:"confirm_password"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.CurrentPassword == "" || req.NewPassword == "" || req.NewPassword != req.ConfirmPassword {
		respond(w, 400, nil, "invalid password change request")
		return
	}
	if _, err := usersDB.Verify(se.User, req.CurrentPassword); err != nil {
		audit("password_change_failed", "", r)
		respond(w, 401, nil, "current password incorrect")
		return
	}
	if err := usersDB.SetPassword(se.User, req.NewPassword); err != nil {
		respond(w, 400, nil, err.Error())
		return
	}
	// kill the user's other sessions — the old credential is dead
	sessions.DeleteForUser(se.User, auth.SessionFromRequest(r))
	audit("password_change", "", r)
	respond(w, 200, nil, "password changed")
}

// --- user management (tech only) ---------------------------------------------

// lastTech reports whether name is the only tech-role account — it must
// never be demoted or deleted or the box loses all management access.
func lastTech(name string) bool {
	techs := 0
	for _, u := range usersDB.List() {
		if u[1] == "tech" {
			if u[0] == name {
				techs++
			} else {
				return false // another tech exists
			}
		}
	}
	return techs == 1
}

func usersList(w http.ResponseWriter, r *http.Request) {
	var out []map[string]string
	for _, u := range usersDB.List() {
		out = append(out, map[string]string{"user": u[0], "role": u[1]})
	}
	respond(w, 200, out, "users listed")
}

func usersSet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Role     string `json:"role"`
		Password string `json:"password"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.Username == "" || req.Password == "" {
		respond(w, 400, nil, "invalid user request")
		return
	}
	if req.Role != "tech" && lastTech(req.Username) {
		respond(w, 400, nil, "cannot demote the last tech account")
		return
	}
	if err := usersDB.Set(req.Username, req.Role, req.Password); err != nil {
		respond(w, 400, nil, err.Error())
		return
	}
	// a role/password change must not ride on a stale session — the user
	// re-authenticates and picks up the new role
	sessions.DeleteForUser(req.Username, "")
	audit("user_set", req.Username+" role="+req.Role, r)
	respond(w, 200, nil, "user saved")
}

func usersDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.Username == "" {
		respond(w, 400, nil, "invalid user request")
		return
	}
	if se, ok := sessionOf(r); ok && se.User == req.Username {
		respond(w, 400, nil, "cannot delete own account")
		return
	}
	if lastTech(req.Username) {
		respond(w, 400, nil, "cannot delete the last tech account")
		return
	}
	if err := usersDB.Remove(req.Username); err != nil {
		respond(w, 400, nil, err.Error())
		return
	}
	sessions.DeleteForUser(req.Username, "")
	audit("user_delete", req.Username, r)
	respond(w, 200, nil, "user removed")
}
