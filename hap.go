package homebase

import (
	"context"
	"fmt"
	"log"
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
	mu         sync.RWMutex
	server     *hap.Server
	bridge     *accessory.Bridge
	devices    map[string]*hapDevice
	sidecar    *SidecarClient
	store      *Store
	pin        string
	storageDir string
}

func NewHAPBridge(sidecar *SidecarClient, store *Store, pin, storageDir string) *HAPBridge {
	return &HAPBridge{
		devices:    make(map[string]*hapDevice),
		sidecar:    sidecar,
		store:      store,
		pin:        pin,
		storageDir: storageDir,
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

	server, err := hap.NewServer(hap.NewFsStore(h.storageDir), h.bridge.A, accs...)
	if err != nil {
		return fmt.Errorf("create HAP server: %w", err)
	}
	server.Pin = h.pin
	h.server = server

	go func() {
		if err := server.ListenAndServe(ctx); err != nil {
			log.Printf("HAP server error: %v", err)
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
	if onoff, ok := state.Clusters["OnOff"]; ok {
		if val, ok := onoff["OnOff"]; ok {
			if b, ok := val.(bool); ok {
				if hd.switchSvc != nil {
					hd.switchSvc.On.SetValue(b)
				}
				if hd.lightSvc != nil {
					hd.lightSvc.On.SetValue(b)
				}
			}
		}
	}

	if level, ok := state.Clusters["LevelControl"]; ok {
		if val, ok := level["CurrentLevel"]; ok {
			if hd.brightness != nil {
				if f, ok := val.(float64); ok {
					pct := int(f / 254.0 * 100.0)
					hd.brightness.SetValue(pct)
				}
			}
		}
	}

	if color, ok := state.Clusters["ColorControl"]; ok {
		if val, ok := color["CurrentHue"]; ok {
			if hd.hue != nil {
				if f, ok := val.(float64); ok {
					hd.hue.SetValue(f / 254.0 * 360.0)
				}
			}
		}
		if val, ok := color["CurrentSaturation"]; ok {
			if hd.saturation != nil {
				if f, ok := val.(float64); ok {
					hd.saturation.SetValue(f / 254.0 * 100.0)
				}
			}
		}
	}

	if thermo, ok := state.Clusters["Thermostat"]; ok {
		if hd.thermoSvc != nil {
			if val, ok := thermo["LocalTemperature"]; ok {
				if f, ok := val.(float64); ok {
					hd.thermoSvc.CurrentTemperature.SetValue(f / 100.0)
				}
			}
		}
	}

	if temp, ok := state.Clusters["TemperatureMeasurement"]; ok {
		if hd.tempSvc != nil {
			if val, ok := temp["MeasuredValue"]; ok {
				if f, ok := val.(float64); ok {
					hd.tempSvc.CurrentTemperature.SetValue(f / 100.0)
				}
			}
		}
	}

	if occ, ok := state.Clusters["OccupancySensing"]; ok {
		if hd.occSvc != nil {
			if val, ok := occ["Occupancy"]; ok {
				if f, ok := val.(float64); ok {
					hd.occSvc.OccupancyDetected.SetValue(int(f))
				}
			}
		}
	}

	if lock, ok := state.Clusters["DoorLock"]; ok {
		if hd.lockSvc != nil {
			if val, ok := lock["LockState"]; ok {
				if f, ok := val.(float64); ok {
					hd.lockSvc.LockCurrentState.SetValue(int(f))
				}
			}
		}
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
		level := int(float64(v) / 100.0 * 254.0)
		h.sendCommand(dev.NodeID, "LevelControl", "MoveToLevel", map[string]interface{}{
			"level":           level,
			"transition_time": 5,
		})
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
		level := int(float64(v) / 100.0 * 254.0)
		h.sendCommand(dev.NodeID, "LevelControl", "MoveToLevel", map[string]interface{}{
			"level":           level,
			"transition_time": 5,
		})
	})
	svc.AddC(bright.C)

	hueC := characteristic.NewHue()
	satC := characteristic.NewSaturation()
	hueC.OnValueRemoteUpdate(func(v float64) {
		hueVal := int(v / 360.0 * 254.0)
		satVal := int(satC.Value() / 100.0 * 254.0)
		h.sendCommand(dev.NodeID, "ColorControl", "MoveToHueAndSaturation", map[string]interface{}{
			"hue":             hueVal,
			"saturation":      satVal,
			"transition_time": 5,
		})
	})
	satC.OnValueRemoteUpdate(func(v float64) {
		hueVal := int(hueC.Value() / 360.0 * 254.0)
		satVal := int(v / 100.0 * 254.0)
		h.sendCommand(dev.NodeID, "ColorControl", "MoveToHueAndSaturation", map[string]interface{}{
			"hue":             hueVal,
			"saturation":      satVal,
			"transition_time": 5,
		})
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
		setpoint := int(v * 100)
		h.sendCommand(dev.NodeID, "Thermostat", "SetpointRaiseLower", map[string]interface{}{
			"mode":   0,
			"amount": setpoint,
		})
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

func (h *HAPBridge) sendCommand(nodeID uint64, cluster, command string, args map[string]interface{}) {
	result, err := h.sidecar.SendCommand(nodeID, cluster, command, args)
	if err != nil {
		log.Printf("HAP command error (node %d, %s/%s): %v", nodeID, cluster, command, err)
		return
	}
	if !result.Success {
		log.Printf("HAP command failed (node %d, %s/%s): %s", nodeID, cluster, command, result.Error)
	}
}
