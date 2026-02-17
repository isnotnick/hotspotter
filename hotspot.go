package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"sync"
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
	Band       string `json:"band"` // "2.4" or "5"
}

// HotspotStatus represents the current state of the hotspot.
type HotspotStatus struct {
	Running  bool     `json:"running"`
	PID      string   `json:"pid,omitempty"`
	SSID     string   `json:"ssid,omitempty"`
	Clients  []Client `json:"clients,omitempty"`
	Message  string   `json:"message,omitempty"`
}

// Client represents a connected WiFi client.
type Client struct {
	MAC string `json:"mac"`
}

// HotspotManager wraps create_ap operations.
type HotspotManager struct {
	mu     sync.Mutex
	config *HotspotConfig
}

// NewHotspotManager creates a new manager instance.
func NewHotspotManager() *HotspotManager {
	return &HotspotManager{}
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
	}

	// List connected clients.
	clients, _ := h.listClientsForPID(pids[0])
	status.Clients = clients

	return status
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

func (h *HotspotManager) listClientsForPID(pid string) ([]Client, error) {
	cmd := exec.Command("create_ap", "--list-clients", pid)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var clients []Client
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		mac := strings.TrimSpace(scanner.Text())
		if mac != "" {
			clients = append(clients, Client{MAC: mac})
		}
	}
	return clients, nil
}
