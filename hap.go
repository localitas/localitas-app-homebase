package homebase

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/brutella/hap"
	"github.com/brutella/hap/accessory"
	"github.com/brutella/hap/characteristic"
	"github.com/brutella/hap/service"
)

type hapDevice struct {
	acc        *accessory.A
	deviceID   string
	nodeID     uint64
	deviceType string
	switchSvc  *service.Switch
	lightSvc   *service.Lightbulb
	thermoSvc  *service.Thermostat
	lockSvc    *service.LockMechanism
	fanSvc     *service.FanV2
	tempSvc    *service.TemperatureSensor
	occSvc     *service.OccupancySensor
	brightness *characteristic.Brightness
	hue        *characteristic.Hue
	saturation *characteristic.Saturation
}

type HAPBridge struct {
	mu      sync.RWMutex
	server  *hap.Server
	bridge  *accessory.Bridge
	devices map[string]*hapDevice
	sidecar *SidecarClient
	store   *Store
	pin     string
}

func NewHAPBridge(sidecar *SidecarClient, store *Store, pin string) *HAPBridge {
	return &HAPBridge{
		devices: make(map[string]*hapDevice),
		sidecar: sidecar,
		store:   store,
		pin:     pin,
	}
}

func (h *HAPBridge) Start(ctx context.Context) error {
	h.bridge = accessory.NewBridge(accessory.Info{
		Name:         "Homebase",
		Manufacturer: "Localitas",
		Model:        "Homebase Bridge",
		SerialNumber: "HB-001",
	})

	devices, err := h.store.ListDevices(ctx)
	if err != nil {
		return fmt.Errorf("list devices for HAP: %w", err)
	}

	var accs []*accessory.A
	for _, dev := range devices {
		hd := h.createHAPDevice(dev)
		if hd != nil {
			h.mu.Lock()
			h.devices[dev.ID] = hd
			h.mu.Unlock()
			accs = append(accs, hd.acc)
		}
	}

	server, err := hap.NewServer(&hapStore{db: h.store.db}, h.bridge.A, accs...)
	if err != nil {
		return fmt.Errorf("create HAP server: %w", err)
	}
	server.Pin = h.pin
	h.server = server

	go func() {
		if err := server.ListenAndServe(ctx); err != nil {
			logger.Error("HAP server error", "error", err)
		}
	}()

	go h.pollLoop(ctx)

	return nil
}

func (h *HAPBridge) pollLoop(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.syncAllDevices()
		}
	}
}

func (h *HAPBridge) syncAllDevices() {
	h.mu.RLock()
	devicesCopy := make(map[string]*hapDevice, len(h.devices))
	for k, v := range h.devices {
		devicesCopy[k] = v
	}
	h.mu.RUnlock()

	for _, hd := range devicesCopy {
		state, err := h.sidecar.GetDeviceState(hd.nodeID)
		if err != nil {
			continue
		}
		h.applyState(hd, state)
	}
}

func (h *HAPBridge) applyState(hd *hapDevice, state *SidecarDeviceState) {
	if b, ok := attrAs[bool](state.Clusters["OnOff"], "OnOff"); ok {
		if hd.switchSvc != nil {
			hd.switchSvc.On.SetValue(b)
		}
		if hd.lightSvc != nil {
			hd.lightSvc.On.SetValue(b)
		}
	}

	if f, ok := attrAs[float64](state.Clusters["LevelControl"], "CurrentLevel"); ok && hd.brightness != nil {
		hd.brightness.SetValue(int(f / 254.0 * 100.0))
	}

	color := state.Clusters["ColorControl"]
	if f, ok := attrAs[float64](color, "CurrentHue"); ok && hd.hue != nil {
		hd.hue.SetValue(f / 254.0 * 360.0)
	}
	if f, ok := attrAs[float64](color, "CurrentSaturation"); ok && hd.saturation != nil {
		hd.saturation.SetValue(f / 254.0 * 100.0)
	}

	if f, ok := attrAs[float64](state.Clusters["Thermostat"], "LocalTemperature"); ok && hd.thermoSvc != nil {
		hd.thermoSvc.CurrentTemperature.SetValue(f / 100.0)
	}

	if f, ok := attrAs[float64](state.Clusters["TemperatureMeasurement"], "MeasuredValue"); ok && hd.tempSvc != nil {
		hd.tempSvc.CurrentTemperature.SetValue(f / 100.0)
	}

	if f, ok := attrAs[float64](state.Clusters["OccupancySensing"], "Occupancy"); ok && hd.occSvc != nil {
		hd.occSvc.OccupancyDetected.SetValue(int(f))
	}

	if f, ok := attrAs[float64](state.Clusters["DoorLock"], "LockState"); ok && hd.lockSvc != nil {
		hd.lockSvc.LockCurrentState.SetValue(int(f))
	}
}

