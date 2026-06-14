package homebase

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type SidecarClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewSidecarClient(baseURL string) *SidecarClient {
	return &SidecarClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

type SidecarDevice struct {
	NodeID   uint64   `json:"node_id"`
	Vendor   string   `json:"vendor"`
	Model    string   `json:"model"`
	Clusters []string `json:"clusters"`
	Online   bool     `json:"online"`
}

type SidecarDeviceState struct {
	NodeID   uint64                            `json:"node_id"`
	Online   bool                              `json:"online"`
	Clusters map[string]map[string]interface{} `json:"clusters"`
}

type SidecarCommissionRequest struct {
	SetupCode string `json:"setup_code"`
}

type SidecarCommissionResponse struct {
	NodeID   uint64   `json:"node_id"`
	Vendor   string   `json:"vendor"`
	Model    string   `json:"model"`
	Clusters []string `json:"clusters"`
}

type SidecarCommandRequest struct {
	Cluster   string                 `json:"cluster"`
	Command   string                 `json:"command"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

type SidecarCommandResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

func (sc *SidecarClient) Commission(setupCode string) (*SidecarCommissionResponse, error) {
	body, err := json.Marshal(SidecarCommissionRequest{SetupCode: setupCode})
	if err != nil {
		return nil, err
	}

	resp, err := sc.httpClient.Post(sc.baseURL+"/commission", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("sidecar commission: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("sidecar commission failed (%d): %s", resp.StatusCode, string(b))
	}

	var result SidecarCommissionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("sidecar commission decode: %w", err)
	}
	return &result, nil
}

func (sc *SidecarClient) Decommission(nodeID uint64) error {
	req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/commission/%d", sc.baseURL, nodeID), nil)
	if err != nil {
		return err
	}

	resp, err := sc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("sidecar decommission: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("sidecar decommission failed (%d): %s", resp.StatusCode, string(b))
	}
	return nil
}

func (sc *SidecarClient) ListDevices() ([]SidecarDevice, error) {
	resp, err := sc.httpClient.Get(sc.baseURL + "/devices")
	if err != nil {
		return nil, fmt.Errorf("sidecar list devices: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("sidecar list devices failed (%d): %s", resp.StatusCode, string(b))
	}

	var devices []SidecarDevice
	if err := json.NewDecoder(resp.Body).Decode(&devices); err != nil {
		return nil, fmt.Errorf("sidecar list devices decode: %w", err)
	}
	return devices, nil
}

func (sc *SidecarClient) GetDeviceState(nodeID uint64) (*SidecarDeviceState, error) {
	resp, err := sc.httpClient.Get(fmt.Sprintf("%s/devices/%d", sc.baseURL, nodeID))
	if err != nil {
		return nil, fmt.Errorf("sidecar get device: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("sidecar get device failed (%d): %s", resp.StatusCode, string(b))
	}

	var state SidecarDeviceState
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		return nil, fmt.Errorf("sidecar get device decode: %w", err)
	}
	return &state, nil
}

func (sc *SidecarClient) SendCommand(nodeID uint64, cluster, command string, args map[string]interface{}) (*SidecarCommandResponse, error) {
	body, err := json.Marshal(SidecarCommandRequest{
		Cluster:   cluster,
		Command:   command,
		Arguments: args,
	})
	if err != nil {
		return nil, err
	}

	resp, err := sc.httpClient.Post(
		fmt.Sprintf("%s/devices/%d/command", sc.baseURL, nodeID),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("sidecar command: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("sidecar command failed (%d): %s", resp.StatusCode, string(b))
	}

	var result SidecarCommandResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("sidecar command decode: %w", err)
	}
	return &result, nil
}

func (sc *SidecarClient) Healthy() bool {
	resp, err := sc.httpClient.Get(sc.baseURL + "/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
