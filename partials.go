package homebase

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
)

type partialHandler struct {
	app *App
}

type roomGroup struct {
	Name    string
	Devices []*Device
}

type sidebarData struct {
	Rooms      []roomGroup
	SelectedID string
}

func (p *partialHandler) handleSidebar(w http.ResponseWriter, r *http.Request) {
	devices, err := p.app.Store.ListDevices(r.Context())
	if err != nil {
		http.Error(w, "failed to list devices", http.StatusInternalServerError)
		return
	}

	data := sidebarData{
		Rooms:      groupByRoom(devices),
		SelectedID: r.URL.Query().Get("id"),
	}
	renderPartial(w, "templates/partials/_sidebar_list.html", "sidebar_list", data)
}

func (p *partialHandler) handleEmpty(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{
		"DocsHTML": RenderDocsHTML(HomebaseAPIDoc),
	}
	renderPartial(w, "templates/partials/_empty.html", "empty", data)
}

func (p *partialHandler) handleDeviceDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	dev, err := p.app.Store.GetDevice(r.Context(), id)
	if err != nil {
		p.handleEmpty(w, r)
		return
	}
	data := map[string]interface{}{
		"Device": dev,
	}
	renderPartial(w, "templates/partials/_device_detail.html", "device_detail", data)
}

func (p *partialHandler) handleUpdateDevice(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	name := r.FormValue("name")
	room := r.FormValue("room")

	if err := p.app.Store.UpdateDevice(r.Context(), id, DeviceUpdate{Name: name, Room: room}); err != nil {
		http.Error(w, "failed to update device", http.StatusInternalServerError)
		return
	}

	p.handleDeviceDetail(w, r)
}

func (p *partialHandler) handleDeleteDevice(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	dev, err := p.app.Store.GetDevice(r.Context(), id)
	if err != nil {
		http.Error(w, "device not found", http.StatusNotFound)
		return
	}

	if !dev.Virtual {
		if err := p.app.Sidecar.Decommission(dev.NodeID); err != nil {
			log.Printf("decommission warning for %s: %v", id, err)
		}
	}

	if p.app.HAP != nil {
		p.app.HAP.RemoveDevice(id)
	}

	if err := p.app.Store.DeleteDevice(r.Context(), id); err != nil {
		http.Error(w, "failed to delete device", http.StatusInternalServerError)
		return
	}

	p.handleEmpty(w, r)
}

func (p *partialHandler) handleAddForm(w http.ResponseWriter, r *http.Request) {
	renderPartial(w, "templates/partials/_add_form.html", "add_form", nil)
}