func (h *HAPBridge) AddDevice(dev *Device) error {
	hd := h.createHAPDevice(dev)
	if hd == nil {
		return fmt.Errorf("unsupported device type for HAP: %s", dev.DeviceType)
	}

	h.mu.Lock()
	h.devices[dev.ID] = hd
	h.mu.Unlock()

	return nil
}

func (h *HAPBridge) RemoveDevice(deviceID string) {
	h.mu.Lock()
	delete(h.devices, deviceID)
	h.mu.Unlock()
}

func (h *HAPBridge) createHAPDevice(dev *Device) *hapDevice {
	info := accessory.Info{
		Name:         dev.Name,
		Manufacturer: dev.Vendor,
		Model:        dev.Model,
		SerialNumber: dev.ID,
	}

	hd := &hapDevice{
		deviceID:   dev.ID,
		nodeID:     dev.NodeID,
		deviceType: dev.DeviceType,
	}

	switch dev.DeviceType {
	case "switch":
		h.buildSwitch(info, dev, hd)
	case "dimmable_light":
		h.buildLightbulb(info, dev, hd)
	case "color_light":
		h.buildColorLight(info, dev, hd)
	case "thermostat":
		h.buildThermostat(info, dev, hd)
	case "lock":
		h.buildLock(info, dev, hd)
	case "fan":
		h.buildFan(info, dev, hd)
	case "temperature_sensor":
		h.buildTemperatureSensor(info, hd)
	case "occupancy_sensor":
		h.buildOccupancySensor(info, hd)
	default:
		h.buildSwitch(info, dev, hd)
	}

	return hd
}

func (h *HAPBridge) buildSwitch(info accessory.Info, dev *Device, hd *hapDevice) {
	acc := accessory.New(info, accessory.TypeSwitch)
	svc := service.NewSwitch()
	svc.On.OnValueRemoteUpdate(func(on bool) {
		cmd := "Off"
		if on {
			cmd = "On"
		}
		h.sendCommand(dev.NodeID, "OnOff", cmd, nil)
	})
	acc.AddS(svc.S)
	hd.acc = acc
	hd.switchSvc = svc
}

func (h *HAPBridge) buildLightbulb(info accessory.Info, dev *Device, hd *hapDevice) {
	acc := accessory.New(info, accessory.TypeLightbulb)
	svc := service.NewLightbulb()
	svc.On.OnValueRemoteUpdate(func(on bool) {
		cmd := "Off"
		if on {
			cmd = "On"
		}
		h.sendCommand(dev.NodeID, "OnOff", cmd, nil)
	})

	bright := characteristic.NewBrightness()
	bright.OnValueRemoteUpdate(func(v int) {
		sendCommandWith(h, dev.NodeID, "LevelControl", "MoveToLevel", moveToLevelFromBrightness(v))
	})
	svc.AddC(bright.C)

	acc.AddS(svc.S)
	hd.acc = acc
	hd.lightSvc = svc
	hd.brightness = bright
}

func (h *HAPBridge) buildColorLight(info accessory.Info, dev *Device, hd *hapDevice) {
	acc := accessory.New(info, accessory.TypeLightbulb)
	svc := service.NewLightbulb()
	svc.On.OnValueRemoteUpdate(func(on bool) {
		cmd := "Off"
		if on {
			cmd = "On"
		}
		h.sendCommand(dev.NodeID, "OnOff", cmd, nil)
	})

	bright := characteristic.NewBrightness()
	bright.OnValueRemoteUpdate(func(v int) {
		sendCommandWith(h, dev.NodeID, "LevelControl", "MoveToLevel", moveToLevelFromBrightness(v))
	})
	svc.AddC(bright.C)

	hueC := characteristic.NewHue()
	satC := characteristic.NewSaturation()
	hueC.OnValueRemoteUpdate(func(v float64) {
		sendCommandWith(h, dev.NodeID, "ColorControl", "MoveToHueAndSaturation", moveToHueAndSaturationFromHAP(v, satC.Value()))
	})
	satC.OnValueRemoteUpdate(func(v float64) {
		sendCommandWith(h, dev.NodeID, "ColorControl", "MoveToHueAndSaturation", moveToHueAndSaturationFromHAP(hueC.Value(), v))
	})
	svc.AddC(hueC.C)
	svc.AddC(satC.C)

	acc.AddS(svc.S)
	hd.acc = acc
	hd.lightSvc = svc
	hd.brightness = bright
	hd.hue = hueC
	hd.saturation = satC
}

