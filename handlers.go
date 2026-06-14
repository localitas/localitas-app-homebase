package homebase

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type handler struct {
	app *App
}

func (h *handler) handleListDevices(w http.ResponseWriter, r *http.Request) {
	room := r.URL.Query().Get("room")

	var devices []*Device
	var err error
	if room != "" {
		devices, err = h.app.Store.ListDevicesByRoom(r.Context(), room)
	} else {
		devices, err = h.app.Store.ListDevices(r.Context())
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to list devices: %v", err)
		return
	}
	writeJSON(w, http.StatusOK, devices)
}

func (h *handler) handleGetDevice(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	dev, err := h.app.Store.GetDevice(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "device not found: %v", err)
		return
	}
	writeJSON(w, http.StatusOK, dev)
}

func (h *handler) handleGetDeviceState(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	dev, err := h.app.Store.GetDevice(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "device not found: %v", err)
		return
	}

	if dev.Virtual {
		writeJSON(w, http.StatusOK, DeviceState{
			NodeID: dev.NodeID,
			Online: true,
		})
		return
	}

	if dev.Source != "" {
		writeJSON(w, http.StatusOK, DeviceState{
			NodeID: dev.NodeID,
			Online: dev.Online,
		})
		return
	}

	state, err := h.app.Sidecar.GetDeviceState(dev.NodeID)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "sidecar error: %v", err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (h *handler) handleCommission(w http.ResponseWriter, r *http.Request) {
	var req CommissionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body: %v", err)
		return
	}
	if req.SetupCode == "" {
		writeErr(w, http.StatusBadRequest, "setup_code is required")
		return
	}
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}

	result, err := h.app.Sidecar.Commission(req.SetupCode)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "commissioning failed: %v", err)
		return
	}

	deviceType := DeviceTypeFromClusters(result.Clusters)

	dev, err := h.app.Store.CreateDevice(
		r.Context(),
		result.NodeID,
		req.Name,
		deviceType,
		req.Room,
		result.Vendor,
		result.Model,
		result.Clusters,
		false,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to save device: %v", err)
		return
	}

	if h.app.HAP != nil {
		if err := h.app.HAP.AddDevice(dev); err != nil {
			fmt.Printf("HAP add device warning: %v\n", err)
		}
	}

	writeJSON(w, http.StatusOK, dev)
}

func (h *handler) handleDecommission(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	dev, err := h.app.Store.GetDevice(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "device not found: %v", err)
		return
	}

	if !dev.Virtual {
		if err := h.app.Sidecar.Decommission(dev.NodeID); err != nil {
			writeErr(w, http.StatusBadGateway, "decommission failed: %v", err)
			return
		}
	}

	if h.app.HAP != nil {
		h.app.HAP.RemoveDevice(id)
	}

	if err := h.app.Store.DeleteDevice(r.Context(), id); err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to delete device: %v", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (h *handler) handleUpdateDevice(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var update DeviceUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body: %v", err)
		return
	}

	if err := h.app.Store.UpdateDevice(r.Context(), id, update); err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to update device: %v", err)
		return
	}

	dev, err := h.app.Store.GetDevice(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "device not found: %v", err)
		return
	}
	writeJSON(w, http.StatusOK, dev)
}

func (h *handler) handleCommand(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	dev, err := h.app.Store.GetDevice(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "device not found: %v", err)
		return
	}

	if dev.Source != "" {
		h.proxyPluginCommand(w, r, dev)
		return
	}

	var cmd CommandRequest
	if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body: %v", err)
		return
	}

	if cmd.Cluster == "" || cmd.Command == "" {
		writeErr(w, http.StatusBadRequest, "cluster and command are required")
		return
	}

	clusterDef, ok := ClusterDefFor(cmd.Cluster)
	if !ok {
		writeErr(w, http.StatusBadRequest, "unsupported cluster: %s", cmd.Cluster)
		return
	}

	validCmd := false
	for _, c := range clusterDef.Commands {
		if c.Name == cmd.Command {
			validCmd = true
			break
		}
	}
	if !validCmd {
		writeErr(w, http.StatusBadRequest, "unsupported command %s for cluster %s", cmd.Command, cmd.Cluster)
		return
	}

	if dev.Virtual {
		writeJSON(w, http.StatusOK, CommandResponse{Success: true})
		return
	}

	result, err := h.app.Sidecar.SendCommand(dev.NodeID, cmd.Cluster, cmd.Command, cmd.Arguments)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "command failed: %v", err)
		return
	}

	writeJSON(w, http.StatusOK, CommandResponse{
		Success: result.Success,
		Error:   result.Error,
	})
}

