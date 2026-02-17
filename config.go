package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

const defaultConfigDir = "/etc/hotspotter"
const configFileName = "config.json"

// SavedNetwork is a remembered WiFi network.
type SavedNetwork struct {
	SSID     string `json:"ssid"`
	Password string `json:"password"`
}

// PersistentConfig holds all state that survives restarts.
type PersistentConfig struct {
	Hotspot       *HotspotConfig `json:"hotspot,omitempty"`
	SavedNetworks []SavedNetwork `json:"saved_networks,omitempty"`
}

// ConfigStore manages persistent configuration on disk.
type ConfigStore struct {
	mu   sync.Mutex
	path string
	data PersistentConfig
}

// NewConfigStore loads or initialises config from the given directory.
func NewConfigStore(dir string) (*ConfigStore, error) {
	if dir == "" {
		dir = defaultConfigDir
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	cs := &ConfigStore{path: filepath.Join(dir, configFileName)}
	_ = cs.load() // ignore error on first run
	return cs, nil
}

func (cs *ConfigStore) load() error {
	data, err := os.ReadFile(cs.path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &cs.data)
}

func (cs *ConfigStore) save() error {
	data, err := json.MarshalIndent(cs.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(cs.path, data, 0644)
}

// GetHotspotConfig returns the saved hotspot config (may be nil).
func (cs *ConfigStore) GetHotspotConfig() *HotspotConfig {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.data.Hotspot
}

// SaveHotspotConfig persists the hotspot config.
func (cs *ConfigStore) SaveHotspotConfig(cfg *HotspotConfig) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.data.Hotspot = cfg
	return cs.save()
}

// GetSavedNetworks returns all remembered networks.
func (cs *ConfigStore) GetSavedNetworks() []SavedNetwork {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	out := make([]SavedNetwork, len(cs.data.SavedNetworks))
	copy(out, cs.data.SavedNetworks)
	return out
}

// SaveNetwork adds or updates a remembered network.
func (cs *ConfigStore) SaveNetwork(ssid, password string) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	for i, n := range cs.data.SavedNetworks {
		if n.SSID == ssid {
			cs.data.SavedNetworks[i].Password = password
			return cs.save()
		}
	}
	cs.data.SavedNetworks = append(cs.data.SavedNetworks, SavedNetwork{
		SSID:     ssid,
		Password: password,
	})
	return cs.save()
}

// RemoveNetwork deletes a remembered network.
func (cs *ConfigStore) RemoveNetwork(ssid string) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	for i, n := range cs.data.SavedNetworks {
		if n.SSID == ssid {
			cs.data.SavedNetworks = append(cs.data.SavedNetworks[:i], cs.data.SavedNetworks[i+1:]...)
			return cs.save()
		}
	}
	return nil
}
