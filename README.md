# HotSpotter

A single-binary Go web application for managing WiFi hotspots on Linux. Provides a browser-based interface for [linux-wifi-hotspot](https://github.com/lakinduakash/linux-wifi-hotspot) (`create_ap`), WiFi network scanning, and upstream connection management.

All assets (HTML, CSS, JS) are embedded in the binary — no external files needed at runtime.

## Features

- Create and manage WiFi hotspots with configurable SSID, passphrase, channel, WPA version, and gateway IP
- Scan for nearby WiFi networks and connect to share internet with hotspot clients
- View connected clients with IP address, MAC address, and hostname (DHCP leases)
- Remember WiFi networks and auto-reconnect on startup
- Single-interface mode: use one WiFi adapter for both AP and upstream connection
- Run as a systemd service for persistence across reboots
- Pre-built binaries for x86_64, ARM64, ARMv7, ARMv6, and RISC-V 64

## Prerequisites

- Linux with a WiFi adapter that supports AP mode
- [linux-wifi-hotspot](https://github.com/lakinduakash/linux-wifi-hotspot) installed (`create_ap` command available)
- NetworkManager (`nmcli`) for WiFi scanning and connection
- Root privileges (required for `create_ap` and network operations)

### Installing linux-wifi-hotspot

```bash
# Debian/Ubuntu
sudo apt install -y libgtk-3-dev build-essential gcc g++ \
  pkg-config make hostapd libqrencode-dev libpng-dev
git clone https://github.com/lakinduakash/linux-wifi-hotspot
cd linux-wifi-hotspot
make
sudo make install-cli-only
```

## Installation

### From release binaries

Download the appropriate binary from the [Releases](../../releases) page:

| Architecture | Binary |
|---|---|
| x86_64 | `hotspotter-linux-amd64` |
| ARM 64-bit | `hotspotter-linux-arm64` |
| ARM v7 (32-bit) | `hotspotter-linux-armv7` |
| ARM v6 (RPi Zero/1) | `hotspotter-linux-armv6` |
| RISC-V 64 | `hotspotter-linux-riscv64` |

```bash
chmod +x hotspotter-linux-amd64
sudo mv hotspotter-linux-amd64 /usr/local/bin/hotspotter
```

### From source

Requires Go 1.22 or later.

```bash
git clone https://github.com/hotspotter/hotspotter
cd hotspotter
go build -o hotspotter .
sudo mv hotspotter /usr/local/bin/
```

## Usage

### Quick start

```bash
sudo hotspotter
```

Open `http://localhost:8080` in a browser.

### Command-line options

```
Usage: hotspotter [options]

Options:
  --addr string             Listen address (default ":8080")
  --config-dir string       Directory for persistent config (default "/etc/hotspotter")
  --version                 Print version and exit
  --install-service         Install and enable systemd service
  --uninstall-service       Disable and remove systemd service
```

### Examples

```bash
# Run on a custom port
sudo hotspotter --addr :9090

# Run with a custom config directory
sudo hotspotter --addr :8080 --config-dir /opt/hotspotter

# Install as a systemd service (starts on boot)
sudo hotspotter --install-service

# Install with custom options baked into the service
sudo hotspotter --install-service --addr :9090 --config-dir /opt/hotspotter

# Remove the systemd service
sudo hotspotter --uninstall-service

# Check version
hotspotter --version
```

### Running as a systemd service

Install the service to start HotSpotter automatically on boot:

```bash
sudo hotspotter --install-service
```

This creates `/etc/systemd/system/hotspotter.service`, enables it, and starts it immediately. Manage with standard systemctl commands:

```bash
sudo systemctl status hotspotter
sudo systemctl restart hotspotter
sudo systemctl stop hotspotter
sudo systemctl journal -u hotspotter -f   # view logs
```

To remove:

```bash
sudo hotspotter --uninstall-service
```

## Web interface workflow

### Single-interface mode (one WiFi adapter)

When using the same WiFi interface for both the hotspot AP and upstream internet:

1. **Select interfaces** — choose the same WiFi interface for both "WiFi Interface (AP)" and "Internet Source"
2. **Connect upstream** — scan for and connect to an upstream WiFi network first
3. **Start hotspot** — configure and start the hotspot; a virtual AP interface is created automatically

### Dual-interface mode (two adapters, or WiFi + Ethernet)

1. **Select interfaces** — choose the WiFi adapter for AP and the internet-connected interface (Ethernet or second WiFi) for Internet Source
2. **Start hotspot** — configure and start directly
3. **Scan** is optional — only needed if the internet source is also WiFi

### Hotspot configuration

| Field | Description | Default |
|---|---|---|
| Network Name (SSID) | The WiFi network name broadcast by the hotspot | `HotSpotter` |
| Passphrase | WPA password; leave empty for an open network | *(empty/open)* |
| Gateway IP | AP gateway and DHCP subnet base (x.x.x.1/24) | `192.168.12.1` |
| Channel | WiFi channel (1-11 for 2.4GHz, 36-48 for 5GHz) | `6` |
| WPA Version | WPA1, WPA2, or WPA1+2 | `WPA2` |
| Hidden Network | Hide the SSID from broadcast | Off |

### Saved networks

- WiFi networks are automatically remembered after a successful connection
- On startup (or service restart), the app auto-reconnects to the first reachable saved network
- Saved networks appear with a "saved" badge in scan results and connect without re-entering the password
- Use the "x" button in the "Remembered Networks" section to forget a network

### Connected clients

When the hotspot is running, the "Connected Clients / DHCP Leases" table shows:

| Column | Source |
|---|---|
| IP Address | DHCP lease from dnsmasq |
| MAC Address | Client hardware address |
| Hostname | Client-reported hostname (if available) |

## REST API

All endpoints are available at the listen address. Responses are JSON.

### Hotspot

| Method | Endpoint | Description |
|---|---|---|
| `POST` | `/api/hotspot/start` | Start hotspot with JSON config body |
| `POST` | `/api/hotspot/stop` | Stop the running hotspot |
| `GET` | `/api/hotspot/status` | Get hotspot state, SSID, gateway, and client leases |

**Start hotspot request body:**

```json
{
  "ssid": "MyNetwork",
  "passphrase": "secret123",
  "wpa_version": "2",
  "channel": 6,
  "interface": "wlan0",
  "internet": "wlan0",
  "hidden": false,
  "gateway": "192.168.12.1"
}
```

### WiFi

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/api/wifi/scan?iface=wlan0` | Scan for nearby networks |
| `POST` | `/api/wifi/connect` | Connect to a network (`{"interface":"wlan0","ssid":"...","password":"..."}`) |
| `POST` | `/api/wifi/disconnect` | Disconnect (`{"interface":"wlan0"}`) |
| `GET` | `/api/wifi/status?iface=wlan0` | Get current connection status |
| `GET` | `/api/wifi/saved` | List remembered networks |
| `POST` | `/api/wifi/saved/remove` | Forget a network (`{"ssid":"..."}`) |

### System

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/api/interfaces` | List network interfaces |
| `GET` | `/api/config/hotspot` | Get saved hotspot configuration |

## Configuration

Persistent configuration is stored in `/etc/hotspotter/config.json` (or the directory specified by `--config-dir`). This file is managed automatically and contains:

- Last-used hotspot settings (restored on page load)
- Saved WiFi network credentials (used for auto-reconnect)

## Building release binaries

Release binaries are built automatically by GitHub Actions when a version tag is pushed:

```bash
git tag v1.0.0
git push --tags
```

This produces static binaries for all supported architectures with no CGO dependencies.

To build locally for a specific target:

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o hotspotter-linux-arm64 .
```

## License

See [LICENSE](LICENSE) for details.
