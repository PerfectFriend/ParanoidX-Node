// Package api provides HTTP handlers and API endpoints for the simplex-node server
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"ParanoidX/internal/radio"
)

func newTestRadio(t *testing.T) (*radio.RadioService, *radio.AnnouncementStore) {
	t.Helper()
	dir := t.TempDir()
	rs := radio.NewRadioService(dir)
	as := radio.NewAnnouncementStore(dir)
	return rs, as
}


// TestRadioHandlerStations handles the TestRadioHandlerStations HTTP request.
func TestRadioHandlerStations(t *testing.T) {
	rs, as := newTestRadio(t)
	handler := RadioHandler(rs, as)

	w := httptest.NewRecorder()
	handler(w, localReq("GET", "/api/radio?action=stations", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["count"] == nil {
		t.Fatal("expected count in response")
	}
	count := int(resp["count"].(float64))
	if count < 3 {
		t.Fatalf("expected at least 3 stations, got %d", count)
	}
}


// TestRadioHandlerStationsFilterLang handles the TestRadioHandlerStationsFilterLang HTTP request.
func TestRadioHandlerStationsFilterLang(t *testing.T) {
	rs, as := newTestRadio(t)
	handler := RadioHandler(rs, as)

	w := httptest.NewRecorder()
	handler(w, localReq("GET", "/api/radio?action=stations&lang=ru", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	stations := resp["stations"].([]any)
	for _, s := range stations {
		st := s.(map[string]any)
		if st["lang"] != "ru" {
			t.Fatalf("expected all stations to have lang=ru, got %v", st["lang"])
		}
	}
}


// TestRadioHandlerPlaylistMissingStation handles the TestRadioHandlerPlaylistMissingStation HTTP request.
func TestRadioHandlerPlaylistMissingStation(t *testing.T) {
	rs, as := newTestRadio(t)
	handler := RadioHandler(rs, as)

	w := httptest.NewRecorder()
	handler(w, localReq("GET", "/api/radio?action=playlist", ""))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing station, got %d", w.Code)
	}
}


// TestRadioHandlerPlaylistNotFound handles the TestRadioHandlerPlaylistNotFound HTTP request.
func TestRadioHandlerPlaylistNotFound(t *testing.T) {
	rs, as := newTestRadio(t)
	handler := RadioHandler(rs, as)

	w := httptest.NewRecorder()
	handler(w, localReq("GET", "/api/radio?action=playlist&station=nonexistent", ""))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for nonexistent station, got %d", w.Code)
	}
}


// TestRadioHandlerPlaylistValid handles the TestRadioHandlerPlaylistValid HTTP request.
func TestRadioHandlerPlaylistValid(t *testing.T) {
	rs, as := newTestRadio(t)
	handler := RadioHandler(rs, as)

	w := httptest.NewRecorder()
	handler(w, localReq("GET", "/api/radio?action=playlist&station=liberty-voice-en", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	station := resp["station"].(map[string]any)
	if station["id"] != "liberty-voice-en" {
		t.Fatalf("expected station liberty-voice-en, got %v", station["id"])
	}
	playlist := resp["playlist"].([]any)
	if len(playlist) == 0 {
		t.Fatal("expected non-empty playlist")
	}
}


// TestRadioHandlerAnnouncePost handles the TestRadioHandlerAnnouncePost HTTP request.
func TestRadioHandlerAnnouncePost(t *testing.T) {
	rs, as := newTestRadio(t)
	handler := RadioHandler(rs, as)

	w := httptest.NewRecorder()
	handler(w, localReq("POST", "/api/radio?action=announce", `{"title":"Test","body":"Hello"}`))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["ok"] != true {
		t.Fatal("expected ok=true")
	}
	if resp["id"] == nil {
		t.Fatal("expected id in response")
	}
}


// TestRadioHandlerAnnounceMissingFields handles the TestRadioHandlerAnnounceMissingFields HTTP request.
func TestRadioHandlerAnnounceMissingFields(t *testing.T) {
	rs, as := newTestRadio(t)
	handler := RadioHandler(rs, as)

	w := httptest.NewRecorder()
	handler(w, localReq("POST", "/api/radio?action=announce", `{"title":"only title"}`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing body, got %d", w.Code)
	}
}


// TestRadioHandlerAnnounceGetMethod handles the TestRadioHandlerAnnounceGetMethod HTTP request.
func TestRadioHandlerAnnounceGetMethod(t *testing.T) {
	rs, as := newTestRadio(t)
	handler := RadioHandler(rs, as)

	w := httptest.NewRecorder()
	handler(w, localReq("GET", "/api/radio?action=announce", ""))

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for GET on announce, got %d", w.Code)
	}
}


// TestRadioHandlerAnnouncementsList handles the TestRadioHandlerAnnouncementsList HTTP request.
func TestRadioHandlerAnnouncementsList(t *testing.T) {
	rs, as := newTestRadio(t)
	handler := RadioHandler(rs, as)

	// Add an announcement first
	handler(httptest.NewRecorder(), localReq("POST", "/api/radio?action=announce", `{"title":"List Me","body":"list body"}`))

	w := httptest.NewRecorder()
	handler(w, localReq("GET", "/api/radio?action=announcements", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["count"].(float64) < 1 {
		t.Fatal("expected at least 1 pending announcement")
	}
}


// TestRadioHandlerHistory handles the TestRadioHandlerHistory HTTP request.
func TestRadioHandlerHistory(t *testing.T) {
	rs, as := newTestRadio(t)
	handler := RadioHandler(rs, as)

	w := httptest.NewRecorder()
	handler(w, localReq("GET", "/api/radio?action=history", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["history"] == nil {
		t.Fatal("expected history field")
	}
}


// TestRadioHandlerDefaultAction handles the TestRadioHandlerDefaultAction HTTP request.
func TestRadioHandlerDefaultAction(t *testing.T) {
	rs, as := newTestRadio(t)
	handler := RadioHandler(rs, as)

	w := httptest.NewRecorder()
	handler(w, localReq("GET", "/api/radio", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["stations"] == nil {
		t.Fatal("expected stations in default response")
	}
	if resp["actions"] == nil {
		t.Fatal("expected actions in default response")
	}
}


// TestTrackStreamHandlerInvalidPath handles the TestTrackStreamHandlerInvalidPath HTTP request.
func TestTrackStreamHandlerInvalidPath(t *testing.T) {
	dir := t.TempDir()
	handler := TrackStreamHandler(dir)

	w := httptest.NewRecorder()
	handler(w, localReq("GET", "/api/radio/track?path=../outside", ""))

	if w.Code != http.StatusBadRequest && w.Code != http.StatusForbidden {
		t.Fatalf("expected 400 or 403 for path traversal, got %d", w.Code)
	}
}


// TestTrackStreamHandlerMissingPath handles the TestTrackStreamHandlerMissingPath HTTP request.
func TestTrackStreamHandlerMissingPath(t *testing.T) {
	dir := t.TempDir()
	handler := TrackStreamHandler(dir)

	w := httptest.NewRecorder()
	handler(w, localReq("GET", "/api/radio/track", ""))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing path, got %d", w.Code)
	}
}