func (p *partialHandler) handleCommission(w http.ResponseWriter, r *http.Request) {
	setupCode := r.FormValue("setup_code")
	name := r.FormValue("name")
	room := r.FormValue("room")

	if setupCode == "" || name == "" {
		http.Error(w, "setup_code and name required", http.StatusBadRequest)
		return
	}

	result, err := p.app.Sidecar.Commission(setupCode)
	if err != nil {
		http.Error(w, "commissioning failed: "+err.Error(), http.StatusBadGateway)
		return
	}

	deviceType := DeviceTypeFromClusters(result.Clusters)

	dev, err := p.app.Store.CreateDevice(
		r.Context(),
		result.NodeID,
		name,
		deviceType,
		room,
		result.Vendor,
		result.Model,
		result.Clusters,
		false,
	)
	if err != nil {
		http.Error(w, "failed to save device: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if p.app.HAP != nil {
		if err := p.app.HAP.AddDevice(dev); err != nil {
			log.Printf("HAP add device warning: %v", err)
		}
	}

	data := map[string]interface{}{"Device": dev}
	renderPartialWithOOB(w, "templates/partials/_device_detail.html", "device_detail", data, p, r)
}

func (p *partialHandler) handleCreateVirtual(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	deviceType := r.FormValue("device_type")
	room := r.FormValue("room")

	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	if deviceType == "" {
		deviceType = "switch"
	}

	clusters := clustersForDeviceType(deviceType)

	nodeID, err := p.app.Store.NextVirtualNodeID(r.Context())
	if err != nil {
		http.Error(w, "failed to allocate node ID", http.StatusInternalServerError)
		return
	}

	dev, err := p.app.Store.CreateDevice(
		r.Context(),
		nodeID,
		name,
		deviceType,
		room,
		"Homebase",
		"Virtual "+deviceType,
		clusters,
		true,
	)
	if err != nil {
		http.Error(w, "failed to create device: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if p.app.HAP != nil {
		if err := p.app.HAP.AddDevice(dev); err != nil {
			log.Printf("HAP add virtual device warning: %v", err)
		}
	}

	data := map[string]interface{}{"Device": dev}
	renderPartialWithOOB(w, "templates/partials/_device_detail.html", "device_detail", data, p, r)
}

func (p *partialHandler) handlePluginSettings(w http.ResponseWriter, r *http.Request) {
	var plugins []DiscoveredPlugin
	if p.app.Plugins != nil {
		plugins = p.app.Plugins.ListPlugins()
	}

	credentials := make(map[string]string)
	creds, _ := p.app.Store.ListPluginCredentials(r.Context())
	for _, c := range creds {
		credentials[c.PluginName] = c.VaultPublicID
	}

	data := map[string]interface{}{
		"Plugins":     plugins,
		"Credentials": credentials,
	}
	renderPartial(w, "templates/partials/_plugin_settings.html", "plugin_settings", data)
}

func (p *partialHandler) handleSetPluginCredential(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	vaultID := r.FormValue("vault_public_id")

	if vaultID == "" {
		http.Error(w, "vault_public_id is required", http.StatusBadRequest)
		return
	}

	if err := p.app.Store.SetPluginCredential(r.Context(), name, vaultID); err != nil {
		http.Error(w, "failed to save credential", http.StatusInternalServerError)
		return
	}

	if p.app.Plugins != nil {
		plugin := p.app.Plugins.GetPlugin(name)
		if plugin != nil {
			p.app.Plugins.ConfigurePlugin(r.Context(), plugin)
		}
	}

	p.handlePluginSettings(w, r)
}

func (p *partialHandler) handleDeletePluginCredential(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	p.app.Store.DeletePluginCredential(r.Context(), name)
	p.handlePluginSettings(w, r)
}

func (p *partialHandler) handleSidecarStatus(w http.ResponseWriter, r *http.Request) {
	healthy := p.app.Sidecar.Healthy()
	data := map[string]bool{"Healthy": healthy}
	renderPartial(w, "templates/partials/_sidecar_status.html", "sidecar_status", data)
}

func renderPartial(w http.ResponseWriter, file, name string, data interface{}) {
	content, err := TemplatesFS.ReadFile(file)
	if err != nil {
		log.Printf("partial read error (%s): %v", file, err)
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}

	tmpl, err := template.New(name).Parse(string(content))
	if err != nil {
		log.Printf("partial parse error (%s): %v", file, err)
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("partial render error (%s): %v", file, err)
	}
}

func renderPartialWithOOB(w http.ResponseWriter, file, name string, data interface{}, p *partialHandler, r *http.Request) {
	content, err := TemplatesFS.ReadFile(file)
	if err != nil {
		log.Printf("partial read error (%s): %v", file, err)
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}

	tmpl, err := template.New(name).Parse(string(content))
	if err != nil {
		log.Printf("partial parse error (%s): %v", file, err)
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("partial render error (%s): %v", file, err)
	}

	devices, err := p.app.Store.ListDevices(r.Context())
	if err != nil {
		return
	}
	sidebarContent, err := TemplatesFS.ReadFile("templates/partials/_sidebar_list.html")
	if err != nil {
		return
	}
	sidebarTmpl, err := template.New("sidebar_list").Parse(string(sidebarContent))
	if err != nil {
		return
	}
	fmt.Fprint(w, `<div id="homebase-sidebar-list" hx-swap-oob="true">`)
	sidebarData := sidebarData{Rooms: groupByRoom(devices)}
	sidebarTmpl.ExecuteTemplate(w, "sidebar_list", sidebarData)
	fmt.Fprint(w, `</div>`)
}

func groupByRoom(devices []*Device) []roomGroup {
	grouped := make(map[string][]*Device)
	var roomOrder []string
	seen := make(map[string]bool)
	for _, d := range devices {
		room := d.Room
		if !seen[room] {
			seen[room] = true
			roomOrder = append(roomOrder, room)
		}
		grouped[room] = append(grouped[room], d)
	}

	var rooms []roomGroup
	for _, name := range roomOrder {
		rooms = append(rooms, roomGroup{Name: name, Devices: grouped[name]})
	}
	return rooms
}
