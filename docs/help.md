# Homebase

Matter smart home control panel with HomeKit bridge and plugin architecture.

## Overview

Homebase is a control panel for smart home devices. It commissions Matter/Thread devices, exposes them to Apple HomeKit via HAP bridge, and provides a REST API for AI/automation control. Third-party integrations (Ring, etc.) are added through plugins.

## Architecture

- **Homebase** (Go, port 9221) — device registry, HAP bridge, REST API, UI, plugin discovery
- **Matter Sidecar** (Python, port 9222) — thin REST wrapper over python-chip-controller for Matter protocol
- **Plugins** (e.g. homebase-ring, port 9223) — standalone apps discovered via mDNS, synced automatically

## Getting Started

### 1. Commission a Matter Device

POST to `/api/devices` with the device's setup code and a friendly name:

```json
POST /api/devices
{"setup_code": "34970112332", "name": "Kitchen Light", "room": "Kitchen"}
```

The device appears in HomeKit and is controllable via REST API.

### 2. Create a Virtual Device

POST to `/api/virtual-devices` to create a dummy switch visible in HomeKit:

```json
POST /api/virtual-devices
{"name": "Test Switch", "device_type": "switch", "room": "Office"}
```

Supported virtual types: `switch`, `dimmable_light`, `color_light`, `fan`, `lock`.

### 3. Set Up a Plugin (e.g. Ring)

1. Store your Ring refresh token in Vault (key: `ring_refresh_token`)
2. Start `homebase-ring` — it broadcasts via mDNS and Homebase discovers it automatically
3. In Homebase UI, click the **Plugins** button in the sidebar
4. Paste the Vault public ID for your Ring credential and click **Save**
5. Homebase pushes the credential to the plugin, which authenticates and starts serving devices
6. Ring devices appear in Homebase within 60 seconds

## Sending Commands

### Matter Devices

POST to `/api/devices/{id}/command` with the Matter cluster and command:

```json
{"cluster": "OnOff", "command": "On"}
```

### Plugin Devices (e.g. Ring)

Commands are proxied to the plugin. Ring commands use a flat format:

```json
{"command": "light_on"}
```

Available Ring commands: `light_on`, `light_off`, `siren_on`, `siren_off`, `chime_play`, `chime_volume`.

## Permissions

| Scope | Allowed Operations |
|-------|-------------------|
| read | List devices, get state, view snapshots, list plugins |
| write | Commission/remove devices, send commands, create virtual devices |
| admin | Configure plugin credentials (vault IDs) |

All requests require `Authorization: Bearer {token}` header.

## Supported Matter Clusters

- **OnOff** — switches, plugs
- **LevelControl** — dimmable lights
- **ColorControl** — color lights (hue, saturation, color temperature)
- **Thermostat** — heating/cooling setpoints
- **DoorLock** — lock/unlock
- **WindowCovering** — blinds, shades
- **FanControl** — fan speed and mode
- **OccupancySensing** — motion/presence sensors
- **TemperatureMeasurement** — temperature sensors
- **RelativeHumidityMeasurement** — humidity sensors

## Plugin Architecture

Plugins are standalone apps that broadcast via mDNS with `plugin_type=homebase-plugin` TXT record. Homebase discovers them automatically and syncs their devices every 60 seconds.

### Writing a Plugin

1. Create a standalone Go HTTP server with these endpoints:
   - `GET /health.json` — must include `"plugin_type": "homebase-plugin"` and `"plugin_for": "homebase"`
   - `GET /api/devices` — return array of devices
   - `POST /api/devices/{id}/command` — accept commands
   - `POST /api/configure` — accept `{"vault_public_id": "..."}` for credential setup
2. Broadcast via mDNS as `_localitas-app._tcp` with TXT records: `name=your-plugin`, `plugin_type=homebase-plugin`, `plugin_for=homebase`
3. Use standard Makefile/plist pattern for launchd management

### Plugin Credential Flow

1. Admin stores credential in Vault (e.g. API key, refresh token)
2. Admin configures the Vault public ID in Homebase via UI or `PUT /api/plugins/{name}/credential`
3. Homebase calls `POST /api/configure` on the plugin with the vault public ID
4. Plugin fetches secrets from Vault and initializes

## API Reference

Full API documentation is available at `/swagger.json`.
