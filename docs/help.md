# Homebase

Matter smart home control panel with HomeKit bridge and plugin architecture.

## Overview

Homebase is a control panel for smart home devices. It commissions Matter/Thread devices, exposes them to Apple HomeKit via HAP bridge, and provides a REST API for AI/automation control. Third-party integrations (Ring, etc.) are added through plugins.

## Architecture

- **Homebase** (Go, port 9221) — device registry, HAP bridge, REST API, UI, plugin discovery
- **Matter Sidecar** (Python, port 9222) — thin REST wrapper over python-matter-server for the Matter protocol; Homebase reaches it at `--sidecar-url` (default `http://localhost:9222`)
- **Plugins** (e.g. homebase-ring, port 9223) — apps Localitas serves that provide homebase's plugin interface; homebase follows them through Localitas

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
2. Start `homebase-ring` — once Localitas serves it, Homebase takes it on
3. In Homebase UI, click the **Plugins** button in the sidebar
4. Paste the Vault public ID for your Ring credential and click **Save**
5. Homebase pushes the credential to the plugin, which authenticates and starts serving devices
6. Ring devices appear in Homebase as soon as the plugin reports them

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

A plugin is any app Localitas serves whose `health.json` lists `homebase-devices/v1` in `provides`. Homebase follows Localitas's app changes: when such an app is registered it is taken on (configured, its devices read); when it is removed its devices go offline. Nothing polls: a plugin tells Homebase its devices changed by publishing `{"plugin": "<its app name>"}` on the `homebase` cache's `devices` channel, and Homebase then reads its devices. Homebase reaches plugins through Localitas (`/apps/ext/<plugin>/`), as the admin it runs as.

A hint is only a hint: anyone with write scope can publish one, so Homebase never acts on its content, only reads the app list or a known plugin's devices.

### Writing a Plugin

1. Create a standalone app with these endpoints:
   - `GET /health.json` — must list `"provides": ["homebase-devices/v1"]`
   - `GET /api/devices` — return array of devices; answer an error (not an empty list) while not configured
   - `POST /api/devices/{id}/command` — accept commands
   - `POST /api/configure` — accept `{"vault_public_id": "..."}` for credential setup
2. Announce it to Localitas (`client.Announce`) or run it through the App Store
3. Publish `{"plugin": "<name>"}` on `homebase`/`devices` whenever its devices change

### Plugin Credential Flow

1. Admin stores credential in Vault (e.g. API key, refresh token)
2. Admin configures the Vault public ID in Homebase via UI or `PUT /api/plugins/{name}/credential`
3. Homebase calls `POST /api/configure` on the plugin with the vault public ID
4. Plugin fetches secrets from Vault and initializes

## API Reference

Full API documentation is available at `/swagger.json`.
