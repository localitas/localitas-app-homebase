package homebase

import "testing"

func TestClusterDefFor_KnownCluster(t *testing.T) {
	def, ok := ClusterDefFor("OnOff")
	if !ok {
		t.Fatal("expected OnOff cluster to exist")
	}
	if def.Name != "OnOff" {
		t.Errorf("expected name OnOff, got %q", def.Name)
	}
	if def.HAPService != "Switch" {
		t.Errorf("expected HAP service Switch, got %q", def.HAPService)
	}
	if len(def.Commands) != 3 {
		t.Errorf("expected 3 commands, got %d", len(def.Commands))
	}
}

func TestClusterDefFor_UnknownCluster(t *testing.T) {
	_, ok := ClusterDefFor("NonExistent")
	if ok {
		t.Error("expected unknown cluster to return false")
	}
}

func TestDeviceTypeFromClusters_Thermostat(t *testing.T) {
	dt := DeviceTypeFromClusters([]string{"OnOff", "Thermostat"})
	if dt != "thermostat" {
		t.Errorf("expected thermostat, got %q", dt)
	}
}

func TestDeviceTypeFromClusters_ColorLight(t *testing.T) {
	dt := DeviceTypeFromClusters([]string{"OnOff", "LevelControl", "ColorControl"})
	if dt != "color_light" {
		t.Errorf("expected color_light, got %q", dt)
	}
}

func TestDeviceTypeFromClusters_DimmableLight(t *testing.T) {
	dt := DeviceTypeFromClusters([]string{"OnOff", "LevelControl"})
	if dt != "dimmable_light" {
		t.Errorf("expected dimmable_light, got %q", dt)
	}
}

func TestDeviceTypeFromClusters_Switch(t *testing.T) {
	dt := DeviceTypeFromClusters([]string{"OnOff"})
	if dt != "switch" {
		t.Errorf("expected switch, got %q", dt)
	}
}

func TestDeviceTypeFromClusters_Lock(t *testing.T) {
	dt := DeviceTypeFromClusters([]string{"DoorLock"})
	if dt != "lock" {
		t.Errorf("expected lock, got %q", dt)
	}
}

func TestDeviceTypeFromClusters_Unknown(t *testing.T) {
	dt := DeviceTypeFromClusters([]string{})
	if dt != "unknown" {
		t.Errorf("expected unknown, got %q", dt)
	}
}

func TestAllSupportedClusters_HaveHAPService(t *testing.T) {
	for name, def := range SupportedClusters {
		if def.HAPService == "" {
			t.Errorf("cluster %s has no HAP service mapping", name)
		}
		if def.Name != name {
			t.Errorf("cluster %s has mismatched Name field: %q", name, def.Name)
		}
	}
}
