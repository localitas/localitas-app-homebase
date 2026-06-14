package homebase

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestApp(sidecarURL string) *App {
	return &App{
		BasePath:   "/",
		Sidecar:    NewSidecarClient(sidecarURL),
		SidecarURL: sidecarURL,
	}
}

func TestHandleListClusters(t *testing.T) {
	app := newTestApp("http://localhost:1")
	h := &handler{app: app}

	req := httptest.NewRequest("GET", "/api/clusters", nil)
	w := httptest.NewRecorder()

	h.handleListClusters(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var clusters map[string]ClusterDef
	if err := json.Unmarshal(w.Body.Bytes(), &clusters); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if _, ok := clusters["OnOff"]; !ok {
		t.Error("expected OnOff cluster in response")
	}
	if _, ok := clusters["Thermostat"]; !ok {
		t.Error("expected Thermostat cluster in response")
	}
}

func TestHandleSidecarHealth_Healthy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	app := newTestApp(server.URL)
	h := &handler{app: app}

	req := httptest.NewRequest("GET", "/api/sidecar/health", nil)
	w := httptest.NewRecorder()

	h.handleSidecarHealth(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var status map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &status)
	if status["sidecar_healthy"] != true {
		t.Error("expected sidecar_healthy to be true")
	}
}

func TestHandleSidecarHealth_Unhealthy(t *testing.T) {
	app := newTestApp("http://localhost:1")
	h := &handler{app: app}

	req := httptest.NewRequest("GET", "/api/sidecar/health", nil)
	w := httptest.NewRecorder()

	h.handleSidecarHealth(w, req)

	var status map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &status)
	if status["sidecar_healthy"] != false {
		t.Error("expected sidecar_healthy to be false")
	}
}

func TestHandleCommission_MissingFields(t *testing.T) {
	app := newTestApp("http://localhost:1")
	h := &handler{app: app}

	req := httptest.NewRequest("POST", "/api/devices", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.handleCommission(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleCommission_MissingName(t *testing.T) {
	app := newTestApp("http://localhost:1")
	h := &handler{app: app}

	req := httptest.NewRequest("POST", "/api/devices", strings.NewReader(`{"setup_code":"123"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.handleCommission(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleCommission_InvalidJSON(t *testing.T) {
	app := newTestApp("http://localhost:1")
	h := &handler{app: app}

	req := httptest.NewRequest("POST", "/api/devices", strings.NewReader(`not json`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.handleCommission(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleCreateVirtual_MissingName(t *testing.T) {
	app := newTestApp("http://localhost:1")
	h := &handler{app: app}

	req := httptest.NewRequest("POST", "/api/virtual-devices", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.handleCreateVirtual(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleCreateVirtual_InvalidJSON(t *testing.T) {
	app := newTestApp("http://localhost:1")
	h := &handler{app: app}

	req := httptest.NewRequest("POST", "/api/virtual-devices", strings.NewReader(`not json`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.handleCreateVirtual(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestClustersForDeviceType(t *testing.T) {
	cases := []struct {
		deviceType string
		expected   int
	}{
		{"switch", 1},
		{"dimmable_light", 2},
		{"color_light", 3},
		{"fan", 2},
		{"lock", 1},
		{"unknown", 1},
	}
	for _, tc := range cases {
		clusters := clustersForDeviceType(tc.deviceType)
		if len(clusters) != tc.expected {
			t.Errorf("clustersForDeviceType(%q): expected %d clusters, got %d", tc.deviceType, tc.expected, len(clusters))
		}
	}
}
