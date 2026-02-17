package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"os/exec"
)

// HotspotConfig holds parameters for creating a WiFi hotspot.
type HotspotConfig struct {
	SSID       string `json:"ssid"`
	Passphrase string `json:"passphrase"` // empty = open network
	WPAVersion string `json:"wpa_version"` // "2" (default), "1", "1+2"
	Channel    int    `json:"channel"`
	Interface  string `json:"interface"`  // WiFi interface (e.g. wlan0)
	Internet   string `json:"internet"`   // upstream interface (e.g. wlan1, eth0)
	Hidden     bool   `json:"hidden"`
	Band       string `json:"band"`    // "2.4" or "5"
	Gateway    string `json:"gateway"` // AP gateway IP (e.g. "192.168.12.1")
}

// HotspotStatus represents the current state of the hotspot.
type HotspotStatus struct {
	Running bool    `json:"running"`
	PID     string  `json:"pid,omitempty"`
	SSID    string  `json:"ssid,omitempty"`
	Gateway string  `json:"gateway,omitempty"`
	Clients []Lease `json:"clients,omitempty"`
	Message string  `json:"message,omitempty"`
}

// Lease represents a DHCP lease from dnsmasq.
type Lease struct {
	MAC      string `json:"mac"`
	IP       string `json:"ip"`
	Hostname string `json:"hostname"`
	Expiry   string `json:"expiry"`
}

// HotspotManager wraps create_ap operations.
type HotspotManager struct {
	mu     sync.Mutex
	config *HotspotConfig
	wifi   *WifiManager
}

// NewHotspotManager creates a new manager instance.
func NewHotspotManager(wifi *WifiManager) *HotspotManager {
	return &HotspotManager{wifi: wifi}
}

// Start creates a hotspot with the given config using create_ap.
func (h *HotspotManager) Start(cfg HotspotConfig) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Stop any existing instance first.
	_ = h.stopLocked()

	args := []string{}

	if cfg.Channel > 0 {
		args = append(args, "-c", fmt.Sprintf("%d", cfg.Channel))
	}
	if cfg.Band != "" {
		args = append(args, "--freq-band", cfg.Band)
	}
	if cfg.Hidden {
		args = append(args, "--hidden")
	}
	if cfg.Passphrase != "" {
		wpa := cfg.WPAVersion
		if wpa == "" {
			wpa = "2"
		}
		args = append(args, "-w", wpa)
	}
	if cfg.Gateway != "" {
		args = append(args, "-g", cfg.Gateway)
	}

	args = append(args, "--daemon")

	// Interfaces
	iface := cfg.Interface
	if iface == "" {
		iface = "wlan0"
	}
	internet := cfg.Internet
	if internet == "" {
		internet = iface
	}

	// When using the same interface for AP and internet, create_ap will create
	// a virtual interface for the AP and NAT through the physical interface's
	// existing upstream connection. Verify that connection exists first.
	if iface == internet && h.wifi != nil {
		connSSID, _ := h.wifi.ConnectionStatus(iface)
		if connSSID == "" {
			return fmt.Errorf(
				"same-interface mode: %s must be connected to an upstream WiFi network before starting the hotspot. "+
					"Connect to a network first, then start the hotspot", iface)
		}
	}

	args = append(args, iface, internet)

	// SSID
	ssid := cfg.SSID
	if ssid == "" {
		ssid = "HotSpotter"
	}
	args = append(args, ssid)

	// Passphrase (if secured)
	if cfg.Passphrase != "" {
		args = append(args, cfg.Passphrase)
	}

	cmd := exec.Command("create_ap", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("create_ap failed: %w\n%s", err, string(out))
	}

	cfg.SSID = ssid
	h.config = &cfg
	return nil
}

// Stop shuts down the running hotspot.
func (h *HotspotManager) Stop() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.stopLocked()
}

func (h *HotspotManager) stopLocked() error {
	// Get list of running instances and stop them all.
	pids, _ := h.listRunningPIDs()
	for _, pid := range pids {
		cmd := exec.Command("create_ap", "--stop", pid)
		_ = cmd.Run()
	}
	h.config = nil
	return nil
}

// Status returns the current hotspot state.
func (h *HotspotManager) Status() HotspotStatus {
	h.mu.Lock()
	defer h.mu.Unlock()

	pids, _ := h.listRunningPIDs()
	if len(pids) == 0 {
		return HotspotStatus{Running: false}
	}

	status := HotspotStatus{
		Running: true,
		PID:     pids[0],
	}
	if h.config != nil {
		status.SSID = h.config.SSID
		status.Gateway = h.config.Gateway
	}

	// Read DHCP leases from dnsmasq.
	status.Clients = h.readLeases()

	return status
}

// readLeases parses dnsmasq lease files created by create_ap.
// Lease format: <expiry_epoch> <MAC> <IP> <hostname> <client-id>
func (h *HotspotManager) readLeases() []Lease {
	var leases []Lease

	matches, _ := filepath.Glob("/tmp/create_ap.*/dnsmasq.leases")
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(bytes.NewReader(data))
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) < 4 {
				continue
			}
			hostname := fields[3]
			if hostname == "*" {
				hostname = ""
			}
			leases = append(leases, Lease{
				Expiry:   fields[0],
				MAC:      fields[1],
				IP:       fields[2],
				Hostname: hostname,
			})
		}
	}
	return leases
}

func (h *HotspotManager) listRunningPIDs() ([]string, error) {
	cmd := exec.Command("create_ap", "--list-running")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var pids []string
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			pids = append(pids, line)
		}
	}
	return pids, nil
}