func (h *handler) handleCreateVirtual(w http.ResponseWriter, r *http.Request) {
	var req CreateVirtualRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body: %v", err)
		return
	}
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}

	deviceType := req.DeviceType
	if deviceType == "" {
		deviceType = "switch"
	}

	clusters := clustersForDeviceType(deviceType)

	nodeID, err := h.app.Store.NextVirtualNodeID(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to allocate node ID: %v", err)
		return
	}

	dev, err := h.app.Store.CreateDevice(
		r.Context(),
		nodeID,
		req.Name,
		deviceType,
		req.Room,
		"Homebase",
		"Virtual "+deviceType,
		clusters,
		true,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to save device: %v", err)
		return
	}

	if h.app.HAP != nil {
		if err := h.app.HAP.AddDevice(dev); err != nil {
			fmt.Printf("HAP add virtual device warning: %v\n", err)
		}
	}

	writeJSON(w, http.StatusOK, dev)
}

func clustersForDeviceType(dt string) []string {
	switch dt {
	case "switch":
		return []string{"OnOff"}
	case "dimmable_light":
		return []string{"OnOff", "LevelControl"}
	case "color_light":
		return []string{"OnOff", "LevelControl", "ColorControl"}
	case "fan":
		return []string{"OnOff", "FanControl"}
	case "lock":
		return []string{"DoorLock"}
	default:
		return []string{"OnOff"}
	}
}

func (h *handler) proxyPluginCommand(w http.ResponseWriter, r *http.Request, dev *Device) {
	if h.app.Plugins == nil {
		writeErr(w, http.StatusBadGateway, "plugin discovery not available")
		return
	}

	plugin := h.app.Plugins.GetPlugin(dev.Source)
	if plugin == nil {
		writeErr(w, http.StatusBadGateway, "plugin %s not found", dev.Source)
		return
	}

	data, status, err := plugin.Client.ProxyCommand(dev.SourceID, r.Body)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "plugin command failed: %v", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(data)
}

func (h *handler) handleGetSnapshot(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	dev, err := h.app.Store.GetDevice(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "device not found: %v", err)
		return
	}

	if dev.Source == "" {
		writeErr(w, http.StatusBadRequest, "snapshots only available for plugin devices")
		return
	}

	if h.app.Plugins == nil {
		writeErr(w, http.StatusBadGateway, "plugin discovery not available")
		return
	}

	plugin := h.app.Plugins.GetPlugin(dev.Source)
	if plugin == nil {
		writeErr(w, http.StatusBadGateway, "plugin %s not found", dev.Source)
		return
	}

	data, err := plugin.Client.GetSnapshot(dev.SourceID)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "snapshot failed: %v", err)
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Write(data)
}

func (h *handler) handleListPlugins(w http.ResponseWriter, r *http.Request) {
	if h.app.Plugins == nil {
		writeJSON(w, http.StatusOK, []DiscoveredPlugin{})
		return
	}
	writeJSON(w, http.StatusOK, h.app.Plugins.ListPlugins())
}

func (h *handler) handleGetPluginCredential(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	cred, err := h.app.Store.GetPluginCredential(r.Context(), name)
	if err != nil {
		writeErr(w, http.StatusNotFound, "no credential configured for plugin %s", name)
		return
	}
	writeJSON(w, http.StatusOK, cred)
}

func (h *handler) handleSetPluginCredential(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var req struct {
		VaultPublicID string `json:"vault_public_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body: %v", err)
		return
	}
	if req.VaultPublicID == "" {
		writeErr(w, http.StatusBadRequest, "vault_public_id is required")
		return
	}

	if err := h.app.Store.SetPluginCredential(r.Context(), name, req.VaultPublicID); err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to save credential: %v", err)
		return
	}

	if h.app.Plugins != nil {
		plugin := h.app.Plugins.GetPlugin(name)
		if plugin != nil {
			h.app.Plugins.ConfigurePlugin(r.Context(), plugin)
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "plugin_name": name, "vault_public_id": req.VaultPublicID})
}

func (h *handler) handleDeletePluginCredential(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := h.app.Store.DeletePluginCredential(r.Context(), name); err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to delete credential: %v", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (h *handler) handleListRooms(w http.ResponseWriter, r *http.Request) {
	rooms, err := h.app.Store.ListRooms(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to list rooms: %v", err)
		return
	}
	writeJSON(w, http.StatusOK, rooms)
}

func (h *handler) handleListClusters(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, SupportedClusters)
}

func (h *handler) handleSidecarHealth(w http.ResponseWriter, r *http.Request) {
	healthy := h.app.Sidecar.Healthy()
	status := map[string]interface{}{
		"sidecar_healthy": healthy,
		"sidecar_url":     h.app.SidecarURL,
	}
	writeJSON(w, http.StatusOK, status)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
