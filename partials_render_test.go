package homebase

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDeviceDetailRenders(t *testing.T) {
	rec := httptest.NewRecorder()
	renderPartial(rec, "templates/partials/_device_detail.html", "device_detail", deviceDetailData{Device: &Device{
		ID: "dev-1", Name: "Desk Lamp", DeviceType: "dimmable_light", Room: "Office",
		Vendor: "Acme", Model: "L1", Clusters: []string{"OnOff", "LevelControl"}, Online: true,
	}})
	body := rec.Body.String()
	for _, want := range []string{"Desk Lamp", "dev-1", "Office", "Acme", "L1", "LevelControl", "dimmable_light"} {
		if !strings.Contains(body, want) {
			t.Errorf("device detail lacks %q", want)
		}
	}
	if !strings.HasSuffix(strings.TrimSpace(body), "</div>") {
		t.Errorf("device detail did not render to its end:\n%s", body)
	}
}

func TestPluginSettingsRenders(t *testing.T) {
	rec := httptest.NewRecorder()
	renderPartial(rec, "templates/partials/_plugin_settings.html", "plugin_settings", pluginSettingsData{
		Plugins: []PluginInfo{
			{Name: "ring", DisplayName: "Ring", URL: "http://127.0.0.1:9000"},
			{Name: "hue", DisplayName: "Hue", URL: "http://127.0.0.1:9001"},
		},
		Credentials: map[string]string{"ring": "vault-ring-1"},
	})
	body := rec.Body.String()
	for _, want := range []string{"Ring", "http://127.0.0.1:9000", `value="vault-ring-1"`, `hx-delete="partials/plugin-credential/ring"`, "Hue"} {
		if !strings.Contains(body, want) {
			t.Errorf("plugin settings lack %q", want)
		}
	}
	if strings.Contains(body, `hx-delete="partials/plugin-credential/hue"`) {
		t.Error("a plugin without a credential offers to delete one")
	}
	if !strings.HasSuffix(strings.TrimSpace(body), "</div>") {
		t.Errorf("plugin settings did not render to the end:\n%s", body)
	}
}

func TestPluginSettingsRenders_NoPlugins(t *testing.T) {
	rec := httptest.NewRecorder()
	renderPartial(rec, "templates/partials/_plugin_settings.html", "plugin_settings", pluginSettingsData{})
	if !strings.Contains(rec.Body.String(), "No plugins discovered yet.") {
		t.Errorf("empty plugin settings:\n%s", rec.Body.String())
	}
}
