// Package api provides HTTP handlers and API endpoints for the simplex-node server
package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"ParanoidX/internal/lock"
	"ParanoidX/internal/middleware"
)

func newTestLockService(t *testing.T) *lock.Service {
	t.Helper()
	dir, err := os.MkdirTemp("", "api-lock-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return lock.New(dir)
}

func localReq(method, target, body string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Content-Type", "application/json")
	return req
}


// TestLockStatusHandler handles the TestLockStatusHandler HTTP request.
func TestLockStatusHandler(t *testing.T) {
	svc := newTestLockService(t)
	handler := LockStatusHandler(svc)

	w := httptest.NewRecorder()
	handler(w, localReq("GET", "/api/lock-status", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}


// TestLockStatusHandlerForbiddenRemote handles the TestLockStatusHandlerForbiddenRemote HTTP request.
func TestLockStatusHandlerForbiddenRemote(t *testing.T) {
	svc := newTestLockService(t)
	handler := LockStatusHandler(svc)

	req := httptest.NewRequest("GET", "/api/lock-status", nil)
	req.RemoteAddr = "203.0.113.1:12345"
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for remote IP, got %d", w.Code)
	}
}


// TestLockHandler handles the TestLockHandler HTTP request.
func TestLockHandler(t *testing.T) {
	svc := newTestLockService(t)
	handler := LockHandler(svc)

	w := httptest.NewRecorder()
	handler(w, localReq("POST", "/api/lock", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !svc.IsLocked() {
		t.Fatal("expected service to be locked")
	}
}


// TestUnlockHandler handles the TestUnlockHandler HTTP request.
func TestUnlockHandler(t *testing.T) {
	svc := newTestLockService(t)
	limiter := middleware.NewRateLimiter(1, 5, 0)
	handler := UnlockHandler(svc, limiter)
	svc.Lock()

	w := httptest.NewRecorder()
	handler(w, localReq("POST", "/api/unlock", `{"code":"123456"}`))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if svc.IsLocked() {
		t.Fatal("expected service to be unlocked")
	}
}


// TestChangeLockCodeHandler handles the TestChangeLockCodeHandler HTTP request.
func TestChangeLockCodeHandler(t *testing.T) {
	svc := newTestLockService(t)
	handler := ChangeLockCodeHandler(svc)

	w := httptest.NewRecorder()
	handler(w, localReq("POST", "/api/change-lock-code", `{"current_code":"123456","new_code":"newpass"}`))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !svc.ValidateUnlock("newpass") {
		t.Fatal("expected new code to work")
	}
}


// TestChangeLockCodeWrongCurrent handles the TestChangeLockCodeWrongCurrent HTTP request.
func TestChangeLockCodeWrongCurrent(t *testing.T) {
	svc := newTestLockService(t)
	handler := ChangeLockCodeHandler(svc)

	w := httptest.NewRecorder()
	handler(w, localReq("POST", "/api/change-lock-code", `{"current_code":"wrong","new_code":"newpass"}`))

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}


// TestChangeLockCodeRemoteForbidden handles the TestChangeLockCodeRemoteForbidden HTTP request.
func TestChangeLockCodeRemoteForbidden(t *testing.T) {
	svc := newTestLockService(t)
	handler := ChangeLockCodeHandler(svc)

	req := httptest.NewRequest("POST", "/api/change-lock-code", strings.NewReader(`{}`))
	req.RemoteAddr = "8.8.8.8:12345"
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for remote IP, got %d", w.Code)
	}
}
