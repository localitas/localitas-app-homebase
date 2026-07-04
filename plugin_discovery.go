package homebase

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/grandcat/zeroconf"
)

const (
	pluginScanInterval = 30 * time.Second
	deviceSyncInterval = 60 * time.Second
)

type DiscoveredPlugin struct {
	Name   string
	URL    string
	Client *PluginClient
	Health *PluginHealth
}

type PluginDiscovery struct {
	mu      sync.RWMutex
	plugins map[string]*DiscoveredPlugin
	store   *Store
	hap     *HAPBridge
}

func NewPluginDiscovery(store *Store, hap *HAPBridge) *PluginDiscovery {
	return &PluginDiscovery{
		plugins: make(map[string]*DiscoveredPlugin),
		store:   store,
		hap:     hap,
	}
}

func (pd *PluginDiscovery) Start(ctx context.Context) {
	logger.Info("starting plugin discovery")
	go pd.scanLoop(ctx)
	go pd.syncLoop(ctx)
}

func (pd *PluginDiscovery) GetPlugin(name string) *DiscoveredPlugin {
	pd.mu.RLock()
	defer pd.mu.RUnlock()
	return pd.plugins[name]
}

func (pd *PluginDiscovery) ListPlugins() []DiscoveredPlugin {
	pd.mu.RLock()
	defer pd.mu.RUnlock()
	out := make([]DiscoveredPlugin, 0, len(pd.plugins))
	for _, p := range pd.plugins {
		out = append(out, *p)
	}
	return out
}

func (pd *PluginDiscovery) scanLoop(ctx context.Context) {
	pd.scan(ctx)

	ticker := time.NewTicker(pluginScanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pd.scan(ctx)
		}
	}
}

func (pd *PluginDiscovery) scan(ctx context.Context) {
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		logger.Error("plugin discovery resolver error", "error", err)
		return
	}

	entries := make(chan *zeroconf.ServiceEntry, 10)
	scanCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	go func() {
		if err := resolver.Browse(scanCtx, AppServiceType, AppServiceDomain, entries); err != nil {
			if err != context.DeadlineExceeded && err != context.Canceled {
				logger.Error("plugin discovery browse error", "error", err)
			}
		}
	}()

	for {
		select {
		case entry := <-entries:
			if entry == nil {
				continue
			}
			pd.handleEntry(entry)
		case <-scanCtx.Done():
			return
		}
	}
}

func (pd *PluginDiscovery) handleEntry(entry *zeroconf.ServiceEntry) {
	txtMap := parseTXTRecords(entry.Text)

	if txtMap["plugin_type"] != "homebase-plugin" {
		return
	}
	if txtMap["plugin_for"] != "homebase" {
		return
	}

	name := txtMap["name"]
	if name == "" {
		return
	}

	pd.mu.RLock()
	_, exists := pd.plugins[name]
	pd.mu.RUnlock()
	if exists {
		return
	}

	if len(entry.AddrIPv4) == 0 {
		return
	}

	addr := entry.AddrIPv4[0].String()
	port := entry.Port
	baseURL := fmt.Sprintf("http://%s:%d", addr, port)

	pc := NewPluginClient(baseURL, name)
	health, err := pc.FetchHealth()
	if err != nil {
		logger.Warn("plugin unreachable", "plugin", name, "url", baseURL, "error", err)
		return
	}

	if health.PluginType != "homebase-plugin" {
		return
	}

	plugin := &DiscoveredPlugin{
		Name:   name,
		URL:    baseURL,
		Client: pc,
		Health: health,
	}

	pd.mu.Lock()
	pd.plugins[name] = plugin
	pd.mu.Unlock()

	logger.Info("discovered plugin", "display_name", health.DisplayName, "plugin", name, "url", baseURL)

	pd.ConfigurePlugin(context.Background(), plugin)
	go pd.syncPlugin(context.Background(), plugin)
}

func (pd *PluginDiscovery) ConfigurePlugin(ctx context.Context, plugin *DiscoveredPlugin) {
	cred, err := pd.store.GetPluginCredential(ctx, plugin.Name)
	if err != nil {
		return
	}
	err = plugin.Client.Configure(cred.VaultPublicID)
	if err != nil {
		logger.Error("plugin configure failed", "plugin", plugin.Name, "error", err)
		return
	}
	logger.Info("plugin configured with vault credential", "plugin", plugin.Name, "vault_public_id", cred.VaultPublicID)
}

func (pd *PluginDiscovery) syncLoop(ctx context.Context) {
	ticker := time.NewTicker(deviceSyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pd.syncAll(ctx)
		}
	}
}

func (pd *PluginDiscovery) syncAll(ctx context.Context) {
	pd.mu.RLock()
	plugins := make([]*DiscoveredPlugin, 0, len(pd.plugins))
	for _, p := range pd.plugins {
		plugins = append(plugins, p)
	}
	pd.mu.RUnlock()

	for _, p := range plugins {
		if !p.Client.Healthy() {
			logger.Warn("plugin unhealthy, marking devices offline", "plugin", p.Name)
			pd.store.MarkOfflineBySource(ctx, p.Name)
			continue
		}
		pd.syncPlugin(ctx, p)
	}
}

func (pd *PluginDiscovery) syncPlugin(ctx context.Context, plugin *DiscoveredPlugin) {
	devices, err := plugin.Client.FetchDevices()
	if err != nil {
		logger.Error("plugin device fetch failed", "plugin", plugin.Name, "error", err)
		return
	}

	seenIDs := make(map[string]bool)

	for _, pd_dev := range devices {
		sourceID := strconv.FormatInt(pd_dev.ID, 10)
		seenIDs[sourceID] = true

		vendor := plugin.Health.DisplayName
		model := pd_dev.Kind

		dev, err := pd.store.UpsertPluginDevice(
			ctx,
			plugin.Name,
			sourceID,
			pd_dev.Name,
			pd_dev.DeviceType,
			vendor,
			model,
			pd_dev.IsOnline,
		)
		if err != nil {
			logger.Error("plugin upsert device failed", "plugin", plugin.Name, "device", pd_dev.Name, "error", err)
			continue
		}

		if pd.hap != nil {
			pd.hap.AddDevice(dev)
		}
	}

	existing, err := pd.store.ListDevicesBySource(ctx, plugin.Name)
	if err != nil {
		return
	}
	for _, d := range existing {
		if !seenIDs[d.SourceID] {
			if pd.hap != nil {
				pd.hap.RemoveDevice(d.ID)
			}
			pd.store.DeleteDevice(ctx, d.ID)
			logger.Info("removed stale device", "plugin", plugin.Name, "device", d.Name, "device_id", d.ID)
		}
	}

	logger.Info("synced plugin devices", "plugin", plugin.Name, "count", len(devices))
}

func parseTXTRecords(records []string) map[string]string {
	m := make(map[string]string)
	for _, r := range records {
		for i, c := range r {
			if c == '=' {
				m[r[:i]] = r[i+1:]
				break
			}
		}
	}
	return m
}
