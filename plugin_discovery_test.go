package homebase

import "testing"

func TestParseTXTRecords(t *testing.T) {
	records := []string{
		"name=homebase-ring",
		"plugin_type=homebase-plugin",
		"plugin_for=homebase",
	}

	m := parseTXTRecords(records)

	if m["name"] != "homebase-ring" {
		t.Errorf("expected name=homebase-ring, got %q", m["name"])
	}
	if m["plugin_type"] != "homebase-plugin" {
		t.Errorf("expected plugin_type=homebase-plugin, got %q", m["plugin_type"])
	}
	if m["plugin_for"] != "homebase" {
		t.Errorf("expected plugin_for=homebase, got %q", m["plugin_for"])
	}
}

func TestParseTXTRecords_Empty(t *testing.T) {
	m := parseTXTRecords(nil)
	if len(m) != 0 {
		t.Errorf("expected empty map, got %d entries", len(m))
	}
}

func TestParseTXTRecords_NoEquals(t *testing.T) {
	m := parseTXTRecords([]string{"noequals"})
	if len(m) != 0 {
		t.Errorf("expected empty map for record without =, got %d entries", len(m))
	}
}

func TestPluginDiscovery_GetPlugin_NotFound(t *testing.T) {
	pd := NewPluginDiscovery(nil, nil)
	if pd.GetPlugin("nonexistent") != nil {
		t.Error("expected nil for nonexistent plugin")
	}
}

func TestPluginDiscovery_ListPlugins_Empty(t *testing.T) {
	pd := NewPluginDiscovery(nil, nil)
	plugins := pd.ListPlugins()
	if len(plugins) != 0 {
		t.Errorf("expected 0 plugins, got %d", len(plugins))
	}
}
