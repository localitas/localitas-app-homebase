package homebase

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	client "github.com/localitas/localitas-go"
)

var HomebaseAPIDoc = client.APISpec{
	AppName:     "Homebase",
	Version:     "0.1.0",
	Description: "Matter smart home control panel with HomeKit bridge and REST API",
	Keywords:    []string{"homebase", "matter", "homekit", "iot", "smart home", "devices", "automation", "lights", "thermostat", "lock"},
	Endpoints: []client.APIEndpoint{
		{
			Method:  "GET",
			Path:    "/api/devices",
			Summary: "List all devices",
			QueryParams: []client.APIParam{
				{Name: "room", Type: "string", Description: "Filter by room name"},
			},
			Response: &client.APIBody{ContentType: "application/json", Example: `[{"id":"abc...","node_id":1,"name":"Living Room Light","device_type":"dimmable_light","room":"Living Room","clusters":["OnOff","LevelControl"]}]`},
		},
		{
			Method:      "POST",
			Path:        "/api/devices",
			Summary:     "Commission a new Matter device",
			RequestBody: &client.APIBody{ContentType: "application/json", Example: `{"setup_code":"34970112332","name":"Kitchen Light","room":"Kitchen"}`},
			Response:    &client.APIBody{ContentType: "application/json", Example: `{"id":"abc...","node_id":2,"name":"Kitchen Light","device_type":"dimmable_light","room":"Kitchen","clusters":["OnOff","LevelControl"]}`},
		},
		{
			Method:   "GET",
			Path:     "/api/devices/{id}",
			Summary:  "Get device by ID",
			Response: &client.APIBody{ContentType: "application/json", Example: `{"id":"abc...","node_id":1,"name":"Living Room Light","device_type":"dimmable_light","room":"Living Room"}`},
		},
		{
			Method:   "GET",
			Path:     "/api/devices/{id}/state",
			Summary:  "Get live device state from Matter fabric",
			Response: &client.APIBody{ContentType: "application/json", Example: `{"node_id":1,"online":true,"clusters":{"OnOff":{"OnOff":true},"LevelControl":{"CurrentLevel":200}}}`},
		},
		{
			Method:      "PUT",
			Path:        "/api/devices/{id}",
			Summary:     "Update device name or room",
			RequestBody: &client.APIBody{ContentType: "application/json", Example: `{"name":"Desk Lamp","room":"Office"}`},
			Response:    &client.APIBody{ContentType: "application/json", Example: `{"id":"abc...","name":"Desk Lamp","room":"Office"}`},
		},
		{
			Method:   "DELETE",
			Path:     "/api/devices/{id}",
			Summary:  "Decommission and remove a device",
			Response: &client.APIBody{ContentType: "application/json", Example: `{"success":true}`},
		},
		{
			Method:      "POST",
			Path:        "/api/devices/{id}/command",
			Summary:     "Send a command to a device",
			RequestBody: &client.APIBody{ContentType: "application/json", Example: `{"cluster":"OnOff","command":"On","arguments":{}}`},
			Response:    &client.APIBody{ContentType: "application/json", Example: `{"success":true}`},
		},
		{
			Method:      "POST",
			Path:        "/api/virtual-devices",
			Summary:     "Create a virtual (dummy) device",
			RequestBody: &client.APIBody{ContentType: "application/json", Example: `{"name":"Test Switch","room":"Office","device_type":"switch"}`},
			Response:    &client.APIBody{ContentType: "application/json", Example: `{"id":"abc...","node_id":1000000,"name":"Test Switch","device_type":"switch","room":"Office","virtual":true}`},
		},
		{
			Method:   "GET",
			Path:     "/api/rooms",
			Summary:  "List all rooms",
			Response: &client.APIBody{ContentType: "application/json", Example: `["Living Room","Kitchen","Bedroom"]`},
		},
		{
			Method:   "GET",
			Path:     "/api/clusters",
			Summary:  "List supported Matter clusters and their commands",
			Response: &client.APIBody{ContentType: "application/json", Example: `{"OnOff":{"name":"OnOff","commands":[{"name":"On"},{"name":"Off"},{"name":"Toggle"}]}}`},
		},
		{
			Method:   "GET",
			Path:     "/api/sidecar/health",
			Summary:  "Check Matter sidecar health",
			Response: &client.APIBody{ContentType: "application/json", Example: `{"sidecar_healthy":true,"sidecar_url":"http://localhost:9222"}`},
		},
	},
}

func HandleSwagger(w http.ResponseWriter, r *http.Request) {
	HomebaseAPIDoc.ServeHTTP(w, r)
}

func RenderDocsHTML(doc client.APISpec) template.HTML {
	var sb strings.Builder
	sb.WriteString(`<h3 style="font-size: 0.875rem; font-weight: 600; text-transform: uppercase; letter-spacing: 0.05em; color: var(--color-text-secondary); margin-bottom: 1rem;">API Endpoints</h3><div class="accordion-list">`)
	for _, ep := range doc.Endpoints {
		title := fmt.Sprintf("%s %s — %s", ep.Method, ep.Path, ep.Summary)
		sb.WriteString(fmt.Sprintf(`<details class="glass-panel" style="border-radius: 0.5rem; margin-bottom: 0.5rem;"><summary style="padding: 0.75rem 1rem; cursor: pointer; font-weight: 500; color: var(--color-text-primary);">%s</summary><div style="padding: 0 1rem 0.75rem; font-size: 0.875rem; color: var(--color-text-secondary);">`, template.HTMLEscapeString(title)))
		var ex strings.Builder
		if ep.RequestBody != nil {
			ex.WriteString("# Request\n")
			ex.WriteString(prettyJSON(ep.RequestBody.Example))
			ex.WriteString("\n\n")
		}
		if ep.Response != nil {
			ex.WriteString("# Response\n")
			ex.WriteString(prettyJSON(ep.Response.Example))
		}
		if ex.Len() > 0 {
			sb.WriteString(fmt.Sprintf(`<pre style="background: var(--color-bg-base); padding: 0.75rem; border-radius: 0.375rem; overflow-x: auto; font-size: 0.8125rem;">%s</pre>`, template.HTMLEscapeString(ex.String())))
		}
		sb.WriteString(`</div></details>`)
	}
	sb.WriteString(`</div>`)
	return template.HTML(sb.String())
}

func prettyJSON(s string) string {
	var v interface{}
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return s
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return s
	}
	return string(b)
}
