package homebase

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// commandSidecar is a sidecar that hands each command body it gets to the
// returned channel and answers success.
func commandSidecar(t *testing.T) (*SidecarClient, <-chan string) {
	t.Helper()
	bodies := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		bodies <- string(b)
		w.Write([]byte(`{"success":true}`))
	}))
	t.Cleanup(srv.Close)
	return NewSidecarClient(srv.URL), bodies
}

// The bodies HAP sends the sidecar are byte for byte what the untyped maps
// they replaced marshaled.
func TestHAPCommands_SidecarBodies(t *testing.T) {
	sc, bodies := commandSidecar(t)
	h := &HAPBridge{sidecar: sc}

	cases := []struct {
		name string
		send func()
		want string
	}{
		{"on", func() { h.sendCommand(1, "OnOff", "On", nil) }, `{"cluster":"OnOff","command":"On"}`},
		{"brightness", func() {
			sendCommandWith(h, 1, "LevelControl", "MoveToLevel", moveToLevelFromBrightness(50))
		}, `{"cluster":"LevelControl","command":"MoveToLevel","arguments":{"level":127,"transition_time":5}}`},
		{"hue and saturation", func() {
			sendCommandWith(h, 1, "ColorControl", "MoveToHueAndSaturation", moveToHueAndSaturationFromHAP(180, 50))
		}, `{"cluster":"ColorControl","command":"MoveToHueAndSaturation","arguments":{"hue":127,"saturation":127,"transition_time":5}}`},
		{"setpoint", func() {
			sendCommandWith(h, 1, "Thermostat", "SetpointRaiseLower", setpointRaiseLowerArgs{Mode: 0, Amount: 2150})
		}, `{"cluster":"Thermostat","command":"SetpointRaiseLower","arguments":{"amount":2150,"mode":0}}`},
	}
	for _, c := range cases {
		c.send()
		if got := <-bodies; got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got, c.want)
		}
	}
}

// An API caller's arguments reach the sidecar as the JSON object it sent,
// compacted; none at all (absent, null, {}) is left out as before. Key
// order is the caller's: the sidecar reads the object into a dict.
func TestAPICommand_SidecarBody(t *testing.T) {
	cases := []struct {
		name, body, want string
	}{
		{"arguments", `{"cluster":"LevelControl","command":"MoveToLevel","arguments":{ "level": 10, "transition_time": 5 }}`,
			`{"cluster":"LevelControl","command":"MoveToLevel","arguments":{"level":10,"transition_time":5}}`},
		{"no arguments", `{"cluster":"OnOff","command":"On"}`, `{"cluster":"OnOff","command":"On"}`},
		{"null arguments", `{"cluster":"OnOff","command":"On","arguments":null}`, `{"cluster":"OnOff","command":"On"}`},
		{"empty arguments", `{"cluster":"OnOff","command":"On","arguments":{ }}`, `{"cluster":"OnOff","command":"On"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sc, bodies := commandSidecar(t)
			var cmd CommandRequest
			if err := json.Unmarshal([]byte(c.body), &cmd); err != nil {
				t.Fatal(err)
			}
			args, err := commandArguments(cmd.Arguments)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := sc.SendCommand(1, cmd.Cluster, cmd.Command, args); err != nil {
				t.Fatal(err)
			}
			if got := <-bodies; got != c.want {
				t.Errorf("got  %s\nwant %s", got, c.want)
			}
		})
	}
}

func TestCommandArguments_NotAnObject(t *testing.T) {
	for _, raw := range []string{`[1]`, `"level"`, `5`} {
		if _, err := commandArguments(json.RawMessage(raw)); err == nil {
			t.Errorf("%s: want an error", raw)
		}
	}
}

func TestAttrAs(t *testing.T) {
	attrs := ClusterAttributes{"OnOff": "true", "CurrentLevel": "254", "OnLevel": "null", "Label": `"lamp"`}
	if on, ok := attrAs[bool](attrs, "OnOff"); !ok || !on {
		t.Errorf("OnOff: %v %v", on, ok)
	}
	if level, ok := attrAs[float64](attrs, "CurrentLevel"); !ok || level != 254 {
		t.Errorf("CurrentLevel: %v %v", level, ok)
	}
	if _, ok := attrAs[float64](attrs, "OnLevel"); ok {
		t.Error("a null attribute is not a value")
	}
	if _, ok := attrAs[float64](attrs, "Label"); ok {
		t.Error("a string attribute is not a number")
	}
	if _, ok := attrAs[bool](attrs, "Missing"); ok {
		t.Error("an absent attribute is not a value")
	}
	if _, ok := attrAs[bool](nil, "OnOff"); ok {
		t.Error("an absent cluster has no values")
	}
}