func (h *HAPBridge) buildThermostat(info accessory.Info, dev *Device, hd *hapDevice) {
	acc := accessory.New(info, accessory.TypeThermostat)
	svc := service.NewThermostat()
	svc.TargetTemperature.OnValueRemoteUpdate(func(v float64) {
		sendCommandWith(h, dev.NodeID, "Thermostat", "SetpointRaiseLower", setpointRaiseLowerArgs{Mode: 0, Amount: int(v * 100)})
	})
	acc.AddS(svc.S)
	hd.acc = acc
	hd.thermoSvc = svc
}

func (h *HAPBridge) buildLock(info accessory.Info, dev *Device, hd *hapDevice) {
	acc := accessory.New(info, accessory.TypeDoorLock)
	svc := service.NewLockMechanism()
	svc.LockTargetState.OnValueRemoteUpdate(func(v int) {
		cmd := "UnlockDoor"
		if v == characteristic.LockTargetStateSecured {
			cmd = "LockDoor"
		}
		h.sendCommand(dev.NodeID, "DoorLock", cmd, nil)
	})
	acc.AddS(svc.S)
	hd.acc = acc
	hd.lockSvc = svc
}

func (h *HAPBridge) buildFan(info accessory.Info, dev *Device, hd *hapDevice) {
	acc := accessory.New(info, accessory.TypeFan)
	svc := service.NewFanV2()
	svc.Active.OnValueRemoteUpdate(func(v int) {
		cmd := "Off"
		if v == characteristic.ActiveActive {
			cmd = "On"
		}
		h.sendCommand(dev.NodeID, "OnOff", cmd, nil)
	})
	acc.AddS(svc.S)
	hd.acc = acc
	hd.fanSvc = svc
}

func (h *HAPBridge) buildTemperatureSensor(info accessory.Info, hd *hapDevice) {
	acc := accessory.New(info, accessory.TypeSensor)
	svc := service.NewTemperatureSensor()
	acc.AddS(svc.S)
	hd.acc = acc
	hd.tempSvc = svc
}

func (h *HAPBridge) buildOccupancySensor(info accessory.Info, hd *hapDevice) {
	acc := accessory.New(info, accessory.TypeSensor)
	svc := service.NewOccupancySensor()
	acc.AddS(svc.S)
	hd.acc = acc
	hd.occSvc = svc
}

// The arguments HAP sends with a Matter command, fields in key order as
// the maps they replaced marshaled them.
type moveToLevelArgs struct {
	Level          int `json:"level"`
	TransitionTime int `json:"transition_time"`
}

type moveToHueAndSaturationArgs struct {
	Hue            int `json:"hue"`
	Saturation     int `json:"saturation"`
	TransitionTime int `json:"transition_time"`
}

type setpointRaiseLowerArgs struct {
	Amount int `json:"amount"`
	Mode   int `json:"mode"`
}

// moveToLevelFromBrightness is MoveToLevel for HAP brightness pct (0-100).
func moveToLevelFromBrightness(pct int) moveToLevelArgs {
	return moveToLevelArgs{Level: int(float64(pct) / 100.0 * 254.0), TransitionTime: 5}
}

// moveToHueAndSaturationFromHAP is MoveToHueAndSaturation for HAP hue
// (0-360) and saturation (0-100).
func moveToHueAndSaturationFromHAP(hue, saturation float64) moveToHueAndSaturationArgs {
	return moveToHueAndSaturationArgs{
		Hue:            int(hue / 360.0 * 254.0),
		Saturation:     int(saturation / 100.0 * 254.0),
		TransitionTime: 5,
	}
}

// sendCommandWith sends a command with one of HAP's typed arguments.
func sendCommandWith[A moveToLevelArgs | moveToHueAndSaturationArgs | setpointRaiseLowerArgs](h *HAPBridge, nodeID uint64, cluster, command string, args A) {
	b, err := json.Marshal(args)
	if err != nil {
		logger.Error("HAP command arguments", "node_id", nodeID, "cluster", cluster, "command", command, "error", err)
		return
	}
	h.sendCommand(nodeID, cluster, command, b)
}

func (h *HAPBridge) sendCommand(nodeID uint64, cluster, command string, args json.RawMessage) {
	result, err := h.sidecar.SendCommand(nodeID, cluster, command, args)
	if err != nil {
		logger.Error("HAP command error", "node_id", nodeID, "cluster", cluster, "command", command, "error", err)
		return
	}
	if !result.Success {
		logger.Warn("HAP command failed", "node_id", nodeID, "cluster", cluster, "command", command, "error", result.Error)
	}
}
