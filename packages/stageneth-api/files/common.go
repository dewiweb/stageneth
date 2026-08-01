package main

import (
	hmac "crypto/hmac"
	sha256 "crypto/sha256"
	base64 "encoding/base64"
	json "encoding/json"
	fmt "fmt"
	http "net/http"
	strings "strings"
	time "time"
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
