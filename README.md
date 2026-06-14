# Homebase

Smart home control panel with Matter support and HomeKit bridge.

Part of the [Localitas](https://github.com/localitas) platform — a self-hosted, privacy-first personal computing system.

## Features

- Commission Matter/Thread smart home devices
- HomeKit bridge via HAP — all devices appear in Apple Home
- Virtual (dummy) devices for testing and automation triggers
- Plugin architecture for third-party integrations (Ring, etc.)
- Plugin credential management via Vault
- REST API with Swagger for AI/automation control
- 10 supported Matter clusters (OnOff, LevelControl, ColorControl, Thermostat, DoorLock, etc.)

## Installation

### Development (via Localitas core)

```bash
# Clone the repo
git clone https://github.com/localitas/localitas-app-homebase.git ~/localitas-app-homebase

# Start with the Localitas dev cluster (builds and runs in Docker automatically)
cd ~/localitas && make dev-core
```

### Standalone

```bash
cd ~/localitas-app-homebase

# Build and run locally
make build
./bin/homebase-server serve --listen :8000

# Or via launchd (macOS)
make start

# Or via Docker
make start-docker
```

## Exposing to the Internet

Localitas apps are accessible remotely through Localitas's built-in tunnel service, powered by FRP. No port forwarding or dynamic DNS required.

1. Sign up at [localitas.com](https://localitas.com) and connect your local Localitas core
2. The tunnel automatically exposes your core (and all apps) at `https://{your-subdomain}.localitas.com`
3. This app is available at `https://{your-subdomain}.localitas.com/apps/ext/homebase/`

All traffic is encrypted end-to-end. Authentication is handled by the Localitas core — only authorized users can access your apps.

## License

MIT
