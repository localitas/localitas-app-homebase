package homebase

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

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
	ClusterName string            `json:"cluster_name"`
	Attributes  ClusterAttributes `json:"attributes"`
}

// ClusterAttributes are a Matter cluster's attribute values by attribute
// name, as the sidecar reports them.
type ClusterAttributes map[string]AttrValue

// AttrValue is one Matter attribute's value: a JSON value whose type is
// the attribute's (a bool, a number, a string, null, a list or a struct).
// It is kept as the sidecar's JSON text, so a 64-bit integer keeps its
// digits. JSON writes it as that text; YAML and the markdown table write
// the value it decodes to.
type AttrValue string

func (v AttrValue) MarshalJSON() ([]byte, error) {
	if v == "" {
		return []byte("null"), nil
	}
	return []byte(v), nil
}

func (v *AttrValue) UnmarshalJSON(b []byte) error {
	*v = AttrValue(b)
	return nil
}

// MarshalYAML is yaml.v3's Marshaler, whose signature returns interface{}:
// YAML writes the decoded JSON value.
func (v AttrValue) MarshalYAML() (interface{}, error) {
	var decoded interface{}
	if err := json.Unmarshal([]byte(v), &decoded); err != nil {
		return nil, fmt.Errorf("attribute value %s: %w", v, err)
	}
	return decoded, nil
}

// String is the value as the markdown table shows it: a scalar as itself
// (null as nothing), a list of scalars joined with ", ", anything else as
// its JSON text.
func (v AttrValue) String() string {
	var decoded interface{}
	if err := json.Unmarshal([]byte(v), &decoded); err != nil {
		return string(v)
	}
	switch d := decoded.(type) {
	case nil:
		return ""
	case bool, float64, string:
		return fmt.Sprintf("%v", d)
	case []interface{}:
		if len(d) == 0 {
			return "[]"
		}
		parts := make([]string, len(d))
		for i, e := range d {
			switch e.(type) {
			case bool, float64, string:
				parts[i] = fmt.Sprintf("%v", e)
			default:
				return compactJSON(v)
			}
		}
		return strings.Join(parts, ", ")
	default:
		return compactJSON(v)
	}
}

func compactJSON(v AttrValue) string {
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(v)); err != nil {
		return string(v)
	}
	return buf.String()
}

// attrAs is cluster attribute name's value as a T, and whether it is one:
// an attribute absent, null or of another type is not.
func attrAs[T bool | float64](c ClusterAttributes, name string) (T, bool) {
	var zero T
	raw, ok := c[name]
	if !ok {
		return zero, false
	}
	var value *T
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		logger.Debug("matter attribute of another type", "attribute", name, "value", string(raw), "error", err)
		return zero, false
	}
	if value == nil {
		return zero, false
	}
	return *value, true
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

// CommandRequest is POST /api/devices/{id}/command's body. Arguments are
// the Matter command's own, a JSON object whose shape is the command's.
type CommandRequest struct {
	Cluster   string          `json:"cluster"`
	Command   string          `json:"command"`
	Arguments json.RawMessage `json:"arguments"`
}

// commandArguments are a command's arguments as sent on to the sidecar: a
// JSON object, compacted. None (absent, null or {}) is nil, which the
// sidecar's body leaves out, as it left out the empty map before.
func commandArguments(raw json.RawMessage) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil, nil
	}
	if trimmed[0] != '{' {
		return nil, fmt.Errorf("arguments must be a JSON object, not %s", trimmed)
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, trimmed); err != nil {
		return nil, fmt.Errorf("arguments: %w", err)
	}
	if buf.String() == "{}" {
		return nil, nil
	}
	return buf.Bytes(), nil
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
