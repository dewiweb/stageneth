package api

import (
	context "context"
	json "encoding/json"
	http "net/http"
	os "os"
	exec "os/exec"
	time "time"

	"stageneth/internal/auth"
)

type apiResponse struct {
	Code    int         `json:"code"`
	Data    interface{} `json:"data"`
	Message string      `json:"message"`
}
type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

var (
	usersDB  *auth.Users
	sessions *auth.Store
)

// Init wires the user database and session store used by the handlers.
func Init(u *auth.Users, s *auth.Store) {
	usersDB = u
	sessions = s
}

func respond(w http.ResponseWriter, code int, data interface{}, message string) {
	status := code
	if status < 100 || status > 599 {
		status = http.StatusOK
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(apiResponse{Code: code, Data: data, Message: message})
}

// initDir returns the init script directory — overridable for tests
// (matches STAGENETH_INIT_DIR handling in the stagenethd engine).
func initDir() string {
	if d := os.Getenv("STAGENETH_INIT_DIR"); d != "" {
		return d
	}
	return "/etc/init.d"
}

// boundedCmd returns a *exec.Cmd killed after timeout — a hung helper
// (init.d, uci, passwd, logread…) must never pin a request goroutine.
// For callers that must set Stdin/Env before running.
func boundedCmd(timeout time.Duration, name string, args ...string) *exec.Cmd {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	cmd := exec.CommandContext(ctx, name, args...)
	_ = cancel // the timeout itself releases the context
	return cmd
}

// outBounded runs a one-shot command returning combined output.
func outBounded(timeout time.Duration, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

// runBounded runs a one-shot command to completion.
func runBounded(timeout time.Duration, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return exec.CommandContext(ctx, name, args...).Run()
}

// sessionOf returns the current session, if any.
func sessionOf(r *http.Request) (auth.Session, bool) {
	if sessions == nil {
		return auth.Session{}, false
	}
	return sessions.Get(auth.SessionFromRequest(r))
}

// requireAuth rejects requests without a valid session.
func requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := sessionOf(r); !ok {
			respond(w, 401, nil, "authentication required")
			return
		}
		next(w, r)
	}
}

// requireTech rejects sessions whose role is not "tech".
// operator sessions may only reach read endpoints and apply.
func requireTech(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		se, ok := sessionOf(r)
		if !ok {
			respond(w, 401, nil, "authentication required")
			return
		}
		if se.Role != "tech" {
			respond(w, 403, nil, "tech role required")
			return
		}
		next(w, r)
	}
}
