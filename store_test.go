package homebase

import "testing"

func TestNewDeviceID_NotEmpty(t *testing.T) {
	id := newDeviceID()
	if id == "" {
		t.Error("newDeviceID() returned empty string")
	}
	if len(id) != 32 {
		t.Errorf("expected 32 char hex id, got %d chars: %s", len(id), id)
	}
}

func TestNewDeviceID_Unique(t *testing.T) {
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := newDeviceID()
		if ids[id] {
			t.Errorf("duplicate id generated: %s", id)
		}
		ids[id] = true
	}
}
