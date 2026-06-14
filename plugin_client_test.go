package homebase

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPluginClient_FetchHealth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health.json" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(PluginHealth{
			Name:       "homebase-ring",
			PluginType: "homebase-plugin",
			PluginFor:  "homebase",
		})
	}))
	defer server.Close()

	pc := NewPluginClient(server.URL, "homebase-ring")
	health, err := pc.FetchHealth()
	if err != nil {
		t.Fatalf("fetch health: %v", err)
	}
	if health.Name != "homebase-ring" {
		t.Errorf("expected name homebase-ring, got %q", health.Name)
	}
	if health.PluginType != "homebase-plugin" {
		t.Errorf("expected plugin_type homebase-plugin, got %q", health.PluginType)
	}
}

func TestPluginClient_FetchDevices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/devices" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode([]PluginDevice{
			{ID: 123, Name: "Front Door", DeviceType: "doorbell", IsOnline: true},
			{ID: 456, Name: "Backyard", DeviceType: "camera", IsOnline: false},
		})
	}))
	defer server.Close()

	pc := NewPluginClient(server.URL, "homebase-ring")
	devices, err := pc.FetchDevices()
	if err != nil {
		t.Fatalf("fetch devices: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devices))
	}
	if devices[0].Name != "Front Door" {
		t.Errorf("expected Front Door, got %q", devices[0].Name)
	}
	if devices[1].IsOnline {
		t.Error("expected Backyard to be offline")
	}
}

func TestPluginClient_Healthy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(PluginHealth{Name: "test"})
	}))
	defer server.Close()

	pc := NewPluginClient(server.URL, "test")
	if !pc.Healthy() {
		t.Error("expected plugin to be healthy")
	}
}

func TestPluginClient_Healthy_Down(t *testing.T) {
	pc := NewPluginClient("http://localhost:1", "test")
	if pc.Healthy() {
		t.Error("expected plugin to be unhealthy")
	}
}

func TestPluginClient_Configure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/configure" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var req map[string]string
		json.NewDecoder(r.Body).Decode(&req)
		if req["vault_public_id"] != "vault-123" {
			t.Errorf("expected vault-123, got %q", req["vault_public_id"])
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "configured"})
	}))
	defer server.Close()

	pc := NewPluginClient(server.URL, "test")
	err := pc.Configure("vault-123")
	if err != nil {
		t.Fatalf("configure: %v", err)
	}
}

func TestPluginClient_ProxyCommand(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/devices/123/command" {
			t.Errorf("expected /api/devices/123/command, got %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]bool{"success": true})
	}))
	defer server.Close()

	pc := NewPluginClient(server.URL, "test")
	data, status, err := pc.ProxyCommand("123", nil)
	if err != nil {
		t.Fatalf("proxy command: %v", err)
	}
	if status != http.StatusOK {
		t.Errorf("expected 200, got %d", status)
	}
	var resp map[string]bool
	json.Unmarshal(data, &resp)
	if !resp["success"] {
		t.Error("expected success")
	}
}
