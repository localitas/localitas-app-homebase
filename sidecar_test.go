package homebase

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSidecarClient_Commission(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/commission" {
			t.Errorf("expected /commission, got %s", r.URL.Path)
		}

		var req SidecarCommissionRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.SetupCode != "34970112332" {
			t.Errorf("expected setup code 34970112332, got %s", req.SetupCode)
		}

		json.NewEncoder(w).Encode(SidecarCommissionResponse{
			NodeID:   1,
			Vendor:   "TestVendor",
			Model:    "TestModel",
			Clusters: []string{"OnOff", "LevelControl"},
		})
	}))
	defer server.Close()

	sc := NewSidecarClient(server.URL)
	result, err := sc.Commission("34970112332")
	if err != nil {
		t.Fatalf("commission error: %v", err)
	}
	if result.NodeID != 1 {
		t.Errorf("expected node_id 1, got %d", result.NodeID)
	}
	if result.Vendor != "TestVendor" {
		t.Errorf("expected vendor TestVendor, got %s", result.Vendor)
	}
	if len(result.Clusters) != 2 {
		t.Errorf("expected 2 clusters, got %d", len(result.Clusters))
	}
}

func TestSidecarClient_ListDevices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/devices" {
			t.Errorf("expected /devices, got %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode([]SidecarDevice{
			{NodeID: 1, Vendor: "V1", Model: "M1", Clusters: []string{"OnOff"}, Online: true},
			{NodeID: 2, Vendor: "V2", Model: "M2", Clusters: []string{"Thermostat"}, Online: false},
		})
	}))
	defer server.Close()

	sc := NewSidecarClient(server.URL)
	devices, err := sc.ListDevices()
	if err != nil {
		t.Fatalf("list devices error: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devices))
	}
	if !devices[0].Online {
		t.Error("expected first device to be online")
	}
	if devices[1].Online {
		t.Error("expected second device to be offline")
	}
}

func TestSidecarClient_GetDeviceState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/devices/1" {
			t.Errorf("expected /devices/1, got %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(SidecarDeviceState{
			NodeID: 1,
			Online: true,
			Clusters: map[string]map[string]interface{}{
				"OnOff": {"OnOff": true},
			},
		})
	}))
	defer server.Close()

	sc := NewSidecarClient(server.URL)
	state, err := sc.GetDeviceState(1)
	if err != nil {
		t.Fatalf("get device state error: %v", err)
	}
	if !state.Online {
		t.Error("expected device to be online")
	}
	if state.Clusters["OnOff"]["OnOff"] != true {
		t.Error("expected OnOff to be true")
	}
}

func TestSidecarClient_SendCommand(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/devices/1/command" {
			t.Errorf("expected /devices/1/command, got %s", r.URL.Path)
		}

		var cmd SidecarCommandRequest
		json.NewDecoder(r.Body).Decode(&cmd)
		if cmd.Cluster != "OnOff" || cmd.Command != "On" {
			t.Errorf("expected OnOff/On, got %s/%s", cmd.Cluster, cmd.Command)
		}

		json.NewEncoder(w).Encode(SidecarCommandResponse{Success: true})
	}))
	defer server.Close()

	sc := NewSidecarClient(server.URL)
	result, err := sc.SendCommand(1, "OnOff", "On", nil)
	if err != nil {
		t.Fatalf("send command error: %v", err)
	}
	if !result.Success {
		t.Error("expected command to succeed")
	}
}

func TestSidecarClient_Decommission(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if r.URL.Path != "/commission/1" {
			t.Errorf("expected /commission/1, got %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]bool{"success": true})
	}))
	defer server.Close()

	sc := NewSidecarClient(server.URL)
	err := sc.Decommission(1)
	if err != nil {
		t.Fatalf("decommission error: %v", err)
	}
}

func TestSidecarClient_Healthy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	sc := NewSidecarClient(server.URL)
	if !sc.Healthy() {
		t.Error("expected sidecar to be healthy")
	}
}

func TestSidecarClient_Healthy_Down(t *testing.T) {
	sc := NewSidecarClient("http://localhost:1")
	if sc.Healthy() {
		t.Error("expected sidecar to be unhealthy when unreachable")
	}
}

func TestSidecarClient_Commission_ErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"chip init failed"}`))
	}))
	defer server.Close()

	sc := NewSidecarClient(server.URL)
	_, err := sc.Commission("badcode")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}
