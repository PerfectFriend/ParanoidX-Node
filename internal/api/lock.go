// Package api provides HTTP handlers and API endpoints for the simplex-node server
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"ParanoidX/internal/lock"
	"ParanoidX/internal/middleware"
)

type unlockLimiter interface {
	Middleware(next http.HandlerFunc) http.HandlerFunc
}


// LockStatusHandler returns whether the node is currently locked.
func LockStatusHandler(lockSvc *lock.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		writeJSON(w, map[string]bool{"is_locked": lockSvc.IsLocked()})
	}
}


// LockHandler locks the node with a security code.
func LockHandler(lockSvc *lock.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		lockSvc.Lock()
		w.WriteHeader(200)
	}
}


// UnlockHandler unlocks the node by validating the security code with rate limiting.
func UnlockHandler(lockSvc *lock.Service, limiter unlockLimiter) http.HandlerFunc {
	return limiter.Middleware(func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		defer r.Body.Close()
		var req struct {
			Code string `json:"code"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
			return
		}
		code := strings.TrimSpace(req.Code)
		slog.Info("unlock request", "code_len", len(code))
		if lockSvc.ValidateUnlock(code) {
			w.WriteHeader(200)
		} else {
			http.Error(w, "WRONG CODE, ACCESS DENIED", 401)
		}
	})
}


// ChangeLockCodeHandler changes the node lock code after validating the current code.
func ChangeLockCodeHandler(lockSvc *lock.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		defer r.Body.Close()
		var req struct {
			CurrentCode string `json:"current_code"`
			NewCode     string `json:"new_code"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
			return
		}
		if lockSvc.ChangeCode(strings.TrimSpace(req.CurrentCode), strings.TrimSpace(req.NewCode)) {
			w.WriteHeader(200)
		} else {
			http.Error(w, "invalid", 401)
		}
	}
}
