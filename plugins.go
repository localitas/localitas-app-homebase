package homebase

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"

	client "github.com/localitas/localitas-go"
)

// Homebase's contract with its plugins. A plugin is an app core serves that
// lists PluginInterface in its health.json "provides". It hints that its
// devices changed by publishing {"plugin": "<its app name>"} on
// DevicesCache/DevicesChannel; homebase then reads its devices.
const (
	PluginInterface = "homebase-devices/v1"
	DevicesCache    = "homebase"
	DevicesChannel  = "devices"
)

type DiscoveredPlugin struct {
	Name        string
	DisplayName string
	URL         string
	Client      *PluginClient
}

// PluginInfo is a plugin as GET /api/plugins lists it.
type PluginInfo struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	URL         string `json:"url"`
	Configured  bool   `json:"configured"`
}

// DeviceHint is a plugin's hint that its devices changed.
type DeviceHint struct {
	Plugin string `json:"plugin"`
}

// PluginRegistry keeps the plugins core serves, following core's app changes
// and the plugins' device hints; nothing polls. Hints are untrusted (any
// user with write scope can publish them): each one only makes homebase read
// core's app list or a known plugin's devices, through core's proxy.
type PluginRegistry struct {
	core    *client.Client
	coreURL string
	store   *Store
	hap     *HAPBridge

	mu      sync.RWMutex
	plugins map[string]*DiscoveredPlugin

	events chan pluginEvent
}

type pluginEvent struct {
	app   string // an app core announced, to read again
	sync  string // a plugin whose devices to read again
	sweep bool   // mark offline the devices of every source not a plugin now
}

func NewPluginRegistry(core *client.Client, coreURL string, store *Store, hap *HAPBridge) *PluginRegistry {
	return &PluginRegistry{
		core:    core,
		coreURL: strings.TrimRight(coreURL, "/"),
		store:   store,
		hap:     hap,
		plugins: make(map[string]*DiscoveredPlugin),
		events:  make(chan pluginEvent, 1024),
	}
}

// Start follows core's apps and the plugins' device hints until ctx ends.
// It watches before it reads, so a change is either in what it reads or
// heard after. Events are handled one at a time, in order.
func (pr *PluginRegistry) Start(ctx context.Context) error {
	if err := pr.core.CreateCache(ctx, DevicesCache); err != nil && !isConflict(err) {
		return err
	}
	apps, err := pr.core.WatchAppChanges(ctx, func(c client.AppChange) {
		pr.events <- pluginEvent{app: c.App}
	})
	if err != nil {
		return err
	}
	devices, err := pr.watchDeviceHints(ctx)
	if err != nil {
		apps.Close()
		return err
	}
	registered, err := pr.core.ListExternalApps(ctx)
	if err != nil {
		apps.Close()
		devices.Close()
		return err
	}
	for _, app := range registered {
		pr.events <- pluginEvent{app: app.Name}
	}
	pr.events <- pluginEvent{sweep: true}

	go func() {
		defer apps.Close()
		defer devices.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case ev := <-pr.events:
				switch {
				case ev.app != "":
					pr.refresh(ctx, ev.app)
				case ev.sync != "":
					if p := pr.GetPlugin(ev.sync); p != nil {
						pr.syncPlugin(ctx, p)
					}
				case ev.sweep:
					pr.sweep(ctx)
				}
			}
		}
	}()
	logger.Info("following core's apps for plugins", "interface", PluginInterface)
	return nil
}

func (pr *PluginRegistry) watchDeviceHints(ctx context.Context) (*client.PubSubWS, error) {
	noted, err := pr.core.Cache(DevicesCache).PubSub(DevicesChannel, client.PubSubOpts{}).LastSeq(ctx)
	if err != nil {
		return nil, err
	}
	ps := client.NewPubSubWS("ws"+strings.TrimPrefix(pr.coreURL, "http")+"/apps/cache/ws/"+DevicesCache, pr.coreToken())
	ps.Watch(DevicesChannel, noted, func(msg client.PubSubMessage) {
		var hint DeviceHint
		if json.Unmarshal([]byte(msg.Value), &hint) == nil && hint.Plugin != "" {
			pr.events <- pluginEvent{sync: hint.Plugin}
		}
	})
	return ps, nil
}

// refresh reads app name again: a plugin when core serves it and its
// health.json provides PluginInterface, else not one (any more).
func (pr *PluginRegistry) refresh(ctx context.Context, name string) {
	apps, err := pr.core.ListExternalApps(ctx)
	if err != nil {
		logger.Warn("app list not read; plugin left as it was", "app", name, "error", err)
		return
	}
	served := false
	for _, a := range apps {
		if a.Name == name {
			served = true
			break
		}
	}
	if !served {
		pr.remove(ctx, name)
		return
	}

	base := pr.coreURL + "/apps/ext/" + name
	pc := NewPluginClient(base, name)
	health, err := pc.FetchHealth()
	if err != nil {
		logger.Warn("app not read; plugin left as it was", "app", name, "error", err)
		return
	}
	if !health.Implements(PluginInterface) {
		pr.remove(ctx, name)
		return
	}

	plugin := &DiscoveredPlugin{Name: name, DisplayName: health.DisplayName, URL: base, Client: pc}
	pr.mu.Lock()
	pr.plugins[name] = plugin
	pr.mu.Unlock()
	logger.Info("plugin taken on", "plugin", name, "display_name", health.DisplayName)
	pr.ConfigurePlugin(ctx, plugin)
	pr.syncPlugin(ctx, plugin)
}

