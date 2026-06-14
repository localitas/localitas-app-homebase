package homebase

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleSwagger_ReturnsValidJSON(t *testing.T) {
	req := httptest.NewRequest("GET", "/swagger.json", nil)
	w := httptest.NewRecorder()

	HandleSwagger(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %q", ct)
	}

	var spec map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &spec); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if spec["openapi"] != "3.0.3" {
		t.Errorf("expected openapi 3.0.3, got %v", spec["openapi"])
	}
}

func TestSwaggerSpec_HasAllEndpoints(t *testing.T) {
	expected := []string{
		"/api/devices",
		"/api/devices/{id}",
		"/api/devices/{id}/state",
		"/api/devices/{id}/command",
		"/api/virtual-devices",
		"/api/rooms",
		"/api/clusters",
		"/api/sidecar/health",
	}
	for _, p := range expected {
		found := false
		for _, ep := range HomebaseAPIDoc.Endpoints {
			if strings.Contains(ep.Path, p) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected endpoint containing %s", p)
		}
	}
}
