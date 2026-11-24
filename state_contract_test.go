package homebase

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// stateThroughHomebase serves body as the sidecar's /devices/7 and answers
// it the way GET /api/devices/{id}/state does, in the format accept asks.
func stateThroughHomebase(t *testing.T, body, accept string) string {
	t.Helper()
	sidecar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
	defer sidecar.Close()

	state, err := NewSidecarClient(sidecar.URL).GetDeviceState(7)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/devices/d/state", nil)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	rec := httptest.NewRecorder()
	writeJSON(rec, req, http.StatusOK, state)
	return rec.Body.String()
}

const sidecarState = `{"node_id":7,"online":true,"clusters":{"OnOff":{"OnOff":true},"LevelControl":{"CurrentLevel":254,"AcceptedCommandList":[0, 1],"NodeLabel":"lamp","OnLevel":null,"Ratio":1.5}}}`

func TestDeviceState_Bytes(t *testing.T) {
	cases := []struct {
		name, accept, want string
	}{
		{"json", "", `{"node_id":7,"online":true,"clusters":{"LevelControl":{"AcceptedCommandList":[0,1],"CurrentLevel":254,"NodeLabel":"lamp","OnLevel":null,"Ratio":1.5},"OnOff":{"OnOff":true}}}` + "\n"},
		{"yaml", "application/yaml", "nodeid: 7\nonline: true\nclusters:\n    LevelControl:\n        AcceptedCommandList:\n            - 0\n            - 1\n        CurrentLevel: 254\n        NodeLabel: lamp\n        OnLevel: null\n        Ratio: 1.5\n    OnOff:\n        OnOff: true\n"},
		{"markdown", "text/markdown", "| field | value |\n|---|---|\n| clusters.LevelControl.AcceptedCommandList | 0, 1 |\n| clusters.LevelControl.CurrentLevel | 254 |\n| clusters.LevelControl.NodeLabel | lamp |\n| clusters.LevelControl.OnLevel |  |\n| clusters.LevelControl.Ratio | 1.5 |\n| clusters.OnOff.OnOff | true |\n| node_id | 7 |\n| online | true |\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := stateThroughHomebase(t, sidecarState, c.accept); got != c.want {
				t.Errorf("got  %q\nwant %q", got, c.want)
			}
		})
	}
}

// A 64-bit attribute (a fabric ID, an event number) keeps its digits on
// the way through homebase.
func TestDeviceState_LargeIntegerKeepsItsDigits(t *testing.T) {
	got := stateThroughHomebase(t, `{"node_id":7,"online":true,"clusters":{"OperationalCredentials":{"FabricID":18446744073709551615}}}`, "")
	want := `{"node_id":7,"online":true,"clusters":{"OperationalCredentials":{"FabricID":18446744073709551615}}}` + "\n"
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}
