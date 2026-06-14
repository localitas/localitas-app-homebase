package homebase

import "time"

type Device struct {
	ID         string    `json:"id"`
	NodeID     uint64    `json:"node_id"`
	Name       string    `json:"name"`
	DeviceType string    `json:"device_type"`
	Room       string    `json:"room"`
	Vendor     string    `json:"vendor"`
	Model      string    `json:"model"`
	Clusters   []string  `json:"clusters"`
	Online     bool      `json:"online"`
	Virtual    bool      `json:"virtual"`
	Source     string    `json:"source,omitempty"`
	SourceID   string    `json:"source_id,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type DeviceState struct {
	NodeID   uint64                  `json:"node_id"`
	Online   bool                    `json:"online"`
	Clusters map[string]ClusterState `json:"clusters"`
	LastSeen time.Time               `json:"last_seen"`
}

type ClusterState struct {
	ClusterName string                 `json:"cluster_name"`
	Attributes  map[string]interface{} `json:"attributes"`
}

type CommissionRequest struct {
	SetupCode string `json:"setup_code"`
	Name      string `json:"name"`
	Room      string `json:"room"`
}

type CommissionResponse struct {
	NodeID   uint64   `json:"node_id"`
	Vendor   string   `json:"vendor"`
	Model    string   `json:"model"`
	Clusters []string `json:"clusters"`
}

type CommandRequest struct {
	Cluster   string                 `json:"cluster"`
	Command   string                 `json:"command"`
	Arguments map[string]interface{} `json:"arguments"`
}

type CommandResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

type DeviceUpdate struct {
	Name string `json:"name,omitempty"`
	Room string `json:"room,omitempty"`
}

type CreateVirtualRequest struct {
	Name       string `json:"name"`
	Room       string `json:"room"`
	DeviceType string `json:"device_type"`
}
