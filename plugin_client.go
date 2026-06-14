package homebase

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type PluginHealth struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Icon        string `json:"icon"`
	Version     string `json:"version"`
	Status      string `json:"status"`
	PluginType  string `json:"plugin_type"`
	PluginFor   string `json:"plugin_for"`
}

type PluginDevice struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	DeviceType   string `json:"device_type"`
	Kind         string `json:"kind"`
	LocationID   string `json:"location_id"`
	BatteryLevel int    `json:"battery_level"`
	HasLight     bool   `json:"has_light"`
	HasSiren     bool   `json:"has_siren"`
	IsOnline     bool   `json:"is_online"`
	LEDStatus    string `json:"led_status"`
	Firmware     string `json:"firmware"`
}

type PluginClient struct {
	baseURL    string
	name       string
	httpClient *http.Client
}

func NewPluginClient(baseURL, name string) *PluginClient {
	return &PluginClient{
		baseURL:    baseURL,
		name:       name,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

func (pc *PluginClient) FetchHealth() (*PluginHealth, error) {
	resp, err := pc.httpClient.Get(pc.baseURL + "/health.json")
	if err != nil {
		return nil, fmt.Errorf("plugin health: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("plugin health returned %d", resp.StatusCode)
	}

	var health PluginHealth
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return nil, fmt.Errorf("decode plugin health: %w", err)
	}
	return &health, nil
}

func (pc *PluginClient) FetchDevices() ([]PluginDevice, error) {
	resp, err := pc.httpClient.Get(pc.baseURL + "/api/devices")
	if err != nil {
		return nil, fmt.Errorf("plugin devices: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("plugin devices returned %d", resp.StatusCode)
	}

	var devices []PluginDevice
	if err := json.NewDecoder(resp.Body).Decode(&devices); err != nil {
		return nil, fmt.Errorf("decode plugin devices: %w", err)
	}
	return devices, nil
}

func (pc *PluginClient) ProxyCommand(deviceSourceID string, body io.Reader) ([]byte, int, error) {
	url := fmt.Sprintf("%s/api/devices/%s/command", pc.baseURL, deviceSourceID)
	resp, err := pc.httpClient.Post(url, "application/json", body)
	if err != nil {
		return nil, 0, fmt.Errorf("plugin command proxy: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return data, resp.StatusCode, nil
}

func (pc *PluginClient) GetSnapshot(deviceSourceID string) ([]byte, error) {
	url := fmt.Sprintf("%s/api/devices/%s/snapshot", pc.baseURL, deviceSourceID)
	resp, err := pc.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("plugin snapshot: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("plugin snapshot returned %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func (pc *PluginClient) Configure(vaultPublicID string) error {
	body := map[string]string{"vault_public_id": vaultPublicID}
	b, _ := json.Marshal(body)
	resp, err := pc.httpClient.Post(pc.baseURL+"/api/configure", "application/json", bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("plugin configure: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("plugin configure returned %d: %s", resp.StatusCode, string(data))
	}
	return nil
}

func (pc *PluginClient) Healthy() bool {
	resp, err := pc.httpClient.Get(pc.baseURL + "/health.json")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
