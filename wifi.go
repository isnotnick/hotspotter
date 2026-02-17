package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// WifiNetwork represents a visible WiFi network from a scan.
type WifiNetwork struct {
	SSID     string `json:"ssid"`
	BSSID    string `json:"bssid"`
	Signal   string `json:"signal"`
	Security string `json:"security"`
	Freq     string `json:"freq"`
}

// NetworkInterface represents a network interface on the system.
type NetworkInterface struct {
	Name     string `json:"name"`
	Type     string `json:"type"` // "wifi" or "ethernet"
	State    string `json:"state"`
	Wireless bool   `json:"wireless"`
}

// WifiManager handles WiFi scanning and connection.
type WifiManager struct{}

// NewWifiManager creates a new WifiManager.
func NewWifiManager() *WifiManager {
	return &WifiManager{}
}

// ScanNetworks scans for available WiFi networks using iw or nmcli.
func (w *WifiManager) ScanNetworks(iface string) ([]WifiNetwork, error) {
	if iface == "" {
		iface = "wlan0"
	}

	// Try nmcli first (more reliable parsing).
	networks, err := w.scanWithNmcli(iface)
	if err == nil {
		return networks, nil
	}

	// Fall back to iw.
	return w.scanWithIw(iface)
}

func (w *WifiManager) scanWithNmcli(iface string) ([]WifiNetwork, error) {
	// Trigger a fresh scan.
	_ = exec.Command("nmcli", "device", "wifi", "rescan", "ifname", iface).Run()

	cmd := exec.Command("nmcli", "-t", "-f", "BSSID,SSID,SIGNAL,SECURITY,FREQ", "device", "wifi", "list", "ifname", iface)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var networks []WifiNetwork
	seen := make(map[string]bool)
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		// nmcli -t uses : as separator, but BSSID contains colons.
		// BSSID is always 17 chars (XX\:XX\:XX\:XX\:XX\:XX in escaped form).
		// Parse carefully.
		parts := splitNmcliLine(line)
		if len(parts) < 5 {
			continue
		}
		ssid := parts[1]
		if ssid == "" || seen[ssid] {
			continue
		}
		seen[ssid] = true
		networks = append(networks, WifiNetwork{
			BSSID:    parts[0],
			SSID:     ssid,
			Signal:   parts[2] + "%",
			Security: parts[3],
			Freq:     parts[4],
		})
	}
	return networks, nil
}

// splitNmcliLine handles nmcli -t output where colons in BSSIDs are escaped.
func splitNmcliLine(line string) []string {
	var parts []string
	var current strings.Builder
	escaped := false

	for i := 0; i < len(line); i++ {
		ch := line[i]
		if escaped {
			current.WriteByte(ch)
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if ch == ':' {
			parts = append(parts, current.String())
			current.Reset()
			continue
		}
		current.WriteByte(ch)
	}
	parts = append(parts, current.String())
	return parts
}

func (w *WifiManager) scanWithIw(iface string) ([]WifiNetwork, error) {
	cmd := exec.Command("iw", "dev", iface, "scan")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("iw scan failed: %w", err)
	}

	var networks []WifiNetwork
	var current *WifiNetwork
	seen := make(map[string]bool)

	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(line, "BSS ") {
			if current != nil && current.SSID != "" && !seen[current.SSID] {
				seen[current.SSID] = true
				networks = append(networks, *current)
			}
			bssid := strings.TrimSuffix(strings.Fields(line)[1], "(on")
			current = &WifiNetwork{BSSID: strings.TrimSpace(bssid)}
		}
		if current == nil {
			continue
		}
		if strings.HasPrefix(line, "SSID:") {
			current.SSID = strings.TrimSpace(strings.TrimPrefix(line, "SSID:"))
		}
		if strings.HasPrefix(line, "signal:") {
			current.Signal = strings.TrimSpace(strings.TrimPrefix(line, "signal:"))
		}
		if strings.HasPrefix(line, "freq:") {
			current.Freq = strings.TrimSpace(strings.TrimPrefix(line, "freq:"))
		}
		if strings.Contains(line, "WPA") || strings.Contains(line, "RSN") {
			if current.Security == "" {
				current.Security = "WPA"
			}
		}
	}
	if current != nil && current.SSID != "" && !seen[current.SSID] {
		networks = append(networks, *current)
	}

	return networks, nil
}

// Connect joins a WiFi network using nmcli.
func (w *WifiManager) Connect(iface, ssid, password string) error {
	if iface == "" {
		iface = "wlan0"
	}

	args := []string{"device", "wifi", "connect", ssid, "ifname", iface}
	if password != "" {
		args = append(args, "password", password)
	}

	cmd := exec.Command("nmcli", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("nmcli connect failed: %w\n%s", err, string(out))
	}
	return nil
}

// Disconnect disconnects from the current WiFi network.
func (w *WifiManager) Disconnect(iface string) error {
	if iface == "" {
		iface = "wlan0"
	}
	cmd := exec.Command("nmcli", "device", "disconnect", iface)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("nmcli disconnect failed: %w\n%s", err, string(out))
	}
	return nil
}

// ListInterfaces returns network interfaces available on the system.
func (w *WifiManager) ListInterfaces() ([]NetworkInterface, error) {
	cmd := exec.Command("nmcli", "-t", "-f", "DEVICE,TYPE,STATE", "device")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var ifaces []NetworkInterface
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		parts := strings.SplitN(line, ":", 3)
		if len(parts) < 3 {
			continue
		}
		name := parts[0]
		if name == "lo" {
			continue
		}
		ifaces = append(ifaces, NetworkInterface{
			Name:     name,
			Type:     parts[1],
			State:    parts[2],
			Wireless: parts[1] == "wifi",
		})
	}
	return ifaces, nil
}

// ConnectionStatus returns what SSID an interface is currently connected to.
func (w *WifiManager) ConnectionStatus(iface string) (string, error) {
	if iface == "" {
		iface = "wlan0"
	}
	cmd := exec.Command("nmcli", "-t", "-f", "GENERAL.CONNECTION", "device", "show", iface)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(string(out))
	parts := strings.SplitN(line, ":", 2)
	if len(parts) == 2 && parts[1] != "" && parts[1] != "--" {
		return parts[1], nil
	}
	return "", nil
}
