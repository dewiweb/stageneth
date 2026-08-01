package main

import (
	json "encoding/json"
	fmt "fmt"
	http "net/http"
	os "os"
	exec "os/exec"
	strings "strings"
)

func login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.Username != "root" {
		respond(w, 401, nil, "invalid credentials")
		return
	}

	hash, err := opensslHash(req.Password)
	if err != nil {
		respond(w, 500, nil, "auth error")
		return
	}
	data, err := os.ReadFile("/etc/shadow")
	if err != nil {
		respond(w, 500, nil, "shadow read error")
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Split(line, ":")
		if len(fields) >= 2 && fields[0] == req.Username && fields[1] == hash {
			respond(w, 200, map[string]string{"token": token(req.Username)}, "login success")
			return
		}
	}
	respond(w, 401, nil, "invalid credentials")
}
func auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			respond(w, 403, nil, "missing token")
			return
		}
		if tokenValid(auth[7:]) == "" {
			respond(w, 403, nil, "invalid token")
			return
		}
		next(w, r)
	}
}
func shadowRootHash() (string, error) {
	data, err := os.ReadFile("/etc/shadow")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Split(line, ":")
		if len(fields) >= 2 && fields[0] == "root" {
			return fields[1], nil
		}
	}
	return "", fmt.Errorf("root not found in shadow")
}
func opensslHash(password string) (string, error) {

	out, err := exec.Command("openssl", "passwd", "-1", "-salt", "stagenet", password).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
func changePassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
		ConfirmPassword string `json:"confirm_password"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.CurrentPassword == "" || req.NewPassword == "" || req.NewPassword != req.ConfirmPassword {
		respond(w, 400, nil, "invalid password change request")
		return
	}
	currentHash, err := opensslHash(req.CurrentPassword)
	if err != nil {
		respond(w, 500, nil, "hash error")
		return
	}
	rootHash, err := shadowRootHash()
	if err != nil {
		respond(w, 500, nil, "shadow read error")
		return
	}
	if currentHash != rootHash {
		respond(w, 401, nil, "current password incorrect")
		return
	}
	newHash, err := opensslHash(req.NewPassword)
	if err != nil {
		respond(w, 500, nil, "hash error")
		return
	}

	if err := exec.Command("sed", "-i", fmt.Sprintf("s|^root:[^:]*:|root:%s:|", newHash), "/etc/shadow").Run(); err != nil {
		respond(w, 500, nil, "shadow update error")
		return
	}
	respond(w, 200, nil, "password changed")
}
