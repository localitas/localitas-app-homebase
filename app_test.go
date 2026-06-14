package homebase

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/localitas/localitas-go"
)

func TestHandleIndex_ContainsBaseHref(t *testing.T) {
	c := client.New("http://localhost:9999")
	app := New(c, "/apps/ext/homebase/", "http://localhost:9222")

	mux := http.NewServeMux()
	app.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, `<base href="/apps/ext/homebase/">`) {
		t.Errorf("expected <base href> with correct path in body:\n%s", body[:min(len(body), 500)])
	}
}

func TestHandleIndex_DefaultBasePath(t *testing.T) {
	c := client.New("http://localhost:9999")
	app := New(c, "/", "http://localhost:9222")

	mux := http.NewServeMux()
	app.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, `<base href="/">`) {
		t.Errorf("expected <base href=\"/\"> for standalone mode:\n%s", body[:min(len(body), 500)])
	}
}

func TestHandleHealth_ReturnsManifest(t *testing.T) {
	req := httptest.NewRequest("GET", "/health.json", nil)
	w := httptest.NewRecorder()

	HandleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var health AppHealth
	if err := json.Unmarshal(w.Body.Bytes(), &health); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if health.Name != "homebase" {
		t.Errorf("expected name=homebase, got %q", health.Name)
	}
	if health.Status != "healthy" {
		t.Errorf("expected status=healthy, got %q", health.Status)
	}
	if health.Icon != "home" {
		t.Errorf("expected icon=home, got %q", health.Icon)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
