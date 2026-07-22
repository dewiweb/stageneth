package main

//
// Copyright (C) 2026 StageNeth Contributors
// SPDX-License-Identifier: GPL-2.0-only
//

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
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

type crudRequest struct {
	Config  string                 `json:"config"`
	Section string                 `json:"section"`
	Type    string                 `json:"type"`
	Values  map[string]interface{} `json:"values"`
}

var secret string

func respond(w http.ResponseWriter, code int, data interface{}, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(apiResponse{Code: code, Data: data, Message: message})
}

func sign(value string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(value))
	return base64.URLEncoding.EncodeToString(mac.Sum(nil))
}

func token(username string) string {
	payload := fmt.Sprintf("%s:%d", username, time.Now().Unix())
	return payload + ":" + sign(payload)
}

func tokenValid(t string) string {
	parts := strings.Split(t, ":")
	if len(parts) != 3 {
		return ""
	}
	if sign(parts[0]+":"+parts[1]) != parts[2] {
		return ""
	}
	username := strings.Split(parts[0], ":")[0]
	return username
}

func login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.Username != "root" {
		respond(w, 401, nil, "invalid credentials")
		return
	}
	// Simple password check against /etc/shadow via openssl passwd
	out, err := exec.Command("openssl", "passwd", "-1", "-salt", "stageneth", req.Password).Output()
	if err != nil {
		respond(w, 500, nil, "auth error")
		return
	}
	hash := strings.TrimSpace(string(out))
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

func ubusCall(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path    string                 `json:"path"`
		Method  string                 `json:"method"`
		Payload map[string]interface{} `json:"payload"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		respond(w, 400, nil, "invalid request")
		return
	}
	payload, _ := json.Marshal(req.Payload)
	out, err := exec.Command("ubus", "call", req.Path, req.Method, string(payload)).Output()
	if err != nil {
		respond(w, 500, nil, "ubus call failed")
		return
	}
	var data interface{}
	json.Unmarshal(out, &data)
	respond(w, 200, data, "ubus call success")
}

func uciSet(w http.ResponseWriter, r *http.Request) {
	var req crudRequest
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		respond(w, 400, nil, "invalid request")
		return
	}
	commands := []string{}
	if req.Type == "" {
		// delete the whole section
		commands = append(commands, fmt.Sprintf("delete %s.%s", req.Config, req.Section))
	} else {
		commands = append(commands, fmt.Sprintf("set %s.%s=%s", req.Config, req.Section, req.Type))
		for k, v := range req.Values {
			commands = append(commands, fmt.Sprintf("set %s.%s.%s='%v'", req.Config, req.Section, k, v))
		}
	}
	in := strings.Join(commands, "\n") + "\n"
	cmd := exec.Command("uci", "-q", "batch")
	cmd.Stdin = strings.NewReader(in)
	if err := cmd.Run(); err != nil {
		respond(w, 500, nil, "uci batch failed")
		return
	}
	respond(w, 200, nil, "uci set success")
}

func uciCommit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Config string `json:"config"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	cmd := exec.Command("uci", "-q", "commit", req.Config)
	if err := cmd.Run(); err != nil {
		respond(w, 500, nil, "uci commit failed")
		return
	}
	respond(w, 200, nil, "uci commit success")
}

func applyNetwork(w http.ResponseWriter, r *http.Request) {
	out, err := exec.Command("/usr/libexec/stageneth/stageneth-network.py", "apply").CombinedOutput()
	if err != nil {
		respond(w, 500, map[string]string{"log": string(out)}, "apply failed")
		return
	}
	respond(w, 200, map[string]string{"log": string(out)}, "apply success")
}

func main() {
	secret = os.Getenv("STAGENETH_SECRET")
	if secret == "" {
		secret = "change-me"
	}
	bind := os.Getenv("STAGENETH_BIND")
	if bind == "" {
		bind = "127.0.0.1"
	}
	port := os.Getenv("STAGENETH_PORT")
	if port == "" {
		port = "8090"
	}

	http.HandleFunc("/api/login", login)
	http.HandleFunc("/api/ubus/call", auth(ubusCall))
	http.HandleFunc("/api/uci/set", auth(uciSet))
	http.HandleFunc("/api/uci/commit", auth(uciCommit))
	http.HandleFunc("/api/stageneth/apply", auth(applyNetwork))

	addr := fmt.Sprintf("%s:%s", bind, port)
	fmt.Println("StageNeth API listening on", addr)
	http.ListenAndServe(addr, nil)
}