// remove drops name as a plugin, if it was one, and marks its devices
// offline.
func (pr *PluginRegistry) remove(ctx context.Context, name string) {
	pr.mu.Lock()
	_, was := pr.plugins[name]
	delete(pr.plugins, name)
	pr.mu.Unlock()
	if !was {
		return
	}
	if err := pr.store.MarkOfflineBySource(ctx, name); err != nil {
		logger.Error("removed plugin's devices not marked offline", "plugin", name, "error", err)
	}
	logger.Info("plugin gone; its devices are offline", "plugin", name)
}

// sweep marks offline the devices of plugins that are no plugin now: those
// that went while homebase was not running.
func (pr *PluginRegistry) sweep(ctx context.Context) {
	devices, err := pr.store.ListDevices(ctx)
	if err != nil {
		logger.Warn("devices not read for the sweep", "error", err)
		return
	}
	gone := map[string]bool{}
	for _, d := range devices {
		if d.Source != "" && d.Online && pr.GetPlugin(d.Source) == nil {
			gone[d.Source] = true
		}
	}
	for source := range gone {
		if err := pr.store.MarkOfflineBySource(ctx, source); err != nil {
			logger.Error("gone plugin's devices not marked offline", "plugin", source, "error", err)
		}
	}
}

func (pr *PluginRegistry) GetPlugin(name string) *DiscoveredPlugin {
	pr.mu.RLock()
	defer pr.mu.RUnlock()
	return pr.plugins[name]
}

// ListPlugins lists the plugins and whether each has its credential.
func (pr *PluginRegistry) ListPlugins(ctx context.Context) []PluginInfo {
	pr.mu.RLock()
	out := make([]PluginInfo, 0, len(pr.plugins))
	for _, p := range pr.plugins {
		out = append(out, PluginInfo{Name: p.Name, DisplayName: p.DisplayName, URL: p.URL})
	}
	pr.mu.RUnlock()
	for i := range out {
		_, err := pr.store.GetPluginCredential(ctx, out[i].Name)
		out[i].Configured = err == nil
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ConfigurePlugin hands a plugin its credential, if one is set for it.
func (pr *PluginRegistry) ConfigurePlugin(ctx context.Context, plugin *DiscoveredPlugin) {
	cred, err := pr.store.GetPluginCredential(ctx, plugin.Name)
	if err != nil {
		return
	}
	if err := plugin.Client.Configure(cred.VaultPublicID); err != nil {
		logger.Error("plugin configure failed", "plugin", plugin.Name, "error", err)
		return
	}
	logger.Info("plugin configured with vault credential", "plugin", plugin.Name, "vault_public_id", cred.VaultPublicID)
}

// syncPlugin reads a plugin's devices and keeps homebase's in step. A
// plugin that cannot list them (not configured, its service down) leaves
// them as they are.
func (pr *PluginRegistry) syncPlugin(ctx context.Context, plugin *DiscoveredPlugin) {
	devices, err := plugin.Client.FetchDevices()
	if err != nil {
		logger.Warn("plugin devices not read; left as they were", "plugin", plugin.Name, "error", err)
		return
	}

	seen := make(map[string]bool, len(devices))
	for _, d := range devices {
		sourceID := strconv.FormatInt(d.ID, 10)
		seen[sourceID] = true
		dev, err := pr.store.UpsertPluginDevice(ctx, plugin.Name, sourceID, d.Name, d.DeviceType, plugin.DisplayName, d.Kind, d.IsOnline)
		if err != nil {
			logger.Error("plugin upsert device failed", "plugin", plugin.Name, "device", d.Name, "error", err)
			continue
		}
		if pr.hap != nil {
			pr.hap.AddDevice(dev)
		}
	}

	existing, err := pr.store.ListDevicesBySource(ctx, plugin.Name)
	if err != nil {
		return
	}
	for _, d := range existing {
		if !seen[d.SourceID] {
			if pr.hap != nil {
				pr.hap.RemoveDevice(d.ID)
			}
			pr.store.DeleteDevice(ctx, d.ID)
			logger.Info("removed stale device", "plugin", plugin.Name, "device", d.Name, "device_id", d.ID)
		}
	}
	logger.Info("synced plugin devices", "plugin", plugin.Name, "count", len(devices))
}

func (pr *PluginRegistry) coreToken() string {
	return systemToken()
}

func isConflict(err error) bool {
	var apiErr *client.APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusConflict
}
